package daemon

import (
	"fmt"
	"os"
	"os/exec"
)

// LaunchConfig describes a detached child process. Args excludes argv[0].
type LaunchConfig struct {
	Executable string
	Args       []string
	Env        []string
	LogPath    string
}

// Launcher allows Manager startup behavior to be tested without a real child.
type Launcher interface {
	Start(LaunchConfig) (int, error)
}

// ProcessLauncher starts a detached OS process with output appended to LogPath.
type ProcessLauncher struct{}

func (ProcessLauncher) Start(config LaunchConfig) (int, error) {
	if config.Executable == "" {
		return 0, fmt.Errorf("daemon executable is empty")
	}
	logFile, err := os.OpenFile(config.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open daemon log: %w", err)
	}
	defer logFile.Close()
	if err := logFile.Chmod(0o600); err != nil {
		return 0, fmt.Errorf("secure daemon log: %w", err)
	}
	command := exec.Command(config.Executable, config.Args...)
	command.Env = append(os.Environ(), config.Env...)
	command.Stdin = nil
	command.Stdout = logFile
	command.Stderr = logFile
	configureDetachedProcess(command)
	if err := command.Start(); err != nil {
		return 0, fmt.Errorf("start daemon process: %w", err)
	}
	pid := command.Process.Pid
	if err := command.Process.Release(); err != nil {
		return 0, fmt.Errorf("release daemon process: %w", err)
	}
	return pid, nil
}
