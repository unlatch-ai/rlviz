# RLViz v1alpha1 contracts

Adapters emit UTF-8 NDJSON: one object matching
`canonical-record.schema.json` per line. The schema validates individual records;
the following stream constraints are also normative:

- IDs are non-empty and unique across the stream.
- A referenced run, case, group, trajectory, event, or parent must have appeared
  earlier in the stream.
- Trajectory parents belong to the same group. Event parents and signal/artifact
  event targets belong to the same trajectory.
- Event `sequence` values are non-negative and strictly increasing within each
  trajectory. They need not be contiguous, so adapters can preserve source order
  without renumbering on append.
- Exactly one `complete` record terminates the stream. Its `records` count is the
  number of preceding records and excludes the completion record itself.
- Artifact paths are untrusted source-relative references. Schema validity never
  grants permission to read a path; the host must enforce its registered-root
  policy.

All contracts are unstable until a non-alpha version is declared. A manifest
selects this version with `api_version: rlviz.dev/v1alpha1`.

Plugin manifests may use `kind: Adapter` with `adapter.probe` and
`adapter.stream`, or `kind: Analyzer` with exactly `analyzer.analyze`. The
strict external-analyzer request and output contracts are
`analyzer-input.schema.json` and `analyzer-output.schema.json`. Valid examples
live at `fixtures/protocol/analyzer-input.json` and
`fixtures/protocol/analyzer-output.json`.

External analyzer CLI execution is available. Use `rlviz plugin init --type
analyzer`, explicitly trust the plugin, then run `rlviz plugin validate` with an
analyzer input JSON file. Validation executes the trusted snapshot twice and
requires byte-identical, schema- and runtime-valid output. See
`docs/analyzer-protocol.md` for the execution and semantic constraints that
JSON Schema cannot express across records or encoded byte lengths.

`presentation-config.schema.json` defines the separate, non-executable viewer
customization contract. Presentation files are strict, bounded JSON and never
grant plugin trust or permission to inject HTML, CSS, JavaScript, selectors,
URLs, or arbitrary inspector templates. Runtime validation additionally checks
semantic color contrast, which JSON Schema alone cannot express.
