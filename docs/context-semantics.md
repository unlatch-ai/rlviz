# Context semantics

## Current evidence

RLViz has three synthetic compaction events in the rich demo and two
dependency-free example adapters shaped to independently designed public
contracts:

- [Inspect AI EvalLog JSON](https://inspect.aisi.org.uk/eval-logs.html), whose
  transcript includes explicit model, tool, score, and compaction events.
- [Prime Intellect Verifiers GenerateOutputs](https://docs.primeintellect.ai/verifiers/reference),
  whose rollout steps include full prompts, completions, token masks, rewards,
  advantages, and truncation flags.

The redistributable fixtures contain synthetic values, but the field mappings
come from the public source contracts. The adapters remain examples rather than
built-in support.

The viewer still treats `context:*` alignment keys conservatively as
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

The two mappings below satisfy the format-diversity gate for designing the
smallest candidate contract. They do not by themselves authorize inferred
membership or interpolated context charts.

## Format mappings

| Candidate fact | Inspect AI | Verifiers |
| --- | --- | --- |
| Ordered model input | `ModelEvent.input`, source native | `TrajectoryStep.prompt`, source native |
| Model output | `ModelEvent.output`, source native | `TrajectoryStep.completion`, source native |
| Lifecycle operation | `CompactionEvent.type`: `summary`/`edit` map to `compaction`; `trim` maps to `truncation` | Unavailable. `is_truncated` is a generation/sequence-limit fact, not proof of input-context truncation |
| Input tokens before/after | `tokens_before` and `tokens_after`, source native when present | Per-step non-padding prompt-token count can be adapter-derived from `prompt_mask` |
| Capacity | Unavailable | Unavailable |
| Retained/dropped/summarized membership | Unavailable. A compaction event does not identify message membership | Unavailable. Comparing repeated prompts would be inference, not source-native membership |
| Summary text | Unavailable on `CompactionEvent` | Unavailable |
| Cumulative usage | Logged separately and must not be treated as occupancy | `input_tokens` counts shared context repeatedly; `final_input_tokens` assumes a linear trajectory and is not valid for rewritten histories |

Inspect compaction changes model input while retaining the full underlying
history for audit. Verifiers token IDs and masks are training data: they support
an exact per-step count but do not say which earlier semantic messages were
retained or removed. Both adapters preserve the source-shaped values so a later
canonical migration remains auditable.

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
