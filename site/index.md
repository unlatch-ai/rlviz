# RLViz

Visualize and compare agent rollouts locally.

RLViz opens canonical trajectories, rollout cohorts, and trusted adapter output in a keyboard-first browser UI or TUI. It reads sources without mutating them and makes no outbound requests during viewing.

## Install

```bash
brew install TheSnakeFang/tap/rlviz
```

Or download a release binary with the repository installer:

```bash
curl -fsSL https://raw.githubusercontent.com/TheSnakeFang/rlviz/main/scripts/install.sh | sh
```

Installation and setup documentation: [rlviz.dev/onboarding.html](https://rlviz.dev/onboarding.html).

## 30-second quickstart

```bash
rlviz init
rlviz demo
rlviz inspect ./path/to/rollout.ndjson
rlviz open ./path/to/rollout.ndjson
```

If `inspect` reports an unsupported format, scaffold a project-local adapter, review it, and explicitly trust it before execution. The [adapter authoring guide](https://rlviz.dev/adapter-authoring.html) covers that boundary.
