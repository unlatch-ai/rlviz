# Implementation plan

The implementation is organized as vertical milestones. Each milestone should leave a usable, testable product rather than a collection of disconnected subsystems.

## Current delivery

The repository now contains the single-trajectory vertical slice, authenticated background daemon lifecycle, and the explicit external-adapter workflow. The daemon currently uses a bounded in-memory registry; automatic plugin discovery, SQLite streaming/indexing, large-file pagination, and file watching remain Milestone 2 work. Group schemas and fixtures exist, while the group and divergence interfaces remain Milestones 4 and 5.

## Milestone 0: contracts and fixtures

### Deliverables

- Canonical entity and event schemas
- Plugin manifest schema
- Adapter request and NDJSON response schemas
- Small linear trajectory fixture
- Rollout-group fixture
- Branched trajectory fixture
- Malformed and adversarial fixtures
- Architecture decision records for storage, plugins, and daemon lifecycle

### Exit criteria

- Schemas validate every fixture.
- Stable IDs and relationship constraints are specified.
- A contributor can understand the format without reading Go code.

## Milestone 1: single-trajectory vertical slice

### Deliverables

- `rlviz open`
- Foreground local server
- Built-in canonical JSONL adapter
- Embedded React UI
- Three-pane trajectory layout
- Event selection and raw payload inspection
- Basic keyboard navigation
- Text, JSON, image, and log artifact renderers

### Exit criteria

- One release binary opens the canonical fixture.
- Every rendered event links to its raw source record.
- The source remains unchanged.
- Browser tests cover the primary navigation flow.

## Milestone 2: daemon, streaming, and large files

### Deliverables

- Background daemon lifecycle
- Loopback authentication token
- SQLite metadata and search index
- Incremental JSONL parsing
- Source byte offsets
- Paginated HTTP API
- Virtualized event list
- File watching and append-only updates
- Cache status and cleanup commands

### Exit criteria

- `rlviz open` returns without holding the invoking shell.
- A 10,000-event trajectory scrolls smoothly.
- Large files reach first render before full indexing completes.
- Appended records appear without reopening the viewer.

## Milestone 3: external adapters and agent workflow

### Deliverables

- Plugin discovery and manifest parsing
- Adapter `probe` and `stream` process host
- Trust-by-path-and-digest flow
- `plugin init`, `list`, `validate`, and `doctor`
- Python adapter template
- Golden fixture test harness
- Structured CLI diagnostics
- Claude Code, Codex, and Cursor integration instructions

### Exit criteria

- A coding agent can scaffold an adapter from an unsupported sample.
- The validator identifies the exact invalid record and field.
- A trusted Python adapter opens a source without rebuilding RolloutViz.
- A changed adapter is not executed until trusted again.

## Milestone 4: rollout groups

### Deliverables

- Run, case, and group navigation
- Trajectory summary table
- Reward, pass, length, token, latency, error, and termination summaries
- Sorting and filtering
- Best/worst shortcuts
- Behavioral path fingerprints

### Exit criteria

- A researcher can identify representative success and failure trajectories without opening each one.
- Aggregates are computed incrementally from indexed fields.
- Group membership remains source-native and distinguishable from user selections.

## Milestone 5: pair comparison and divergence

### Deliverables

- Comparison-set model
- Deterministic event fingerprints
- Sequence alignment engine
- Aligned two-lane viewer
- Common-prefix compression
- First meaningful divergence marker
- State, reward, artifact, and termination differences
- Adapter-provided alignment-key support

### Exit criteria

- Equivalent actions align despite irrelevant text differences.
- Insertions, deletions, retries, and later realignment remain understandable.
- Alignment output has deterministic golden tests.
- Users can deep-link to a divergence.

## Milestone 6: compact paths and analyzers

### Deliverables

- Aggregated behavioral-prefix tree
- Explicit distinction between derived paths and source-native branches
- Analyzer plugin protocol
- Loop and retry analyzer
- Domain-specific signal output
- Cached analysis results with provenance

### Exit criteria

- A rollout group can be summarized without a spaghetti graph.
- Analyzer results identify plugin name, version, and input digest.
- Removing analyzer output never changes source data.

## Milestone 7: open-source release quality

### Deliverables

- macOS arm64/x64 and Linux arm64/x64 releases
- Checksums and signed artifacts
- Homebrew tap
- Reproducible release workflow
- Security policy and threat-model review
- Performance benchmarks
- Adapter-authoring tutorial
- Public example dataset

### Exit criteria

- A clean machine can install and open the example in under one minute.
- Release artifacts require no language runtime.
- Normal viewing makes no outbound requests.
- Documentation covers unsupported formats and safe plugin review.

## Near-term issue sequence

1. Define `v1alpha1` canonical schemas.
2. Add representative canonical fixtures.
3. Implement schema validation tests.
4. Implement canonical JSONL streaming reader.
5. Add foreground HTTP server and embedded placeholder UI.
6. Implement the trajectory timeline and inspector.
7. Add keyboard navigation and artifact rendering.
8. Add Playwright end-to-end coverage.
9. Implement the SQLite streaming index and paginated API.
10. Build the rollout-group summary and selection interface.
11. Add pair alignment and first-divergence navigation.

## Quality gates

Every change should keep these commands green:

```bash
make format
make test
make check
make build
```

Protocol changes require updated schemas, fixtures, and conformance tests. UI behavior changes require a browser-level test when the behavior is externally visible.
