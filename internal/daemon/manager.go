package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

var (
	ErrDaemonUnavailable = errors.New("daemon is unavailable")
	ErrStartupTimeout    = errors.New("timed out waiting for daemon startup")
)

// Manager starts or reuses the per-user daemon. Args should select the
// application's internal foreground-daemon command.
type Manager struct {
	Paths          Paths
	Client         Client
	Launcher       Launcher
	Executable     string
	Args           []string
	Env            []string
	Version        string
	StartupTimeout time.Duration
	PollInterval   time.Duration
}

// EnsureResult reports the authenticated daemon and whether this call started
// its process.
type EnsureResult struct {
	Metadata Metadata
	Started  bool
}

// LoadLiveMetadata reads metadata and verifies it with an authenticated status
// call. Invalid or unreachable metadata is removed so future opens can recover.
func LoadLiveMetadata(ctx context.Context, paths Paths, client Client) (Metadata, error) {
	metadata, err := ReadMetadata(paths)
	if err != nil {
		if !errors.Is(err, ErrNoMetadata) {
			_ = RemoveMetadata(paths)
			return Metadata{}, fmt.Errorf("%w: %v", ErrDaemonUnavailable, err)
		}
		return Metadata{}, err
	}
	if err := verifyLive(ctx, client, metadata); err != nil {
		if ctx.Err() != nil {
			return Metadata{}, fmt.Errorf("%w: %v", ErrDaemonUnavailable, err)
		}
		_ = RemoveMetadata(paths)
		return Metadata{}, fmt.Errorf("%w: %v", ErrDaemonUnavailable, err)
	}
	return metadata, nil
}

func verifyLive(ctx context.Context, client Client, metadata Metadata) error {
	status, err := client.Status(ctx, metadata)
	if err != nil {
		return err
	}
	if status.Status != "ok" && status.Status != "running" {
		return fmt.Errorf("daemon is not ready: status=%q", status.Status)
	}
	if status.PID != 0 && status.PID != metadata.PID {
		return fmt.Errorf("daemon pid mismatch: metadata=%d status=%d", metadata.PID, status.PID)
	}
	if status.Version != "" && status.Version != metadata.Version {
		return fmt.Errorf("daemon version mismatch: metadata=%q status=%q", metadata.Version, status.Version)
	}
	return nil
}

// Ensure returns a live daemon, serializing concurrent startup attempts with a
// private lock file and polling until the child publishes usable metadata.
func (manager Manager) Ensure(ctx context.Context) (EnsureResult, error) {
	manager.applyDefaults()
	if err := manager.Paths.EnsureRuntimeDir(); err != nil {
		return EnsureResult{}, err
	}
	if manager.Version == "" {
		return EnsureResult{}, fmt.Errorf("daemon version is empty")
	}
	if metadata, err := LoadLiveMetadata(ctx, manager.Paths, manager.Client); err == nil {
		if metadata.Version != manager.Version {
			if err := manager.stopForUpgrade(ctx, metadata); err != nil {
				return EnsureResult{}, err
			}
		} else {
			return EnsureResult{Metadata: metadata}, nil
		}
	} else if !errors.Is(err, ErrNoMetadata) && !errors.Is(err, ErrDaemonUnavailable) {
		return EnsureResult{}, err
	}

	startupContext, cancel := context.WithTimeout(ctx, manager.StartupTimeout)
	defer cancel()
	locked, err := manager.acquireStartupLock(startupContext)
	if err != nil {
		return EnsureResult{}, err
	}
	if !locked {
		metadata, err := manager.waitForLive(startupContext)
		if err != nil {
			return EnsureResult{}, err
		}
		return EnsureResult{Metadata: metadata}, nil
	}
	defer os.Remove(manager.Paths.LockFile)

	// Another caller may have completed startup immediately before we acquired
	// the lock. Check once more while holding it.
	if metadata, err := LoadLiveMetadata(startupContext, manager.Paths, manager.Client); err == nil {
		if metadata.Version != manager.Version {
			if err := manager.stopForUpgrade(startupContext, metadata); err != nil {
				return EnsureResult{}, err
			}
		} else {
			return EnsureResult{Metadata: metadata}, nil
		}
	}
	launcher := manager.Launcher
	if launcher == nil {
		launcher = ProcessLauncher{}
	}
	if _, err := launcher.Start(LaunchConfig{
		Executable: manager.Executable,
		Args:       append([]string(nil), manager.Args...),
		Env:        append([]string(nil), manager.Env...),
		LogPath:    manager.Paths.LogFile,
	}); err != nil {
		return EnsureResult{}, err
	}
	metadata, err := manager.waitForLive(startupContext)
	if err != nil {
		return EnsureResult{}, err
	}
	return EnsureResult{Metadata: metadata, Started: true}, nil
}

