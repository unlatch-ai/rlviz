# Adapter protocol

## Purpose

Adapters translate source-specific trace formats into RolloutViz's canonical model. They are deterministic local programs and do not require an LLM at runtime.

Coding agents can generate adapters by inspecting representative source records, implementing this protocol, and running the conformance validator.

## Planned discovery

RolloutViz discovers adapters in this order:

1. Explicit `--adapter`
2. Project-local `.rolloutviz/plugins`
3. User plugins under the platform configuration directory
4. `rolloutviz-adapter-*` executables on `PATH`
5. Built-in adapters

The current slice requires explicit `--adapter`. Automatic discovery will use this order. External plugins include a manifest:

```yaml
api_version: rolloutviz.dev/v1alpha1
kind: Adapter
name: customer-banking-env
version: 0.1.0
command:
  - python3
  - adapter.py
capabilities:
  - adapter.probe
  - adapter.stream
```

## Operations

### `probe`

Determine whether an adapter understands a source without fully parsing it.

```bash
adapter probe --request request.json
```

Response:

```json
{
  "supported": true,
  "confidence": 0.95,
  "format": "customer-banking-rollout-v2",
  "reason": "recognized header and event fields"
}
```

Probe must be fast, bounded, and free of source mutations.

### `stream`

Emit canonical records as NDJSON:

```bash
adapter stream --request request.json
```

Example output:

```json
{"record_type":"run","id":"run-42","metadata":{"checkpoint":"1200"}}
{"record_type":"case","id":"task-184","run_id":"run-42"}
{"record_type":"group","id":"group-7","case_id":"task-184"}
{"record_type":"trajectory","id":"traj-3","group_id":"group-7"}
{"record_type":"event","id":"evt-1","trajectory_id":"traj-3","sequence":1,"kind":"message","input":{"role":"user","content":"..."}}
```

The final record reports completion:

```json
{"record_type":"complete","records":5,"warnings":0}
```

## Required adapter behavior

- Emit stable IDs for unchanged source data.
- Preserve source ordering.
- Include raw source locations where possible.
- Emit warnings for recoverable omissions.
- Exit nonzero for malformed or unsupported required data.
- Write protocol records only to stdout.
- Write human diagnostics only to stderr.
- Never mutate the source.
- Never perform network requests unless the user explicitly configures that adapter to do so.

## Optional alignment hints

Adapters may provide:

- `alignment_key`: domain-normalized identity for an action or observation
- `state_hash`: stable equivalence hash for environment state
- `parent_id`: source-native event or branch parent
- `branch_id`: source-native branch identifier

These fields enrich comparison but are not required for single-trajectory viewing.

## Validation

```bash
rlviz plugin trust ./plugins/customer-x
rlviz plugin validate ./plugins/customer-x ./fixtures/sample.jsonl
```

Validation checks:

- manifest schema
- executable availability
- probe determinism
- canonical record schemas
- stable IDs across repeated runs
- ordering and relationship integrity
- source-location validity
- clean termination
- bounded sample execution

The validator emits both concise human output and `--json` diagnostics suitable for coding agents.

## Trust

Adapters are executable code. RolloutViz records trust by absolute plugin path and content digest. A modified plugin must be trusted again before execution.

Each execution uses a private snapshot whose digest must still match the approved code. Python bytecode and imported helpers are part of that digest. Adapter stdout is currently capped at 32 MiB and stderr at 1 MiB.

Project repositories may commit adapter code and manifests, but opening the repository does not automatically trust them.
