package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unlatch-ai/rlviz/internal/model"
)

const validManifest = `api_version: rlviz.dev/v1alpha1
kind: Adapter
name: test-adapter
version: 0.1.0
command:
  - /bin/sh
  - adapter.sh
capabilities: ["adapter.probe", "adapter.stream"]
description: "test adapter"
`

func TestParseManifestDocumentedYAMLAndJSON(t *testing.T) {
	t.Parallel()
	m, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if m.Command[1] != "adapter.sh" {
		t.Fatalf("command = %v", m.Command)
	}
	jsonManifest := `{"api_version":"rlviz.dev/v1alpha1","kind":"Adapter","name":"json","version":"1.0.0","command":["adapter"],"capabilities":["adapter.probe","adapter.stream"]}`
	m, err = ParseManifest([]byte(jsonManifest))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyzerManifestCapabilities(t *testing.T) {
	t.Parallel()
	analyzer := strings.NewReplacer(
		"kind: Adapter", "kind: Analyzer",
		"name: test-adapter", "name: test-analyzer",
		`capabilities: ["adapter.probe", "adapter.stream"]`, `capabilities: ["analyzer.analyze"]`,
	).Replace(validManifest)
	m, err := ParseManifest([]byte(analyzer))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}

	for name, capabilities := range map[string][]string{
		"adapter with analyzer capability":   {"analyzer.analyze"},
		"analyzer with adapter capabilities": {"adapter.probe", "adapter.stream"},
		"analyzer with extra capability":     {"analyzer.analyze", "adapter.stream"},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := m
			candidate.Capabilities = capabilities
			if strings.HasPrefix(name, "adapter") {
				candidate.Kind = "Adapter"
			}
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestAdapterHostRejectsAnalyzerPlugin(t *testing.T) {
	t.Parallel()
	plugin := &Plugin{Manifest: Manifest{Kind: "Analyzer", Name: "test-analyzer"}}
	if _, _, err := NewHost(nil).Probe(context.Background(), plugin, Request{}); err == nil || !strings.Contains(err.Error(), "want Adapter") {
		t.Fatalf("error = %v", err)
	}
}

func TestPluginManifestSchemaContainsManifestKindsAndCapabilities(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "schemas", "v1alpha1", "plugin-manifest.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	values := map[string]bool{}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if key == "const" {
					if text, ok := child.(string); ok {
						values[text] = true
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range v {
				if text, ok := child.(string); ok {
					values[text] = true
				}
				walk(child)
			}
		}
	}
	walk(schema)
	for _, value := range []string{"Adapter", "Analyzer", "adapter.probe", "adapter.stream", "analyzer.analyze"} {
		if !values[value] {
			t.Errorf("schema is missing %q", value)
		}
	}
}

func TestParseManifestRejectsAdversarialInput(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]string{
		"unknown":       validManifest + "surprise: value\n",
		"duplicate":     validManifest + "name: second\n",
		"nested":        "api_version:\n  nope: value\n",
		"json trailing": `{"api_version":"x"} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest([]byte(input)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestDigestAndTrustIncludeImportedHelpers(t *testing.T) {
	t.Parallel()
	dir := newPlugin(t, stableScript)
	helper := filepath.Join(dir, "helper.py")
	writeFile(t, helper, "VALUE = 1\n")
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &TrustStore{Path: filepath.Join(t.TempDir(), "trust.json")}
	if err := store.Trust(p); err != nil {
		t.Fatal(err)
	}
	if err := store.Require(p); err != nil {
		t.Fatal(err)
	}
	writeFile(t, helper, "VALUE = 2\n")
	if err := store.Require(p); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("error = %v, want ErrUntrusted", err)
	}
	writeFile(t, helper, "VALUE = 1\n")
	p, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Trust(p); err != nil {
		t.Fatal(err)
	}
	bytecode := filepath.Join(dir, "helper.pyc")
	writeFile(t, bytecode, "first")
	p, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Trust(p); err != nil {
		t.Fatal(err)
	}
	writeFile(t, bytecode, "second")
	if err := store.Require(p); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("bytecode mutation error = %v, want ErrUntrusted", err)
	}
	cache := filepath.Join(dir, "nested", "__pycache__")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	bytecode = filepath.Join(cache, "helper.cpython-313.pyc")
	writeFile(t, bytecode, "first")
	p, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Trust(p); err != nil {
		t.Fatal(err)
	}
	writeFile(t, bytecode, "second")
	if err := store.Require(p); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("nested bytecode mutation error = %v, want ErrUntrusted", err)
	}
}

func TestPluginSnapshotIsImmutable(t *testing.T) {
	t.Parallel()
	dir := newPlugin(t, stableScript)
	plugin, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, cleanup, err := snapshotPlugin(plugin)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	writeFile(t, filepath.Join(dir, "adapter.sh"), "changed")
	data, err := os.ReadFile(filepath.Join(snapshot.Path, "adapter.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != stableScript || snapshot.Digest != plugin.Digest {
		t.Fatal("verified snapshot changed with the source plugin")
	}
}

func TestDigestRejectsEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics")
	}
	t.Parallel()
	dir := newPlugin(t, stableScript)
	outside := filepath.Join(t.TempDir(), "outside")
	writeFile(t, outside, "x")
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "escapes plugin root") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsAbsolutePluginLocalCommandPath(t *testing.T) {
	t.Parallel()
	dir := newPlugin(t, stableScript)
	absolute := filepath.Join(dir, "adapter.sh")
	manifest := strings.Replace(validManifest, "  - adapter.sh\n", "  - "+absolute+"\n", 1)
	writeFile(t, filepath.Join(dir, ManifestName), manifest)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "must be relative") {
		t.Fatalf("error = %v", err)
	}
}

func TestTrustDigestIncludesExternalCommandFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	external := filepath.Join(t.TempDir(), "adapter-command")
	writeFile(t, external, "version one\n")
	manifest := strings.Replace(validManifest, "  - /bin/sh\n  - adapter.sh\n", "  - "+external+"\n", 1)
	writeFile(t, filepath.Join(dir, ManifestName), manifest)
	plugin, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &TrustStore{Path: filepath.Join(t.TempDir(), "trust.json")}
	if err := store.Trust(plugin); err != nil {
		t.Fatal(err)
	}
	writeFile(t, external, "version two\n")
	if err := store.Require(plugin); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("external command mutation error = %v, want ErrUntrusted", err)
	}
}

func TestTrustStorePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	t.Parallel()
	dir := newPlugin(t, stableScript)
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &TrustStore{Path: filepath.Join(t.TempDir(), "nested", "trust.json")}
	if err := store.Trust(p); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	if err := os.Chmod(store.Path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.IsTrusted(p); err == nil || !strings.Contains(err.Error(), "insecure") {
		t.Fatalf("error = %v", err)
	}
}

func TestTrustStoreListAndRevoke(t *testing.T) {
	t.Parallel()
	dir := newPlugin(t, stableScript)
	plugin, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &TrustStore{Path: filepath.Join(t.TempDir(), "trust.json")}
	if err := store.Trust(plugin); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != plugin.Path || entries[0].Digest != plugin.Digest {
		t.Fatalf("entries = %#v", entries)
	}
	if err := store.Revoke(plugin.Path); err != nil {
		t.Fatal(err)
	}
	trusted, err := store.IsTrusted(plugin)
	if err != nil {
		t.Fatal(err)
	}
	if trusted {
		t.Fatal("plugin remained trusted after revoke")
	}
}

func TestHostProbeStreamAndValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	t.Parallel()
	dir := newPlugin(t, stableScript)
	source := filepath.Join(t.TempDir(), "trace.jsonl")
	writeFile(t, source, "source\n")
	p, store := loadAndTrust(t, dir)
	host := NewHost(store)
	req, err := NewRequest("probe", source, "")
	if err != nil {
		t.Fatal(err)
	}
	probe, stderr, err := host.Probe(context.Background(), p, req)
	if err != nil {
		t.Fatal(err)
	}
	if !probe.Supported || probe.Format != "test-v1" || strings.TrimSpace(stderr) != "diagnostic" {
		t.Fatalf("probe=%+v stderr=%q", probe, stderr)
	}
	report, err := host.ValidateAdapter(context.Background(), p, source, "")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Deterministic || report.Records != 5 || report.Format != "test-v1" {
		t.Fatalf("report=%+v", report)
	}
}

func TestHostRejectsUntrustedAndProtocolPollution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	t.Parallel()
	source := filepath.Join(t.TempDir(), "trace")
	writeFile(t, source, "x")
	dir := newPlugin(t, stableScript)
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &TrustStore{Path: filepath.Join(t.TempDir(), "trust.json")}
	host := NewHost(store)
	req, _ := NewRequest("probe", source, "")
	if _, _, err := host.Probe(context.Background(), p, req); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("error=%v", err)
	}
	dir = newPlugin(t, pollutedScript)
	p, store = loadAndTrust(t, dir)
	host = NewHost(store)
	req, _ = NewRequest("probe", source, "")
	if _, _, err := host.Probe(context.Background(), p, req); err == nil {
		t.Fatal("expected polluted stdout error")
	}
}

func TestHostStreamEnforcesSourceProvenance(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "trace")
	writeFile(t, source, "x")
	outside := filepath.Join(t.TempDir(), "outside")
	writeFile(t, outside, "secret")
	script := `#!/bin/sh
printf '%s\n' \
  '{"record_type":"run","id":"run"}' \
  '{"record_type":"case","id":"case","run_id":"run"}' \
  '{"record_type":"group","id":"group","case_id":"case"}' \
  '{"record_type":"trajectory","id":"trajectory","group_id":"group"}' \
  '{"record_type":"event","id":"event","trajectory_id":"trajectory","sequence":0,"kind":"message","source":{"path":"` + outside + `","byte_offset":0,"byte_length":1}}' \
  '{"record_type":"complete","records":5,"warnings":0}'
`
	dir := newPlugin(t, script)
	plugin, store := loadAndTrust(t, dir)
	request, err := NewRequest("stream", source, root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewHost(store).Stream(context.Background(), plugin, request, func(*model.Record) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "escapes registered root") {
		t.Fatalf("error = %v", err)
	}
}

func TestHostStreamVisitsBeforeAdapterExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	source := filepath.Join(t.TempDir(), "trace")
	writeFile(t, source, "x")
	script := `#!/bin/sh
printf '%s\n' '{"record_type":"run","id":"run"}'
sleep 2
printf '%s\n' \
  '{"record_type":"case","id":"case","run_id":"run"}' \
  '{"record_type":"group","id":"group","case_id":"case"}' \
  '{"record_type":"trajectory","id":"trajectory","group_id":"group"}' \
  '{"record_type":"complete","records":4,"warnings":0}'
`
	plugin, store := loadAndTrust(t, newPlugin(t, script))
	request, err := NewRequest("stream", source, "")
	if err != nil {
		t.Fatal(err)
	}
	visited := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, streamErr := NewHost(store).Stream(context.Background(), plugin, request, func(*model.Record) error {
			select {
			case visited <- struct{}{}:
			default:
			}
			return nil
		})
		done <- streamErr
	}()
	select {
	case <-visited:
	case <-time.After(time.Second):
		t.Fatal("first record was not delivered while adapter was still running")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestHostStreamSupportsLargeIncrementalOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	source := filepath.Join(t.TempDir(), "trace")
	writeFile(t, source, "x")
	script := `#!/bin/sh
printf '%s\n' \
  '{"record_type":"run","id":"run"}' \
  '{"record_type":"case","id":"case","run_id":"run"}' \
  '{"record_type":"group","id":"group","case_id":"case"}' \
  '{"record_type":"trajectory","id":"trajectory","group_id":"group"}'
padding=$(awk 'BEGIN { for (i=0; i<20000; i++) printf "x" }')
i=0
while [ "$i" -lt 1700 ]; do
  printf '{"record_type":"event","id":"event-%s","trajectory_id":"trajectory","sequence":%s,"kind":"message","data":"%s"}\n' "$i" "$i" "$padding"
  i=$((i + 1))
done
printf '%s\n' '{"record_type":"complete","records":1704,"warnings":0}'
`
	plugin, store := loadAndTrust(t, newPlugin(t, script))
	request, err := NewRequest("stream", source, "")
	if err != nil {
		t.Fatal(err)
	}
	host := NewHost(store)
	host.Timeout = 30 * time.Second
	host.MaxStdoutBytes = 40 << 20
	count := 0
	if _, err := host.Stream(context.Background(), plugin, request, func(*model.Record) error {
		count++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1705 {
		t.Fatalf("visited %d records, want 1705", count)
	}
}

func TestHostStreamCancellationKillsAdapter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	source := filepath.Join(t.TempDir(), "trace")
	writeFile(t, source, "x")
	plugin, store := loadAndTrust(t, newPlugin(t, "#!/bin/sh\nprintf '%s\\n' '{\"record_type\":\"run\",\"id\":\"run\"}'\nsleep 10\n"))
	request, _ := NewRequest("stream", source, "")
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	_, err := NewHost(store).Stream(ctx, plugin, request, func(*model.Record) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("adapter cancellation took %s", elapsed)
	}
}

func TestHostStreamBoundsOutputAndDiagnostics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	source := filepath.Join(t.TempDir(), "trace")
	writeFile(t, source, "x")
	t.Run("stdout", func(t *testing.T) {
		plugin, store := loadAndTrust(t, newPlugin(t, stableScript))
		request, _ := NewRequest("stream", source, "")
		host := NewHost(store)
		host.MaxStdoutBytes = 32
		_, err := host.Stream(context.Background(), plugin, request, func(*model.Record) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "stdout exceeded 32 bytes") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("stderr", func(t *testing.T) {
		script := "#!/bin/sh\nprintf 'this diagnostic is deliberately too long' >&2\nprintf '%s\\n' '{\"record_type\":\"complete\",\"records\":0,\"warnings\":0}'\n"
		plugin, store := loadAndTrust(t, newPlugin(t, script))
		request, _ := NewRequest("stream", source, "")
		host := NewHost(store)
		host.MaxStderrBytes = 12
		stderr, err := host.Stream(context.Background(), plugin, request, func(*model.Record) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "stderr exceeded 12 bytes") {
			t.Fatalf("stderr=%q len=%d error = %v", stderr, len(stderr), err)
		}
		if len(stderr) != 12 {
			t.Fatalf("stderr length = %d, want 12", len(stderr))
		}
	})
	t.Run("invalid stream", func(t *testing.T) {
		script := "#!/bin/sh\nprintf 'adapter detail' >&2\nprintf 'not-json\\n'\n"
		plugin, store := loadAndTrust(t, newPlugin(t, script))
		request, _ := NewRequest("stream", source, "")
		stderr, err := NewHost(store).Stream(context.Background(), plugin, request, func(*model.Record) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "invalid canonical stream") || stderr != "adapter detail" {
			t.Fatalf("stderr=%q error=%v", stderr, err)
		}
	})
}

func TestProbeRequiresSchemaFields(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	t.Parallel()
	source := filepath.Join(t.TempDir(), "trace")
	writeFile(t, source, "x")
	dir := newPlugin(t, "#!/bin/sh\necho '{}'\n")
	p, store := loadAndTrust(t, dir)
	host := NewHost(store)
	req, _ := NewRequest("probe", source, "")
	if _, _, err := host.Probe(context.Background(), p, req); err == nil || !strings.Contains(err.Error(), "requires supported and confidence") {
		t.Fatalf("error=%v", err)
	}
	_, err := host.ValidateAdapter(context.Background(), p, source, "")
	var failure *AdapterValidationError
	if !errors.As(err, &failure) || failure.Phase != ValidationPhaseProbe || failure.Kind != ValidationKindProtocol || failure.Pass != 1 {
		t.Fatalf("failure=%#v error=%v", failure, err)
	}
}

func TestHostTimeoutAndNondeterminism(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	t.Parallel()
	source := filepath.Join(t.TempDir(), "trace")
	writeFile(t, source, "x")
	dir := newPlugin(t, timeoutScript)
	p, store := loadAndTrust(t, dir)
	host := NewHost(store)
	host.Timeout = 30 * time.Millisecond
	req, _ := NewRequest("probe", source, "")
	if _, _, err := host.Probe(context.Background(), p, req); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error=%v", err)
	}
	dir = newPlugin(t, nondeterministicScript)
	p, store = loadAndTrust(t, dir)
	host = NewHost(store)
	if _, err := host.ValidateAdapter(context.Background(), p, source, ""); err == nil || !strings.Contains(err.Error(), "nondeterministic") {
		t.Fatalf("error=%v", err)
	} else {
		var failure *AdapterValidationError
		if !errors.As(err, &failure) || failure.Phase != ValidationPhaseProbe || failure.Kind != ValidationKindNondeterministic {
			t.Fatalf("failure=%#v error=%v", failure, err)
		}
	}
}

func TestAdapterValidationPreservesUntrustedIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	t.Parallel()
	source := filepath.Join(t.TempDir(), "trace")
	writeFile(t, source, "x")
	plugin, err := Load(newPlugin(t, stableScript))
	if err != nil {
		t.Fatal(err)
	}
	store := &TrustStore{Path: filepath.Join(t.TempDir(), "trust.json")}
	_, err = NewHost(store).ValidateAdapter(context.Background(), plugin, source, "")
	if !errors.Is(err, ErrUntrusted) {
		t.Fatalf("error=%v, want ErrUntrusted", err)
	}
	var failure *AdapterValidationError
	if !errors.As(err, &failure) || failure.Phase != ValidationPhaseProbe || failure.Kind != ValidationKindExecution || failure.Pass != 1 {
		t.Fatalf("failure=%#v error=%v", failure, err)
	}
}

func TestAdapterValidationErrorPreservesPhaseAndRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	t.Parallel()
	source := filepath.Join(t.TempDir(), "trace")
	writeFile(t, source, "x")
	dir := newPlugin(t, invalidCanonicalScript)
	plugin, store := loadAndTrust(t, dir)
	_, err := NewHost(store).ValidateAdapter(context.Background(), plugin, source, "")
	var failure *AdapterValidationError
	if !errors.As(err, &failure) {
		t.Fatalf("error=%v, want AdapterValidationError", err)
	}
	if failure.Phase != "stream" || failure.Kind != "protocol" || failure.Pass != 1 || failure.Line != 5 || failure.RecordType != model.RecordEvent || failure.RecordID != "event" || failure.Field != "sequence" {
		t.Fatalf("failure=%#v", failure)
	}
	details := failure.DiagnosticFields()
	if details["phase"] != "stream" || details["kind"] != "protocol" || details["pass"] != 1 || details["record_type"] != model.RecordEvent || details["record_id"] != "event" || details["field"] != "sequence" {
		t.Fatalf("details=%#v", details)
	}
}

func TestAdapterValidationClassifiesMalformedSecondPassBeforeNondeterminism(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	t.Parallel()
	source := filepath.Join(t.TempDir(), "trace")
	writeFile(t, source, "x")
	counter := filepath.Join(t.TempDir(), "stream-count")
	dir := newPlugin(t, malformedSecondStreamScript(counter))
	plugin, store := loadAndTrust(t, dir)
	_, err := NewHost(store).ValidateAdapter(context.Background(), plugin, source, "")
	var failure *AdapterValidationError
	if !errors.As(err, &failure) {
		t.Fatalf("error=%v, want AdapterValidationError", err)
	}
	if failure.Phase != ValidationPhaseStream || failure.Kind != ValidationKindProtocol || failure.Pass != 2 {
		t.Fatalf("failure=%#v error=%v", failure, err)
	}
}

func TestSourceRootAndScaffold(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "trace")
	writeFile(t, outside, "x")
	if _, err := NewRequest("probe", outside, root); err == nil {
		t.Fatal("expected root escape error")
	}
	destination := filepath.Join(t.TempDir(), "adapter")
	files, err := ScaffoldPython(destination, ScaffoldOptions{Name: "customer-x"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{ManifestName, "adapter.py", "test_adapter.py", "testdata/cases.json", "README.md"}; !reflect.DeepEqual(files, want) {
		t.Fatalf("files=%#v want=%#v", files, want)
	}
	if _, err := Load(destination); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("python3"); err == nil {
		plugin, err := Load(destination)
		if err != nil {
			t.Fatal(err)
		}
		store := &TrustStore{Path: filepath.Join(t.TempDir(), "trust.json")}
		if err := store.Trust(plugin); err != nil {
			t.Fatal(err)
		}
		request, err := NewRequest("probe", outside, "")
		if err != nil {
			t.Fatal(err)
		}
		probe, _, err := NewHost(store).Probe(context.Background(), plugin, request)
		if err != nil || probe.Supported || !strings.Contains(probe.Reason, "implement format detection") {
			t.Fatalf("generated probe=%#v err=%v", probe, err)
		}
		_, err = NewHost(store).ValidateAdapter(context.Background(), plugin, outside, "")
		var failure *AdapterValidationError
		if !errors.As(err, &failure) || failure.Phase != ValidationPhaseProbe || failure.Kind != ValidationKindUnsupported {
			t.Fatalf("generated validation failure=%#v err=%v", failure, err)
		}
	}
	if _, err := ScaffoldPython(destination, ScaffoldOptions{Name: "customer-x"}); err == nil {
		t.Fatal("expected overwrite refusal")
	}
}

func TestScaffoldAdapterRunnerUsesStableSafeCases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable fixture semantics")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is unavailable")
	}
	root := t.TempDir()
	destination := filepath.Join(root, "adapter")
	if _, err := ScaffoldPython(destination, ScaffoldOptions{Name: "runner-test"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.source", "second;unsafe.source"} {
		writeFile(t, filepath.Join(destination, "testdata", name), "synthetic\n")
	}
	cases := `{"schema_version":1,"cases":[{"source":"first.source","expected_format":"customer-v2","min_records":4},{"source":"second;unsafe.source","expected_format":"customer-v2","min_records":5}]}
`
	writeExistingFile(t, filepath.Join(destination, "testdata", "cases.json"), cases)
	logPath := filepath.Join(root, "argv.jsonl")
	fake := filepath.Join(root, "fake-rlviz")
	fakeScript := `#!` + python + `
import json
import os
from pathlib import Path
import sys

with open(os.environ["RLVIZ_FAKE_LOG"], "a", encoding="utf-8") as handle:
    handle.write(json.dumps(sys.argv[1:]) + "\n")
mode = os.environ.get("RLVIZ_FAKE_MODE", "ok")
if mode == "malformed":
    print("not-json")
elif mode == "wrong-format":
    print(json.dumps({"format":"other","records":9,"deterministic":True}))
elif mode == "too-few":
    print(json.dumps({"format":"customer-v2","records":1,"deterministic":True}))
else:
    count = 4 if Path(sys.argv[-1]).name == "first.source" else 5
    print(json.dumps({"format":"customer-v2","records":count,"deterministic":True}))
`
	if err := os.WriteFile(fake, []byte(fakeScript), 0o700); err != nil {
		t.Fatal(err)
	}

	run := exec.Command(python, "test_adapter.py")
	run.Dir = destination
	run.Env = append(os.Environ(), "RLVIZ_BIN="+fake, "RLVIZ_FAKE_LOG="+logPath)
	output, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("runner failed: %v\n%s", err, output)
	}
	if got := string(output); !strings.Contains(got, "ok first.source") || !strings.Contains(got, "ok second;unsafe.source") {
		t.Fatalf("runner output=%q", got)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if len(lines) != 2 {
		t.Fatalf("argv log=%q", logData)
	}
	resolvedDestination, err := filepath.EvalSymlinks(destination)
	if err != nil {
		t.Fatal(err)
	}
	for index, line := range lines {
		var argv []string
		if err := json.Unmarshal([]byte(line), &argv); err != nil {
			t.Fatal(err)
		}
		if len(argv) != 5 || argv[0] != "plugin" || argv[1] != "validate" || argv[2] != "--json" || argv[3] != resolvedDestination {
			t.Fatalf("argv[%d]=%#v", index, argv)
		}
	}
	if !strings.HasSuffix(lines[0], `first.source"]`) || !strings.HasSuffix(lines[1], `second;unsafe.source"]`) {
		t.Fatalf("case order changed: %q", logData)
	}
}

func TestScaffoldAdapterRunnerRejectsUnsafeOrInvalidCases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is unavailable")
	}
	root := t.TempDir()
	destination := filepath.Join(root, "adapter")
	if _, err := ScaffoldPython(destination, ScaffoldOptions{Name: "runner-negative"}); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(root, "fake-rlviz")
	if err := os.WriteFile(fake, []byte("#!"+python+"\nimport os\nprint(os.environ.get('RLVIZ_FAKE_OUTPUT', '{}'))\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(destination, "testdata", "sample.source")
	writeFile(t, fixture, "synthetic\n")
	outside := filepath.Join(root, "outside.source")
	writeFile(t, outside, "private\n")
	link := filepath.Join(destination, "testdata", "escape.source")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		cases  string
		output string
		want   string
	}{
		{name: "zero cases", cases: `{"schema_version":1,"cases":[]}`, want: "no adapter cases"},
		{name: "traversal", cases: `{"schema_version":1,"cases":[{"source":"../outside.source","expected_format":"x","min_records":1}]}`, want: "must stay inside testdata"},
		{name: "symlink escape", cases: `{"schema_version":1,"cases":[{"source":"escape.source","expected_format":"x","min_records":1}]}`, want: "must be a file inside testdata"},
		{name: "malformed result", cases: `{"schema_version":1,"cases":[{"source":"sample.source","expected_format":"x","min_records":1}]}`, output: "not-json", want: "invalid JSON"},
		{name: "nondeterministic result", cases: `{"schema_version":1,"cases":[{"source":"sample.source","expected_format":"x","min_records":1}]}`, output: `{"format":"x","records":2,"deterministic":false}`, want: "did not report deterministic"},
		{name: "wrong format", cases: `{"schema_version":1,"cases":[{"source":"sample.source","expected_format":"x","min_records":1}]}`, output: `{"format":"y","records":2,"deterministic":true}`, want: "expected 'x'"},
		{name: "too few records", cases: `{"schema_version":1,"cases":[{"source":"sample.source","expected_format":"x","min_records":3}]}`, output: `{"format":"x","records":2,"deterministic":true}`, want: "expected at least 3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeExistingFile(t, filepath.Join(destination, "testdata", "cases.json"), test.cases+"\n")
			run := exec.Command(python, "test_adapter.py")
			run.Dir = destination
			run.Env = append(os.Environ(), "RLVIZ_BIN="+fake, "RLVIZ_FAKE_OUTPUT="+test.output)
			output, err := run.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.want) {
				t.Fatalf("error=%v output=%q, want %q", err, output, test.want)
			}
		})
	}
}

func TestScaffoldRefusesSymlinkAndPreflightsAllFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ScaffoldPython(link, ScaffoldOptions{Name: "test"}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symlink error=%v", err)
	}
	if entries, err := os.ReadDir(target); err != nil || len(entries) != 0 {
		t.Fatalf("scaffold traversed symlink: entries=%v err=%v", entries, err)
	}

	destination := filepath.Join(root, "existing")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(destination, "README.md"), "keep\n")
	if _, err := ScaffoldPython(destination, ScaffoldOptions{Name: "test"}); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("preflight error=%v", err)
	}
	for _, name := range []string{ManifestName, "adapter.py", "test_adapter.py", "testdata"} {
		if _, err := os.Lstat(filepath.Join(destination, name)); !os.IsNotExist(err) {
			t.Fatalf("partial file %s exists: %v", name, err)
		}
	}

	nestedDestination := filepath.Join(root, "nested-link")
	if err := os.Mkdir(nestedDestination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(nestedDestination, "testdata")); err != nil {
		t.Fatal(err)
	}
	if _, err := ScaffoldPython(nestedDestination, ScaffoldOptions{Name: "test"}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("nested symlink error=%v", err)
	}
	for _, name := range []string{ManifestName, "adapter.py", "test_adapter.py", "README.md"} {
		if _, err := os.Lstat(filepath.Join(nestedDestination, name)); !os.IsNotExist(err) {
			t.Fatalf("rollback left %s: %v", name, err)
		}
	}
}

func newPlugin(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ManifestName), validManifest)
	writeFile(t, filepath.Join(dir, "adapter.sh"), script)
	return dir
}
func loadAndTrust(t *testing.T, dir string) (*Plugin, *TrustStore) {
	t.Helper()
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &TrustStore{Path: filepath.Join(t.TempDir(), "trust.json")}
	if err := store.Trust(p); err != nil {
		t.Fatal(err)
	}
	return p, store
}
func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeExistingFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

const stableScript = `#!/bin/sh
op="$1"
if [ "$op" = probe ]; then
  echo diagnostic >&2
  printf '%s\n' '{"supported":true,"confidence":0.9,"format":"test-v1","reason":"recognized"}'
else
  printf '%s\n' \
    '{"record_type":"run","id":"run"}' \
    '{"record_type":"case","id":"case","run_id":"run"}' \
    '{"record_type":"group","id":"group","case_id":"case"}' \
    '{"record_type":"trajectory","id":"trajectory","group_id":"group"}' \
    '{"record_type":"event","id":"event","trajectory_id":"trajectory","sequence":0,"kind":"message"}' \
    '{"record_type":"complete","records":5,"warnings":0}'
fi
`
const pollutedScript = `#!/bin/sh
echo hello
echo '{"supported":true,"confidence":1,"format":"x"}'
`
const timeoutScript = `#!/bin/sh
sleep 2
`
const nondeterministicScript = `#!/bin/sh
if [ "$1" = probe ]; then
  printf '{"supported":true,"confidence":1,"format":"x-'; date +%s%N | tr -d '\n'; printf '"}\n'
else
  echo '{"record_type":"complete","records":0,"warnings":0}'
fi
`
const invalidCanonicalScript = `#!/bin/sh
if [ "$1" = probe ]; then
  printf '%s\n' '{"supported":true,"confidence":1,"format":"invalid-v1","reason":"fixture"}'
else
  printf '%s\n' \
    '{"record_type":"run","id":"run"}' \
    '{"record_type":"case","id":"case","run_id":"run"}' \
    '{"record_type":"group","id":"group","case_id":"case"}' \
    '{"record_type":"trajectory","id":"trajectory","group_id":"group"}' \
    '{"record_type":"event","id":"event","trajectory_id":"trajectory","sequence":-1,"kind":"message"}' \
    '{"record_type":"complete","records":5,"warnings":0}'
fi
`

func malformedSecondStreamScript(counter string) string {
	counter = strings.ReplaceAll(counter, "'", "'\"'\"'")
	return `#!/bin/sh
if [ "$1" = probe ]; then
  printf '%s\n' '{"supported":true,"confidence":1,"format":"second-pass-v1","reason":"fixture"}'
else
  counter='` + counter + `'
  count=0
  if [ -f "$counter" ]; then read count < "$counter"; fi
  count=$((count + 1))
  printf '%s\n' "$count" > "$counter"
  if [ "$count" -eq 1 ]; then
    printf '%s\n' '{"record_type":"complete","records":0,"warnings":0}'
  else
    printf '%s\n' 'not-json'
  fi
fi
`
}
