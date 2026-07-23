# System architecture

This document is the current structural map of RLViz. It describes what exists
in the repository today, the boundaries that must remain stable, and the main
request paths. Product intent lives in `product-spec.md`; target UI behavior
lives in `ui-information-architecture.md`.

## Product shape

RLViz is one native Go binary with an embedded React application. It reads
existing rollout artifacts, normalizes them into a versioned canonical model,
indexes them locally, and serves a private browser UI from loopback.

```text
coding agent or human
        |
        v
rlviz CLI --------------------------------------------------+
  open / trajectories / workspace / setup / plugin          |
        |                                                    |
        v                                                    |
background daemon on 127.0.0.1                              |
  authenticated source registry                              |
  canonical decoder or trusted process adapter               |
  progressive SQLite index + source provenance               |
  deterministic analyzers + comparison engine                |
  embedded HTTP API and React assets                          |
        |                                                    |
        v                                                    |
browser viewer                                                |
  collection / rollout / detail / guide / settings modules   |
  event details / artifacts / analyzer findings              |
                                                             |
source files <---------- always read-only -------------------+
```

The binary contains no model and does not execute recorded tools. Coding agents
use structured CLI queries and named-workspace commands, and can author
project-local adapters; the browser remains the only trajectory renderer.

## Repository map

| Path | Responsibility |
| --- | --- |
| `cmd/rlviz` | CLI parsing, human and JSON output, daemon and plugin commands |
| `internal/model` | Canonical v1alpha1 records, decoding, validation, schema conformance |
| `internal/app` | Source loading, progressive indexing, refresh orchestration |
| `internal/daemon` | Detached process lifecycle, metadata, authentication token, private runtime paths |
| `internal/index` | SQLite schema, bounded writes, queries, metrics, analyzer cache |
| `internal/plugins` | Manifest validation, trust store, verified snapshots, adapter/analyzer process host |
| `internal/plugins/sourceprofile` | Bounded, value-free source structure profiling for adapter onboarding |
| `internal/analyzers` | Built-in deterministic findings and analyzer protocol |
| `internal/alignment` | Behavioral fingerprints, compact paths, deterministic pair alignment |
| `internal/server` | Loopback API, source registry, artifact policy, embedded UI routes |
| `internal/watch` | Growing-file and replacement detection |
| `web/src` | React viewer and typed API client |
| `web/dist` | Generated production UI embedded by `web/embed.go` |
| `webapp` | Static browser entry point for `rlviz.dev`; reuses `web/src` through the provider boundary |
| `internal/browsercore` | WASM-safe normalization, validation, in-memory browse/read data, analysis, and comparison |
| `schemas/v1alpha1` | Public canonical and plugin contracts |
| `fixtures` | Canonical, malformed, adversarial, and protocol conformance data |
| `examples` | Runnable adapters and deterministic public gallery traces |
| `integrations` | Codex, Claude Code, and Cursor project instructions |
| `site` | External documentation generated beside the browser entry point in `site/dist` |
| `docs/adr` | Durable architectural decisions and their tradeoffs |

## Runtime paths

### Open a source in the hosted browser viewer

`rlviz.dev` serves the browser viewer and generated external documentation from
one static deployment. The former `app.rlviz.dev` surface redirects to the same
path on `rlviz.dev`. The root loads a bundled synthetic cohort into the viewer;
Settings can replace it with a dropped or selected `File` in the current tab.
A Go WASM core detects canonical NDJSON, Inspect AI EvalLog
JSON, or Verifiers GenerateOutputs JSON, validates canonical records, and
builds an in-memory collection. The shared React instrument consumes a
`ViewerProvider`; the CLI uses the daemon HTTP provider and the static app uses
the in-memory provider. Viewer components are not forked.

The app makes no outbound request containing trace bytes. Its JavaScript, Go
WASM runtime, WASM binary, and examples are local static build assets, with no
CDN dependencies. Uploaded browser adapters execute only after an explicit
SHA-256 and size confirmation and are never persisted.

### Open a source

