# Coding-agent integrations

RLViz is designed to be operated and extended by coding agents. These files
are project-instruction snippets, not runtime plugins. Print the relevant
version-matched file from the installed binary:

```bash
rlviz setup agent codex --print
rlviz setup agent claude-code --print
rlviz setup agent cursor --print
```

To install one bundle, first dry-run and then explicitly create a
project-relative destination:

```bash
rlviz setup agent codex --dry-run --destination .agents/rlviz.md
rlviz setup agent codex --write --destination .agents/rlviz.md
```

Writes are create-only. RLViz never overwrites, appends to, or implicitly
chooses a project instruction file.

The `--print` and `--dry-run` modes never write project files. Review the
output before creating a dedicated file or merging it into an existing one:

| Agent | Source | Project destination |
| --- | --- | --- |
| Codex | `codex/AGENTS.md` | `AGENTS.md`, or merge into an existing file |
| Claude Code | `claude-code/CLAUDE.md` | `CLAUDE.md`, or merge into an existing file |
| Cursor | `cursor/rlviz.mdc` | `.cursor/rules/rlviz.mdc` |

Each integration gives the agent the same workflow:

1. Locate the source without changing it.
2. Try `rlviz open --json`.
3. If the format is unsupported, scaffold a project-local Python adapter.
4. Review the adapter and get explicit approval before trusting it.
5. Validate the trusted adapter, then open the source with its explicit path.

The command surface is documented in
[`docs/adapter-authoring.md`](../docs/adapter-authoring.md).
