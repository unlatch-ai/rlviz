# Supported formats

This document is the public source-of-truth for what RLViz can open. Keep it
literal; examples and planned support are not built-in support.

## Built in

### Canonical NDJSON (`rlviz.dev/v1alpha1`)

RLViz directly decodes newline-delimited canonical records matching the schemas
under `schemas/v1alpha1/`. A valid stream emits parent records before children,
orders events strictly within each trajectory, and ends with one `complete`
record.

The smallest examples are under `fixtures/canonical/`.

## Example adapter

### Simple JSONL

`examples/adapters/simple-jsonl` demonstrates a process adapter for a small
non-canonical JSONL format. It is reference code, not automatically discovered
or trusted built-in support.

### Inspect AI EvalLog JSON

`examples/adapters/inspect-ai` maps the documented JSON `EvalLog` shape,
including model, tool, score, and compaction events. It intentionally does not
claim support for the compressed `.eval` container.

### Prime Intellect Verifiers GenerateOutputs JSON

`examples/adapters/verifiers` maps the JSON-compatible `GenerateOutputs`
contract, including rollout steps, rewards, metrics, token masks, and explicit
generation truncation flags.

These are dependency-free reference adapters backed by synthetic,
contract-shaped fixtures. They are not automatically discovered or trusted
built-in support.

## Project-local and user adapters

Private or organization-specific formats are supported through trusted process
adapters. Today an adapter is selected explicitly:

```bash
rlviz open SOURCE --adapter .rlviz/plugins/my-format
```

`rlviz formats` discovers bounded manifest metadata from project and user plugin
directories. This inventory does not execute probes, select an adapter, or grant
trust. Bounded automatic probing remains future work. Documentation and CLI
output must not imply that RLViz natively recognizes arbitrary JSON, databases,
directories, or vendor formats.

## Format-support policy

A format can become built in when it has:

- a stable or well-documented source contract
- redistributable synthetic fixtures
- deterministic mapping to canonical semantics with source provenance
- bounded probing and streaming behavior
- conformance, malformed-input, and large-input tests
- a maintainer path for upstream format changes

Formats with private schemas, rapid internal churn, or customer-specific fields
should remain adapters. Useful adapters may be published separately without
expanding the core binary.

`rlviz inspect [--json] [--adapter PATH] SOURCE` performs bounded, read-only
format detection without starting the daemon or browser. An explicit adapter
must already be trusted; inspection invokes its `probe` operation but never
`stream`.

`rlviz formats [--json] [--project DIR] [--plugin-root DIR]...` generates a
schema-versioned inventory from the built-in decoder, current trust store, and
bounded manifest discovery. It reports unavailable, digest-changed, untrusted,
and invalid entries without executing them. Discovery rank reflects only the
documented root and path order; it is not a compatibility score.
