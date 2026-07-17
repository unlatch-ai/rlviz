package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ScaffoldOptions struct {
	Name string
	Kind string
}

type scaffoldFile struct {
	name     string
	contents string
}

// ScaffoldPython writes a minimal, dependency-free plugin project and returns
// its deterministic file list. It refuses symlink destinations and existing
// generated files so coding agents cannot silently replace local work.
func ScaffoldPython(destination string, options ScaffoldOptions) ([]string, error) {
	if !pluginName.MatchString(options.Name) {
		return nil, errors.New("invalid plugin name")
	}
	kind := strings.ToLower(options.Kind)
	if kind == "" {
		kind = "adapter"
	}
	if kind != "adapter" && kind != "analyzer" {
		return nil, errors.New("plugin type must be adapter or analyzer")
	}
	abs, err := canonicalScaffoldDestination(destination)
	if err != nil {
		return nil, err
	}
	if err := rejectScaffoldSymlinks(abs); err != nil {
		return nil, err
	}
	files := []scaffoldFile{}
	if kind == "analyzer" {
		files = []scaffoldFile{
			{ManifestName, strings.ReplaceAll(pythonAnalyzerManifest, "{{NAME}}", options.Name)},
			{"analyzer.py", pythonAnalyzer},
			{"sample-input.json", analyzerSampleInput},
			{"README.md", strings.ReplaceAll(pythonAnalyzerReadme, "{{NAME}}", options.Name)},
		}
	} else {
		files = []scaffoldFile{
			{ManifestName, strings.ReplaceAll(pythonManifest, "{{NAME}}", options.Name)},
			{"adapter.py", pythonAdapter},
			{"test_adapter.py", pythonAdapterTest},
			{"testdata/cases.json", adapterTestCases},
			{"README.md", strings.ReplaceAll(pythonReadme, "{{NAME}}", options.Name)},
		}
	}
	for _, file := range files {
		path := filepath.Join(abs, file.name)
		if _, err := os.Lstat(path); err == nil {
			return nil, fmt.Errorf("refusing to overwrite %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	createdDestination := false
	if info, err := os.Lstat(abs); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return nil, err
		}
		createdDestination = true
	} else if err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("plugin destination %s is not a directory", abs)
	}
	if err := rejectScaffoldSymlinks(abs); err != nil {
		if createdDestination {
			_ = os.Remove(abs)
		}
		return nil, err
	}
	created := make([]string, 0, len(files))
	createdDirectories := []string{}
	rollback := func() {
		for _, path := range created {
			_ = os.Remove(path)
		}
		for index := len(createdDirectories) - 1; index >= 0; index-- {
			_ = os.Remove(createdDirectories[index])
		}
		if createdDestination {
			_ = os.Remove(abs)
		}
	}
	for _, file := range files {
		path := filepath.Join(abs, file.name)
		parent := filepath.Dir(path)
		if parent != abs {
			info, statErr := os.Lstat(parent)
			switch {
			case errors.Is(statErr, os.ErrNotExist):
				if err := os.Mkdir(parent, 0o755); err != nil {
					rollback()
					return nil, fmt.Errorf("create scaffold directory %s: %w", parent, err)
				}
				createdDirectories = append(createdDirectories, parent)
			case statErr != nil:
				rollback()
				return nil, fmt.Errorf("inspect scaffold directory %s: %w", parent, statErr)
			case info.Mode()&os.ModeSymlink != 0:
				rollback()
				return nil, fmt.Errorf("scaffold directory %s is a symbolic link", parent)
			case !info.IsDir():
				rollback()
				return nil, fmt.Errorf("scaffold path %s is not a directory", parent)
			}
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("create %s: %w", path, err)
		}
		created = append(created, path)
		if _, err := f.WriteString(file.contents); err != nil {
			f.Close()
			rollback()
			return nil, err
		}
		if err := f.Close(); err != nil {
			rollback()
			return nil, err
		}
	}
	names := make([]string, len(files))
	for index, file := range files {
		names[index] = file.name
	}
	return names, nil
}

func canonicalScaffoldDestination(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if info, err := os.Lstat(abs); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("plugin destination %s is a symbolic link", abs)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	ancestor := abs
	missing := []string{}
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("no existing ancestor for plugin destination %s", abs)
		}
		missing = append([]string{filepath.Base(ancestor)}, missing...)
		ancestor = parent
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", fmt.Errorf("resolve plugin destination ancestor: %w", err)
	}
	return filepath.Join(append([]string{resolved}, missing...)...), nil
}

func rejectScaffoldSymlinks(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(abs)
	remainder := strings.TrimPrefix(abs, volume)
	remainder = strings.TrimPrefix(remainder, string(filepath.Separator))
	current := volume + string(filepath.Separator)
	for _, part := range strings.Split(remainder, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect plugin destination %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("plugin destination component %s is a symbolic link", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("plugin destination component %s is not a directory", current)
		}
	}
	return nil
}

