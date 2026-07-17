# Inspect AI EvalLog JSON adapter

This dependency-free example translates an Inspect AI EvalLog JSON document
into RLViz's canonical stream. It supports JSON logs produced directly by
Inspect or by `inspect log dump`. It does not decode Inspect's compressed
`.eval` archive format.

The adapter preserves model, tool, compaction, and score events with their raw
source records. Inspect summary and edit events become explicit
`context:compaction` landmarks; trim events become `context:truncation`
landmarks. Their source-provided type, role, token counts, and source are
retained; the adapter does not infer retained messages, dropped messages, or a
generated summary.

```bash
rlviz plugin trust ./examples/adapters/inspect-ai
rlviz plugin validate ./examples/adapters/inspect-ai ./examples/traces/inspect-ai-eval.json
rlviz open ./examples/traces/inspect-ai-eval.json --adapter ./examples/adapters/inspect-ai
```

Inspect may condense repeated event content or replace it with attachment
references. This adapter preserves those records but does not resolve them.
Export resolved JSON first when the full content is required.

Format references:

- <https://inspect.aisi.org.uk/eval-logs.html>
- <https://inspect.aisi.org.uk/reference/inspect_ai.log.html>
- <https://inspect.aisi.org.uk/reference/inspect_ai.event.html>
