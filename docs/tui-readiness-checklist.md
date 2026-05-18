# TUI Readiness Checklist

This checklist tracks practical readiness for the first Brevity TUI
implementation phase. The first TUI should remain a thin CLI orchestration
layer that reads runtime state and invokes Brevity commands, not a replacement
runtime.

## Completed Foundations

- `runtime state --json` provides the primary state source for TUI rendering.
- Runtime state schema documents the JSON read model.
- Provider lifecycle commands support health updates and reset flows.
- Runtime inspection commands expose operator-facing workspace state.
- Logs commands expose recent runtime activity.
- Orphan detection identifies unmanaged task worktrees.
- `task runtime-info` exposes task-level runtime details.
- Context materialization gives tasks durable Markdown context.
- Dry-run cleanup guidance supports safer cleanup review before mutation.
- TUI command safety contract defines read-only, dry-run, mutating, and
  destructive command boundaries.

## Remaining Gaps

- Add formal destructive confirmations at the CLI boundary, not only in future
  TUI presentation.
- Track worker process lifecycle state so the TUI can distinguish queued,
  running, paused, completed, failed, and unknown work.
- Normalize severity levels across provider health, runtime warnings, logs, and
  command output.
- Define an event or refresh model for active TUI views without tight polling.
- Define a command result contract for success, warning, failure, changed
  resources, and suggested follow-up actions.
- Design interactive selection and navigation around tasks, providers, logs,
  orphans, and actions.
- Implement real parallel execution orchestration before presenting parallel
  task control as operationally complete.

## Nice-To-Have Future Improvements

- Stable machine-readable output for more inspection commands.
- Structured log records alongside Markdown logs.
- Saved TUI filters for task groups, provider state, and warning severity.
- Operator audit trails for destructive and force actions.
- Recovery hints that connect failed commands to safe next steps.

## First TUI Boundary

- Runtime state JSON is the primary state source.
- The TUI should not bypass CLI or runtime contracts.
- The TUI should execute explicit CLI commands for every mutation.
- The TUI should refresh from `runtime state --json` after command completion.
