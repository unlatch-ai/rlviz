# Workspace specification (v2 architecture)

Supersedes the mode-swap architecture in `interaction-spec.md` §1. All other
interaction-spec sections (selection model, depth/fidelity/zoom semantics,
compare alignment, tiers, command-ID rules, color budget, invariants) remain
binding and apply within this structure.

## The rack

One screen, always. Three fixed zones; no floating windows, no
drag-to-rearrange:

- **Rail** (left, collapsible `t`): the collection — sources, cases,
  rollouts; attention ordering, filter, and verdict tags. The rail has one
  view governed by the fidelity ladder: L0 hairlines for shape scan, L1 marks
  for failure spotting, L2 texture and L3 glyph caterpillars for event mix,
  L4 one-line previews with event/reward columns for triage, and L5 complete
  rows for metadata inspection.
  Selecting in the rail feeds the stage; it never navigates away.
- **Stage** (center): trace lanes in two bands.
  - **Focus band**: up to 2 full-height lanes — depth descent, zoom, and
    the detail region live here. Rows (top→bottom) is the default
    direction; `Shift+V`/`Shift+H` toggles columns.
  - **Context band**: any number of fixed-height thin lanes (Surface strip
    + verdict glyph), scrollable/virtualized. Never resized by focus-band
    activity.
- **Console** (bottom): active selection detail, per-judge verdicts,
  alignment readout when a reference is pinned, breadcrumb, keys.

Former modes are **arrangements** of this one screen: Browse = rail
expanded + empty stage; Read = one focus lane maximized; Compare = 2+
lanes with a pinned reference and the alignment overlay. Deep links encode
arrangements; legacy mode URLs map onto them.

## Lane grammar (complete)

| Command | Effect |
|---|---|
| `Enter`/`Space` on rail row | open in active focus lane (replace) |
| `A` | add as new lane (fills focus band, then context band) |
| `x` | close active lane |
| `Tab` / `Shift+Tab` | cycle active lane (rail included) |
| `n` / `p` | sweep active lane through rail's filtered order, preserving that lane's depth/zoom/axis |
| `Shift+Enter` | promote/demote: swap active context lane with a focus lane (discrete animated swap; total layout constant) |
| `Shift+A` | pin active lane as reference: alignment overlay renders all lanes against it |
| `[` `]`, `+` `-` `0` | fidelity / axis zoom, active lane |
| `{` `}` | fidelity down/up, all lanes |
| `<` `>` `)` | zoom out / in / fit, all lanes (`+` is already Shift+=, so all-lane zoom has its own characters) |

Workspace state = (rail state, ordered lane list with per-lane view state,
direction, reference, active lane, seam ratios). Fully serializable to the
deep link; every state reachable and testable.

**Anti-jitter invariant:** a lane's track height/width never changes as a
side effect. Depth, fidelity, and zoom re-render a lane's interior only.
The only geometry changes are the explicit promote/demote swap and seam
drags.

## Seams

Fixed regions separated by draggable sashes (the VSCode model — no
floating windows):

- Four continuous ratios: rail width, focus/context split, focus-lane
  split, console height.
- Drag to resize; double-click a sash resets its default; ratios persist
  and serialize with the workspace.
- Keyboard route: `Ctrl+w` enters resize mode, arrows adjust the seam
  nearest the active zone, `Esc` exits.

## Layers within a lane

Depth is a real representation change (this retires the decorative depth
counter):

- **Surface**: shape strip only (context-lane rendering).
- **Episodes**: episode bands are the reading unit; bands come from adapter
  episode/alignment keys, with deterministic tool-run, error, and context
  boundaries as fallback. Width reflects event extent; the selected band's
  detail reports event-kind counts, errors, and span addresses. `j`/`k` move
  by episode; **click a band = descend into it** (zoom to its extent).
- **Events**: event stream scoped to the current episode, compressed strip
  above; click the strip = ascend one level. Landmark jumps switch episode
  and pan when their target is outside the current scope.
- **Source**: raw record + provenance for the selected event, with the
  compressed strip retained above.
- `Enter`/`Esc` are the keyboard equivalents of the click grammar. Anchor
  stability holds across every transition.
- Context-band lanes always render Surface regardless of their stored depth.
  Promoting one back to focus restores that stored depth.
- Every descent stores its pre-descent axis with the prior depth. Ascent
  restores the axis you descended from; strip-click and `Esc` are identical,
  and jumplist snapshots include the full descent stack.
- With a reference pinned, stage anchors draw as vertical alignment lines
  across all lanes (including context lanes' tick marks).

## History

Every arrangement change (lane open/close/swap, depth change, reference
pin/unpin, rail jump) pushes a workspace snapshot onto a jumplist.
`Ctrl+o` back, `Ctrl+i` forward (vim semantics). Browser back/forward map
to the same jumplist in the web surfaces. The console shows a short
breadcrumb.

## Agent analysis workbooks

RLViz never calls a model. `rlviz analyze <kind>` prints the prompt and
file paths for the user's own coding agent, which writes schema-validated
sidecar JSON that the UI renders with `agent` provenance. Kinds:
`failure-groups` (cohort failure buckets → rail filter
chips), `cross-session` (pass-vs-fail behavioral findings per stage →
console + anchor annotations), `root-cause` (hypothesis chain for one
rollout), `stage-labels` (semantic episode names). Validation rule: every
claim must carry event addresses as evidence; claims without evidence fail
validation and are not rendered. Deleting a sidecar restores the
deterministic view. With no agent available, RLViz is a pure Tier-D
display instrument.

## Flow QA

The arrangement space is a finite state machine; the flow harness
(`web/e2e/flows.ts`) walks its edges forward, backward via the jumplist,
and across via `Tab`, keyboard-only plus pointer variants that must land
in identical states. Required additional flows for this architecture:
lane add/close/swap, promote/demote, reference pin with overlay
assertions, seam resize (pointer + keyboard) with persistence, jumplist
restoration depth, layer pointer/keyboard parity, cross-episode landmarks,
anchor stability, and the rail fidelity ladder with lanes open.

## TUI mapping

Rail = left pane; focus band = one full pane (two on wide terminals);
context band = stacked single-row caterpillars; seams = fixed steps via
the same `Ctrl+w` resize mode; no pointer requirements.

## Build phases

1. **Shell**: rack zones, lanes + tracks, grammar commands, seams,
   jumplist, arrangement deep links, flow tests for all of the above.
2. **Layers**: real per-depth representations with the click grammar.
3. **Alignment overlay**: reference pin, cross-lane anchors, divergence
   readout in console, context-lane ticks.
4. **Workbooks**: `rlviz analyze`, sidecar schemas, rail/console
   renderings.
