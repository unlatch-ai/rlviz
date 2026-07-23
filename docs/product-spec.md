# Product specification

## Product

| Field | Value |
| --- | --- |
| Name | RLViz |
| npm package | `rlviz` |
| CLI | `rlviz` |
| Tagline | Inspect agent rollouts locally. |

## Problem

Researchers and engineers building agent environments, evaluations, and post-training systems inspect trajectories constantly. Those trajectories are commonly stored as JSON, JSONL, database rows, directories of artifacts, or harness-specific logs.

The available options usually require one of the following:

- reading raw structured data manually
- writing one-off notebooks
- instrumenting the system before the run
- uploading sensitive traces to a hosted platform
- standing up a larger observability stack

The immediate use case is simpler. An engineer is debugging an environment with a coding agent. The agent reports that a test trajectory failed at a particular step. The engineer asks it to open that trajectory, and a fast local viewer appears with the relevant events and artifacts.

## Product boundary

RLViz provides visualization, navigation, comparison, and a plugin boundary. It does not provide an LLM, execute the agent, train the model, or require a hosted control plane.

Coding agents act as operators and extension authors:

- locate the requested trace
- invoke the CLI
- scaffold adapters for unsupported formats
- validate adapters against local fixtures
- optionally contribute reusable adapters or product changes

## Primary users

### Environment engineer

Needs to understand whether a failure came from the policy, tool contract, seed state, environment implementation, grader, permissions, or infrastructure.

### Post-training researcher

Needs to inspect rollout groups, compare successful and failed trajectories, understand reward composition, identify behavioral divergence, and evaluate checkpoint changes.

### Agent engineer

Needs to inspect tool sequences, retries, subagents, artifacts, termination behavior, latency, tokens, and final outcomes without instrumenting the agent again.

## Core concepts

### Run

A source-native execution or experiment with shared configuration such as model, checkpoint, environment, harness version, and timestamp.

### Case

A task or input evaluated within a run. A stable case ID allows the same task to be compared across runs.

### Rollout group

Trajectories generated together for the same case and sampling condition. For example, a GRPO sampling group of eight trajectories.

### Trajectory

One episode or path through the agent-environment interaction.

### Event

One ordered unit in a trajectory. Initial event kinds are:

- message
- generation
- tool
- environment action
- observation
- state
- reward
- grader
- artifact
- error
- log

### Signal

A numeric, categorical, or textual assessment attached to a trajectory or event. Examples include reward components, pass/fail, advantage, grader labels, latency, and token counts.

### Comparison set

A viewer-created selection of trajectories, potentially across runs, checkpoints, models, or configurations. This is distinct from a source-native rollout group.

## MVP user experience

### Open in the browser without installing

`rlviz.dev` opens local traces without an account or upload. The trace is parsed
in the tab and never uploaded. The browser uses the same instrument viewer as
the CLI, with a Go WebAssembly core and an in-memory collection instead of the
local daemon and SQLite index.

The browser viewer is for individual files and modest cohorts. The UI and Go
core both refuse files above 32 MiB with a CLI next step because the raw bytes,
canonical output, and browse index coexist in tab memory. The 300-event gallery
trace is a required build and browser test fixture. Private formats and large
cohorts remain the full CLI's job.

### Installation

Users download a release binary or install it through a package manager.

```bash
brew install <tap>/rlviz
```

### Open a supported trajectory

```bash
rlviz open ./artifacts/task-184.jsonl
```

Expected behavior:

1. Detect a compatible built-in or installed adapter.
2. Start or reuse a loopback-only local daemon.
3. Register and index the source without mutating it.
4. Open the system browser at a stable local URL.
5. Return immediately so coding-agent shell calls do not hang.
6. Watch active source files and stream appended events into the UI.

With no explicit source, `rlviz` and `rlviz open` restore the last usable local
source. With no usable history they open bundled synthetic data. The browser
root also enters the shared viewer directly with a bundled synthetic cohort.

### Open an unsupported trajectory

The CLI returns a machine-readable diagnostic and scaffold command:

```json
{
  "code": "unsupported_format",
  "path": "/absolute/path/trajectory.jsonl",
  "suggested_command": "rlviz plugin init --type adapter --lang python --from /absolute/path/trajectory.jsonl .rlviz/plugins/local-adapter"
}
```

A coding agent can then create, validate, and use a local adapter:

