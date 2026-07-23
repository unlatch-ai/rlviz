# Welcome to RLViz

RLViz is an open-source viewer for agent rollouts: model output, tool calls, observations, graders, rewards, errors, and source provenance in one local workspace.

## Please read

The hosted browser directly supports RLViz canonical NDJSON, Inspect AI EvalLog JSON, and Verifiers GenerateOutputs JSON. A selected directory must contain exactly one supported `.ndjson` or `.json` trace export.

We highly recommend installing the local CLI. Coding agents can operate its workspaces and build adapters or plugins for private formats, larger cohorts, and multi-file datasets. Browser adapters run as reviewed WASM modules; local adapters can use the source system's native files and libraries.

## Install the local CLI

Homebrew:

```sh
brew install TheSnakeFang/tap/rlviz
```

npm:

```sh
npm install --global rlviz
```

Verified shell installer:

```sh
curl -fsSL https://rlviz.dev/install.sh | sh
```

Then configure RLViz and open a trace:

```sh
rlviz init
rlviz open ./path/to/trace.ndjson
```

The CLI adds persistent indexing, larger cohorts, private adapters, structured queries for coding agents, and named workspaces that an agent can update after the GUI opens.

## What it does

- Visualizes, navigates, filters, groups, and compares recorded trajectories.
- Keeps source traces read-only. Browser parsing stays in the tab; CLI viewing stays on loopback.
- Does not run agents, replay recorded tools, train models, manage prompts, or provide hosted monitoring.
