# Research data model

## Current contract

Canonical v1alpha1 defines run, case, group, trajectory, event, signal,
artifact, and complete records. Events are deliberately generic: kind, ordered
sequence, parent/branch and alignment hints, input/output/data, source location,
raw record, and metadata.

That envelope supports lossless basic viewing and deterministic alignment, but
free-form metadata is not enough for consistent research UX across formats.

## Semantic vocabulary needed next

Before changing the protocol, gather representative traces from agent harnesses,
environment rollouts, post-training pipelines, and evaluation systems. The next
vocabulary should standardize only concepts supported by multiple real formats.

Candidate concepts:

- message role: system, developer, user, assistant, tool, environment
- prompt layer and stable message identity
- turn, span, tool call, and tool result linkage
- subagent, parent, branch, and delegation relationships
- context lifecycle: compaction, truncation, injection, restore
- retained, dropped, and summarized event/message references
- input, cached, reasoning, and output token accounting plus context capacity
- verifier/judge verdict, score, rubric, provenance, and evidence references
- final answer and termination provenance
- source-native versus adapter-derived versus analyzer-inferred status

The evidence gate and smallest candidate context contract are specified in
`context-semantics.md`. Do not implement a context-usage track from the
synthetic compaction fixture alone.

## Modeling rules

- Preserve the ordered raw event stream even when the UI groups turns or spans.
- Prefer stable references over nested copies of messages or events.
- Store source-native facts separately from adapter-derived and inferred facts.
- Never infer literal branches from independently sampled trajectories.
- Signals carry scalar or textual assessments; events carry ordered activity;
  artifacts carry addressable payloads; analyzer findings remain removable.
- A verifier result can reference supporting events without duplicating them.
- Context changes must express what the source actually knows. Do not fabricate
  before/after membership from token totals alone.
- Every normalized semantic value should retain raw/source provenance.

## Pair-comparison semantics

The indexed pair-comparison response adds conservative summaries under
`differences` without changing either canonical side:

- `success` uses the first boolean `pass` signal, then `success`; absent or
  non-boolean values remain absent.
- `token_count` uses the first non-negative integer `token_count`, then
  `total_tokens`, then `tokens`. `delta` is present only when both sides have a
  total, and is `right - left`.
- `context_event_count` counts events with an explicit `context:*`
  `alignment_key`; `compaction_count` counts the exact
  `context:compaction` key. Event kind or free-form data is never guessed.
- `verifier_results` contains canonical `grader` event output with `event_id`,
  `sequence`, and optional `alignment_key`. The output remains source-shaped;
  RLViz does not reinterpret a domain grader as a common verdict. Its `changed`
  flag compares alignment keys and outputs, not side-specific event IDs or
  sequence numbers.

These fields are additive to the original status, termination, reward, event
count, and alignment response. The referenced canonical events and existing
event provenance remain the source of truth.

## Protocol evolution

The protocol remains pre-stable. A semantic revision requires:

1. examples from multiple real source formats
2. a written mapping and provenance model
3. schema and Go/TypeScript type changes
4. canonical, malformed, and compatibility fixtures
5. decoder, validator, index, API, and UI support
6. adapter and analyzer protocol guidance
7. golden rendering and conformance tests
8. an explicit migration note

Do not add UI-only interpretations of arbitrary metadata keys. If a concept is
important enough for core navigation, it needs a canonical contract or a
validated declarative presentation hint.
