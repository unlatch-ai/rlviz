package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

func artifactHandler(t *testing.T, root string, artifact *model.Artifact) http.Handler {
	t.Helper()
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "artifact.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	records := []any{
		&model.Run{RecordType: model.RecordRun, ID: "run"},
		&model.Case{RecordType: model.RecordCase, ID: "case", RunID: "run"},
		&model.Group{RecordType: model.RecordGroup, ID: "group", CaseID: "case"},
		&model.Trajectory{RecordType: model.RecordTrajectory, ID: "trajectory", GroupID: "group"},
		artifact,
		&model.Complete{RecordType: model.RecordComplete, Records: 5},
	}
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(root, "trace.ndjson")
	if err := os.WriteFile(source, input.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(t.Context(), rolloutindex.Source{ID: "source", Path: source, Size: info.Size(), ModTime: info.ModTime()}, bytes.NewReader(input.Bytes())); err != nil {
		t.Fatal(err)
	}
	return NewIndexedHandler(store, "secret")
}

func pathArtifact(path, mediaType, hash string) *model.Artifact {
	return &model.Artifact{RecordType: model.RecordArtifact, ID: "artifact", TrajectoryID: "trajectory", Name: "result", MediaType: mediaType, Path: path, SHA256: hash}
}

func artifactRequest(t *testing.T, handler http.Handler, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/indexed/artifact/content?trajectory=source&trajectory_id=trajectory&artifact_id=artifact", nil)
	if authenticated {
		request.Header.Set("Authorization", "Bearer secret")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestArtifactContentIsAuthenticatedHashedAndSafelyTyped(t *testing.T) {
	root := t.TempDir()
	content := []byte("line one\nline two\n")
	if err := os.WriteFile(filepath.Join(root, "result.log"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(content)
	handler := artifactHandler(t, root, pathArtifact("result.log", "text/x-log", hex.EncodeToString(hash[:])))
	unauthorized := artifactRequest(t, handler, false)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	response := artifactRequest(t, handler, true)
	if response.Code != http.StatusOK || response.Body.String() != string(content) {
		t.Fatalf("status/body = %d %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff = %q", got)
	}
	if disposition := response.Header().Get("Content-Disposition"); !strings.HasPrefix(disposition, "inline;") || !strings.Contains(disposition, "result") {
		t.Fatalf("content disposition = %q", disposition)
	}
}

func TestArtifactContentRejectsTraversalAndEscapingSymlinks(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "source")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		path string
	}{
		{name: "parent traversal", path: "../secret.txt"},
		{name: "absolute escape", path: outside},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := artifactRequest(t, artifactHandler(t, root, pathArtifact(test.path, "text/plain", "")), true)
			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "artifact_path_forbidden") {
				t.Fatalf("status/body = %d %s", response.Code, response.Body.String())
			}
		})
	}
	link := filepath.Join(root, "escape.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	response := artifactRequest(t, artifactHandler(t, root, pathArtifact("escape.txt", "text/plain", "")), true)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "artifact_path_forbidden") {
		t.Fatalf("symlink status/body = %d %s", response.Code, response.Body.String())
	}
}

func TestArtifactContentRejectsOversizeUnsafeTypesAndHashMismatch(t *testing.T) {
	t.Run("oversize", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "large.log")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(MaxArtifactContentBytes + 1); err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
		response := artifactRequest(t, artifactHandler(t, root, pathArtifact("large.log", "text/plain", "")), true)
		if response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), "artifact_too_large") {
			t.Fatalf("status/body = %d %s", response.Code, response.Body.String())
		}
	})
	for _, mediaType := range []string{"text/html", "image/svg+xml", "application/javascript"} {
		t.Run(mediaType, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "unsafe"), []byte("<script>alert(1)</script>"), 0o600); err != nil {
				t.Fatal(err)
			}
			response := artifactRequest(t, artifactHandler(t, root, pathArtifact("unsafe", mediaType, "")), true)
			if response.Code != http.StatusUnsupportedMediaType || !strings.Contains(response.Body.String(), "artifact_type_unsupported") {
				t.Fatalf("status/body = %d %s", response.Code, response.Body.String())
			}
		})
	}
	t.Run("hash mismatch", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "result.txt"), []byte("actual"), 0o600); err != nil {
			t.Fatal(err)
		}
		response := artifactRequest(t, artifactHandler(t, root, pathArtifact("result.txt", "text/plain", strings.Repeat("0", 64))), true)
		if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "artifact_hash_mismatch") {
			t.Fatalf("status/body = %d %s", response.Code, response.Body.String())
		}
	})
}

func TestArtifactContentAllowsContainedSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	response := artifactRequest(t, artifactHandler(t, root, pathArtifact("link.txt", "text/plain", "")), true)
	if response.Code != http.StatusOK || response.Body.String() != "inside" {
		t.Fatalf("status/body = %d %q", response.Code, response.Body.String())
	}
}

func TestOpenVerifiedArtifactRejectsPathSwap(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	if err := os.WriteFile(first, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	if file, _, err := openVerifiedArtifact(second, expected); err == nil {
		_ = file.Close()
		t.Fatal("open accepted a file different from the pre-open stat")
	}
}
