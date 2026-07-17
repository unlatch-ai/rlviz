# Context semantics

## Current evidence

RLViz currently has three synthetic compaction events in the rich demo. They
use `alignment_key: context:compaction` with free-form before/after token values
and a summary. The bundled example adapter does not map context lifecycle or
context-window usage from a real source format.

The viewer therefore treats `context:*` alignment keys conservatively as
source-provided landmarks. It can jump to them, render their raw payload, and
count them in comparisons. It does not infer context membership, interpolate
window usage, or treat cumulative billing tokens as prompt occupancy.

## Evidence gate

A canonical context contract requires representative mappings from at least
two independently designed real formats. Each mapping must distinguish:

- facts recorded directly by the source
- deterministic facts derived by the adapter
- values unavailable from that format
- cumulative token accounting versus tokens present in a model input
- explicit retained, dropped, or summarized membership versus inference

Synthetic fixtures exercise rendering and validation but do not satisfy this
gate. Until the gate is met, context-window charts and membership claims remain
out of the core UI.

## Smallest candidate contract

The smallest useful extension is an optional `context` object on an ordered
canonical event. It should support:

- an optional lifecycle operation: `compaction`, `truncation`, `injection`, or
  `restore`
- non-negative input tokens immediately after the observation or change
- an optional immediately-before input-token value for lifecycle changes
- an optional positive context capacity
- explicit retained, dropped, and summarized references to earlier events in
  the same trajectory
- a source-provided or adapter-generated summary
- required `source_native` or `adapter_derived` provenance, with a derivation
  description for adapter-derived values

An object without an operation is a usage observation, not an implied context
change. Membership arrays are valid only when the source identifies membership;
token differences never imply which messages were lost. Output, reasoning,
cached, and cumulative token totals must not be mapped into input occupancy.

## Compatibility and implementation sequence

Once the evidence gate is met:

1. Write the two format mappings and provenance table.
2. Add the Go, JSON Schema, and TypeScript contract plus malformed fixtures.
3. Validate reference ordering, trajectory ownership, uniqueness, and overlap.
4. Index structured context observations for bounded sparse queries.
5. Return structured events through the existing trajectory API.
6. Prefer structured context in comparison while retaining legacy
   `context:*` landmarks as a compatibility fallback.
7. Add a context surface that shows observations and unknown gaps without
   interpolation.
8. Add long-run, pagination, keyboard, provenance, and source-link tests.

The canonical stream is currently pre-stable and strict readers reject unknown
fields. Adding `context` therefore requires an explicit compatibility note or a
new negotiated protocol version before the contract is described as stable.
