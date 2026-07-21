package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	fixturedata "github.com/TheSnakeFang/rlviz/fixtures"
	"github.com/TheSnakeFang/rlviz/internal/app"
	"github.com/TheSnakeFang/rlviz/internal/daemon"
	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/server"
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

func TestEnsureGallerySourcesPopulateBrowse(t *testing.T) {
	paths := daemon.PathsAt(filepath.Join(t.TempDir(), "runtime"))
	galleryPaths, err := ensureGallerySources(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(galleryPaths) != 3 {
		t.Fatalf("gallery paths = %d, want 3", len(galleryPaths))
	}
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "gallery.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, path := range galleryPaths {
		if _, err := app.IndexSource(context.Background(), store, path, ""); err != nil {
			t.Fatalf("index %s: %v", path, err)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/indexed/browse", nil)
	request.Header.Set("Authorization", "Bearer gallery-token")
	response := httptest.NewRecorder()
	server.NewIndexedHandler(store, "gallery-token").ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("browse status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Sources      []any `json:"sources"`
		Trajectories []any `json:"trajectories"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Sources) != 3 || len(payload.Trajectories) != 18 {
		t.Fatalf("browse sources=%d trajectories=%d", len(payload.Sources), len(payload.Trajectories))
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
