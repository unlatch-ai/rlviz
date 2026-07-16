# Simple JSONL adapter

This dependency-free example translates `examples/traces/simple-agent.jsonl`
into RolloutViz's canonical stream. It demonstrates deterministic IDs, source
locations, format probing, tool alignment keys, and protocol-only stdout.

```bash
rlviz plugin trust ./examples/adapters/simple-jsonl
rlviz plugin validate ./examples/adapters/simple-jsonl ./examples/traces/simple-agent.jsonl
rlviz open ./examples/traces/simple-agent.jsonl --adapter ./examples/adapters/simple-jsonl
```
