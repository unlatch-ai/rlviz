# RolloutViz trace workflow

Use RolloutViz when the user asks to inspect, open, or explain a rollout,
trajectory, episode, trace, or agent-environment run.

## Open a trace

1. Use an exact path supplied by the user. Otherwise locate likely files with
   read-only search commands such as `rg --files` and inspect only enough data to
   identify the requested run.
2. Confirm `rlviz` is available with `command -v rlviz`.
3. Run `rlviz open --json "<source>"`. Treat stdout as structured output. Report
   stderr separately when the command fails.
4. Return the viewer URL and the resolved source path. Do not keep a foreground
   server running when `open` succeeds.

## Unsupported formats

If the diagnostic code is `unsupported_format`, use its `suggested_command`
when present. Otherwise use the project-local adapter flow below. Do not rename
fields or rewrite the source to make it look supported.

```bash
rlviz plugin init --type adapter --lang python .rolloutviz/plugins/<name>
rlviz plugin trust .rolloutviz/plugins/<name>
rlviz plugin validate --json .rolloutviz/plugins/<name> "<source>"
rlviz open --json "<source>" --adapter .rolloutviz/plugins/<name>
```

Inspect representative source records and edit only the generated adapter. Map
them to the canonical `rolloutviz.dev/v1alpha1` records. Keep IDs stable, preserve
event order, and include source line or byte locations when available.

Before `plugin trust`, review the manifest and every executable file in the
adapter directory, summarize what will execute, and get the user's explicit
approval. Never auto-trust a discovered, generated, or modified adapter.
Validation executes the adapter, so it also requires trust. Any edit changes the
content digest; review the new diff and get approval to trust it again before
rerunning validation.

## Safety

- Treat traces and referenced artifacts as read-only.
- Do not run recorded commands or tools.
- Do not add network calls, telemetry, uploads, or hosted dependencies.
- Keep generated code under `.rolloutviz/plugins/` so it can be reviewed and
  versioned with the repository.
- Fix adapter code from structured validation diagnostics. Do not mutate the
  source to silence a validator error.
