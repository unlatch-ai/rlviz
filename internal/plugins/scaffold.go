package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ScaffoldOptions struct{ Name string }

// ScaffoldPython writes a minimal, dependency-free adapter project. It refuses
// to overwrite existing files so it is safe for coding agents to invoke.
func ScaffoldPython(destination string, options ScaffoldOptions) error {
	if !pluginName.MatchString(options.Name) {
		return errors.New("invalid adapter name")
	}
	abs, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		ManifestName: strings.ReplaceAll(pythonManifest, "{{NAME}}", options.Name),
		"adapter.py": pythonAdapter,
		"README.md":  strings.ReplaceAll(pythonReadme, "{{NAME}}", options.Name),
	}
	for name := range files {
		path := filepath.Join(abs, name)
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing to overwrite %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for name, contents := range files {
		path := filepath.Join(abs, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		if _, err := f.WriteString(contents); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

const pythonManifest = `api_version: rolloutviz.dev/v1alpha1
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
"""Dependency-free RolloutViz adapter scaffold."""
import argparse
import json
import sys

def load_request(path):
    with open(path, "r", encoding="utf-8") as handle:
        request = json.load(handle)
    if request.get("api_version") != "rolloutviz.dev/v1alpha1":
        raise ValueError("unsupported api_version")
    return request

def emit(record):
    # stdout is protocol-only. Send diagnostics to stderr.
    print(json.dumps(record, separators=(",", ":"), ensure_ascii=False))

def probe(request):
    # TODO: inspect a bounded prefix and recognize the source format.
    print(json.dumps({"supported": False, "confidence": 0, "reason": "implement format detection"}, separators=(",", ":")))

def stream(request):
    # TODO: emit run/case/group/trajectory/event records with stable IDs.
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

const pythonReadme = `# {{NAME}}

This is a local RolloutViz adapter. Implement bounded format detection in
probe and canonical NDJSON emission in stream.

Validate it with:

    rlviz plugin trust .
    rlviz plugin validate . /path/to/sample
`
