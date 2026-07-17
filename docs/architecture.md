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
  open / status / stop / doctor / cache / plugin            |
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
  trajectory / group / compact paths / pair comparison       |
  event details / artifacts / analyzer findings              |
                                                             |
source files <---------- always read-only -------------------+
```

The binary contains no model and does not execute recorded tools. Coding agents
operate the CLI and can author project-local adapters; RLViz remains the viewer.

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
| `schemas/v1alpha1` | Public canonical and plugin contracts |
| `fixtures` | Canonical, malformed, adversarial, and protocol conformance data |
| `examples` | Runnable adapters and public example traces |
| `integrations` | Codex, Claude Code, and Cursor project instructions |
| `docs/adr` | Durable architectural decisions and their tradeoffs |

## Runtime paths

### Open a source

```text
rlviz open SOURCE
  1. validate and normalize an explicit presentation file, if supplied
  2. resolve the source path without modifying it
  3. load or start the per-user daemon
  4. send an authenticated registration request
  5. independently validate presentation input and choose the decoder
  6. validate and commit the first bounded record batch
  7. store normalized presentation separately from source content
  8. return an authenticated viewer URL and continue watching
  9. open the browser and let the CLI exit promptly
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

- `App.tsx`: trajectory loading, routing, shared event selection, landmark rail,
  and selected-event-first inspector
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

`research.ts` derives conservative, provenance-labeled display semantics from
canonical records. `commands.ts` is the single command/keymap registry.
`api.ts` is the typed daemon client, `types.ts` mirrors API records, and
`VirtualList.tsx` bounds DOM work for long lists while reporting the exact
non-overscanned viewport to the overview. Structured context observations
take precedence over legacy `context:*` alignment landmarks; richer context
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
