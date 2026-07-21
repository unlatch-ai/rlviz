package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/plugins"
	"github.com/TheSnakeFang/rlviz/internal/plugins/sourceprofile"
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
	wantFiles := []string{"rlviz-plugin.yaml", "adapter.py", "test_adapter.py", "testdata/cases.json", "README.md"}
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
	if result.Source.Profile == nil || result.Source.Profile.Kind != sourceprofile.KindJSONObject || result.Source.Profile.SampleBytes != 3 {
		t.Fatalf("source profile=%#v", result.Source.Profile)
	}
	wantCommands := []string{
		shellCommand("rlviz", "plugin", "trust", "--json", result.Path),
		shellCommand("python3", filepath.Join(result.Path, "test_adapter.py")),
		shellCommand("rlviz", "plugin", "validate", "--json", result.Path, resolvedSource),
		shellCommand("rlviz", "open", "--json", "--adapter", result.Path, resolvedSource),
	}
	if !reflect.DeepEqual(result.NextCommands, wantCommands) {
		t.Fatalf("next commands=%#v", result.NextCommands)
	}
}

func TestInitPluginProfileDoesNotCopySourceValues(t *testing.T) {
	root := t.TempDir()
	secret := "customer-secret-value-9f2d"
	source := filepath.Join(root, "private.json")
	if err := os.WriteFile(source, []byte(`{"prompt":"`+secret+`","steps":[{"reward":0.75}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := initPlugin(filepath.Join(root, "adapter"), "private", "adapter", source)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "0.75") {
		t.Fatalf("source values leaked into init plan: %s", raw)
	}
	for _, name := range result.Files {
		contents, err := os.ReadFile(filepath.Join(result.Path, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(contents), secret) || strings.Contains(string(contents), "0.75") {
			t.Fatalf("source values leaked into generated file %s", name)
		}
	}
}

func TestAddDiagnosticFieldsUsesStructuredPluginFailure(t *testing.T) {
	target := map[string]any{"code": "plugin_validate_failed"}
	addDiagnosticFields(target, &plugins.AdapterValidationError{Phase: "stream", Kind: "protocol", Pass: 2, RecordID: "event-7", Field: "sequence", Err: errors.New("bad sequence")})
	if target["phase"] != "stream" || target["kind"] != "protocol" || target["pass"] != 2 || target["record_id"] != "event-7" || target["field"] != "sequence" {
		t.Fatalf("details=%#v", target)
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