const pythonManifest = `api_version: rlviz.dev/v1alpha1
kind: Adapter
name: {{NAME}}
version: 0.1.0
command:
  - python3
  - adapter.py
capabilities:
  - adapter.probe
  - adapter.stream
`

const pythonAdapter = `#!/usr/bin/env python3
"""Dependency-free RLViz adapter scaffold."""
import argparse
import hashlib
import json
import sys

MAX_PROBE_BYTES = 1_048_576

def load_request(path):
    with open(path, "r", encoding="utf-8") as handle:
        request = json.load(handle)
    if request.get("api_version") != "rlviz.dev/v1alpha1":
        raise ValueError("unsupported api_version")
    return request

def emit(record):
    # stdout is protocol-only. Send diagnostics to stderr.
    print(json.dumps(record, separators=(",", ":"), ensure_ascii=False))

def read_file_prefix(request):
    """Read at most the bounded probe allowance for file-shaped formats."""
    source = request.get("source", {})
    if source.get("kind") != "file":
        raise ValueError("read_file_prefix requires a file source")
    requested = request.get("limits", {}).get("probe_bytes", MAX_PROBE_BYTES)
    if not isinstance(requested, int) or requested < 1:
        raise ValueError("invalid probe byte limit")
    with open(source["path"], "rb") as handle:
        return handle.read(min(requested, MAX_PROBE_BYTES))

def stable_id(prefix, *parts):
    """Build a repeatable ID from source-native identity, never wall time."""
    payload = "\x1f".join(str(part) for part in parts).encode("utf-8")
    return f"{prefix}-{hashlib.sha256(payload).hexdigest()[:16]}"

def probe(request):
    # TODO: inspect request["source"] or read_file_prefix(request), then match
    # evidence specific to the format. Do not fully load a large source.
    print(json.dumps({"supported": False, "confidence": 0, "reason": "implement format detection"}, separators=(",", ":")))

def stream(request):
    # TODO: emit parents before children and use stable_id for derived IDs.
    # Preserve useful source records in raw and attach source locations.
    # The final count excludes the complete record.
    emit({"record_type": "complete", "records": 0, "warnings": 0})

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("operation", choices=("probe", "stream"))
    parser.add_argument("--request", required=True)
    args = parser.parse_args()
    request = load_request(args.request)
    if request.get("operation") != args.operation:
        raise ValueError("request operation does not match command")
    (probe if args.operation == "probe" else stream)(request)

if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
`

const pythonAdapterTest = `#!/usr/bin/env python3
"""Run reviewed synthetic fixtures through RLViz's trusted validator."""
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parent
TESTDATA = (ROOT / "testdata").resolve()
CASES_PATH = TESTDATA / "cases.json"

def fail(message):
    raise ValueError(message)

def fixture_path(value, index):
    if not isinstance(value, str) or not value:
        fail(f"case {index}: source must be a non-empty relative path")
    relative = Path(value)
    if relative.is_absolute() or ".." in relative.parts:
        fail(f"case {index}: source must stay inside testdata")
    try:
        resolved = (TESTDATA / relative).resolve(strict=True)
    except OSError as error:
        fail(f"case {index}: cannot resolve fixture {value!r}: {error}")
    try:
        common = os.path.commonpath((str(TESTDATA), str(resolved)))
    except ValueError:
        common = ""
    if common != str(TESTDATA) or not resolved.is_file():
        fail(f"case {index}: fixture must be a file inside testdata")
    return resolved

def load_cases():
    with CASES_PATH.open("r", encoding="utf-8") as handle:
        document = json.load(handle)
    if not isinstance(document, dict) or set(document) != {"schema_version", "cases"}:
        fail("testdata/cases.json must contain only schema_version and cases")
    version = document["schema_version"]
    if isinstance(version, bool) or version != 1 or not isinstance(document["cases"], list):
        fail("testdata/cases.json must use schema_version 1 with a cases array")
    if not document["cases"]:
        fail("no adapter cases; add synthetic fixtures to testdata/cases.json")
    cases = []
    for index, case in enumerate(document["cases"], start=1):
        if not isinstance(case, dict) or set(case) != {"source", "expected_format", "min_records"}:
            fail(f"case {index}: expected source, expected_format, and min_records")
        expected_format = case["expected_format"]
        minimum = case["min_records"]
        if not isinstance(expected_format, str) or not expected_format:
            fail(f"case {index}: expected_format must be a non-empty string")
        if isinstance(minimum, bool) or not isinstance(minimum, int) or minimum < 1:
            fail(f"case {index}: min_records must be a positive integer")
        cases.append((case["source"], fixture_path(case["source"], index), expected_format, minimum))
    return cases

def validate_case(binary, label, source, expected_format, minimum):
    command = [binary, "plugin", "validate", "--json", str(ROOT), str(source)]
    completed = subprocess.run(
        command,
        cwd=ROOT,
        stdin=subprocess.DEVNULL,
        capture_output=True,
        text=True,
        timeout=45,
        check=False,
    )
    if completed.returncode != 0:
        detail = completed.stderr.strip() or completed.stdout.strip() or f"exit {completed.returncode}"
        fail(f"{label}: validation failed: {detail}")
    try:
        report = json.loads(completed.stdout)
    except json.JSONDecodeError as error:
        fail(f"{label}: validator returned invalid JSON: {error}")
    if not isinstance(report, dict):
        fail(f"{label}: validator result must be an object")
    if report.get("deterministic") is not True:
        fail(f"{label}: validator did not report deterministic output")
    if report.get("format") != expected_format:
        fail(f"{label}: format {report.get('format')!r}, expected {expected_format!r}")
    records = report.get("records")
    if isinstance(records, bool) or not isinstance(records, int) or records < minimum:
        fail(f"{label}: records {records!r}, expected at least {minimum}")
    print(f"ok {label}: {records} records, {expected_format}")

def main():
    binary = os.environ.get("RLVIZ_BIN", "rlviz")
    if not binary:
        fail("RLVIZ_BIN must not be empty")
    for label, source, expected_format, minimum in load_cases():
        validate_case(binary, label, source, expected_format, minimum)

if __name__ == "__main__":
    try:
        main()
    except (OSError, subprocess.SubprocessError, ValueError) as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
`

