package daemon

import (
	"fmt"
	"os"
	"path/filepath"
)

const stateDirectoryName = "rolloutviz"

// Paths names the private files used to coordinate one daemon per user.
type Paths struct {
	RuntimeDir   string
	MetadataFile string
	LockFile     string
	LogFile      string
}

// DefaultPaths locates daemon state below the current user's cache directory.
// The caller should call EnsureRuntimeDir before creating files there.
func DefaultPaths() (Paths, error) {
	if override := os.Getenv("ROLLOUTVIZ_RUNTIME_DIR"); override != "" {
		return PathsAt(override), nil
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return Paths{}, fmt.Errorf("locate user cache directory: %w", err)
	}
	return PathsAt(filepath.Join(cacheDir, stateDirectoryName, "runtime")), nil
}

// PathsAt constructs a path set rooted at dir. It is primarily useful to
// embedders and tests that need an isolated daemon runtime.
func PathsAt(dir string) Paths {
	return Paths{
		RuntimeDir:   dir,
		MetadataFile: filepath.Join(dir, "daemon.json"),
		LockFile:     filepath.Join(dir, "daemon.lock"),
		LogFile:      filepath.Join(dir, "daemon.log"),
	}
}

// EnsureRuntimeDir creates a user-only runtime directory and rejects a symlink
// in its final path component.
func (paths Paths) EnsureRuntimeDir() error {
	if paths.RuntimeDir == "" {
		return fmt.Errorf("daemon runtime directory is empty")
	}
	if err := os.MkdirAll(paths.RuntimeDir, 0o700); err != nil {
		return fmt.Errorf("create daemon runtime directory: %w", err)
	}
	info, err := os.Lstat(paths.RuntimeDir)
	if err != nil {
		return fmt.Errorf("inspect daemon runtime directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("daemon runtime path is not a directory")
	}
	if err := os.Chmod(paths.RuntimeDir, 0o700); err != nil {
		return fmt.Errorf("secure daemon runtime directory: %w", err)
	}
	return nil
}