```text
rlviz open [SOURCE]
  1. use SOURCE when supplied; otherwise restore the last usable source or materialize the bundled gallery
  2. validate and normalize an explicit presentation file, if supplied
  3. resolve the source path without modifying it
  4. load or start the per-user daemon
  5. send an authenticated registration request
  6. independently validate presentation input and choose the decoder
  7. validate and commit the first bounded record batch
  8. store normalized presentation separately from source content
  9. return an authenticated viewer URL and continue watching
  10. open the browser and let the CLI exit promptly
```

The daemon records `indexing`, `complete`, `refreshing`, or `failed`. An initial
source becomes visible after a valid header and first event batch. A refresh
keeps the prior valid generation queryable until its replacement validates and
commits atomically. Invalid or cancelled refreshes never replace known-good
data.

Presentation configuration is stored in a dedicated SQLite table keyed by
source ID, without participating in the source fingerprint. Canonical source
replacement therefore preserves presentation across refreshes and daemon
restarts; explicit registration without a presentation deletes the prior row.

`rlviz serve SOURCE` uses the same indexed read handler against an isolated
temporary SQLite index. The temporary index is removed when the foreground
server exits, while the source remains read-only. This keeps Browse and Read
API behavior identical without adding foreground data to the per-user daemon.

### Read in the browser

The browser receives the daemon token in the URL fragment. Fragments are not
sent in HTTP requests or referrers; the application reads the token and sends
it as a bearer credential to versioned local API routes.

The UI requests trajectory metadata and a bounded first page, then loads later
event, signal, and artifact pages while indexing continues. Event lists are
virtualized. Raw source locations remain available so a normalized event can be
traced back to its original record.

### Run a plugin

Adapters and analyzers are subprocesses using versioned JSON/NDJSON protocols.
RLViz never uses Go native plugins.

```text
manifest -> schema validation -> path and digest trust check
         -> copy executable plugin files to a private snapshot
         -> execute the verified snapshot with bounded I/O
         -> validate every returned record
```

Stdout is protocol-only. Stderr is captured for diagnostics. Any plugin edit
changes the digest and invalidates trust.

### Profile an unsupported source

`inspect --json` and `plugin init --json --from` can profile regular files for
adapter authoring. The profiler takes a sample of at most 256 KiB, bounds depth,
paths, and array sampling, and emits only container classification plus
field-path/type observations. It does not return scalar values or sample records.
Profiles are deterministic but intentionally incomplete; agents must treat them
as a work-order hint rather than a source schema. Profiling never executes
source content.

## Canonical and derived data

Canonical v1alpha1 entities are run, case, group, trajectory, event, signal,
artifact, and complete. Relationships use stable IDs so large sources can be
streamed without constructing a nested in-memory graph.

Adapters own source-to-canonical mapping. Analyzers produce deterministic,
removable findings and signals. Alignment and compact paths are derived views;
they do not rewrite the source or imply that independently sampled trajectories
were literal execution branches.

The generic event envelope supports an optional, sparse structured context
observation for source-backed input-token usage and explicit lifecycle changes.
It does not infer membership or interpolate unknown context usage. Message
roles, tool spans, and verifier evidence remain source-shaped or derived display
semantics pending broader real-format evidence. See `data-model.md` and
`context-semantics.md`.

## Frontend structure

The React application has three levels of research surfaces:

- `App.tsx`: composes the workspace modules and command handlers, including the
  collection evaluation hierarchy, source-backed aggregate headers, run facts,
  maximum-fidelity resource labels, and outcome-first verifier summary
- `workspaceController.ts`: reducer-backed logical workspace state, compact URL
  history, jumplist history, and topology-matched local geometry restore
- `laneLoader.ts`: cancellable per-lane and per-slot loading, bounded off-lane
  cache ownership, and stale-response rejection
- `workspaceDock.ts`: the adapter between logical workspace modules and
  Dockview panel IDs, default placement, reconciliation, and focus targets
- `useWorkspaceDock.ts`: owns the Dockview runtime lifecycle, subscriptions,
  geometry persistence, module focus, and teardown
- `viewerMetadata.ts`: bounded device-local collection and trajectory labels;
  source traces and shared workspace URLs remain unchanged
- `ContextTrack.tsx`: sparse context observations, lifecycle navigation, and
  exact selected-event context evidence without interpolation
- `ContextDetails.tsx`: selected context facts, provenance, derivation, and
  explicit retained/dropped/summarized event references
