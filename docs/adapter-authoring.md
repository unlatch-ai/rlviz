# Adapter authoring

Adapters let RLViz open a trace format without changing RLViz or the
source data. They are small local programs that translate source records into a
canonical NDJSON stream.

## Agent-first workflow

Start with the original source. Do not make a converted copy just to use the
viewer.

```bash
rlviz open --json ./artifacts/task-184.trace
```

An unsupported source returns a structured diagnostic similar to:

```json
{
  "code": "unsupported_format",
  "path": "/workspace/artifacts/task-184.trace",
  "suggested_command": "rlviz plugin init --type adapter --lang python --from /workspace/artifacts/task-184.trace .rlviz/plugins/task-184"
}
```

An agent should parse the diagnostic rather than match human error text. Run the
suggested command when present, then inspect a small representative sample of the
source and implement the generated mapping.

```bash
rlviz plugin init --type adapter --lang python --from ./artifacts/task-184.trace .rlviz/plugins/customer-x
```

Review the manifest and every executable file in the generated directory. The
adapter is local code with the user's permissions. Summarize what it reads and
emits, then get the user's explicit approval before trusting the reviewed digest:

```bash
rlviz plugin trust --json .rlviz/plugins/customer-x
rlviz plugin validate --json .rlviz/plugins/customer-x ./artifacts/task-184.trace
rlviz open --json ./artifacts/task-184.trace --adapter .rlviz/plugins/customer-x
```

Validation executes adapter code, so it requires trust. Fix adapter code using
the location and field in each structured finding, then review the change and
get approval to trust its new digest before validating again. Do not edit the
trace to make a finding disappear.

Adapter mapping failures from `plugin validate --json` keep the stable top-level
`code: plugin_validate_failed` and add an adapter diagnostic taxonomy:

```json
{
  "code": "plugin_validate_failed",
  "phase": "stream",
  "kind": "protocol",
  "pass": 1,
  "line": 5,
  "record_type": "event",
  "record_id": "evt-7",
  "field": "sequence",
  "error": "line 5: event sequence must be non-negative"
}
```

`phase` is `source`, `probe`, or `stream`. `kind` is `execution`,
`protocol`, `unsupported`, `nondeterministic`, or `provenance`. `pass`, `line`,
`record_type`, `record_id`, and `field` are included only when that context is
known without parsing error prose. RLViz validates both stream passes before
comparing bytes, so a malformed second pass is reported as pass 2 protocol
failure rather than being mislabeled as nondeterminism.

Trust is bound to an absolute path and content digest. Committing an adapter does
not make it trusted on another machine.

`plugin init --json` returns a versioned plan with the resolved source shape,
deterministic generated-file list, `review_required: true`, and exact trust,
validate, and open commands. For regular files, `source.profile` describes at
most 256 KiB using only container kind, bounds, truncation state, and observed
field paths with JSON types. It never includes scalar values or copies a sample
into the plugin. The profile is structural evidence, not a complete schema. A
missing source, unreadable profile, or analyzer use of `--from` fails before any
scaffold files are created.

## Project layout

Keep source-specific adapters next to the code that produces the source format:

```text
.rlviz/
  plugins/
    customer-x/
      rlviz-plugin.yaml
      adapter.py
      test_adapter.py
      testdata/
        cases.json
        sample.trace
```

Use small, synthetic, non-sensitive fixtures in `testdata`. Do not commit
customer traces, credentials, model outputs, or proprietary artifacts merely to
test an adapter.

`cases.json` is a strict, ordered fixture manifest:

```json
{
  "schema_version": 1,
  "cases": [
    {
      "source": "sample.trace",
      "expected_format": "customer-x-rollout-v2",
      "min_records": 4
    }
  ]
}
```

The generated `test_adapter.py` rejects absolute paths, traversal, and symlink
escapes, then delegates each case to `rlviz plugin validate --json`. It does not
reimplement canonical validation. Because the runner executes the adapter, first
review the runner, adapter, manifest, case manifest, and fixtures; then trust the
complete digest. Run `python3 test_adapter.py` before validating the private
source. Changing code or fixtures invalidates trust as intended.

