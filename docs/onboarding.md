# Expert onboarding

## Goal

A research engineer can open a real rollout directly, or run the optional
terminal-only first-run wizard. The happy path remains operational and
agent-friendly:

```text
install -> optional init -> demo -> open real source -> inspect or adapt -> view
```

Every human-readable command result should have a stable `--json` equivalent so
Codex, Claude Code, Cursor, and other coding agents can operate the same flow.

## Command journey

### 0. Optional first-run setup

```bash
rlviz init
```

The stdin/stdout wizard works over SSH. It asks for the default `rlviz open`
interface (`browser`, `tui`, or `both`), then asks which Codex, Claude Code, or
Cursor integrations to preview. Every selected file is printed in full and has
its own write confirmation. Finally, the wizard offers to open the synthetic
gallery. Codex and Claude Code use `~/.codex/skills/rlviz/SKILL.md` and
`~/.claude/skills/rlviz/SKILL.md`; Cursor uses the current project's
`.cursor/rules/rlviz.mdc`. Existing files are never replaced.

`rlviz init --yes` and non-TTY stdin select the browser default, skip optional
agent writes and gallery launch, print the agent prompt, and exit without
waiting for input. A normal `rlviz open` with no config prints one hint and
continues immediately.

### 1. Verify the installation

```bash
rlviz doctor
rlviz version
```

`doctor` reports a versioned local-readiness snapshot: binary and platform,
cache/runtime/index locations, live/stopped/degraded daemon state,
browser-launch and Python availability, trusted plugin paths, and actionable
problems. It does not start the daemon, open a browser, execute plugins, or read
rollout contents. Use `rlviz doctor --json` for stable agent-readable output.

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
With `--adapter PATH`, it executes only the explicit trusted adapter's bounded
`probe` operation; it never invokes `stream`.

### 5. Open it

```bash
rlviz open ./path/to/rollout
```

Recognized sources open normally. Unsupported sources return the existing
structured diagnostic and an adapter scaffold command.

If a team wants stable labels, scalar formatting, cohort columns, inspector
section order, or semantic theme tokens, pass an explicit declarative
configuration:

```bash
rlviz presentation validate ./presentation.json
rlviz open ./path/to/rollout --presentation ./presentation.json
```

The file is strict, bounded JSON rather than executable UI code. It is
validated before daemon startup and again at registration. The normalized
configuration survives source refreshes and daemon restarts. Reopening that
source without `--presentation` intentionally clears the prior configuration.
Portable keymap entries address stable core command IDs; browser-local shortcut
edits take precedence over those project defaults.

### 6. Adapt a private format

The target convenience flow is:

```bash
rlviz plugin init --type adapter --from ./path/to/rollout .rlviz/plugins/my-format
```

The command takes a structural sample of at most 256 KiB from regular files and
returns a value-free profile: container kind, sampling limits, truncation state,
and observed field paths with JSON types. It never returns scalar values or
copies sample records into the plugin. An agent uses that bounded evidence before
inspecting only the representative records needed to edit the adapter. With
`--json`, the command also returns generated files, `review_required: true`,
and exact next commands. The user reviews the executable files before trust:

```bash
rlviz plugin trust --json .rlviz/plugins/my-format
python3 .rlviz/plugins/my-format/test_adapter.py
rlviz plugin validate --json .rlviz/plugins/my-format ./path/to/rollout
rlviz open --json ./path/to/rollout --adapter .rlviz/plugins/my-format
```

Trust remains path-and-digest bound. Onboarding must not weaken this security
step or imply that generated code is automatically safe.

## Coding-agent setup

The binary ships equivalent version-matched instructions for Codex, Claude
Code, and Cursor. Print them without overwriting existing project rules:

```bash
rlviz setup agent codex --print
rlviz setup agent claude-code --print
rlviz setup agent cursor --print
```

Add `--json` for a stable envelope containing the agent, bundled source,
suggested project destination, and instruction content. The command is
read-only when `--print` is used; it does not create or modify project files.

To inspect a specific installation without changing the repository, provide a
project-relative destination explicitly:

```bash
rlviz setup agent codex --dry-run --destination .agents/rlviz.md
rlviz setup agent codex --dry-run --destination .agents/rlviz.md --json
```

Dry-run validates the destination and prints the exact bundled content. Its
JSON result uses schema version `1`, mode `dry_run`, status `would_create`, and
write policy `create_only`, with a SHA-256 digest of the content.

Writing requires both `--write` and `--destination`:

```bash
rlviz setup agent codex --write --destination .agents/rlviz.md
```

Agent setup writes are deliberately create-only. The destination must remain
inside the current project, absolute paths and parent traversal are rejected,
and symbolic-link path components are refused. Missing parent directories may
be created. The final file uses exclusive-create semantics, so an existing
file or a concurrent creator wins and RLViz exits
without replacing any content. There is no overwrite, append, or managed-block
mode: merging general-purpose `AGENTS.md` or `CLAUDE.md` files safely requires
project-specific judgment and stays with the user or coding agent.

Use the suggested destination only as guidance; RLViz never selects it
implicitly. `--print`, `--dry-run`, and `--write` are mutually exclusive.
Successful JSON output always names the mode, status, destination when
applicable, write policy, bundled source, exact content, and content digest.
Failures use the existing stable `setup_agent_failed` diagnostic code,
including invalid or conflicting mode flags when `--json` is requested.

These instructions remain small, link to canonical local docs, and preserve
the adapter review and trust confirmation boundary.

Package-manager postinstall scripts should not modify project instructions.

## Gallery

`rlviz demo` registers three generated, clearly synthetic canonical sources:
a 300-event coding bugfix, a 120-event web research run, and a 16-rollout
checkout cohort. The browser opens in Browse so the sources and cohort can be
read and compared without falling back to embedded sample data.

Normal `rlviz open` still goes directly to requested data and never blocks on
onboarding state.

## Acceptance criteria

- A clean install reaches the rich demo in under one minute.
- A supported real source opens with one command.
- Unsupported-format JSON gives a coding agent everything needed for the next
  safe step.
- The user can state exactly which formats are built in versus plugin-provided.
- Agent setup never overwrites existing project instructions.
- No onboarding step uploads a trace, contacts a service, or changes the source.
