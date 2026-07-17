package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePresentationFileReturnsStableNormalizedResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "presentation.json")
	document := `{"api_version":"rlviz.dev/v1alpha1","fields":{"reward":{"label":"Return"}},"group":{"columns":["reward"]}}`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := validatePresentationFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != 1 || result.Status != "valid" || result.Config.Fields["reward"].Label != "Return" || !strings.HasPrefix(result.Digest, "sha256:") {
		t.Fatalf("result = %#v", result)
	}
	expected, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != expected {
		t.Fatalf("path = %q, want %q", result.Path, expected)
	}
	if string(result.Normalized) != `{"api_version":"rlviz.dev/v1alpha1","fields":{"reward":{"label":"Return"}},"group":{"columns":["reward"]}}` {
		t.Fatalf("normalized = %s", result.Normalized)
	}
}

func TestValidatePresentationFileRejectsInvalidAndNonRegularInputs(t *testing.T) {
	root := t.TempDir()
	invalid := filepath.Join(root, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"api_version":"rlviz.dev/v1alpha1","script":"alert(1)"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validatePresentationFile(invalid); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("invalid error = %v", err)
	}
	if _, err := validatePresentationFile(root); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory error = %v", err)
	}
}

func TestValidatePresentationFileResolvesExplicitSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.json")
	link := filepath.Join(root, "config.json")
	if err := os.WriteFile(target, []byte(`{"api_version":"rlviz.dev/v1alpha1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	result, err := validatePresentationFile(link)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != expected {
		t.Fatalf("path = %q, want resolved %q", result.Path, expected)
	}
}
