# Go Dashboard UX and Action Plan

This plan defines the operator UX direction for the Go dashboard before adding
keyboard controls or adopting a terminal UI framework. It is documentation only:
PowerShell remains the authoritative backend/runtime, and the Go dashboard
remains a frontend/runtime client until native Go parity is proven.

The current Go dashboard and watch mode are read-only. Existing Go action
runners dispatch explicit PowerShell-backed commands, but there is no
interactive mutation UI yet.

## UX Principles

- Stay terminal-native and useful in a plain console.
- Keep output low-noise, stable, and readable as plain text.
- Optimize for fast comprehension before dense interaction.
- Make warnings, stale state, and cleanup risks visible without requiring drill
  down.
- Avoid hidden mutations: the dashboard should never change runtime state just
  because it rendered or refreshed.
- Keep PowerShell-backed contracts authoritative until native Go behavior has
  explicit parity.

## Read-Only Dashboard Layout

The read-only dashboard should keep a predictable information hierarchy:

1. Header and overall runtime status.
2. Provider health summary.
3. Task state summary.
4. Task list or recent tasks.
5. Cleanup warnings, orphaned worktrees, stale tasks, and run-index warnings.
6. Run indicators for active, failed, stale, or incomplete runs.
7. Suggested next actions from runtime state.
8. Footer/help with refresh and exit hints.

The layout should remain readable when copied into an issue, chat, or log. It
should prefer compact labels and aligned text over decorative terminal effects.

## Watch-Mode Behavior

Watch mode should continue to be conservative:

- Poll the PowerShell runtime-state contract at the configured refresh interval.
- Redraw only when stable dashboard content changes.
- Keep `--no-clear` for terminals, logs, and debugging flows where screen
  clearing is undesirable.
- Display polling failures as dashboard state, including enough error context to
  act, instead of crashing on the first transient failure.
- Exit cleanly on Ctrl+C without mutating `.brevity`.
- Add terminal resize handling later when interaction pressure justifies it.

## Future Keyboard and Action Model

Initial keys should be conservative and easy to explain. Keyboard support should
start with navigation and inspection before mutation:

- Refresh dashboard state.
- Open task or provider details.
- Set or reset provider health.
- Refresh task context.
- Cleanup completed or stale task worktrees.
- Run a task through the existing PowerShell-backed execution path.

All mutations require confirmation. Keyboard shortcuts should select an intent;
they should not immediately dispatch a mutating command.

## Confirmation Model

Before dispatching a mutating action, the Go UI should show:

- The exact PowerShell-backed command that will run.
- The expected `brevity.command-result.v1` schema.
- The resources likely to be affected, such as provider, task slug, branch, or
  worktree path when known.
- Any destructive flag or execution flag, including `--force` or `--execute`.

Destructive actions require explicit confirmation. After any command result,
including structured failures and ambiguous process failures, the UI should
refresh runtime state from `.\brevity.ps1 runtime state --json` and render from
that fresh snapshot rather than from optimistic local edits.

## Action Rollout Phases

### Phase 1: Read-Only Dashboard and Watch

Keep improving the current read-only Go dashboard/watch path. Validate stable
redraw, refresh intervals, `--no-clear`, failure display, and Ctrl+C exit.

### Phase 2: Keyboard Navigation and Details Only

Add navigation, selection, details panes or detail views, and manual refresh.
Do not add mutation keys in this phase.

### Phase 3: Low-Risk Actions

Add confirmed PowerShell-backed actions with low blast radius, such as provider
reset, provider set, and task context refresh. Every action must parse
`brevity.command-result.v1` and refresh runtime state afterward.

### Phase 4: Destructive Actions

Add confirmed cleanup flows only after the UI can show exact commands, affected
resources, dry-run guidance, and command-result failures clearly.

### Phase 5: Execution Actions

Add task run flows after action confirmation, command-result handling,
lock/conflict display, and refresh-after-command behavior are stable.

### Phase 6: Native Go Runtime Ownership Later

Move runtime ownership to Go only after PowerShell parity, contracts, smoke
validation, and rollback paths are in place. Until then, Go is a frontend and
action dispatcher.

## Framework Decision

Stay dependency-free while the dashboard is read-only and simple keyboard
interaction is still speculative. Adopt a TUI framework only when interaction
pressure justifies the dependency and architecture cost.

If a framework becomes necessary, evaluate Bubble Tea first because it fits
command-line application structure and simple model/update/view flows. Consider
tcell later for lower-level terminal control if Bubble Tea is not a fit.

No framework adoption belongs in this task.

## Non-Goals

- No direct `.brevity` mutation from Go yet.
- No daemon.
- No background worker lifecycle.
- No PowerShell replacement yet.
- No mutation UI without command-result handling.
- No optimistic dashboard writes to runtime state.
- No TUI framework adoption in this plan.

## Recommended Next Implementation Task

The recommended next implementation task is:

```text
go-dashboard-keyboard-navigation-details-v1
```

Implement keyboard navigation and read-only detail views in the Go dashboard,
without mutation keys, direct `.brevity` writes, or framework adoption unless a
separate framework decision task approves it.
