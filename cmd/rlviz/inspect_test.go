package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/plugins"
	"github.com/TheSnakeFang/rlviz/internal/plugins/sourceprofile"
)

const canonicalPrefix = `{"record_type":"run","id":"run-test"}
{"record_type":"case","id":"case-test","run_id":"run-test"}
{"record_type":"group","id":"group-test","case_id":"case-test"}
{"record_type":"trajectory","id":"trajectory-test","group_id":"group-test","status":"completed"}
`

const canonicalTrajectory = canonicalPrefix + `{"record_type":"complete","records":4,"warnings":0}
`

func TestInspectCanonicalReturnsStableSupportedResult(t *testing.T) {
	source := filepath.Join(t.TempDir(), "trace with spaces.ndjson")
	if err := os.WriteFile(source, []byte(canonicalTrajectory), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := inspectSource(context.Background(), source, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported || result.Format != "canonical-ndjson" || result.Confidence != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.Adapter == nil || result.Adapter.Kind != "built_in" || result.Adapter.Name != "canonical-ndjson" {
		t.Fatalf("adapter = %#v", result.Adapter)
	}
	if len(result.Warnings) != 0 || !strings.Contains(result.NextCommand, "trace with spaces.ndjson'") {
		t.Fatalf("warnings/next command = %#v / %q", result.Warnings, result.NextCommand)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"path":`, `"shape":`, `"supported":`, `"format":`, `"adapter":`, `"confidence":`, `"reason":`, `"warnings":[]`, `"next_command":`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("JSON %s lacks %s", raw, field)
		}
	}
}

func TestInspectCanonicalUnsupportedIsSuccessful(t *testing.T) {
	source := filepath.Join(t.TempDir(), "unknown.json")
	if err := os.WriteFile(source, []byte("{not json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := inspectSource(context.Background(), source, "", nil)
	if err != nil {
		t.Fatalf("unsupported inspection returned error: %v", err)
	}
	if result.Supported || result.Confidence != 0 || result.Reason == "" || !strings.Contains(result.NextCommand, "plugin init") || !strings.Contains(result.NextCommand, "--from") || !strings.Contains(result.NextCommand, result.Path) {
		t.Fatalf("result = %#v", result)
	}
}

func TestInspectUnsupportedJSONIncludesValueFreeProfile(t *testing.T) {
	secret := "private-prompt-32c1"
	source := filepath.Join(t.TempDir(), "unknown.json")
	if err := os.WriteFile(source, []byte(`{"prompt":"`+secret+`","events":[{"kind":"tool","score":1}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := inspectSource(context.Background(), source, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Supported || result.Shape.Profile == nil || result.Shape.Profile.Kind != sourceprofile.KindJSONObject {
		t.Fatalf("result = %#v", result)
	}
	if result.Reason != "source is a JSON object document, not canonical NDJSON" {
		t.Fatalf("reason = %q", result.Reason)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("inspect profile exposed source value: %s", raw)
	}
}

func TestInspectCanonicalProbeIsBounded(t *testing.T) {
	source := filepath.Join(t.TempDir(), "large.ndjson")
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(canonicalPrefix); err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(inspectProbeBytes + 1024); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := inspectSource(context.Background(), source, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported || result.Confidence != 0.99 || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "limited") {
		t.Fatalf("result = %#v", result)
	}
}

func TestInspectCanonicalAcceptsCompleteSourceAtRecordLimit(t *testing.T) {
	var source strings.Builder
	source.WriteString(canonicalPrefix)
	for index := 0; index < inspectProbeRecords-5; index++ {
		_, _ = source.WriteString(fmt.Sprintf("{\"record_type\":\"signal\",\"id\":\"signal-%d\",\"trajectory_id\":\"trajectory-test\",\"name\":\"metric.%d\",\"value\":%d}\n", index, index, index))
	}
	_, _ = source.WriteString(fmt.Sprintf("{\"record_type\":\"complete\",\"records\":%d,\"warnings\":0}\n", inspectProbeRecords-1))
	path := filepath.Join(t.TempDir(), "exact-limit.ndjson")
	if err := os.WriteFile(path, []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := inspectSource(context.Background(), path, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported || result.Confidence != 1 || len(result.Warnings) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestInspectTrustedAdapterOnlyProbes(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "private.trace")
	if err := os.WriteFile(source, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	pluginDir := filepath.Join(root, "adapter")
	if err := os.Mkdir(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `api_version: rlviz.dev/v1alpha1
kind: Adapter
name: inspect-test
version: 1.0.0
command:
  - /bin/sh
  - adapter.sh
capabilities:
  - adapter.probe
  - adapter.stream
`
	if err := os.WriteFile(filepath.Join(pluginDir, plugins.ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
printf '%s\n' "$1" >> "$RLVIZ_INSPECT_TEST_LOG"
printf '%s\n' '{"supported":true,"confidence":0.87,"format":"private-v2","reason":"recognized header"}'
`
	if err := os.WriteFile(filepath.Join(pluginDir, "adapter.sh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "operations.log")
	t.Setenv("RLVIZ_INSPECT_TEST_LOG", logPath)
	plugin, err := plugins.Load(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	store := &plugins.TrustStore{Path: filepath.Join(root, "trust.json")}
	if err := store.Trust(plugin); err != nil {
		t.Fatal(err)
	}

	result, err := inspectSource(context.Background(), source, pluginDir, store)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported || result.Format != "private-v2" || result.Confidence != 0.87 || result.Adapter == nil || result.Adapter.Name != "inspect-test" {
		t.Fatalf("result = %#v", result)
	}
	operations, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(operations); got != "probe\n" {
		t.Fatalf("adapter operations = %q, want only probe", got)
	}
	if !strings.Contains(result.NextCommand, "--adapter") {
		t.Fatalf("next command = %q", result.NextCommand)
	}

	unsupportedScript := `#!/bin/sh
printf '%s\n' "$1" >> "$RLVIZ_INSPECT_TEST_LOG"
printf '%s\n' '{"supported":false,"confidence":0.12,"reason":"header mismatch"}'
`
	if err := os.WriteFile(filepath.Join(pluginDir, "adapter.sh"), []byte(unsupportedScript), 0o700); err != nil {
		t.Fatal(err)
	}
	plugin, err = plugins.Load(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Trust(plugin); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = inspectSource(context.Background(), source, pluginDir, store)
	if err != nil {
		t.Fatalf("unsupported adapter probe returned error: %v", err)
	}
	if result.Supported || result.Confidence != 0.12 || result.Reason != "header mismatch" || !strings.Contains(result.NextCommand, "plugin validate") {
		t.Fatalf("unsupported result = %#v", result)
	}
	operations, err = os.ReadFile(logPath)
	if err != nil || string(operations) != "probe\n" {
		t.Fatalf("unsupported adapter operations = %q, %v", operations, err)
	}
}
