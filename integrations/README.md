# Coding-agent integrations

RolloutViz is designed to be operated and extended by coding agents. These files
are project-instruction snippets, not runtime plugins. Copy the relevant file
into a repository where an agent needs to inspect rollout data:

| Agent | Source | Project destination |
| --- | --- | --- |
| Codex | `codex/AGENTS.md` | `AGENTS.md`, or merge into an existing file |
| Claude Code | `claude-code/CLAUDE.md` | `CLAUDE.md`, or merge into an existing file |
| Cursor | `cursor/rolloutviz.mdc` | `.cursor/rules/rolloutviz.mdc` |

Each integration gives the agent the same workflow:

1. Locate the source without changing it.
2. Try `rlviz open --json`.
3. If the format is unsupported, scaffold a project-local Python adapter.
4. Review the adapter and get explicit approval before trusting it.
5. Validate the trusted adapter, then open the source with its explicit path.

The command surface is documented in
[`docs/adapter-authoring.md`](../docs/adapter-authoring.md).
