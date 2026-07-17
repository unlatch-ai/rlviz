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

## Project-local and user adapters

Private or organization-specific formats are supported through trusted process
adapters. Today an adapter is selected explicitly:

```bash
rlviz open SOURCE --adapter .rlviz/plugins/my-format
```

Adapter discovery and bounded automatic probing are future work. Until they are
implemented, documentation and CLI output must not imply that RLViz natively
recognizes arbitrary JSON, databases, directories, or vendor formats.

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

`rlviz formats [--json]` generates its output from the built-in decoder and
current trust store. It reports unavailable and digest-changed plugin entries
without executing them. Future adapter discovery will extend the same result
instead of creating a separate format registry.
