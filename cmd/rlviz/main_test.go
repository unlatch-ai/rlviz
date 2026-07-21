package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/daemon"
)

func TestNormalizeViewerArgumentsAllowsFlagsAfterPath(t *testing.T) {
	got := normalizeViewerArguments([]string{"trace.ndjson", "--no-open", "--port", "7317", "--json"})
	want := []string{"--no-open", "--port", "7317", "--json", "trace.ndjson"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeViewerArguments() = %#v, want %#v", got, want)
	}
}

func TestInspectCacheReportsAbsentAndPresent(t *testing.T) {
	paths := daemon.PathsAt(t.TempDir())
	status, err := inspectCache(paths, func() (bool, error) { return true, nil })
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "absent" || status.Path != paths.IndexFile || status.SizeBytes != 0 || !status.DaemonRunning {
		t.Fatalf("absent status = %#v", status)
	}

	if err := os.WriteFile(paths.IndexFile, []byte("sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err = inspectCache(paths, func() (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "present" || status.SizeBytes != 6 || status.DaemonRunning {
		t.Fatalf("present status = %#v", status)
	}
}

func TestInspectCachePropagatesDaemonCheckError(t *testing.T) {
	want := errors.New("metadata denied")
	_, err := inspectCache(daemon.PathsAt(t.TempDir()), func() (bool, error) { return false, want })
	if !errors.Is(err, want) {
		t.Fatalf("inspectCache() error = %v, want %v", err, want)
	}
}

func TestCleanCacheRemovesOnlySQLiteIndexFiles(t *testing.T) {
	paths := daemon.PathsAt(t.TempDir())
	files := []string{paths.IndexFile, paths.IndexFile + "-wal", paths.IndexFile + "-shm"}
	for _, path := range files {
		if err := os.WriteFile(path, []byte("cache"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sentinel := filepath.Join(paths.RuntimeDir, "daemon.log")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := cleanCache(paths, func() (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Removed, files) {
		t.Fatalf("removed = %#v, want %#v", result.Removed, files)
	}
	for _, path := range files {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cache file %s still exists: %v", path, err)
		}
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "keep" {
		t.Fatalf("sentinel = %q, %v", got, err)
	}

	result, err = cleanCache(paths, func() (bool, error) { return false, nil })
	if err != nil || len(result.Removed) != 0 {
		t.Fatalf("idempotent clean = %#v, %v", result, err)
	}
}

func TestCleanCacheRefusesWhileDaemonIsRunning(t *testing.T) {
	paths := daemon.PathsAt(t.TempDir())
	if err := os.WriteFile(paths.IndexFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := cleanCache(paths, func() (bool, error) { return true, nil })
	if err == nil || !strings.Contains(err.Error(), "daemon is running") {
		t.Fatalf("cleanCache() error = %v", err)
	}
	if _, err := os.Stat(paths.IndexFile); err != nil {
		t.Fatalf("cache was removed: %v", err)
	}
}

func TestCleanCacheRejectsDirectoryBeforeRemovingAnything(t *testing.T) {
	paths := daemon.PathsAt(t.TempDir())
	if err := os.WriteFile(paths.IndexFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(paths.IndexFile+"-wal", 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanCache(paths, func() (bool, error) { return false, nil }); err == nil {
		t.Fatal("cleanCache() accepted a directory")
	}
	if _, err := os.Stat(paths.IndexFile); err != nil {
		t.Fatalf("index was removed before validation completed: %v", err)
	}
}

func TestResolveViewerURLStaysOnDaemonOrigin(t *testing.T) {
	metadata := daemon.Metadata{Address: "127.0.0.1:7317", Token: "secret"}
	got, err := resolveViewerURL(metadata, "/?trajectory=trace-1")
	if err != nil || got != "http://127.0.0.1:7317/?trajectory=trace-1#token=secret" {
		t.Fatalf("resolveViewerURL() = %q, %v", got, err)
	}
	if _, err := resolveViewerURL(metadata, "https://example.com/steal"); err == nil {
		t.Fatal("resolveViewerURL() accepted another origin")
	}
}

func TestSafePluginName(t *testing.T) {
	if got := safePluginName("Customer Trace V2"); got != "customer-trace-v2" {
		t.Fatalf("safePluginName() = %q", got)
	}
}

func TestNormalizeViewerArgumentsPreservesEqualsFlag(t *testing.T) {
	got := normalizeViewerArguments([]string{"trace.ndjson", "--port=7317", "--presentation", "view.json"})
	want := []string{"--port=7317", "--presentation", "view.json", "trace.ndjson"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeViewerArguments() = %#v, want %#v", got, want)
	}
}
