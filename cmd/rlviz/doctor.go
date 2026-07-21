package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/daemon"
	"github.com/TheSnakeFang/rlviz/internal/plugins"
)

const doctorSchemaVersion = 1

type doctorReport struct {
	SchemaVersion int             `json:"schema_version"`
	Status        string          `json:"status"`
	Version       string          `json:"version"`
	Platform      doctorPlatform  `json:"platform"`
	Locations     doctorLocations `json:"locations"`
	Runtime       doctorRuntime   `json:"runtime"`
	Daemon        doctorDaemon    `json:"daemon"`
	Browser       doctorCommand   `json:"browser"`
	Python3       doctorCommand   `json:"python3"`
	Plugins       doctorPlugins   `json:"plugins"`
	Issues        []doctorIssue   `json:"issues"`
}

type doctorPlatform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type doctorLocations struct {
	CacheDir   string `json:"cache_dir"`
	RuntimeDir string `json:"runtime_dir"`
	IndexFile  string `json:"index_file"`
	TrustStore string `json:"trust_store"`
}

type doctorRuntime struct {
	State  string `json:"state"`
	Detail string `json:"detail"`
}

type doctorDaemon struct {
	State   string `json:"state"`
	PID     int    `json:"pid"`
	Address string `json:"address"`
	Version string `json:"version"`
	Detail  string `json:"detail"`
}

type doctorCommand struct {
	Available bool   `json:"available"`
	Command   string `json:"command"`
	Path      string `json:"path"`
}

type doctorPlugins struct {
	TrustedCount int      `json:"trusted_count"`
	Paths        []string `json:"paths"`
}

type doctorIssue struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
	Action string `json:"action"`
}

type doctorDependencies struct {
	Version        string
	GOOS           string
	GOARCH         string
	Paths          daemon.Paths
	TrustStorePath string
	Stat           func(string) (fs.FileInfo, error)
	LookPath       func(string) (string, error)
	ReadMetadata   func(daemon.Paths) (daemon.Metadata, error)
	ProbeDaemon    func(context.Context, daemon.Metadata) (daemon.Status, error)
	ListTrusted    func() ([]plugins.TrustEntry, error)
	DaemonTimeout  time.Duration
}

func defaultDoctorDependencies() (doctorDependencies, error) {
	paths, err := daemon.DefaultPaths()
	if err != nil {
		return doctorDependencies{}, err
	}
	store, err := plugins.DefaultTrustStore()
	if err != nil {
		return doctorDependencies{}, fmt.Errorf("locate plugin trust store: %w", err)
	}
	client := daemon.Client{}
	return doctorDependencies{
		Version:        version,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Paths:          paths,
		TrustStorePath: store.Path,
		Stat:           os.Stat,
		LookPath:       exec.LookPath,
		ReadMetadata:   daemon.ReadMetadata,
		ProbeDaemon:    client.Status,
		ListTrusted:    store.List,
		DaemonTimeout:  2 * time.Second,
	}, nil
}

func collectDoctorReport(ctx context.Context, dependencies doctorDependencies) doctorReport {
	report := doctorReport{
		SchemaVersion: doctorSchemaVersion,
		Status:        "ok",
		Version:       dependencies.Version,
		Platform:      doctorPlatform{OS: dependencies.GOOS, Arch: dependencies.GOARCH},
		Locations: doctorLocations{
			CacheDir:   filepath.Dir(dependencies.Paths.RuntimeDir),
			RuntimeDir: dependencies.Paths.RuntimeDir,
			IndexFile:  dependencies.Paths.IndexFile,
			TrustStore: dependencies.TrustStorePath,
		},
		Plugins: doctorPlugins{Paths: []string{}},
		Issues:  []doctorIssue{},
	}

	report.collectRuntime(dependencies)
	report.collectDaemon(ctx, dependencies)
	report.Browser = commandAvailability(browserCommand(dependencies.GOOS), dependencies.LookPath)
	if !report.Browser.Available {
		report.addIssue("browser_launcher_unavailable", fmt.Sprintf("browser launcher %q is not on PATH", report.Browser.Command), browserAction(dependencies.GOOS))
	}
	report.Python3 = commandAvailability("python3", dependencies.LookPath)
	if !report.Python3.Available {
		report.addIssue("python3_unavailable", "python3 is not on PATH; built-in viewing still works, but Python plugins do not", "Install Python 3 before using Python adapter or analyzer plugins.")
	}
	report.collectPlugins(dependencies)
	return report
}

func (report *doctorReport) collectRuntime(dependencies doctorDependencies) {
	info, err := dependencies.Stat(dependencies.Paths.RuntimeDir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		report.Runtime = doctorRuntime{State: "absent", Detail: "will be created on first daemon start"}
	case err != nil:
		report.Runtime = doctorRuntime{State: "degraded", Detail: err.Error()}
		report.addIssue("runtime_directory_unreadable", fmt.Sprintf("cannot inspect runtime directory: %v", err), "Check the runtime path and its parent directory permissions.")
	case !info.IsDir():
		report.Runtime = doctorRuntime{State: "degraded", Detail: "path exists but is not a directory"}
		report.addIssue("runtime_path_not_directory", "runtime path exists but is not a directory", "Move or remove the conflicting path before opening a rollout.")
	default:
		report.Runtime = doctorRuntime{State: "ready", Detail: "runtime directory is present"}
	}
}

