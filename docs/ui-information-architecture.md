# UI information architecture

## Product stance

RLViz is a research workbench, not a generic log viewer. The interface should
answer four questions quickly:

1. What happened in this rollout?
2. Why did it end with this outcome?
3. What did the model know at the moment it acted?
4. Where did this rollout begin to behave differently from another one?

The current three-pane viewer is a useful implementation baseline, but it gives
the event outline, event cards, and raw inspector nearly equal weight. The
target hierarchy should follow the researcher's mental model: outcome, turns,
context, evidence, and source detail.

## Core reading unit

The canonical storage unit remains an event. The primary visual unit should be
a **turn or span**: a model interaction containing the prompt context, model
generation, zero or more tool calls and results, environment observations, and
signals produced from that work.

An adapter may not always provide explicit turns. RLViz can group events only
when the mapping is deterministic and must label inferred structure as inferred.
The raw ordered event stream always remains available.

## Application frame

The workspace can also dock a concise Guide module. It is open on a fresh
workspace, can be toggled with `Shift+G`, and uses the same source as the CLI
and public guide. A blank Detail module is not shown before a rollout exists.

```text
+-----------------------------------------------------------------------+
| run / case / rollout                         outcome   reward   command |
+-------------+-----------------------------------------+-----------------+
| landmarks   | primary reading surface                 | details         |
|             |                                         |                 |
| turns       | Transcript / Timeline / Outcome /       | selected turn   |
| tools       | Context / Raw                           | source / raw    |
| errors      |                                         | artifacts       |
| compactions | system and user messages                | evidence        |
| graders     | assistant generation                    |                 |
| branches    | tool spans and observations             |                 |
|             | compaction boundaries                   |                 |
+-------------+-----------------------------------------+-----------------+
| context usage and global timeline landmarks           | key hints       |
+-----------------------------------------------------------------------+
```

Only the primary reading surface should dominate. The landmark rail and details
panel are supporting navigation and can collapse. Dense does not mean tiny: the
product should offer compact and comfortable density without reducing essential
text below a readable size.

## Persistent run header

The header establishes identity and outcome before event-level detail:

- run, case, group, trajectory, checkpoint/model, and harness version
- status and termination reason
- final reward and pass/fail/verifier verdict
- duration, total turns, tokens, tool calls, errors, and compactions
- indexing/refresh state and source freshness
- actions: switch trajectory, compare, copy deep link, command palette

Unknown values are omitted or shown once as unavailable; rows of decorative
dashes should not compete with real information.

## Trajectory modes

### Transcript

The default mode for agent rollouts. It groups system/developer/user/assistant
messages, tool calls, results, and environment observations into readable turns.
Large payloads collapse with meaningful summaries. Tool call and result remain
visually paired even when other events occur between them.

### Timeline

A dense chronological mode for event-level inspection. It is useful for logs,
high-frequency environment actions, latency, retries, and sources that cannot
provide reliable turn semantics.

### Outcome

A first-class explanation of the final result:

- final model output
- status and termination
- verifier and judge verdicts, scores, rubrics, and provenance
- reward components and aggregate reward
- evidence links back to relevant turns and events
- errors or infrastructure failures distinguished from policy failures

Grader records remain inspectable as raw events, but the researcher should not
have to find the final verdict by scrolling.

### Context

Shows what was available to the model over time:

- system/developer prompt layers
- input, cached, and output token tracks
- context capacity and utilization
- compaction, truncation, restoration, and injected-context boundaries
- before/after membership for a context change when the source provides it
- generated summary and its provenance

RLViz must distinguish source-native facts from adapter-derived or inferred
facts. Missing context data should degrade honestly rather than simulate it.

### Raw

The lossless normalized records and source locations. This is the debugging
escape hatch, not the default reading experience.

### Current implementation

Transcript, event timeline, and outcome are implemented as one shared-selection
workspace. The transcript is virtualized, deterministic turn boundaries are
labeled inferred, and final output, graders, reward components, errors, and
evidence link back to canonical events. Explicit `context:*` alignment keys
appear as compatibility landmarks and are keyboard reachable. Sparse structured
context observations render on a separate source-backed track without
interpolating unobserved gaps.

## Landmark rail and minimap

Long rollouts need semantic navigation rather than a second copy of the event
list. The rail should expose:

- turns and subagent spans
- tool calls and retries
- errors
- reward and grader events
- artifacts
- context changes and compactions
- branch or parent changes
- analyzer findings

A thin overview track should show distribution across the full trajectory and
the visible window. Filters operate on landmarks and the primary surface
together. Search results and deep links must keep stable event/turn identity.

The first overview is implemented as three bounded aggregate lanes for model,
interaction, and evaluation activity. It shows loaded extent, current event-list
viewport, and selected-event position across at most 64 bins. Unloaded space is
explicitly unavailable. Context evidence remains in the context track; the
overview does not duplicate individual landmarks or infer missing events.

