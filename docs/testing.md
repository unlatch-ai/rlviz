# Testing and quality gates

## Purpose

Tests should make RLViz easier to change without turning the repository into a
heavy framework project. Favor a small number of representative end-to-end
flows, strong protocol fixtures, and focused component/unit tests.

## Existing baseline

`make check` runs `make lint`, Go tests, React/Vitest tests,
npm installer tests, the production web build, the curl installer, and Homebrew
formula rendering. The embedded production bundle uses the cross-platform
Terser minifier so committed assets are byte-reproducible on macOS and Linux.
CI also runs the Go race detector, release-target builds, and a Playwright
Chromium navigation flow at 1440 x 900.

`make lint` runs the repository's `.golangci.yml` suite (`govet`,
`staticcheck`, `errcheck`, `ineffassign`, `misspell`, and `gofmt`) when
`golangci-lint` is installed. On a machine without that binary it falls back to
`go vet ./...` plus a failing `gofmt -l` check, so a missing optional local tool
does not make `make check` unusable. CI installs and runs the full suite.

Useful focused targets are:

```bash
make lint       # Go static analysis and formatting
make test       # Go, Vitest, npm installer, and shell installer tests
make web-e2e    # deterministic Playwright flows against the built viewer
make check      # full local release gate except Playwright
```

The web build begins with strict `tsc -b`. ESLint is not currently installed;
add it in a follow-up dependency review rather than introducing an unreviewed
network install solely for this release pass.

## Test layers

### Contract and fixture tests

Every canonical or plugin protocol feature has valid, malformed, and adversarial
fixtures. Decoder and schema tests must agree. Semantic additions include source
provenance and compatibility coverage.

Required representative fixtures:

- rich single rollout with messages, tools, artifacts, reward, and verifier
- long rollout with multiple compactions and context accounting
- nested/subagent and branch relationships
- group with pass, policy failure, and infrastructure failure cohorts
- pair with textual difference, behavioral divergence, and later realignment
- partial/growing source and failed refresh
- generated 10,000+ event performance source

Keep committed fixtures small, synthetic, deterministic, and non-sensitive.
Generate large fixtures during tests when practical.

Generated adapters include `test_adapter.py` and `testdata/cases.json`. The
runner must preserve listed case order, pass argv without a shell, reject paths
outside `testdata`, and require the Go validator to report the expected format,
deterministic output, and minimum record count. Scaffold tests cover zero cases,
traversal and symlink escape, malformed validator output, format mismatch, and
record-count failure. These tests do not weaken the review-and-trust boundary:
running the fixture harness executes adapter code.

### Go subsystem tests

Keep table-driven tests at model, plugin, index, daemon, server, alignment, and
watch boundaries. Race-test daemon, indexing, refresh, and shared-job behavior.

### React unit and interaction tests

Use Vitest and Testing Library for component state, keyboard commands, focus,
filtering, deep-link restoration, partial data, and error behavior. Test the
central command registry independently for defaults, scope, overrides,
conflicts, and reset.

Whole-trajectory overview tests assert deterministic aggregation, partial loaded
extent, viewport and selection positions, filtered nearest-event navigation, and
a constant 192-cell ceiling for 10,000-event runs. Virtual-list tests keep the
reported viewport independent from overscan and forced selected-row rendering.
Landmark-rail tests assert stable reason precedence, no premature endpoint for
partial indexes, bounded rendering on 10,000 events, raw-event keyboard
navigation, and pinned selection when filters exclude the current event.
Presentation tests keep JSON Schema, Go validation, and TypeScript mirrors
aligned. Inspector tests assert exact declarative ordering, omission, and the
invariant raw-record escape hatch. Keymap tests cover project-default versus
browser-local precedence, modifier syntax, command allowlists, and same-scope
conflict rejection.

### Real-browser end-to-end tests

The first Chromium flow covers deterministic repeated keyboard navigation,
text-entry suppression, manual landmark-rail scroll retention, one-time
deep-link reveal, and Help focus return. Grow the suite toward five flows:

1. Rich demo opens; transcript and keyboard navigation work.
2. Long trajectory searches, filters, selects landmarks, and restores a deep link.
3. Group sorting, cohort selection, best/worst, and compare work.
4. Pair comparison reaches divergence, next change, and restores its URL.
5. The packaged binary starts a daemon, serves an authenticated fixture, loads
   the browser UI, and stops cleanly.

CLI unsupported-format and adapter diagnostics remain Go/process integration
tests unless a browser surface is involved.

### Visual and accessibility checks

Capture deterministic screenshots for trajectory, group, and comparison at one
fixed desktop viewport and font environment. Check comfortable and compact
density only after both exist. Keep screenshot scope broad enough to catch
hierarchy regressions but small enough to review intentionally.

Run automated accessibility checks on the same three surfaces. Also test focus
return, keyboard-only completion, semantic landmarks, and non-color status cues.

## Quality budgets

Initial budgets should be measured on a documented reference machine and then
kept stable:

- warm small-source first meaningful paint: about one second
- `rlviz open` returns after registration rather than full indexing
- smooth navigation and bounded DOM size at 10,000 events
- search and next-landmark commands respond within one animation frame after
  indexes are ready
- progressive indexing does not block already committed pages
- normal viewing produces zero outbound network requests

Avoid exact cross-machine millisecond gates until the benchmark harness can
control noise. Use relative regression thresholds and structural assertions
first.

## Change matrix

| Change | Minimum verification |
| --- | --- |
| Go-only internal behavior | focused Go tests, `make check` |
| Canonical/protocol | schemas, fixtures, Go and TypeScript types, conformance tests, docs, `make check` |
| Visible UI | focused component tests, real-browser flow, fixed-viewport visual review, `make check` |
| Keymap or focus | command registry tests, keyboard E2E, accessibility check |
| Installer/release | relevant installer tests, snapshot/release check |
| Performance path | representative benchmark plus full check |

## Dependency policy

Add tools only when they enforce a durable quality boundary. A browser runner
and an accessibility scanner are justified. Do not add Storybook or a large UI
framework solely to host components; a small fixture/dev route can exercise the
real embedded application with less duplication.
