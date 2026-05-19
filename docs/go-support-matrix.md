# Go Frontend Support Matrix

Brevity's Go command under `cmd\brevity` is currently a frontend/runtime
client. PowerShell remains the authoritative runtime backend and the source of
truth for state interpretation, orchestration behavior, worker lifecycle,
cleanup, branch integration, `.brevity` mutation, and JSON contracts.

The Go client does not mutate `.brevity` files directly. Current Go actions are
PowerShell-backed: they invoke `.\brevity.ps1 ... --json`, parse the structured
`brevity.command-result.v1` contract, and render concise operator output. The
dashboard path reads the PowerShell-produced `brevity.runtime-state.v1`
snapshot.

This matrix is intentionally conservative. "Mutating" means the command can
cause PowerShell to change runtime state, worktrees, branches, logs, or provider
metadata. It does not mean Go writes those files itself.

| Command | Category | Backend | Read-only / Mutating | Status | Notes |
| --- | --- | --- | --- | --- | --- |
| `go run ./cmd/brevity` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Polls `.\brevity.ps1 runtime state --json` and renders the dashboard. |
| `go run ./cmd/brevity --once` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Renders one runtime-state snapshot and exits. |
| `go run ./cmd/brevity --watch` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Keeps polling runtime state until interrupted; Ctrl+C exits cleanly. |
| `go run ./cmd/brevity --watch --refresh 5s` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Uses a Go duration value for the watch-mode polling interval. |
| `go run ./cmd/brevity --watch --no-clear` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Suppresses screen clearing on changed renders; unchanged stable dashboard content does not redraw. |
| `go run ./cmd/brevity provider set <provider> <status>` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Implemented | Updates provider health through `.\brevity.ps1 provider set ... --json`. |
| `go run ./cmd/brevity provider reset <provider>` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Implemented | Resets provider health through `.\brevity.ps1 provider reset ... --json`. |
| `go run ./cmd/brevity task new <slug>` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Implemented | Creates task runtime metadata and worktree through PowerShell. |
| `go run ./cmd/brevity task cleanup <slug> --force` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Implemented | Requires `--force`; cleanup behavior is owned by PowerShell. |
| `go run ./cmd/brevity task context refresh <slug>` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Implemented | Refreshes materialized task context through PowerShell. |
| `go run ./cmd/brevity task runtime-info <slug>` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Displays task runtime details from PowerShell. |
| `go run ./cmd/brevity task runs <slug>` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Displays recent run-index records for one task. |
| `go run ./cmd/brevity task run <slug> --execute [--profile <profile>] [--smoke]` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Implemented | Starts a synchronous worker run through PowerShell and records run metadata. |
| `go run ./cmd/brevity task runs reconcile --dry-run` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Reports stale or incomplete run records; dry-run only. |
| `go run ./cmd/brevity task runs retention --dry-run` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Reports run-index retention signals; dry-run only. |
| `go run ./cmd/brevity task runs compact --dry-run` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Reports a compaction plan; dry-run only. |
| `go run ./cmd/brevity doctor` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Displays runtime diagnostics from `.\brevity.ps1 doctor --json`. |
| Native Go `.brevity` reader | Runtime migration | Future Go reader | Read-only | Planned/deferred | Future migration step after JSON-first behavior stays stable. |
| TUI mutation UI | Frontend mutation UI | Future command-result actions | Mutating | Planned/deferred | No interactive mutation UI exists yet. |
| `go run ./cmd/brevity task merge <slug>` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Planned/deferred | PowerShell has merge behavior; Go command surface does not expose it yet. |
| Orphan cleanup execute | PowerShell-backed action | PowerShell command-result JSON | Mutating | Planned/deferred | Go does not expose mutating orphan cleanup execution yet. |
| `go run ./cmd/brevity task runs compact --execute --archive` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Planned/deferred | Go exposes only dry-run compaction planning. |
| Background worker lifecycle | Runtime migration | PowerShell today, future Go TBD | Mutating | Planned/deferred | Worker lifecycle remains PowerShell-owned. |

## Documentation Notes

- PowerShell remains the source of truth for behavior and JSON contracts.
- Dashboard watch mode is still read-only: each refresh reads
  `brevity.runtime-state.v1` from PowerShell and renders it.
- Watch mode suppresses redraws when stable dashboard content is unchanged.
  Runtime `GeneratedAt` and poll timestamps do not force redraws by themselves.
- `--no-clear` disables clear-screen behavior on changed dashboard renders.
- Go support should stay narrower than the PowerShell surface until parity is
  explicit and tested.
- New Go commands should be added here when they are exposed, even if the
  backend remains PowerShell.
