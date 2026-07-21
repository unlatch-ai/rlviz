# Plugin and customization model

## Boundaries

RLViz has three extension layers with different trust and ownership:

| Layer | Owns | Execution |
| --- | --- | --- |
| Adapter | Source detection and canonical mapping | Trusted local process |
| Analyzer | Deterministic derived findings and signals | Trusted local process or built in |
| Presentation configuration | Labels, scalar formats, columns, inspector sections, semantic theme tokens | Validated declarative data |

Core RLViz owns navigation, layout, accessibility, performance, source/raw
provenance, and the design system.

## Adapters

Adapters translate a source into canonical records. They may preserve
source-specific information in raw records and metadata, but core navigation
semantics require canonical fields. Adapters do not emit HTML, React components,
CSS, or executable renderer code.

Use `adapter-protocol.md` and `adapter-authoring.md` for the executable contract.

## Analyzers

Analyzers consume a bounded canonical trajectory and produce derived findings
and signals with provenance. Results are cached by analyzer identity and input
digest. Removing an analyzer or its cache never changes source or canonical
records.

Analyzers may identify loops, retries, suspicious state transitions, or
domain-specific verifier patterns. They do not control layout or run recorded
actions. Use `analyzer-protocol.md` for the executable contract.

## Declarative presentation

Safe customization can eventually include:

- field display names and descriptions
- scalar formatting and units
- signal grouping and table columns
- semantic badges and landmark categories
- artifact media-type associations
- inspector sections composed from known safe primitives
- command keymap overrides
- validated design-token theme overrides

Every declaration is schema-validated, has bounded size, and renders through
core components. Unknown declarations fail closed with a useful diagnostic.

## Explicit non-goal: arbitrary viewer code

RLViz does not load arbitrary plugin JavaScript or CSS into the main viewer.
Doing so would undermine local trace safety, visual consistency, accessibility,
performance budgets, and the claim that the base product is battle-tested.

If a future use case requires a fully custom visualization, evaluate a separate
sandboxed document/iframe protocol with an explicit trust boundary. Do not
silently expand adapters or analyzers into UI-code plugins.

## Discovery and trust

`rlviz formats` now provides bounded manifest inventory from explicit roots,
project `.rlviz/plugins`, and the user plugin directory. Its rank is a stable
inventory order only; it does not imply compatibility with a source. Discovery
does not probe, execute, select, or trust a plugin. Executable plugins still
require an explicit adapter path and an approved content digest. Automatic
source probing remains future work and must preserve those trust boundaries.
Presentation-only configuration may use a separate schema-validation path
because it cannot execute code.

## Presentation v1alpha1 contract

`schemas/v1alpha1/presentation-config.schema.json` defines the first bounded
contract. It supports display labels and descriptions for built-in group fields
and named signals, scalar format and unit hints, default group columns, a fixed
inspector section order, and a fixed allowlist of semantic color tokens. Files
are JSON-only and capped at 64 KiB; maps and lists have independent count
limits. Unknown keys fail closed.

Field identifiers are the built-in group column names (`reward`, `pass`,
`status`, `termination`, `events`, `errors`, `tokens`, and `latency`) or
`signal:<canonical signal name>`, where the signal name uses letters, digits,
periods, underscores, or hyphens. Scalar formats are core-owned primitives:
`number`, `integer`, `percent_fraction`, `duration_ms`, `bytes`, and
`scientific`. `percent_fraction` explicitly treats `1` as 100 percent. A unit is
plain suffix text, not a formatting template. Scalar formatting applies only to
numeric built-ins (`reward`, `events`, `errors`, `tokens`, and `latency`) and
canonical signals; nonnumeric fields fail validation.

`palette` is the current bounded color contract. It accepts partial `light` and
`dark` overrides for the ten tokens defined in `interaction-spec.md`, using
three- or six-digit opaque hex colors, and resolves them against the built-in
defaults. `palette.name: "high-contrast"` selects the shipped alternate palette
before applying explicit overrides. An invalid color falls back to the default
palette and produces a visible non-blocking viewer notice; unknown tokens and
palette names remain validation errors. Typography, spacing, selectors, CSS
functions, and URLs are not configurable.

The older `theme` map remains schema-bounded for v1alpha1 compatibility. New
configuration should use `palette` so browser and TUI semantics stay aligned.

`inspector.sections` is an exact ordered list of one to ten core-owned section
IDs: `properties`, `context`, `source`, `input`, `output`, `content`, `metadata`,
`linked_artifacts`, `analysis`, and `other_artifacts`. Omitted IDs are hidden and
sections without data remain absent. The selected-event heading and raw-record
mode are invariant. The declaration does not disable analysis, artifact
navigation, deep links, or other data behavior; it changes presentation only.

`keymap.bindings` maps stable core command IDs to one to four key strings. Only
`Mod`, `Ctrl`, `Meta`, `Alt`, and `Shift` modifiers are accepted. Unknown
commands, malformed or duplicate bindings, and same-scope conflicts fail
validation. Project bindings replace shipped defaults for listed commands;
browser-local user overrides remain higher priority and Reset returns to the
project defaults. Configuration cannot add commands or handlers.

Validate a file before using it:

```bash
rlviz presentation validate --json examples/presentation/research.json
```

The Go contract lives in `internal/presentation`. `rlviz open SOURCE
--presentation FILE` and foreground `rlviz serve SOURCE --presentation FILE`
load explicit files only. The CLI validates before any daemon request; the
daemon independently validates registration input and exposes only normalized
JSON to the viewer. There is deliberately no implicit project or user config
discovery yet. Opening a source without the flag clears its prior presentation.

The normalized config is stored separately from source identity and data
fingerprints, so it survives daemon restarts and source refreshes without
causing re-indexing. TypeScript mirrors the Go types;
`GroupView` maps configured columns onto its existing built-in/signal model and
uses the current user-saved column layout as a higher-priority preference.
The command registry applies configured keymaps below browser-local overrides.
Palette keys map to the fixed CSS custom properties only after validation. No
presentation file is discovered from executable plugin output.