const adapterTestCases = `{"schema_version":1,"cases":[]}
`

const pythonReadme = `# {{NAME}}

This is executable local adapter code. RLViz did not copy the source trace or
trust this directory automatically. Review every generated and edited file
before trusting its digest.

Implementation checklist:

1. Inspect only a small representative source sample.
2. Implement format-specific detection in ` + "`probe`" + `. Use ` + "`read_file_prefix`" + `
   for file formats so probing stays bounded.
3. Implement canonical NDJSON emission in ` + "`stream`" + `. Emit parents before
   children, preserve source order, and end with one ` + "`complete`" + ` record.
4. Use source-native IDs or ` + "`stable_id`" + `; never derive IDs from wall time.
5. Put rewards, grader results, latency, tokens, and pass/fail in signals.
6. Create only small synthetic fixtures under ` + "`testdata/`" + ` and list them in
   ` + "`testdata/cases.json`" + `. Do not copy a customer trace into this plugin or
   commit proprietary model output.

After reviewing the manifest, executable files, case manifest, and fixtures,
trust the complete digest and run the synthetic cases before the private source:

    rlviz plugin trust --json .
    python3 test_adapter.py
    rlviz plugin validate --json . /path/to/sample

Any code change produces a new digest and intentionally requires review and
trust again. See docs/adapter-authoring.md in the RLViz repository for the full
mapping and security contract.
`

const pythonAnalyzerManifest = `api_version: rlviz.dev/v1alpha1
kind: Analyzer
name: {{NAME}}
version: 0.1.0
command:
  - python3
  - analyzer.py
capabilities:
  - analyzer.analyze
`

const pythonAnalyzer = `#!/usr/bin/env python3
"""Dependency-free RLViz analyzer scaffold."""
import argparse
import json
import os
import sys

API_VERSION = "rlviz.dev/analyzer/v1alpha1"

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("operation", choices=("analyze",))
    parser.add_argument("--request", required=True)
    args = parser.parse_args()
    with open(args.request, "r", encoding="utf-8") as handle:
        request = json.load(handle)
    if request.get("api_version") != API_VERSION or request.get("operation") != args.operation:
        raise ValueError("unsupported analyzer request")

    # TODO: inspect request["events"] and request.get("signals", []). Findings
    # must use stable IDs and may only reference events in this request.
    output = {
        "api_version": API_VERSION,
        "provenance": {
            "name": os.environ["RLVIZ_ANALYZER_NAME"],
            "version": os.environ["RLVIZ_ANALYZER_VERSION"],
            "digest": os.environ["RLVIZ_ANALYZER_DIGEST"],
            "input_digest": os.environ["RLVIZ_ANALYZER_INPUT_DIGEST"],
        },
        "findings": [],
        "signals": [],
    }
    print(json.dumps(output, separators=(",", ":"), ensure_ascii=False))

if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
`

const analyzerSampleInput = `{"api_version":"rlviz.dev/analyzer/v1alpha1","operation":"analyze","trajectory_id":"trajectory-1","events":[{"record_type":"event","id":"event-1","trajectory_id":"trajectory-1","sequence":0,"kind":"tool","input":{"name":"example"}}],"signals":[]}
`

const pythonAnalyzerReadme = `# {{NAME}}

This is a local RLViz analyzer. It receives one normalized trajectory and
returns supplemental findings and signals without changing source data.

Validate it with:

    rlviz plugin trust .
    rlviz plugin validate . sample-input.json
`