- `TrajectoryOverview.tsx`: bounded whole-run model, interaction, and evaluation
  density with loaded extent, viewport, and selected-event position
- `ResearchViews.tsx`: virtualized transcript and outcome/evidence views
- `TrajectoryTabs.tsx`: transcript, raw event timeline, and outcome switching
- `GroupView.tsx`: trajectory cohort table and compact behavioral paths
- `ComparisonView.tsx`: aligned pair comparison and divergence navigation
- `StructuredDiffView.tsx` and `structuredDiff.ts`: bounded selected-tool
  argument/result differences without executing or interpreting payloads
- `AnalysisPanel.tsx` and `ArtifactPanel.tsx`: derived findings and artifacts

`Inspector.tsx` renders a validated order of fixed, core-owned sections while
keeping selected-event identity and raw normalized access invariant.
`research.ts` derives conservative, provenance-labeled display semantics from
canonical records. Its pure `deriveLandmarkRail` selector keeps only source-backed
turns, prompts, context changes, failures, evaluations, artifacts, analyzer
references, endpoints, and the current selection. Filtering switches the rail
to raw matching events without changing canonical event navigation.
`commands.ts` is the single command/keymap registry. Validated presentation
bindings act as portable project defaults; browser-local overrides take
precedence and executable handlers remain core-owned. Its browser listener is
stable across React renders and reads current handlers through a ref, while
text-entry and modal surfaces suppress unrelated global commands.
`api.ts` is the typed daemon client, `types.ts` mirrors API records, and
`VirtualList.tsx` bounds DOM work for long lists while reporting the exact
non-overscanned viewport to the overview. Selection reveal is keyed to selected
item identity rather than measurement renders, so manual scroll remains under
the user's control. `web/e2e` and `playwright.config.ts` exercise these browser
contracts against deterministic intercepted daemon responses. Structured
context observations take precedence over legacy `context:*` alignment
landmarks; richer context
membership is shown only when the source explicitly supplies it.

The SQLite schema is currently version 5. Event rows retain their complete raw
canonical envelope while indexing nullable context operation, token, capacity,
and provenance fields for sparse access. `context_present` distinguishes a
structured observation from absent data. The indexed events API accepts strict
`context=true` and `context=false` filters; `true` includes legacy
`context:*` landmarks for compatibility, and each event appears at most once.

## Security invariants

- Bind only to loopback.
- Require the per-daemon secret for source registration and data reads.
- Make no outbound requests during normal viewing.
- Never send browser-viewer trace bytes or uploaded adapter bytes in a request.
- Treat rollout sources and referenced data as read-only.
- Resolve symlinks and prevent artifact reads outside registered roots.
- Render trace HTML as text or sanitized content, never trusted markup.
- Never execute recorded commands, tool calls, or model output.
- Execute external code only after explicit path-and-digest trust.
- Keep caches separate, discoverable, and removable.

These are product boundaries, not implementation details. Changing one requires
an explicit product decision and an ADR.

## Performance invariants

- Decode and validate streams incrementally.
- Commit bounded SQLite batches.
- Paginate events, signals, artifacts, and group summaries.
- Virtualize long browser collections.
- Preserve the old valid generation during refresh.
- Cache derived analysis by input and implementation digest.
- Profile representative traces before introducing lower-level complexity.

Quality budgets and representative fixture requirements live in `testing.md`.

## Where to start a change

| Change | Read first | Typical implementation surface |
| --- | --- | --- |
| CLI or machine output | `onboarding.md`, public command docs | `cmd/rlviz`, command tests |
| Canonical semantics | `data-model.md`, protocol docs | model, schemas, fixtures, index, API, UI |
| Adapter/analyzer behavior | `plugin-model.md`, relevant protocol | plugins, schemas, fixtures, conformance tests |
| Context lifecycle or usage | `context-semantics.md`, `data-model.md` | model, schemas, fixtures, index, API, UI |
| Viewer behavior | `ui-information-architecture.md`, `design-system.md` | `web/src`, API only when required |
| Daemon or cache | ADR 0001/0003, this document | daemon, app, index, server |
| Release/install | `releasing.md` | workflows, package, formula, scripts |

Keep this document current when a subsystem, boundary, request path, or
repository responsibility changes.
