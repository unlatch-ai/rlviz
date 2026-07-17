# Prime Intellect Verifiers adapter

This dependency-free example maps the JSON-compatible `GenerateOutputs`
contract from Prime Intellect's Verifiers library into RLViz. It preserves each
trajectory step's full prompt, completion, token masks, reward, advantage, and
truncation flags. It does not treat cumulative usage or output truncation as
context-window occupancy.

The bundled fixture is synthetic data shaped to the public Verifiers contract;
it is not copied from a user run.

```bash
rlviz plugin trust ./examples/adapters/verifiers
rlviz plugin validate ./examples/adapters/verifiers ./examples/traces/verifiers-generate.json
rlviz open ./examples/traces/verifiers-generate.json --adapter ./examples/adapters/verifiers
```

Contract reference: https://docs.primeintellect.ai/verifiers/reference