A manifest declares the versioned protocol and executable entrypoint:

```yaml
api_version: rlviz.dev/v1alpha1
kind: Adapter
name: customer-x
version: 0.1.0
command:
  - python3
  - adapter.py
capabilities:
  - adapter.probe
  - adapter.stream
```

Prefer the generated Python scaffold over writing the process protocol from
scratch. It owns request parsing, stdout discipline, bounded file-prefix reads,
and stable derived-ID helpers so adapter code can focus on source detection and
record mapping. Generated file order is stable, existing generated files are
never replaced, and a symbolic-link destination is refused. Existing parent
aliases are resolved to their canonical location before files are created.

Keep plugin-local command paths such as `adapter.py` relative to the manifest.
External interpreters such as `python3` or `/usr/bin/python3` may be bare or
absolute. Relative plugin paths let RLViz execute the approved code from a
private verified snapshot.

## Implement `probe`

`probe` answers whether the adapter understands a source. It should inspect a
bounded prefix, recognizable metadata, or a small directory listing. It must not
fully load a large trace.

Return a high confidence only for evidence specific to the format:

```json
{
  "supported": true,
  "confidence": 0.95,
  "format": "customer-x-rollout-v2",
  "reason": "recognized schema_version=2 and ordered steps"
}
```

Use `supported: false` for ordinary format mismatch. Reserve a nonzero exit for
malformed requests or failures that prevent a reliable probe.

## Implement `stream`

`stream` emits one canonical JSON object per stdout line. Emit parents before
children: run, case, group, trajectory, then events, signals, and artifacts. End
with exactly one `complete` record.

```jsonl
{"record_type":"run","id":"run-42"}
{"record_type":"case","id":"case-184","run_id":"run-42"}
{"record_type":"group","id":"group-184","case_id":"case-184"}
{"record_type":"trajectory","id":"traj-3","group_id":"group-184"}
{"record_type":"event","id":"evt-1","trajectory_id":"traj-3","sequence":1,"kind":"tool","data":{"name":"search"},"source":{"path":"task-184.trace","line":7}}
{"record_type":"complete","records":5,"warnings":0}
```

Mapping rules:

- Derive stable IDs from source-native IDs when possible. Otherwise hash stable
  source identity, not timestamps or iteration counters that can change.
- Preserve the source event order. `sequence` must strictly increase within a
  trajectory.
- Keep the unmodified source record in `raw` when it helps debugging.
- Add source line, byte offset, and byte length when available.
- Put rewards, grader results, latency, tokens, and pass/fail values in `signal`
  records rather than hiding them in display text.
- Use `alignment_key`, `state_hash`, `parent_id`, and `branch_id` only when the
  source provides a defensible meaning. Do not infer literal branching from a
  set of independently sampled trajectories.
- Write protocol records only to stdout. Send human diagnostics to stderr.

The complete schemas live under `schemas/v1alpha1/`. The canonical fixtures
under `fixtures/canonical/` are the smallest working examples.

## Security contract

An adapter executes with the user's local permissions, so keep its behavior
narrow and reviewable:

- Open sources read-only and never rename, delete, truncate, normalize, or
  annotate them in place.
- Make no network requests. Do not upload traces, fetch schemas, or emit
  telemetry during probing, validation, or viewing.
- Resolve only files inside the source root supplied by RLViz. Reject path
  traversal and unexpected symlink escapes.
- Treat trace strings as data. Never pass recorded commands, tool calls, HTML,
  or paths to a shell.
- Bound prefix reads, directory walks, memory use, and subprocess time.
- Do not place secrets or raw customer data in diagnostics, fixtures, or logs.

Run validation after every behavior change. A changed digest intentionally
invalidates trust, even when the manifest version is unchanged.
