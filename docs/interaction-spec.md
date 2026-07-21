# RLViz interaction specification (v1)

Binding spec for the viewer's structure and behavior. Supersedes
`ui-information-architecture.md` and the layout assumptions in
`trajectory-workspace-v2.md` (both retained as historical evidence).
Working reference implementations of the interaction model live in
`web/concepts/*.html` — treat them as executable spec for feel and keys,
not as production code or visual design.

The two tests for any screen: (1) it must be describable in this spec's
vocabulary; (2) it must survive being rendered as a TUI without losing its
structure.

## 1. Modes

Three modes, one overlay class. No other top-level surfaces.

- **Browse** — home: everything the daemon knows (sources, runs, cases,
  groups) as one navigable collection with multiple *projections* of the
  same list — dense table, caterpillar strips, by-case grouping — switched
  by view keys, never separate pages. Triage (attention ordering, one-key
  verdict tags, shrinking unresolved queue) is a capability of Browse when
  the collection is a cohort. Search/filter narrows in place.
- **Read** — one trajectory. Default only for single-trajectory sources or
  deep links.
- **Compare** — 2..N trajectories, stage-aligned. Entered from Browse
  (mark set + `v`) or Read (vs reference).
- **Overlays** — search, command palette, keymap help `?`, artifact viewer,
  raw-source inspector. Overlays stack on a mode; `Esc` dismisses topmost
  first, never destructive.

Browse is the hub; every descent remembers the Browse state (filter,
projection, queue position) it came from.

## 2. Read mode at t=0

Deterministic layout per source type (a hanging protocol, never
user-rearranged panes). Reading order:

1. **Verdict header** — task identity; outcome per judge (verifier, reward,
   grader — disagreement shown, never collapsed); termination reason.
   Every verdict element links to its evidence event.
2. **Shape strip** (dominant object) — the whole trajectory on one shared
   axis: episode bands, event texture (shape encodes kind, never hue),
   context-pressure lane, health/reward curve, landmark marks. Pathologies
   must be legible as shapes (retry comb, context ramp, compaction cliff).
3. **Selected moment** — pre-placed at the first meaningful anomaly (first
   error, else first divergence, else first finding, else event 0); its
   content renders in the detail region.

No transcript wall at t=0; prose is descended into.

## 3. Selection model

- One selection state per mode; keyboard, mouse, search, deep links, and
  the strip all read/write it.
- Selection ≠ focus ≠ hover. Hover is a *skimmer*: previews content in a
  fixed slot without moving selection or scrolling; mouse-out restores.
- Every event has a stable short address (seq index; `turn.step` when
  canonical), typeable via goto, used in deep links.
- Reveal-once: deliberate navigation/deep-link restore scrolls to selection
  exactly once; unrelated renders never yank a scrolled surface.

## 4. Depth, fidelity, zoom (three orthogonal controls)

- **Depth** (`Enter` descend / `Esc` ascend): Surface → Episodes → Events →
  Source. Layers are representations of one axis; descending changes
  representation and screen allocation, not place. Other layers compress to
  orientation strips; the shared axis and playhead stay visible everywhere.
- **Fidelity** (`[` / `]`): how much each unit renders — hairline strip →
  marks → texture → glyph rows → one-line previews → full content. Global
  per mode ("caterpillar" control in Browse).
- **Axis zoom** (`+` / `-` / `0` fit): which span of the axis fills the
  viewport, anchored on the selection — the selected event's screen
  position does not move across zoom (verified property of the concept
  implementation; keep it testable). Navigation outside the window pans it.
- Anchor stability across all three: the selected moment stays fixed on
  screen; the world moves around it.
- `j`/`k` traverse the active layer's units. Landmark jumps (`e` error,
  `c` context, `r` reward, `a` finding) work at any depth.
- Registration: an episode's raw events render beneath it at the same axis
  extent; a summary that cannot point at its constituent records may not be
  drawn.

## 5. Browse / triage

- Queue ordered by attention-worthiness (findings, judge disagreement,
  marginal reward first); confident-clean sinks and can be swept in bulk.
- Rows are caterpillars: verdict glyph + strip with length ∝ event count.
  Nominal renders silent; abnormal carries marks.
- `n`/`p` move to next/prev rollout *while staying in Read*, preserving
  depth, filter, and aligned position.
- One-key verdict tags advance the queue; the unresolved count visibly
  shrinks; "next unresolved" skips triaged rows. Tags are local annotations
  (sidecar/cache), never writes to the source.
- Tagging a rollout with unvisited findings says so instead of silently
  advancing.

## 6. Compare

- One side is the designated reference; everything renders relative to it.
- **Alignment is on stages, never step index or tool-call order.**
  Independent rollouts permute tool order immediately and meaninglessly.
  Anchors resolve in order: adapter-declared episode boundaries → configured
  anchors (presentation config) → agent-annotation sidecar stages →
  outcome-only. The UI states which tier produced the alignment.
- Within a stage, sides are independent-length and unaligned; tool-order
  permutations and reasoning-text differences with identical behavior are
  not divergence. Divergence is the first stage whose behavior differs in
  an outcome-relevant way (`d` jumps there).
- Divergence curve: cumulative cost delta (steps/tokens) vs reference per
  stage.
- N-way: rollouts as rows, stage boundaries as shared anchors, per-stage
  cell width ∝ steps, per-stage delta vs reference. Pair compare is N=2
  with richer in-stage rendering and per-stage lockstep scrolling.

