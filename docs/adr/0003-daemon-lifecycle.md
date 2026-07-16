# ADR 0003: Per-user loopback daemon

- Status: Accepted
- Date: 2026-07-16

## Context

Coding agents need `rlviz open` to return promptly, while researchers need repeated opens to reuse a stable local viewer.

## Decision

The finished local workflow uses one per-user daemon bound to `127.0.0.1`. `rlviz open` starts it when absent, registers the requested source through an authenticated mutation endpoint, opens or focuses a browser URL, prints a machine-readable result when requested, and exits.

Daemon metadata and its secret are stored in a user-only runtime directory. The HTTP listener uses loopback only by default. Development retains `rlviz serve PATH` as an explicit foreground mode.

When a newly installed CLI finds a live daemon with another version, it authenticates a clean shutdown, waits for the old metadata to disappear, and starts the new daemon automatically.

Milestone 1 may ship the foreground server before detachment is implemented, but the CLI output and API boundaries must not imply that foreground behavior is permanent.

## Consequences

- Agent shell calls do not remain occupied after daemon support lands.
- Multiple repositories can share one local viewer without sharing source ownership or trust decisions.
- Source registration and trajectory reads require the daemon secret; ordinary local pages and processes cannot register or read traces.
- Lifecycle commands include `status`, `stop`, and `doctor`, with stale metadata recovery.