func (report *doctorReport) collectDaemon(ctx context.Context, dependencies doctorDependencies) {
	metadata, err := dependencies.ReadMetadata(dependencies.Paths)
	if errors.Is(err, daemon.ErrNoMetadata) {
		report.Daemon = doctorDaemon{State: "stopped", Detail: "starts automatically when a rollout is opened"}
		return
	}
	if err != nil {
		report.Daemon = doctorDaemon{State: "degraded", Detail: err.Error()}
		report.addIssue("daemon_metadata_invalid", fmt.Sprintf("daemon metadata cannot be read: %v", err), "Run `rlviz stop`; if the problem remains, inspect the reported runtime directory.")
		return
	}

	timeout := dependencies.DaemonTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	probeContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	status, err := dependencies.ProbeDaemon(probeContext, metadata)
	if err != nil {
		report.Daemon = doctorDaemon{State: "degraded", PID: metadata.PID, Address: metadata.Address, Version: metadata.Version, Detail: err.Error()}
		report.addIssue("daemon_unreachable", fmt.Sprintf("daemon metadata exists but the daemon did not answer: %v", err), "Run `rlviz stop`, then retry the open command.")
		return
	}
	if detail := daemonMismatchDetail(metadata, status); detail != "" {
		report.Daemon = doctorDaemon{State: "degraded", PID: metadata.PID, Address: metadata.Address, Version: metadata.Version, Detail: detail}
		report.addIssue("daemon_status_mismatch", detail, "Run `rlviz stop`, then retry the open command.")
		return
	}
	report.Daemon = doctorDaemon{State: "live", PID: metadata.PID, Address: metadata.Address, Version: metadata.Version, Detail: "authenticated loopback health check passed"}
}

func daemonMismatchDetail(metadata daemon.Metadata, status daemon.Status) string {
	if status.Status != "ok" && status.Status != "running" {
		return fmt.Sprintf("unexpected daemon status %q", status.Status)
	}
	if status.PID != 0 && status.PID != metadata.PID {
		return fmt.Sprintf("daemon pid mismatch: metadata=%d response=%d", metadata.PID, status.PID)
	}
	if status.Version != "" && status.Version != metadata.Version {
		return fmt.Sprintf("daemon version mismatch: metadata=%q response=%q", metadata.Version, status.Version)
	}
	return ""
}

func (report *doctorReport) collectPlugins(dependencies doctorDependencies) {
	entries, err := dependencies.ListTrusted()
	if err != nil {
		report.addIssue("trust_store_unreadable", fmt.Sprintf("cannot read plugin trust store: %v", err), "Check the reported trust-store path and its permissions.")
		return
	}
	for _, entry := range entries {
		report.Plugins.Paths = append(report.Plugins.Paths, entry.Path)
	}
	sort.Strings(report.Plugins.Paths)
	report.Plugins.TrustedCount = len(report.Plugins.Paths)
}

func (report *doctorReport) addIssue(code, detail, action string) {
	report.Status = "degraded"
	report.Issues = append(report.Issues, doctorIssue{Code: code, Detail: detail, Action: action})
}

func browserCommand(goos string) string {
	switch goos {
	case "darwin":
		return "open"
	case "windows":
		return "rundll32"
	default:
		return "xdg-open"
	}
}

func browserAction(goos string) string {
	if goos == "linux" {
		return "Install xdg-utils or use `rlviz open --no-open` and open the reported URL manually."
	}
	return "Use `rlviz open --no-open` and open the reported URL manually."
}

func commandAvailability(command string, lookPath func(string) (string, error)) doctorCommand {
	path, err := lookPath(command)
	return doctorCommand{Available: err == nil, Command: command, Path: path}
}

func formatDoctorReport(report doctorReport) string {
	var output strings.Builder
	fmt.Fprintf(&output, "RLViz doctor: %s (%s, %s/%s)\n", report.Status, report.Version, report.Platform.OS, report.Platform.Arch)
	fmt.Fprintf(&output, "  daemon: %s", report.Daemon.State)
	if report.Daemon.State == "live" {
		fmt.Fprintf(&output, " at http://%s (pid %d, %s)", report.Daemon.Address, report.Daemon.PID, report.Daemon.Version)
	}
	fmt.Fprintf(&output, "\n  browser: %s\n", formatCommand(report.Browser))
	fmt.Fprintf(&output, "  python3: %s\n", formatCommand(report.Python3))
	fmt.Fprintf(&output, "  trusted plugins: %d\n", report.Plugins.TrustedCount)
	fmt.Fprintf(&output, "  cache: %s\n  runtime: %s (%s)\n  index: %s\n  trust store: %s", report.Locations.CacheDir, report.Locations.RuntimeDir, report.Runtime.State, report.Locations.IndexFile, report.Locations.TrustStore)
	if len(report.Plugins.Paths) > 0 {
		output.WriteString("\n  plugin paths:")
		for _, path := range report.Plugins.Paths {
			fmt.Fprintf(&output, "\n    %s", path)
		}
	}
	if len(report.Issues) > 0 {
		output.WriteString("\nIssues:")
		for _, issue := range report.Issues {
			fmt.Fprintf(&output, "\n  - %s\n    %s", issue.Detail, issue.Action)
		}
	}
	return output.String()
}

func formatCommand(command doctorCommand) string {
	if !command.Available {
		return command.Command + " unavailable"
	}
	return fmt.Sprintf("%s (%s)", command.Command, command.Path)
}
