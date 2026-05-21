# Go Frontend Support Matrix

Brevity's Go command under `cmd\brevity` is a frontend/runtime client plus the
native runtime authority slices. PowerShell remains the legacy reference and
fallback for orchestration behavior, but Go now owns provider health state
mutation plus task/runtime state reading.

The original PowerShell `.\brevity.ps1 tui` command is a lightweight read-only
runtime/operator scaffold. The Go dashboard and `--watch` mode are the active
frontend direction for the future operator UX. The default dashboard source
still consumes PowerShell-produced runtime-state data for compatibility, while
`runtime state --json`, `task status`, and `--json-source native` read runtime
state directly from Go. Neither path provides an interactive mutation UI yet.

Go-owned `.brevity` writes must go through `internal/state` and the advisory
`.brevity/state.lock` protocol. Provider execution and worker execution are not
implemented by this migration.

The dashboard UX and interactive action roadmap is documented in
[`docs/go-dashboard-ux-plan.md`](go-dashboard-ux-plan.md).

This matrix is intentionally conservative. "Mutating" means the command can
cause PowerShell to change runtime state, worktrees, branches, logs, or provider
metadata. It does not mean Go writes those files itself.

| Command | Category | Backend | Read-only / Mutating | Status | Notes |
| --- | --- | --- | --- | --- | --- |
| `go run ./cmd/brevity` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Polls `.\brevity.ps1 runtime state --json` and renders the dashboard. |
| `go run ./cmd/brevity --once` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Renders one runtime-state snapshot and exits. |
| `go run ./cmd/brevity --watch` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Keeps polling runtime state until interrupted; line-oriented input supports `j`/`k` then Enter for movement, `d` or Enter for details, `r` then Enter for refresh, `?` then Enter for help, and `q` then Enter to quit. |
| `go run ./cmd/brevity --watch --refresh 5s` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Uses a Go duration value for the watch-mode polling interval. |
| `go run ./cmd/brevity --watch --no-clear` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Suppresses screen clearing on changed renders; unchanged stable dashboard content does not redraw. |
| `go run ./cmd/brevity --bubble --json-source native` | Bubble Tea dashboard | Go native runtime state builder | Read-only | Implemented | Reads provider health, tasks, latest runs, and worktree cleanup signals without PowerShell. |
| `go run ./cmd/brevity runtime state --json` | Native state inspection | Go native runtime state builder | Read-only | Implemented | Emits stable `brevity.runtime-state.v1` JSON from native Go; no PowerShell call. |
| `go run ./cmd/brevity provider status` | Native state inspection | Go `.brevity/provider-health.json` reader | Read-only | Implemented | Reads provider health through `internal/state`; no PowerShell call. |
| `go run ./cmd/brevity provider set <provider> <status> [--note <note>]` | Native state action | Go state store + `.brevity/state.lock` | Mutating | Implemented | Updates provider health without PowerShell or provider execution. |
| `go run ./cmd/brevity provider reset <provider>` | Native state action | Go state store + `.brevity/state.lock` | Mutating | Implemented | Resets provider health to `unknown` without PowerShell or provider execution. |
| `go run ./cmd/brevity task status` | Native state inspection | Go `.brevity/tasks.json` reader | Read-only | Implemented | Lists tracked task metadata through `internal/state`; no PowerShell call and no task mutation. |
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
| `.\brevity.ps1 tui` | PowerShell TUI scaffold | PowerShell runtime state JSON | Read-only | Implemented | Original lightweight operator scaffold; useful as a reference, not the active future frontend direction. |
| Native Go `.brevity` reader | Runtime migration | Go state readers | Read-only | Implemented for current read slice | Provider health, tasks, runs, and worktree scans are available for native runtime state; provider health and task metadata use `internal/state`. |
| Interactive mutation UI | Frontend mutation UI | Future command-result actions | Mutating | Planned/deferred | No interactive mutation UI exists yet in the PowerShell TUI or Go dashboard. |
| `go run ./cmd/brevity task merge <slug>` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Planned/deferred | PowerShell has merge behavior; Go command surface does not expose it yet. |
| Orphan cleanup execute | PowerShell-backed action | PowerShell command-result JSON | Mutating | Planned/deferred | Go does not expose mutating orphan cleanup execution yet. |
| `go run ./cmd/brevity task runs compact --execute --archive` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Planned/deferred | Go exposes only dry-run compaction planning. |
| Background worker lifecycle | Runtime migration | PowerShell today, future Go TBD | Mutating | Planned/deferred | Worker lifecycle remains PowerShell-owned. |

## Documentation Notes

- Go owns provider health read/write, `.brevity/tasks.json` reading,
  `.brevity/runs.jsonl` run-history reading, native runtime-state building,
  native task status, and the Bubble Tea native source.
- PowerShell remains the authority for task mutation, worker/provider execution,
  task new/start/run/merge/cleanup, and legacy compatibility.
- Provider health writes use `.brevity/state.lock` with exclusive create,
  `pid` and UTC `createdAt` contents, timeout waiting, and stale-lock cleanup
  when configured by tests/services.
- `.\brevity.ps1 provider status/set/reset` remains available for compatibility,
  but the Go CLI no longer shells to it for provider health.
- PowerShell `tui` remains a lightweight read-only scaffold; Go dashboard/watch
  mode is the active frontend direction.
- Dashboard watch mode is still read-only. The default source remains
  PowerShell for compatibility, while `--json-source native` uses Go runtime
  state directly.
- Watch mode uses line-oriented input for now. Operators type a key and press
  Enter: `j`/`k` move the selection, `d` or Enter toggles details, `r`
  refreshes, `?` toggles help, and `q` quits.
- Detail panes currently cover providers, tasks, cleanup candidates, and
  suggested actions. Suggested actions are read-only guidance and are not
  executed by the dashboard.
- Watch mode suppresses redraws when stable dashboard content is unchanged.
  Runtime `GeneratedAt` and poll timestamps do not force redraws by themselves.
- `--no-clear` disables clear-screen behavior on changed dashboard renders.
- Raw terminal input and Bubble Tea-style framework adoption are deferred. The
  current dashboard stays dependency-free, Windows-friendly, and read-only.
- Go support should stay narrower than the PowerShell surface until parity is
  explicit and tested.
- New Go commands should be added here when they are exposed, even if the
  backend remains PowerShell.
