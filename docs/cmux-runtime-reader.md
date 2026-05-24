# CMUX Runtime Reader

This document describes the architecture and design decisions for the first
CMUX runtime reader implementation: `internal/cmux`.

See [`docs/cmux-operator.md`](cmux-operator.md) for the full CMUX design spec
and ownership boundaries. See [`docs/cmux-roadmap.md`](cmux-roadmap.md) for the
phased rollout.

## What This Is

`internal/cmux` is the read-only Phase 1 CMUX operator layer. It:

1. Fetches two Brevity runtime contracts.
2. Parses them into a `Snapshot`.
3. Renders a compact plain-text dashboard.

Nothing else. No mutations, no watch mode, no keyboard handling, no ANSI,
no TUI framework, no direct `.brevity` file access.

## Architecture Boundaries

```
cmd/brevity cmux
    │
    ▼
cmux.Read(cmux.NativeFetcher{})       ← contract fetch + parse
    │
    ├─ NativeFetcher.RuntimeStateJSON()
    │     └─ runtimeclient.NativeClient.RuntimeStateJSON()
    │           └─ [brevity.runtime-state.v1]
    │
    └─ NativeFetcher.SchedulerPlanJSON()
          └─ runtimescheduler.Planner{Queue: store}.Plan()
                └─ [brevity.runtime-scheduler-plan.v1]

cmux.Render(w, snap)                  ← deterministic plain-text output
```

### Three distinct units

| Unit | File | Responsibility |
| --- | --- | --- |
| `Fetcher` interface + `NativeFetcher` | `reader.go` | Contract execution — maps to `brevity runtime state --json` and `brevity scheduler plan --json` |
| `Snapshot` struct | `model.go` | Parsed view of both contracts; carries error state without panicking |
| `Render` function | `render.go`, `render_markdown.go`, `render_json.go` | Deterministic rendering from a `Snapshot`; dispatches by `OutputMode`; no side effects |

Each unit has a single job. Parsing is separated from rendering. Command
execution is separated from parsing. This makes every unit independently
testable with stub data.

## Why CMUX Consumes Contracts Instead of Files

CMUX must not read `.brevity` files directly. The Brevity runtime contracts
(`brevity.runtime-state.v1`, `brevity.runtime-scheduler-plan.v1`) are the
established machine-readable boundaries for operator tooling. They are:

- **Versioned**: schema fields are additive and forward-compatible.
- **Tolerant**: consumers can tolerate unknown fields.
- **Tested**: the contracts are exercised by existing runtime and TUI tests.
- **Owned**: the Brevity CLI owns the state; consumers do not.

If CMUX bypassed the contracts and read `.brevity\tasks.json` directly, it
would couple itself to the raw file format, skip the normalization logic
(task state derivation, run history attachment, cleanup candidate detection),
and risk diverging from what the CLI surfaces. The contract layer exists
precisely to prevent this.

The `Fetcher` interface makes the boundary explicit: CMUX never calls `os.Open`,
`state.LoadTasks`, or any other direct file read. Tests replace `NativeFetcher`
with a `stubFetcher` that injects raw JSON bytes, which keeps tests deterministic
and environment-independent.

## Why the First Implementation Is Intentionally Minimal

The CMUX roadmap defines six phases. Phase 1 is strictly read-only:

- No mutation keys.
- No watch mode.
- No keyboard handling.
- No TUI framework.
- No ANSI escape sequences.
- No worker log streaming.
- No vault reads.

This is deliberate. Adding interaction before the rendering and contract layers
are stable risks:

- **Framework lock-in** before the interaction model is validated.
- **Hidden mutations** from premature action dispatch.
- **Divergence** between CMUX state and Brevity ground truth if refresh
  behavior is wrong.
- **Test surface explosion** if rendering, keyboard, and mutation code mix early.

The minimal implementation ships something useful immediately (a readable
one-shot dashboard at `go run ./cmd/brevity cmux`) while keeping the surface
small enough to validate before layering interaction.

## Snapshot Error Semantics

`Read` captures fetch and parse errors inside the `Snapshot` rather than
returning them as function errors. This is intentional:

- If `RuntimeStateJSON()` fails but `SchedulerPlanJSON()` succeeds, the
  renderer can still show the queue and scheduler summary — a partial view is
  more useful than a hard crash.
- The renderer surfaces the error text inline so the operator knows what failed
  without losing the rest of the dashboard.
- Tests can assert on partial snapshots easily.

## Future Extension Points

The minimal implementation is designed to accommodate future phases without
structural rewrites:

