package plugins

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unlatch-ai/rlviz/internal/analyzers"
	"github.com/unlatch-ai/rlviz/internal/model"
)

func TestAnalyzerHostTrustedSnapshotValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	plugin, store := analyzerPlugin(t, validAnalyzerScript, true)
	host := NewHost(store)
	output, stderr, err := host.Analyze(context.Background(), plugin, analyzerInput())
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" || len(output.Findings) != 1 || output.Findings[0].EventIDs[0] != "event-1" {
		t.Fatalf("output=%+v stderr=%q", output, stderr)
	}
	report, err := host.ValidateAnalyzer(context.Background(), plugin, analyzerInput())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Deterministic || report.Findings != 1 || report.Signals != 0 {
		t.Fatalf("report=%+v", report)
	}
}

func TestAnalyzerHostRejectsUntrustedAndChangedPlugins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	plugin, store := analyzerPlugin(t, validAnalyzerScript, false)
	if _, _, err := NewHost(store).Analyze(context.Background(), plugin, analyzerInput()); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("untrusted error=%v", err)
	}
	if err := store.Trust(plugin); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugin.Path, "analyzer.sh"), []byte(validAnalyzerScript+"\n# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewHost(store).Analyze(context.Background(), plugin, analyzerInput()); err == nil || !strings.Contains(err.Error(), "changed after it was loaded") {
		t.Fatalf("changed error=%v", err)
	}
}

func TestAnalyzerHostBoundsCancelsAndValidatesProtocol(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	t.Run("stdout", func(t *testing.T) {
		plugin, store := analyzerPlugin(t, validAnalyzerScript, true)
		host := NewHost(store)
		host.MaxStdoutBytes = 32
		if _, _, err := host.Analyze(context.Background(), plugin, analyzerInput()); err == nil || !strings.Contains(err.Error(), "stdout exceeded 32 bytes") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		plugin, store := analyzerPlugin(t, "#!/bin/sh\nsleep 5\n", true)
		host := NewHost(store)
		host.Timeout = 30 * time.Millisecond
		if _, _, err := host.Analyze(context.Background(), plugin, analyzerInput()); err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("provenance", func(t *testing.T) {
		script := strings.Replace(validAnalyzerScript, `"$RLVIZ_ANALYZER_NAME"`, `"wrong"`, 1)
		plugin, store := analyzerPlugin(t, script, true)
		if _, _, err := NewHost(store).Analyze(context.Background(), plugin, analyzerInput()); err == nil || !strings.Contains(err.Error(), "provenance") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("nondeterministic", func(t *testing.T) {
		counter := filepath.Join(t.TempDir(), "counter")
		script := validAnalyzerScript + "\nn=0; test ! -f '" + counter + "' || n=$(cat '" + counter + "'); n=$((n + 1)); echo \"$n\" >'" + counter + "'; i=0; while [ \"$i\" -lt \"$n\" ]; do printf ' '; i=$((i + 1)); done\n"
		plugin, store := analyzerPlugin(t, script, true)
		if _, err := NewHost(store).ValidateAnalyzer(context.Background(), plugin, analyzerInput()); err == nil || !strings.Contains(err.Error(), "nondeterministic") {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestAnalyzerInputAndScaffold(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "input.json")
	data := `{"api_version":"rlviz.dev/analyzer/v1alpha1","operation":"analyze","trajectory_id":"trajectory-1","events":[{"record_type":"event","id":"event-1","trajectory_id":"trajectory-1","sequence":0,"kind":"tool"}]}`
	if err := os.WriteFile(inputPath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if input, err := LoadAnalyzerInput(inputPath); err != nil || len(input.Events) != 1 {
		t.Fatalf("input=%+v err=%v", input, err)
	}
	destination := filepath.Join(t.TempDir(), "analyzer")
	if _, err := ScaffoldPython(destination, ScaffoldOptions{Name: "test-analyzer", Kind: "analyzer"}); err != nil {
		t.Fatal(err)
	}
	plugin, err := Load(destination)
	if err != nil {
		t.Fatal(err)
	}
	if plugin.Manifest.Kind != "Analyzer" {
		t.Fatalf("kind=%s", plugin.Manifest.Kind)
	}
	if _, err := os.Stat(filepath.Join(destination, "sample-input.json")); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("python3"); err == nil {
			store := &TrustStore{Path: filepath.Join(t.TempDir(), "trust.json")}
			if err := store.Trust(plugin); err != nil {
				t.Fatal(err)
			}
			input, err := LoadAnalyzerInput(filepath.Join(destination, "sample-input.json"))
			if err != nil {
				t.Fatal(err)
			}
			if report, err := NewHost(store).ValidateAnalyzer(context.Background(), plugin, input); err != nil || !report.Deterministic {
				t.Fatalf("report=%+v err=%v", report, err)
			}
		}
	}
}

func analyzerInput() analyzers.Input {
	return analyzers.Input{APIVersion: analyzers.APIVersion, Operation: analyzers.OperationAnalyze, TrajectoryID: "trajectory-1", Events: []model.Event{{RecordType: model.RecordEvent, ID: "event-1", TrajectoryID: "trajectory-1", Sequence: 0, Kind: "tool"}}}
}

func analyzerPlugin(t *testing.T, script string, trust bool) (*Plugin, *TrustStore) {
	t.Helper()
	dir := t.TempDir()
	manifest := `api_version: rlviz.dev/v1alpha1
kind: Analyzer
name: test-analyzer
version: 1.2.3
command: ["sh", "analyzer.sh"]
capabilities: ["analyzer.analyze"]
`
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "analyzer.sh"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	plugin, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &TrustStore{Path: filepath.Join(t.TempDir(), "trust.json")}
	if trust {
		if err := store.Trust(plugin); err != nil {
			t.Fatal(err)
		}
	}
	return plugin, store
}

const validAnalyzerScript = `#!/bin/sh
printf '{"api_version":"rlviz.dev/analyzer/v1alpha1","provenance":{"name":"%s","version":"%s","digest":"%s","input_digest":"%s"},"findings":[{"id":"finding-1","trajectory_id":"trajectory-1","event_ids":["event-1"],"kind":"example","severity":"info","title":"Example"}],"signals":[]}' "$RLVIZ_ANALYZER_NAME" "$RLVIZ_ANALYZER_VERSION" "$RLVIZ_ANALYZER_DIGEST" "$RLVIZ_ANALYZER_INPUT_DIGEST"
`
