package main

import (
	"os"
	"path/filepath"
	"testing"

	fixturedata "github.com/unlatch-ai/rlviz/fixtures"
	"github.com/unlatch-ai/rlviz/internal/daemon"
)

func TestEnsureDemoSourceInstallsPrivateEmbeddedFixture(t *testing.T) {
	paths := daemon.PathsAt(filepath.Join(t.TempDir(), "runtime"))
	path, err := ensureDemoSource(paths)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(paths.RuntimeDir, demoFilename) {
		t.Fatalf("path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(fixturedata.DemoNDJSON) {
		t.Fatal("installed demo differs from embedded fixture")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureDemoSource(paths); err != nil {
		t.Fatalf("idempotent ensure: %v", err)
	}
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("secured idempotent mode = %o, %v", info.Mode().Perm(), err)
	}
}

func TestMarkDemoURLPreservesTokenAndAddsDemoMarker(t *testing.T) {
	got, err := markDemoURL("http://127.0.0.1:7317/?trajectory=trace-1&indexed=1#token=secret")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:7317/?demo=1&indexed=1&trajectory=trace-1#token=secret" {
		t.Fatalf("URL = %q", got)
	}
}