**Phase 2 (detail views):** Add fields to `Snapshot` or add a separate
`TaskDetail` function that accepts a task slug. The `Fetcher` interface can
gain new methods as new contracts are needed. The renderer gains new
`renderTaskDetail`, `renderCleanupCandidates`, and `renderRunHistory` functions.

**Phase 3 (low-risk actions):** Introduce a separate `Dispatcher` interface
alongside `Fetcher`. The Dispatcher takes an explicit action, shows the planned
CLI command, and awaits confirmation before calling `brevity.command-result.v1`
consumers. The renderer never needs to know about mutation — it only renders
`Snapshot`.

**Phase 4-5 (destructive/execution actions):** Add `Plan` methods to the
Dispatcher. The confirmation model (show exact argv, show destructive flags, two
explicit confirmation steps for destructive actions) lives in the dispatcher
layer, not in the renderer.

**Watch mode:** Add a `Watch(ctx context.Context, fetcher Fetcher, refresh time.Duration, w io.Writer)` function that polls `Read` and calls `Render`. The
renderer's determinism (same Snapshot → same output) is what makes watch-mode
redraw suppression possible: compare the previous render string to the new one.

**TUI framework:** If Bubble Tea is adopted, the `Snapshot` becomes the model
and `Render` becomes a pure view function. The framework calls `Read` in its
update loop and passes the resulting `Snapshot` to the view. The contract
boundary (`Fetcher`) stays intact.

**Alternative fetchers:** A `PowerShellFetcher` that invokes `.\brevity.ps1`
subprocesses can be added alongside `NativeFetcher` for compatibility with
PowerShell-backed runtime state, following the same pattern as
`runtimeclient.PowerShellClient`.

## Usage

```powershell
# Show all sections (default)
brevity cmux

# Show help
brevity cmux --help
```

`brevity cmux` exits after a single render. No watch mode, no terminal
clearing, no keyboard input. Safe in remote sessions, CI pipelines, and AI
agent contexts.

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--limit <n>` | `10` | Maximum tasks shown in the task list. |
| `--section <name>` | `all` | Restrict output to one section: `all`, `providers`, `tasks`, `queue`, `actions`. |
| `--task <slug>` | _(none)_ | Show only the task with this exact slug. |
| `--state <state>` | _(none)_ | Show only tasks whose normalized state matches (case-insensitive). |
| `--output <mode>` | `text` | Output format: `text`, `markdown`, or `json`. |
| `-h`, `--help` | — | Show help and exit. |

### Output modes

| Mode | Description |
| --- | --- |
| `text` | Plain-text terminal report. No ANSI sequences. Pipe-safe and log-safe. |
| `markdown` | GitHub-Flavoured Markdown. Suitable for AI context, shared reports, and Markdown renderers. No HTML, no code fences wrapping the document. |
| `json` | Structured JSON. Schema `brevity.cmux-report.v1`. Respects `--section`, `--task`, `--state`, `--limit`. Empty collections are `[]` not `null`. |

### Examples

```powershell
# Default: all sections, text output
brevity cmux

# Show only the task list
brevity cmux --section tasks

# Show tasks in reviewing state
brevity cmux --section tasks --state reviewing

# Show one specific task
brevity cmux --task my-feature

# Show 20 tasks instead of the default 10
brevity cmux --limit 20

# GitHub-Flavoured Markdown (AI context, shared report)
brevity cmux --output markdown

# Pipe-friendly JSON (CI, scripting, machine consumers)
brevity cmux --output json

# JSON for just the queue section
brevity cmux --output json --section queue

# JSON filtered to a single task
brevity cmux --output json --task my-feature
```

### Safe travel and remote usage

`brevity cmux` is safe to run in any context where a one-shot command is
acceptable:

- **Remote sessions (SSH, WinRM, CI):** exits cleanly after one render; no
  interactive redraw loop, no terminal control sequences.
- **AI agent contexts:** output is deterministic for a given runtime state;
  `--output json` provides a typed schema for programmatic consumption;
  `--output markdown` provides structured text without ANSI noise.
- **Pipes and log capture:** all output modes avoid ANSI escape sequences.

## Contract Dependencies

| Contract | Schema | Source |
| --- | --- | --- |
| Runtime state | `brevity.runtime-state.v1` | `runtimeclient.NativeClient.RuntimeStateJSON()` |
| Scheduler plan | `brevity.runtime-scheduler-plan.v1` | `runtimescheduler.Planner.Plan()` marshaled to JSON |

Both contracts evolve additively. CMUX tolerates unknown fields via
`json.Unmarshal` into typed structs.