The landmark rail is sparse by default. It includes source-backed prompts and
turns, context lifecycle events, failures, evaluations, artifacts, analyzer
references, loaded endpoints, and the current selection. Tool titles and payload
text alone do not promote an event. Search or kind filtering changes the rail to
Results and shows every raw match; a selected event outside the filter remains
pinned for orientation. Raw events remain available in transcript/timeline
surfaces, deep links, and `j`/`k` navigation.

## Details panel

The trajectory outcome and structured verifier contract are persistent context:
verifier type, rubric, verdict or score, reason, and cited evidence precede raw
grader logs. The selected turn or event follows and remains directly navigable.
Trajectory-wide findings and artifacts belong in explicit tabs
or drawers so they do not push selected-event information below the fold.

Suggested tabs:

- Details: semantic properties and linked records
- Payload: input/output/content with structured viewers
- Source: raw normalized record and original file location
- Evidence: verifier references, signals, analyzer findings, artifacts

## Group workspace

The group view is for choosing cohorts and representative trajectories before
opening individual runs. Collection trial mode exposes the canonical run →
case/task → rollout group/variant → trajectory hierarchy. Headers aggregate only
reported outcomes and metrics; missing values stay visibly unknown.

- persistent cohort definition: run/case/sampling/checkpoint conditions
- distributions for reward, pass rate, termination, length, tokens, latency,
  errors, retries, and user-selected signals
- filterable and configurable trajectory table
- shortcuts for best, worst, median, outliers, pass, fail, and infra failure
- explicit comparison selection and saved column layout
- compact behavioral paths as a secondary explanation, clearly labeled derived
- adapter-declared scalar columns alongside pass, infrastructure, timeout, step,
  and token summaries

The current table supports ANDed plain-text, outcome, core-metric, and scalar
signal filters. The active cohort query is URL-backed so reloads and copied
links preserve the selection without a hosted saved-view service.

Pair comparison is the first comparison mode. Later comparison sets may support
pass-versus-fail and checkpoint-versus-checkpoint cohorts.

## Pair comparison

Preserve the current useful distinction between text difference and meaningful
behavioral divergence. The default comparison should show:

- outcome, verifier, reward, token, context, and termination deltas
- shared behavioral prefix compression
- first meaningful divergence and later realignment
- synchronized turn/tool expansion
- structured tool argument/result differences
- context and compaction differences
- next change and next behavioral divergence navigation

Raw JSON remains available in a details tab, not as the default inspector.

## Commands and keymaps

Keyboard behavior is defined by stable command IDs, not component-local key
handlers. A central registry owns:

- command ID, label, scope, default bindings, and enablement
- active-view precedence and text-entry suppression
- user overrides stored locally
- conflict detection, reset, import, and export
- command palette and shortcut reference generated from the same registry

The registry, local rebind UI, persistence, conflict detection, generated key
hints, and portable project defaults through presentation configuration are
implemented. Browser-local edits override project defaults. Command-palette
and standalone keymap-file import/export remain future work.

Core commands include next/previous landmark, next/previous turn, expand,
inspect, search, next error, next context change, next grader, open outcome,
open group, compare, copy deep link, and descend to Source. Default Vim-style bindings
can coexist with arrows and accessible button navigation.

## Extension boundary

Adapters provide semantics; analyzers provide deterministic derived data; core
RLViz owns layout and interaction. Customization may declare fields, labels,
signal formats, badges, artifact media types, inspector sections, themes, and
keymaps. Arbitrary plugin JavaScript or CSS is not allowed in the core viewer.

Inspector ordering and visibility are implemented as an exact list of fixed
core sections. Selected-event identity and raw normalized access remain fixed;
presentation configuration cannot inject markup or disable underlying data and
keyboard behavior.

Portable keymaps may only replace bindings for stable core command IDs. They
cannot add commands or handlers, and invalid or conflicting maps fail closed.

Group-table layouts follow the same boundary. Trajectory identity, comparison
selection, and the open action are fixed; researchers can hide optional metrics
and choose up to eight scalar canonical signals from a bounded discovery list.
The preference is small, versioned local data. Invalid or unavailable browser
storage falls back to coverage-ranked defaults without blocking the viewer.

## Empty, loading, and partial states

- A source that is still indexing shows which data is usable now.
- Missing optional semantics explain what the adapter did not provide.
- An unsupported format gives the exact CLI/agent next step.
- Empty groups distinguish no trajectories from filters hiding trajectories.
- Failed refreshes keep the last valid data visible and disclose staleness.
- The demo is explicitly labeled synthetic; it never appears as silent fallback
  when the local API is unavailable.

## Acceptance questions

Before shipping a viewer change, verify that a researcher can answer:

- What is the result and why?
- Where is the currently selected action in the full rollout?
- What messages and context preceded it?
- Did compaction or truncation change what the model knew?
- Which verifier or reward evidence refers to this step?
- Can I reach the next error, grader, context change, or divergence by keyboard?
- Can I trace every derived view back to canonical and raw source data?
