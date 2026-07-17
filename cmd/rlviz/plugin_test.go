package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInitPluginFromSourceReturnsAgentReadyPlan(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "customer trace.jsonl")
	if err := os.WriteFile(source, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "adapter")
	result, err := initPlugin(destination, "customer-trace", "adapter", source)
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := []string{"rlviz-plugin.yaml", "adapter.py", "README.md"}
	if result.SchemaVersion != 1 || result.Status != "created" || !result.ReviewRequired || !reflect.DeepEqual(result.Files, wantFiles) {
		t.Fatalf("result=%#v", result)
	}
	resolvedSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source == nil || result.Source.Path != resolvedSource || result.Source.Kind != "file" || result.Source.SizeBytes != 3 {
		t.Fatalf("source=%#v", result.Source)
	}
	if len(result.NextCommands) != 3 || !strings.Contains(result.NextCommands[1], "plugin validate --json") || !strings.Contains(result.NextCommands[1], "'"+resolvedSource+"'") {
		t.Fatalf("next commands=%#v", result.NextCommands)
	}
}

func TestInitPluginRejectsInvalidFromBeforeCreatingFiles(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "adapter")
	if _, err := initPlugin(destination, "test", "adapter", filepath.Join(root, "missing.trace")); err == nil {
		t.Fatal("expected missing source error")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination was created: %v", err)
	}
	if _, err := initPlugin(destination, "test", "analyzer", filepath.Join(root, "source")); err == nil || !strings.Contains(err.Error(), "only for adapter") {
		t.Fatalf("analyzer error=%v", err)
	}
}
