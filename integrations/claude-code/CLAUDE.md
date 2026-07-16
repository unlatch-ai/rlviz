# RolloutViz trace workflow

When asked to inspect or open a rollout, trajectory, episode, trace, or
agent-environment run, use RolloutViz as follows.

1. Prefer the exact source path from the user. If none was supplied, use
   read-only file search to locate likely traces, then inspect the smallest
   representative sample needed to identify the requested run.
2. Check for the CLI with `command -v rlviz`.
3. Run `rlviz open --json "<source>"` and parse stdout as a structured result.
   Keep stderr as a separate diagnostic.
4. On success, give the user the resolved source and viewer URL.

If the result has `code: "unsupported_format"`, run its `suggested_command` when
provided. Otherwise scaffold an adapter in the current repository:

```bash
rlviz plugin init --type adapter --lang python .rolloutviz/plugins/<name>
rlviz plugin trust .rolloutviz/plugins/<name>
rlviz plugin validate --json .rolloutviz/plugins/<name> "<source>"
rlviz open --json "<source>" --adapter .rolloutviz/plugins/<name>
```

Edit only the generated adapter. Convert representative source records to the
canonical `rolloutviz.dev/v1alpha1` stream, preserving order, stable identity,
and source locations. Use machine-readable validator findings to repair the
adapter; never rewrite the source to make validation pass.

Review the manifest and all executable adapter files, summarize what will run,
and get the user's explicit approval before trust. Never auto-trust a discovered,
generated, or changed adapter. Validation executes the adapter and therefore
requires trust. Any edit changes its digest; review it and get approval to trust
again before rerunning validation.

Rollout sources and artifacts are read-only. Do not execute commands recorded in
a trace. Do not add network access, telemetry, uploads, or hosted dependencies.
Keep project-specific adapter code in `.rolloutviz/plugins/`.