func (manager Manager) stopForUpgrade(ctx context.Context, metadata Metadata) error {
	if err := manager.Client.Stop(ctx, metadata); err != nil {
		return fmt.Errorf("stop daemon version %q before upgrade: %w", metadata.Version, err)
	}
	for {
		_, err := ReadMetadata(manager.Paths)
		if errors.Is(err, ErrNoMetadata) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("wait for old daemon metadata removal: %w", err)
		}
		if err := waitInterval(ctx, manager.PollInterval); err != nil {
			return fmt.Errorf("wait for daemon version %q to stop: %w", metadata.Version, err)
		}
	}
}

func (manager *Manager) applyDefaults() {
	if manager.StartupTimeout <= 0 {
		manager.StartupTimeout = 5 * time.Second
	}
	if manager.PollInterval <= 0 {
		manager.PollInterval = 50 * time.Millisecond
	}
}

func (manager Manager) acquireStartupLock(ctx context.Context) (bool, error) {
	for {
		lock, err := os.OpenFile(manager.Paths.LockFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			if closeErr := lock.Close(); closeErr != nil {
				_ = os.Remove(manager.Paths.LockFile)
				return false, fmt.Errorf("close daemon startup lock: %w", closeErr)
			}
			if err := os.Chmod(manager.Paths.LockFile, 0o600); err != nil {
				_ = os.Remove(manager.Paths.LockFile)
				return false, fmt.Errorf("secure daemon startup lock: %w", err)
			}
			return true, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return false, fmt.Errorf("create daemon startup lock: %w", err)
		}
		if metadata, liveErr := manager.readLiveWithoutCleanup(ctx); liveErr == nil {
			if metadata.Version != manager.Version {
				return false, fmt.Errorf("running daemon version %q does not match CLI version %q", metadata.Version, manager.Version)
			}
			return false, nil
		}
		info, statErr := os.Stat(manager.Paths.LockFile)
		if statErr == nil && time.Since(info.ModTime()) > manager.StartupTimeout {
			_ = os.Remove(manager.Paths.LockFile)
			continue
		}
		if err := waitInterval(ctx, manager.PollInterval); err != nil {
			return false, fmt.Errorf("%w: %v", ErrStartupTimeout, err)
		}
	}
}

func (manager Manager) waitForLive(ctx context.Context) (Metadata, error) {
	var lastErr error
	for {
		metadata, err := manager.readLiveWithoutCleanup(ctx)
		if err == nil {
			if metadata.Version != manager.Version {
				return Metadata{}, fmt.Errorf("started daemon version %q does not match CLI version %q", metadata.Version, manager.Version)
			}
			return metadata, nil
		}
		lastErr = err
		if err := waitInterval(ctx, manager.PollInterval); err != nil {
			return Metadata{}, fmt.Errorf("%w: %v (last probe: %v)", ErrStartupTimeout, err, lastErr)
		}
	}
}

func (manager Manager) readLiveWithoutCleanup(ctx context.Context) (Metadata, error) {
	metadata, err := ReadMetadata(manager.Paths)
	if err != nil {
		return Metadata{}, err
	}
	if err := verifyLive(ctx, manager.Client, metadata); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func waitInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
