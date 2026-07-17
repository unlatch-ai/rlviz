# Expert onboarding

## Goal

A research engineer should understand RLViz and open a real rollout without a
consumer-style wizard. The happy path is operational and agent-friendly:

```text
install -> demo -> open real source -> inspect or adapt -> view
```

Every human-readable command result should have a stable `--json` equivalent so
Codex, Claude Code, Cursor, and other coding agents can operate the same flow.

## Command journey

### 1. Verify the installation

```bash
rlviz doctor
rlviz version
```

`doctor` should report the binary version, platform, cache/runtime locations,
daemon state, browser-open capability, discovered plugins, and actionable
problems without reading rollout contents.

### 2. Learn on a rich synthetic rollout

```bash
rlviz demo
```

The bundled demo should be a realistic rollout group, not a two-event toy. It
should include:

- system and user messages
- model generations
- successful and failed tool calls
- observations and artifacts
- a context compaction
- reward components
- verifier evidence and final output
- pass, policy failure, and infrastructure failure trajectories
- a meaningful behavioral divergence

The viewer clearly labels demo data synthetic. Demo mode is explicit and never
silently replaces a failed API connection.

### 3. See available format support

```bash
rlviz formats
rlviz formats --json
```

The output distinguishes built-in canonical formats, example adapters,
discovered project adapters, trusted user adapters, and unavailable/untrusted
adapters. See `supported-formats.md`.

### 4. Probe a real source without opening a browser

```bash
rlviz inspect ./path/to/rollout
rlviz inspect --json ./path/to/rollout
```

`inspect` performs bounded, read-only probing and reports:

- resolved source path and shape
- selected adapter and confidence
- detected format and capabilities
- warnings or unsupported-format diagnosis
- exact next command

It never starts the viewer or mutates the source.

`inspect` is the next onboarding command to implement. Until then, use
`rlviz open --json SOURCE`; unsupported inputs already return a structured
diagnostic without mutating the source.

### 5. Open it

```bash
rlviz open ./path/to/rollout
```

Recognized sources open normally. Unsupported sources return the existing
structured diagnostic and an adapter scaffold command.

### 6. Adapt a private format

The target convenience flow is:

```bash
rlviz plugin init --type adapter --from ./path/to/rollout .rlviz/plugins/my-format
```

An agent inspects a bounded representative sample and edits the generated
adapter. The user reviews the executable files before trust:

```bash
rlviz plugin trust .rlviz/plugins/my-format
rlviz plugin validate --json .rlviz/plugins/my-format ./path/to/rollout
rlviz open ./path/to/rollout --adapter .rlviz/plugins/my-format
```

Trust remains path-and-digest bound. Onboarding must not weaken this security
step or imply that generated code is automatically safe.

## Coding-agent setup

The repository already ships equivalent instructions for Codex, Claude Code,
and Cursor. The target CLI makes them discoverable without overwriting existing
project rules:

```bash
rlviz setup agent codex --print
rlviz setup agent claude-code --print
rlviz setup agent cursor --print
```

A future `--write` mode may create a dedicated include or rule file. It must:

- refuse to replace an existing instruction file
- show the destination and exact content before writing unless explicitly
  requested non-interactively
- keep instructions small and link to canonical local docs
- preserve the adapter review and trust confirmation boundary

Package-manager postinstall scripts should not modify project instructions.

## Browser welcome surface

`rlviz demo` or an explicit future `rlviz welcome` may open a welcome surface
with three concise actions:

1. Open the demo
2. Open a source from the CLI
3. Add support for a private format

Normal `rlviz open` goes directly to the requested data and never interrupts an
expert with onboarding state.

## Acceptance criteria

- A clean install reaches the rich demo in under one minute.
- A supported real source opens with one command.
- Unsupported-format JSON gives a coding agent everything needed for the next
  safe step.
- The user can state exactly which formats are built in versus plugin-provided.
- Agent setup never overwrites existing project instructions.
- No onboarding step uploads a trace, contacts a service, or changes the source.
