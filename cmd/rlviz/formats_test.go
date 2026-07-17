package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unlatch-ai/rlviz/internal/model"
	"github.com/unlatch-ai/rlviz/internal/plugins"
)

func TestCollectFormatsAlwaysReportsCanonicalNDJSON(t *testing.T) {
	result := collectFormats(nil)
	if len(result.Formats) != 1 {
		t.Fatalf("formats = %#v", result.Formats)
	}
	format := result.Formats[0]
	if format.ID != "canonical-ndjson" || format.Source != "built_in" || format.Status != "available" || format.APIVersion != model.APIVersion {
		t.Fatalf("canonical format = %#v", format)
	}
	if text := formatListText(result.Formats); !strings.Contains(text, "Trusted plugins:\n  none") {
		t.Fatalf("format list = %q", text)
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
