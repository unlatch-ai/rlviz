# RLViz features and workflows

This is the product-facing feature registry. Use it to scope work, write tests,
and keep the site and docs aligned with the viewer. A feature is `current` only
when its primary workflow is implemented and covered at the appropriate layer.

Status: `current` ships in the working product, `next` is committed near-term
work, and `later` is an intended capability without a committed release.

## Supported workflows

| Workflow | User outcome | Status |
| --- | --- | --- |
| Open a local trace | Run `rlviz open PATH`; inspect it in a loopback-only viewer without mutating or uploading it | current |
| Resume or learn without a source | Run `rlviz` or `rlviz open`; restore the last usable source or open bundled samples on first use | current |
| Open in the browser | Drop a modest trace into the browser viewer; parsing and indexing stay in the tab | current |
| Browse a collection | Filter rollouts, change visual fidelity, switch between a flat list and run → task → variant → rollout evaluation hierarchy, and open or add a rollout | current |
| Inspect a rollout | Read compact run facts, move through events and landmarks, change depth and fidelity, adjust the timeline viewport, and inspect normalized and raw source data | current |
| Build a workspace | Arrange rollout modules in rows or columns, move and resize modules, keep context lanes, and share the logical workspace without viewport-specific geometry | current |
| Keep one rollout in detail | Open a rollout-pinned detail module, navigate it directly, and inspect other lanes without replacing it | current |
| Triage a rollout group | Inspect outcome and metric distributions, sort or filter trajectories, and jump to representative or failing rollouts | current |
| Compare behavior | Align two trajectories around behavioral anchors and inspect their first meaningful divergence and structured differences | current |
| Extend an unsupported format | Scaffold, trust, validate, and run a local adapter without changing the viewer | current |
| Configure the viewer | Use the default Settings module for theme, data opening, and format guidance | current |
| Let an agent compose the GUI | Query canonical trajectories and open or update named browser workspaces from structured CLI commands | current |
| Inspect alignment in context | Overlay aligned reference events and divergence evidence directly on rollout lanes | next |
| Hand work to an agent | Generate a local workbook that gives a coding agent bounded trace context and explicit tasks | next |
| Use one public surface | Read the short product explanation, install, open a trace, and continue into the viewer on `rlviz.dev` | current |

## Feature areas

### Local data and formats

- `current` Canonical run, case, rollout-group, trajectory, event, signal, and artifact model.
- `current` Built-in format detection plus process-isolated adapters and analyzers.
- `current` Source provenance down to line, byte range, or original raw record.
- `current` Read-only source handling, removable caches, loopback binding, and no outbound requests during normal viewing.
- `current` Progressive indexing and updates for growing local files.
- `next` Clear first-run format diagnostics and an in-product adapter handoff.

### Collections and triage

- `current` Keyboard-first filtering and rollout selection with visible-selection follow.
- `current` Wheel scrolling inside the collection module.
- `current` Flat rollout view and canonical run → task → variant → rollout evaluation hierarchy.
- `current` Source-backed pass, infrastructure, timeout, step, token, and adapter-declared scalar aggregates on evaluation headers.
- `current` Three truthful collection fidelity levels: hairline, glyphs, and detail.
- `current` Outcome, reward, pass, error, token, latency, retry, and termination signals when present.
- `later` Saved collection presets for recurring evaluation suites.

### Rollout inspection

- `current` Four semantic depths: overview, episodes, events, and source.
- `current` Independent zoom and timeline viewport controls with click-to-center, drag-to-pan, and edge resizing.
- `current` Overview fidelity from hairline to every visible step, including tool-call names.
- `current` Compact model, variant, outcome, reward, step, tool, error, token, and duration run facts when supplied.
- `current` Per-step token, duration, context-resource, and reward facts at maximum fidelity when supplied.
- `current` Outcome-first verifier summary with type, rubric, verdict, reason, evidence, and raw grader-event access.
- `current` Event, error, reward/grader, context, finding, artifact, and marker navigation.
- `current` Selected-event detail with payload, provenance, signals, and artifacts.
- `current` Pinned detail modules tied to one rollout.
- `later` Persisted analyst annotations and exportable evidence ranges.

### Workspace and interaction

- `current` Docked collection, rollout lanes, global detail, and rollout-pinned detail modules.
- `current` Docked Guide module backed by the same agent-readable CLI and public documentation.
- `current` Rows-first additional rollouts, optional columns, pointer docking, and keyboard move/resize modes.
- `current` `Alt` + arrow spatial navigation and `Tab` cycling.
- `current` Active-module shortcuts in the persistent bottom keybar; the Guide contains the complete default keybinding reference.
- `current` Customizable keymap storage and toggleable move/resize modes.
- `current` Docked Settings module for appearance, browser data loading, and adapter guidance.
- `current` Shareable logical workspace links and bounded, device-local dock geometry.
- `current` Device-local editable titles and descriptions for collections and trajectories, without changing source traces.
- `next` Complete keybinding-remap UI backed by the shared config.

### Comparison and derived analysis

- `current` Reference lanes and two-trajectory comparison.
- `current` Deterministic alignment on tool calls, environment actions, state, rewards, errors, and termination.
- `current` First meaningful divergence and later-event realignment.
- `current` Compact behavioral-path aggregation labeled as derived, never source-native branching.
- `next` Alignment overlays inside the main workspace.
- `later` Domain-specific analyzer overlays that remain distinguishable from source facts.

### Presentation and distribution

- `current` CLI daemon viewer and zero-upload WebAssembly browser viewer sharing one instrument UI.
- `current` Light/dark themes, presentation palettes, semantic glyphs, and accessible text equivalents.
- `current` One public surface at `rlviz.dev`, external-user documentation, and plain metadata with a product-specific favicon and social preview.
- `current` Agent-readable setup instructions plus query and named-workspace control commands.
- `next` Agent-readable analysis workbooks.

## Product boundaries

RLViz is a local inspection and comparison tool. Hosted accounts,
collaboration, production alerting, training or evaluation execution, prompt
management, automatic LLM summaries, arbitrary viewer JavaScript, and tool
re-execution are not product goals unless this document is deliberately
changed first.

## How work maps to this file

Every product task should name one supported workflow and one feature area.
UI changes need an observable browser-flow assertion; data and protocol changes
need source-truth tests; privacy changes need both. Docs and site copy should
describe only `current` behavior as available and label `next` or `later`
behavior explicitly.