```bash
rlviz plugin init --type adapter --lang python --from ./trajectory.jsonl ./plugins/customer-x
rlviz plugin trust ./plugins/customer-x
rlviz plugin validate ./plugins/customer-x ./trajectory.jsonl
rlviz open ./trajectory.jsonl --adapter ./plugins/customer-x
```

## Single-trajectory viewer

The viewer is a docked workspace with a collection module, one or more rollout
modules, a selected-event detail module, and optional rollout-pinned detail
modules. Additional rollouts open as rows by default. Modules can be moved and
resized with the pointer or keyboard.

The viewer must support:

- collapsed rendering for large model and tool payloads
- adjustable overview fidelity and semantic depth
- compact source-backed run facts for model/configuration, outcome, work, resources, and duration
- per-step tool names and resource facts at maximum overview fidelity
- a draggable and resizable full-rollout timeline viewport
- active-module shortcuts in the persistent bottom keybar and the complete default map in Guide
- distinct visual treatment for actions, observations, errors, rewards, and grader output
- outcome-first verifier inspection with structured type, rubric, verdict, reason, and evidence before raw logs
- raw JSON inspection for every normalized event
- source file and byte/line location
- inline text, JSON, image, log, and diff artifacts
- full-text search
- stable deep links to trajectories and events
- keyboard navigation
- live updates for growing files

Initial keybindings:

| Key | Action |
| --- | --- |
| `j` / `k` | Next or previous event |
| `Enter` | Descend toward the selected event's Source layer |
| `Space` | Expand or collapse |
| `/` | Search |
| `e` | Next error |
| `r` | Next reward or grader event |
| `x` | Close active module |
| `o` | Open selected artifact |
| `m` | Show trajectory metadata |
| `?` | Show keybindings |

## Rollout-group viewer

Group support follows the single-trajectory viewer but is represented in the canonical schema from the start.

Collection trial mode follows the canonical evaluation hierarchy: run → case/task
→ rollout group/variant → trajectory. It never infers an unreported pass or
failure. Aggregate labels describe only source-backed values.

The first group view includes:

- trajectory table
- reward and pass distributions
- outcome and termination summaries
- step, token, latency, retry, and error counts
- best and worst trajectory shortcuts
- behavioral path fingerprint
- sorting and filtering by any signal
- adapter-declared scalar metrics in group headers

## Divergence

RLViz distinguishes textual difference from behavioral divergence. Different reasoning text that leads to the same action should not automatically be treated as the first meaningful divergence.

The deterministic baseline algorithm will:

1. Normalize events into behavioral fingerprints.
2. Anchor on tool calls, environment actions, state hashes, rewards, errors, and termination.
3. Align event sequences using a sequence-diff algorithm.
4. Identify the common behavioral prefix.
5. Mark the first meaningful divergence.
6. Continue aligning equivalent later events when possible.

Adapters may provide optional alignment keys and state hashes. Domain-specific analyzer plugins may enrich alignment later. LLM-based semantic alignment is optional and never required for core viewing.

## Compact path aggregation

For independent trajectories in a group, RLViz may aggregate equal behavioral prefixes into a compact path tree. This is a derived visualization, not a claim that the source execution literally branched.

Real parent-child branches recorded by a harness remain separate and preserve source-native relationships.

## Non-goals for the first release

- hosted accounts or collaboration
- authentication and organization management
- production observability and alerting
- training orchestration
- evaluation execution
- prompt management
- automatic LLM summaries
- arbitrary third-party JavaScript in the viewer
- trajectory replay or tool re-execution

## Privacy and security

- Bind the server to `127.0.0.1` by default.
- Make no outbound network requests during normal viewing.
- Treat source trajectories as read-only.
- Do not serve arbitrary local paths referenced by untrusted trace data.
- Require explicit trust before executing external adapter or analyzer plugins.
- Store indexes and normalized caches separately from source data.
- Make caches discoverable and removable with CLI commands.
- Never silently redact or rewrite source records.

## MVP success criteria

- A new user can install and open a supported trajectory in under one minute.
- A coding agent can scaffold a working adapter without learning core internals.
- Small traces reach first meaningful paint in about one second on a warm machine.
- Large JSONL inputs render progressively without loading the entire file into memory.
- Navigation remains smooth with at least 10,000 events.
- Every normalized event can be traced back to its raw source record.
- The viewer performs zero source mutations and zero outbound network requests.
