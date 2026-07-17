# Implementation plan

The implementation is organized as vertical milestones. Each milestone should leave a usable, testable product rather than a collection of disconnected subsystems.

## Current delivery

Milestones 0–7 are implemented in the repository: versioned contracts and fixtures, the single-trajectory viewer, authenticated daemon lifecycle, progressive SQLite indexing, paginated and virtualized reads, growing-file updates, trusted external adapters and analyzers, rollout-group summaries, compact behavioral paths, deterministic long-trace comparison, cached loop/retry findings, release automation, and verified installers. `v0.1.0` is published with native macOS/Linux archives, checksums, attestations, a curl installer, the `unlatch-ai/tap/rlviz` Homebrew formula, and the `rlviz` npm package. npm trusted publishing is configured for future tags.

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
- A trusted Python adapter opens a source without rebuilding RLViz.
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

## Milestone 8: product and design foundations

Status: implemented for the trajectory workspace; screenshot automation and
additional representative real-format fixtures remain ongoing quality work.

### Deliverables

- Current system architecture and change map
- Researcher-centered UI information architecture
- Design tokens, typography, density, and core component primitives
- Central command registry and rebindable local keymap
- Representative rich, context-compaction, verifier, group, and long-run fixtures
- Initial real-browser, screenshot, and accessibility quality gates

### Exit criteria

- The UI has one dominant reading surface and consistent supporting panels.
- Shortcut help, command palette, and displayed key hints come from one registry.
- Essential text remains readable in comfortable and compact density.
- Visible workflows have deterministic browser coverage and visual evidence.

## Milestone 9: expert onboarding

Status: rich demo, format inventory, bounded inspection, and version-matched
agent setup are implemented for Codex, Claude Code, and Cursor. Setup writes
are explicit and create-only. Source-aware adapter scaffolding now returns a
versioned review plan without reading or copying trace contents.

### Deliverables

- Rich bundled `rlviz demo` (implemented)
- `rlviz formats [--json]` (implemented)
- Read-only `rlviz inspect [--json] [--adapter PATH] SOURCE` (implemented)
- Safe agent-integration print command (implemented)
- Explicitly reviewed agent-integration write/setup workflow
- Explicit supported-format documentation
- Improved unsupported-format and adapter-scaffold guidance

### Exit criteria

- A clean install reaches a representative demo in under one minute.
- Users can distinguish built-in, example, discovered, trusted, and unsupported formats.
- A coding agent can move from probe to reviewed adapter without matching human error text.
- Agent setup never overwrites existing project instructions.

## Milestone 10: research-grade trajectory reader

Status: transcript, event timeline, outcome/evidence, selected-event-first
details, semantic landmarks, context-change jump, and long-run virtualization
are implemented. A full context-usage track and minimap require the next
source-backed data-model work.

### Deliverables

- Transcript and event-timeline modes
- Turn and tool-span grouping with raw-event fallback
- First-class outcome, verifier, reward, and final-output surface
- Context-usage track and compaction/truncation landmarks
- Selected-event-first details and evidence panel
- Semantic landmark rail and whole-trajectory minimap

### Exit criteria

- A researcher can explain the outcome without hunting through raw events.
- Context gained, lost, compacted, or restored is visible when the source provides it.
- Every grouped or derived surface links to canonical and raw source records.
- A 10,000-event trajectory remains keyboard-navigable and responsive.

## Milestone 11: group, divergence, and safe customization

Status: deterministic behavioral alignment, divergence navigation, and pair
summary deltas for outcome, tokens, explicit context events, compactions, and
source-shaped verifier results are implemented. Reproducible cohort filters
cover outcome fields, core metrics, and scalar signals. Selected tool arguments
and results have a bounded field-level diff. Reward-rank, outlier, failure, and
infrastructure shortcuts are command-registry backed. Optional metric and scalar
signal column layouts are bounded, locally persisted, and fail safe. The first
declarative presentation contract is wired end-to-end through explicit CLI
flags, independent daemon validation, persistent normalized storage, and the
trajectory API. Broader inspector and keymap customization remains.

### Deliverables

- Cohort distributions, multi-signal filters, outlier shortcuts, and saved columns
- Outcome, verifier, context, and compaction deltas in pair comparison
- Synchronized detail and structured tool argument/result differences
- Declarative field, signal, inspector, theme-token, and keymap customization
- Bounded adapter discovery and probe ranking without implicit executable trust

### Exit criteria

- Researchers can choose representative cohorts before opening individual runs.
- Comparison explains meaningful behavioral and context divergence, not only raw JSON difference.
- Customization uses validated core primitives and cannot inject arbitrary viewer JavaScript or CSS.

## Near-term issue sequence

1. Gather representative adapter-validation failures and extend structured
   fields only where the validation origin can identify them without parsing
   error prose.
2. Validate the Inspect AI and Verifiers mappings against additional upstream
   samples as their public contracts evolve.
3. Design the smallest canonical context contract and migration from the now
   documented two-format evidence, then build the context-usage track and
   minimap only from structured observations.
4. Apply the trajectory design system to richer cohort distributions,
   structured pair diffs, and safe declarative customization.
5. Validate the clean-machine install-to-open path on Linux; macOS curl,
   Homebrew, and npm paths are verified.

## Quality gates

Every change should keep these commands green:

```bash
make format
make test
make check
make build
```

Protocol changes require updated schemas, fixtures, and conformance tests. UI behavior changes require a browser-level test when the behavior is externally visible.
