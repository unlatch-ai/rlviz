package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/plugins"
)

func TestLoadSourceReturnsStructuredUnsupportedFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, []byte("not canonical\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadSource(context.Background(), path, "")
	var unsupported *UnsupportedFormatError
	if !errors.As(err, &unsupported) {
		t.Fatalf("error = %v, want UnsupportedFormatError", err)
	}
	if unsupported.DiagnosticCode() != "unsupported_format" || unsupported.DiagnosticFields()["path"] == "" {
		t.Fatalf("diagnostic = %s %#v", unsupported.DiagnosticCode(), unsupported.DiagnosticFields())
	}
	if command, _ := unsupported.DiagnosticFields()["suggested_command"].(string); !strings.Contains(command, "--from") || !strings.Contains(command, unsupported.Path) {
		t.Fatalf("unexpected scaffold command: %q", command)
	}
}

func TestLoadSourceRequiresTrustThenRunsExampleAdapter(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not available")
	}
	t.Setenv("RLVIZ_CONFIG_DIR", t.TempDir())
	adapter := filepath.Join("..", "..", "examples", "adapters", "simple-jsonl")
	source := filepath.Join("..", "..", "examples", "traces", "simple-agent.jsonl")

	_, _, err := LoadSource(context.Background(), source, adapter)
	var untrusted *PluginUntrustedError
	if !errors.As(err, &untrusted) {
		t.Fatalf("error = %v, want PluginUntrustedError", err)
	}

	plugin, err := plugins.Load(adapter)
	if err != nil {
		t.Fatal(err)
	}
	store, err := plugins.DefaultTrustStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Trust(plugin); err != nil {
		t.Fatal(err)
	}
	resolved, document, err := LoadSource(context.Background(), source, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if resolved == "" || document.Trajectory == nil || document.Run == nil || len(document.Events) != 4 || document.Run.Metadata["adapter"] != "simple-jsonl" {
		t.Fatalf("resolved/document = %q %#v", resolved, document)
	}
}
