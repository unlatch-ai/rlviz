package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/model"
	"github.com/TheSnakeFang/rlviz/internal/plugins"
)

func TestCollectFormatsAlwaysReportsCanonicalNDJSON(t *testing.T) {
	result := collectFormats(nil)
	if len(result.Formats) != 4 {
		t.Fatalf("formats = %#v", result.Formats)
	}
	format := result.Formats[0]
	if format.ID != "canonical-ndjson" || format.Source != "built_in" || format.Status != "available" || format.APIVersion != model.APIVersion {
		t.Fatalf("canonical format = %#v", format)
	}
	if text := formatListText(result.Formats); !strings.Contains(text, "Trusted plugins:\n  none") || !strings.Contains(text, "inspect-ai-eval-log-json-v2  example") {
		t.Fatalf("format list = %q", text)
	}
	for _, example := range result.Formats[1:] {
		if example.Source != "example_adapter" || example.Status != "example" {
			t.Fatalf("example format = %#v", example)
		}
	}
}

func TestCollectFormatsIncludesSchemaVersionedDiscoveryInventory(t *testing.T) {
	discovery := plugins.DiscoveryResult{
		SchemaVersion: 1,
		Plugins: []plugins.DiscoveredPlugin{{
			Rank: 1, Source: "project", Path: "/repo/.rlviz/plugins/customer",
			ManifestPath: "/repo/.rlviz/plugins/customer/rlviz-plugin.yaml",
			Name:         "customer", Kind: "Adapter", APIVersion: plugins.APIVersion,
			Version: "1.0.0", Status: "untrusted",
		}},
		Issues: []plugins.DiscoveryIssue{{Root: "/extra", Code: "root_unreadable", Error: "denied"}},
	}
	result := collectFormats(nil, discovery)
	if result.SchemaVersion != 1 || len(result.DiscoveryIssues) != 1 || len(result.Formats) != 5 {
		t.Fatalf("result = %#v", result)
	}
	got := result.Formats[1]
	if got.Name != "customer" || got.Source != "project_plugin" || got.Status != "untrusted" || got.Rank != 1 {
		t.Fatalf("discovered format = %#v", got)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"schema_version":1`, `"discovery_issues"`, `"status":"untrusted"`, `"rank":1`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("JSON %s missing %s", raw, field)
		}
	}
	if text := formatListText(result.Formats); !strings.Contains(text, "inventory only; not executed") {
		t.Fatalf("human output obscures trust boundary: %q", text)
	}
}

func TestCollectFormatsReportsTrustedChangedAndUnavailablePlugins(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "adapter")
	if err := os.Mkdir(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `api_version: rlviz.dev/v1alpha1
kind: Adapter
name: research-trace
version: 1.2.3
command:
  - python3
  - adapter.py
capabilities:
  - adapter.probe
  - adapter.stream
description: Synthetic research trace adapter
`
	if err := os.WriteFile(filepath.Join(pluginDir, plugins.ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "adapter.py"), []byte("print('ok')\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	plugin, err := plugins.Load(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing")
	result := collectFormats([]plugins.TrustEntry{
		{Path: pluginDir, Digest: plugin.Digest},
		{Path: pluginDir, Digest: "sha256:changed"},
		{Path: missing, Digest: "sha256:missing"},
	})
	if got := result.Formats[1]; got.Name != "research-trace" || got.Status != "trusted" || got.Version != "1.2.3" {
		t.Fatalf("trusted = %#v", got)
	}
	if got := result.Formats[2]; got.Status != "changed" || got.Error == "" {
		t.Fatalf("changed = %#v", got)
	}
	if got := result.Formats[3]; got.Status != "unavailable" || got.Error == "" {
		t.Fatalf("unavailable = %#v", got)
	}
}
