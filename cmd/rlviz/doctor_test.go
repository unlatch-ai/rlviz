package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/daemon"
	"github.com/TheSnakeFang/rlviz/internal/plugins"
)

func TestCollectDoctorReportReadyAndStopped(t *testing.T) {
	dependencies := testDoctorDependencies(t)
	report := collectDoctorReport(context.Background(), dependencies)

	if report.SchemaVersion != 1 || report.Status != "ok" || report.Version != "1.2.3" {
		t.Fatalf("identity = %#v", report)
	}
	if report.Daemon.State != "stopped" || report.Runtime.State != "ready" {
		t.Fatalf("state = daemon %q, runtime %q", report.Daemon.State, report.Runtime.State)
	}
	if !report.Browser.Available || report.Browser.Command != "open" || !report.Python3.Available {
		t.Fatalf("commands = browser %#v, python %#v", report.Browser, report.Python3)
	}
	wantPaths := []string{"/plugins/a", "/plugins/z"}
	if report.Plugins.TrustedCount != 2 || !reflect.DeepEqual(report.Plugins.Paths, wantPaths) {
		t.Fatalf("plugins = %#v", report.Plugins)
	}
	if len(report.Issues) != 0 {
		t.Fatalf("issues = %#v", report.Issues)
	}
}

func TestCollectDoctorReportLiveDoesNotExposeToken(t *testing.T) {
	dependencies := testDoctorDependencies(t)
	metadata := daemon.Metadata{PID: 42, Address: "127.0.0.1:7317", Token: "sensitive-token", Version: "1.2.3"}
	dependencies.ReadMetadata = func(daemon.Paths) (daemon.Metadata, error) { return metadata, nil }
	dependencies.ProbeDaemon = func(_ context.Context, got daemon.Metadata) (daemon.Status, error) {
		if got.Token != metadata.Token {
			t.Fatal("probe did not receive private token")
		}
		return daemon.Status{Status: "ok", PID: 42, Version: "1.2.3"}, nil
	}
	report := collectDoctorReport(context.Background(), dependencies)
	if report.Daemon.State != "live" || report.Daemon.PID != 42 {
		t.Fatalf("daemon = %#v", report.Daemon)
	}
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), metadata.Token) || strings.Contains(formatDoctorReport(report), metadata.Token) {
		t.Fatal("doctor output exposed the daemon token")
	}
}

func TestCollectDoctorReportDegradedIsActionable(t *testing.T) {
	dependencies := testDoctorDependencies(t)
	dependencies.GOOS = "linux"
	dependencies.Stat = func(string) (fs.FileInfo, error) { return nil, errors.New("permission denied") }
	dependencies.ReadMetadata = func(daemon.Paths) (daemon.Metadata, error) {
		return daemon.Metadata{}, errors.New("invalid metadata")
	}
	dependencies.LookPath = func(command string) (string, error) {
		return "", errors.New(command + " missing")
	}
	dependencies.ListTrusted = func() ([]plugins.TrustEntry, error) {
		return nil, errors.New("trust store denied")
	}

	report := collectDoctorReport(context.Background(), dependencies)
	if report.Status != "degraded" || report.Daemon.State != "degraded" || report.Runtime.State != "degraded" {
		t.Fatalf("report = %#v", report)
	}
	codes := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		codes = append(codes, issue.Code)
		if issue.Action == "" {
			t.Fatalf("issue has no action: %#v", issue)
		}
	}
	want := []string{"runtime_directory_unreadable", "daemon_metadata_invalid", "browser_launcher_unavailable", "python3_unavailable", "trust_store_unreadable"}
	if !reflect.DeepEqual(codes, want) {
		t.Fatalf("issue codes = %#v, want %#v", codes, want)
	}
	human := formatDoctorReport(report)
	if !strings.Contains(human, "Issues:") || !strings.Contains(human, "xdg-open") || !strings.Contains(human, "--no-open") {
		t.Fatalf("human output is not actionable:\n%s", human)
	}
}

func TestCollectDoctorReportRuntimeAbsenceIsHealthy(t *testing.T) {
	dependencies := testDoctorDependencies(t)
	dependencies.Stat = func(string) (fs.FileInfo, error) { return nil, os.ErrNotExist }
	report := collectDoctorReport(context.Background(), dependencies)
	if report.Status != "ok" || report.Runtime.State != "absent" {
		t.Fatalf("report = %#v", report)
	}
}

func TestCollectDoctorReportDaemonMismatchIsDegraded(t *testing.T) {
	dependencies := testDoctorDependencies(t)
	dependencies.ReadMetadata = func(daemon.Paths) (daemon.Metadata, error) {
		return daemon.Metadata{PID: 42, Address: "127.0.0.1:7317", Version: "1.2.3"}, nil
	}
	dependencies.ProbeDaemon = func(context.Context, daemon.Metadata) (daemon.Status, error) {
		return daemon.Status{Status: "ok", PID: 99, Version: "1.2.3"}, nil
	}
	report := collectDoctorReport(context.Background(), dependencies)
	if report.Status != "degraded" || report.Daemon.State != "degraded" || report.Issues[0].Code != "daemon_status_mismatch" {
		t.Fatalf("report = %#v", report)
	}
}

func TestDoctorReportJSONHasStableTopLevelShape(t *testing.T) {
	report := collectDoctorReport(context.Background(), testDoctorDependencies(t))
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	want := []string{"browser", "daemon", "issues", "locations", "platform", "plugins", "python3", "runtime", "schema_version", "status", "version"}
	got := make([]string, 0, len(decoded))
	for key := range decoded {
		got = append(got, key)
	}
	sortStrings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON keys = %#v, want %#v", got, want)
	}
}

func testDoctorDependencies(t *testing.T) doctorDependencies {
	t.Helper()
	runtimeDir := t.TempDir()
	paths := daemon.PathsAt(runtimeDir)
	return doctorDependencies{
		Version:        "1.2.3",
		GOOS:           "darwin",
		GOARCH:         "arm64",
		Paths:          paths,
		TrustStorePath: filepath.Join(t.TempDir(), "trusted-plugins.json"),
		Stat:           os.Stat,
		LookPath: func(command string) (string, error) {
			return "/usr/bin/" + command, nil
		},
		ReadMetadata: func(daemon.Paths) (daemon.Metadata, error) {
			return daemon.Metadata{}, daemon.ErrNoMetadata
		},
		ProbeDaemon: func(context.Context, daemon.Metadata) (daemon.Status, error) {
			t.Fatal("stopped daemon should not be probed")
			return daemon.Status{}, nil
		},
		ListTrusted: func() ([]plugins.TrustEntry, error) {
			return []plugins.TrustEntry{{Path: "/plugins/z"}, {Path: "/plugins/a"}}, nil
		},
		DaemonTimeout: 10 * time.Millisecond,
	}
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
