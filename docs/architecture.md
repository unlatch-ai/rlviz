# Architecture

## Overview

RolloutViz ships as one Go binary with an embedded browser UI.

```text
CLI
 ├── daemon client
 ├── plugin commands
 └── machine-readable diagnostics
         │
         ▼
Local daemon on 127.0.0.1
 ├── source registry
 ├── adapter host
 ├── canonical event stream
 ├── index and cache
 ├── artifact access policy
 └── embedded web server
         │
         ▼
React UI
 ├── trajectory timeline
 ├── event inspector
 ├── group overview
 └── comparison views
```

## Technology choices

### Go core

Go provides straightforward cross-compilation, a small deployment surface, strong streaming and concurrency support, and a practical implementation target for coding agents.

### React and TypeScript UI

The viewer requires virtualized timelines, structured payload rendering, keyboard interaction, artifacts, and eventual comparison views. The built frontend is embedded with Go's `embed` package.

### SQLite index

SQLite stores normalized metadata, event indexes, search fields, and cache provenance. Raw source payloads remain in their original files and are read by source location where practical.

Use a pure-Go SQLite driver unless profiling proves that CGO is necessary.

### Process plugins

Adapters and analyzers run as subprocesses over a versioned JSON/NDJSON protocol. Do not use Go's native plugin mechanism because it is compiler-version-sensitive and not portable across all target platforms.

## Proposed repository structure

```text
cmd/rolloutviz/        CLI entrypoint
internal/app/          command orchestration
internal/cli/          arguments and output contracts
internal/daemon/       background-process lifecycle
internal/model/        canonical rollout types
internal/adapters/     built-in adapters and detection
internal/plugins/      manifests, discovery, trust, protocol host
internal/alignment/    fingerprints, sequence alignment, divergence
internal/index/        SQLite index and cache provenance
internal/server/       local HTTP API and embedded assets
internal/security/     path and origin validation
web/                   React application
schemas/               versioned machine-readable contracts
fixtures/              small public test trajectories
integrations/          coding-agent skills and rules
docs/                  product and engineering documentation
```

## Daemon lifecycle

`rlviz open` must return promptly when invoked by a coding agent.

```text
rlviz open PATH
  -> read daemon metadata
  -> start a detached daemon when absent
  -> authenticate to the loopback daemon with a local token
  -> register PATH and adapter selection
  -> receive a viewer URL
  -> open the system browser
  -> print structured result and exit
```

Development and troubleshooting commands:

```bash
rlviz serve PATH
rlviz status
rlviz stop
rlviz doctor
```

## Canonical data model

The initial canonical model includes run, case, rollout group, trajectory, event, signal, artifact, and annotation entities. Relationships use stable IDs rather than nested in-memory ownership so large collections can be streamed and indexed incrementally.

Events retain optional `parent_id`, `alignment_key`, and `state_hash` fields before the branch and comparison UIs ship.

## Data flow

1. Probe a path with built-in and trusted external adapters.
2. Select the highest-confidence compatible adapter or use an explicit selection.
3. Stream canonical entities and events from the adapter.
4. Validate every record against protocol constraints.
5. Index metadata and searchable text incrementally.
6. Serve paginated event data to the browser.
7. Read raw source records and artifacts on demand under a strict path policy.
8. Invalidate or extend indexes when watched files change.

## API boundaries

The local HTTP API is private to the bundled frontend in the first release. It should still use versioned routes and typed request/response schemas to keep future desktop or editor integrations possible.

The plugin protocol is public from its first release. Backward compatibility begins once `v1` is declared stable; pre-stable versions use explicit `v1alphaN` identifiers.

## Security boundaries

- The local server binds to loopback only.
- A per-daemon secret protects source registration and trajectory reads from unrelated local processes and web pages. Browser viewer URLs carry it in the URL fragment, which is not sent in HTTP requests or referrers; the UI adds it as a bearer token.
- Source registration resolves symlinks and records an allowed root.
- Artifact reads cannot escape registered roots without explicit user approval.
- HTML from traces is rendered as text or sanitized content, never trusted markup.
- External plugins require explicit trust by path and content digest.
- Trusted plugin code is copied into a private verified snapshot before execution, preventing edits to the project tree from racing execution.
- Plugin stderr is captured for diagnostics; stdout remains protocol-only.
- The viewer never re-executes recorded commands or tools.

The current vertical slice keeps at most eight opened documents in memory and caps each canonical source or adapter output at 32 MiB. The SQLite streaming index replaces these temporary bounds in the next storage milestone.

## Performance approach

- Stream JSONL and plugin output.
- Store source byte offsets where formats permit random access.
- Paginate events in the HTTP API.
- Virtualize the browser timeline.
- Truncate previews without truncating accessible raw content.
- Compute group aggregates from indexed fields.
- Cache behavioral fingerprints and alignment results by content digest.
- Profile before introducing memory mapping or lower-level storage.