## 7. Replay

Exists for hindsight control only: truncates everything after the playhead
so the reader judges the next action against exactly what the model could
see, then reveals it (`→` step, optional auto-advance). All lanes truncate
together. Sequence-only sources replay by reveal-in-order; no wall clock
implied. Exit restores the record, playhead becomes selection.

## 8. Derived-information tiers

- **Tier D — deterministic** (core/adapters/analyzers): parsing, addresses,
  counts, retry/loop detection, context lifecycle, outcome extraction,
  anchor candidates. Always available; the UI must be fully functional on
  Tier D alone.
- **Tier C — configured semantics** (validated data, per plugin model):
  glyph/label/palette bindings, columns, landmark and anchor definitions,
  keymaps.
- **Tier A — agent-derived annotations** (sidecar JSON, schema-validated):
  stage labels, root-cause notes, clustering, titles. Produced by external
  coding agents against a documented schema; RLViz never calls a model.
  Rendered with visible inferred-provenance; never overrides Tier D;
  deleting the sidecar restores the deterministic view.

## 9. Command grammar

Stable command IDs in the existing registry (`web/src/commands.ts`);
bindings are remaps; IDs never change meaning. Letters are commands only
while a reading surface has focus. Every pointer action has a keyboard
route. `?` shows active-scope bindings.

Keep existing bindings. Additions (conflict-checked per scope):

| Command ID | Default | Scope |
|---|---|---|
| `trajectory.nextRollout` / `previousRollout` | `n` / `p` | trajectory |
| `trajectory.ascend` | `Escape` | trajectory |
| `trajectory.markIn` / `markOut` | `i` / `Shift+O` | trajectory |
| `trajectory.goto` | `:` | trajectory |
| `trajectory.replay` | `Shift+R` | trajectory |
| `trajectory.pivotAggregate` | `.` | trajectory |
| `trajectory.dropMarker` / `cycleMarkers` | `m` / `Shift+M` | trajectory |
| `view.fidelityUp` / `fidelityDown` | `]` / `[` | all |
| `view.zoomIn` / `zoomOut` / `zoomFit` | `+` / `-` / `0` | trajectory, comparison |
| `group.tagVerdict*` | `1..4` | group |
| `comparison.toggleDivergenceCurve` | `Shift+D` | comparison |

## 10. Visual and semantic invariants

1. Selection never moves except by explicit user action or first-open
   pre-placement.
2. Shared axis + playhead visible at every mode and depth.
3. Grayscale base; color only for selection, focus, failure (by class),
   context lifecycle, evidence. Categories get shape/position/texture,
   never hue. Nominal is silent — no pass-badge spam.
4. Flag budget: ≤3 severities; identical findings aggregate; dozens of
   badges on one rollout is an analyzer-contract bug.
5. Provenance (source / adapter / analyzer / inferred) distinguishable at
   every depth; every summary one descent from raw source.
6. Icons: bundled open-source set (Lucide; embedded, no CDN) mapped from a
   semantic token table `kind → {icon, unicode glyph, label}`; the glyph
   column serves the TUI and high-density views; Tier C rebinds by *naming*
   bundled icons, never supplying image data.
7. Ship complete defaults (icons, 2–3 validated palettes); customization is
   token override with contrast validation, not design-from-scratch.
8. Existing accessibility/performance gates hold: AA contrast, full
   keyboard completion, reduced motion, 10k-event smoothness, zero
   outbound requests.

### Colors

The grayscale chrome uses `--page` (`#f9f9f7` light / `#0d0d0d` dark),
`--surface` (`#fcfcfb` / `#1a1a19`), `--ink` (`#0b0b0b` / `#ffffff`),
`--ink-secondary` (`#52514e` / `#c3c2b7`), `--muted` (`#898781` in both
modes), and `--hairline` (`#e1e0d9` / `#2c2c2a`). Selection is achromatic:
an ink playhead, a 1.5px ink outline, and a low-alpha ink wash. Focus uses a
2px `--ctx` outline.

Exactly four semantic hue tokens are available:

- `--ctx`: `#2a78d6` light / `#3987e5` dark for context lifecycle,
  compaction marks, context-lane tint, and focus.
- `--fail-policy`: `#d03b3b` in both modes for policy or model failures and
  failing judge chips.
- `--fail-infra`: `#ec835a` in both modes for infrastructure or environment
  failures. It is never used for text and always has a shape or label because
  it is below 3:1 contrast on the light theme.
- `--good`: `#006300` light / `#0ca30c` dark, used only for verifier-pass chip
  text and ahead deltas in comparison views; it is never a badge background.

Mild-attention states such as stale, truncated, or partial data use muted ink
with dashed or hollow shapes and no hue. Inferred and Tier-A data use muted ink
with a hatched texture and no hue.

The TUI maps `--ctx`, `--fail-policy`, `--fail-infra`, and `--good` to the
ANSI-16 blue, red, yellow, and green codes respectively. If `NO_COLOR` is set,
it emits no color escape codes.

## 11. TUI

A second renderer of the same spec, in the same binary (`--tui`): same
modes, same command grammar, glyph column instead of SVG. v1 TUI scope:
Browse (caterpillar + table projections), Read (strip + event list, depth/
fidelity/zoom/landmarks), pair compare, search. Artifacts open via
`$EDITOR`/browser handoff.
