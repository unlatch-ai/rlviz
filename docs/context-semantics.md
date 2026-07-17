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

RLViz now accepts an optional structured `context` observation on canonical
events. The reference adapters map only facts supported by their source
contracts. Legacy `context:*` alignment keys remain navigation and comparison
fallbacks when an event does not contain structured context. RLViz does not
infer context membership, interpolate window usage, or treat cumulative
billing tokens as prompt occupancy.

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

## Canonical context contract

The optional `context` object on an ordered canonical event supports:

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

The protocol, reference adapters, sparse index, comparison fallback, and viewer
surface are implemented. The context rail positions discrete observations by
event order, labels gaps as unobserved, and never draws an interpolated usage
line. Exact lifecycle facts, provenance, derivation, summaries, and explicit
membership references remain available in the selected-event Inspector.

Long-run and pagination coverage should continue expanding as representative
real traces become available. Any denser visualization still requires evidence
that its encoded quantity is source-native or deterministically derived.

SQLite schema version 5 stores the raw event unchanged and separately indexes
whether structured context exists plus its operation, token counts, capacity,
and provenance. `GET /api/v1/indexed/events?...&context=true` returns structured
context observations plus legacy `context:*` landmarks; `context=false` returns
the complement. The parameter accepts only lowercase `true` or `false`.

The canonical stream remains pre-stable. New RLViz readers accept streams that
omit `context`, and database storage retains the raw event envelope. Older
strict readers reject the new field, so streams containing structured context
require a RLViz release that includes this contract. This additive change does
not imply a stable v1 protocol guarantee.
