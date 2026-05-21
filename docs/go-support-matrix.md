# Go Frontend Support Matrix

Brevity's Go command under `cmd\brevity` is a frontend/runtime client plus the
native runtime authority slices. PowerShell remains the legacy reference and
fallback for orchestration behavior, but Go now owns provider health state
mutation, task/runtime state reading, run-history inspection, read-only
doctor/detail diagnostics, cleanup/orphan inspection reports, native task
mutation preflight gates, `task start <slug>` metadata mutation, and native
task prompt/context refresh.

The original PowerShell `.\brevity.ps1 tui` command is a lightweight read-only
runtime/operator scaffold. The Go dashboard and `--watch` mode are the active
frontend direction for the future operator UX. The default dashboard source
still consumes PowerShell-produced runtime-state data for compatibility, while
`runtime state --json`, `task status`, and `--json-source native` read runtime
state directly from Go. The Bubble Tea dashboard can run confirmed Start task
and Refresh context through native Go. Run Worker remains dry-run/plan-only and
provider execution is still disabled from Go.

Go-owned `.brevity` writes must go through `internal/state` and the advisory
`.brevity/state.lock` protocol. Provider execution and worker execution are not
implemented by this migration.

Native preflight is the safety contract for Go-owned task mutation.
Preflight is read-only: it does not create/delete worktrees, create/delete
branches, write `tasks.json`, or launch providers/workers. PowerShell still
owns mutation execution for task new/run/merge/cleanup flows.

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
| `go run ./cmd/brevity task preflight <new|start|run|merge|cleanup> <slug> [--json]` | Native mutation safety gate | Go state readers + read-only cleanup/provider checks | Read-only | Implemented | Emits human or stable `brevity.task-preflight.v1` JSON with status, checks, blockers, warnings, expected mutations, destructive/provider-execution flags, and suggested next action. |
| `go run ./cmd/brevity task start <slug> [--json]` | Native state action | Go preflight + state store + `.brevity/state.lock` | Mutating | Implemented | Transitions allowed task states to `ready-for-worker`, updates `updatedAt` and `startedAt` when absent, preserves unrelated and unknown task fields, emits `brevity.command-result.v1`, and launches no provider/worker. |
| `go run ./cmd/brevity task new <slug>` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Implemented | Creates task runtime metadata and worktree through PowerShell. |
| `go run ./cmd/brevity task cleanup <slug> --force` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Implemented | Requires `--force`; cleanup behavior is owned by PowerShell. |
| `go run ./cmd/brevity task refresh-context <slug> [--json]` | Native state action | Go preflight + vault loader + state store + `.brevity/state.lock` | Mutating | Implemented | Rewrites `prompt.md`, refreshes bounded `.brevity\context` files from configured vault memory, updates prompt refresh metadata, emits `brevity.command-result.v1`, and launches no provider/worker. |
| `go run ./cmd/brevity task context refresh <slug> [--json]` | Native state action | Go native refresh service | Mutating | Compatibility alias | Legacy command shape routed to the same Go service; PowerShell remains reference/fallback only. |
| `go run ./cmd/brevity task runtime-info <slug>` | Native read-only inspection | Go `.brevity/tasks.json` + `.brevity/runs.jsonl` reader | Read-only | Implemented | Displays task runtime details without PowerShell. |
| `go run ./cmd/brevity task detail <slug>` | Native read-only inspection | Go `.brevity/tasks.json` + `.brevity/runs.jsonl` reader | Read-only | Implemented | Alias-style focused task detail view with metadata, worktree, prompt, provider/profile, latest run, and operator interpretation. |
| `go run ./cmd/brevity task runs <slug>` | Native read-only inspection | Go `.brevity/runs.jsonl` reader | Read-only | Implemented | Displays recent run-index records for one task without PowerShell. |
| `go run ./cmd/brevity task run <slug> --execute [--profile <profile>] [--smoke]` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Implemented | Starts a synchronous worker run through PowerShell and records run metadata. |
| `go run ./cmd/brevity task runs reconcile --dry-run` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Reports stale or incomplete run records; dry-run only. |
| `go run ./cmd/brevity task runs retention --dry-run` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Reports run-index retention signals; dry-run only. |
| `go run ./cmd/brevity task runs compact --dry-run` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Reports a compaction plan; dry-run only. |
| `go run ./cmd/brevity cleanup inspect` | Native cleanup inspection | Go task store + read-only Git/worktree inspector | Read-only | Implemented | Reports missing worktrees, orphan task worktrees, orphan task branches, dirty worktrees, and stale runs; explicitly executes no cleanup. |
| `go run ./cmd/brevity cleanup inspect --json` | Native cleanup inspection | Go task store + read-only Git/worktree inspector | Read-only | Implemented | Emits stable `brevity.cleanup-inspection.v1` JSON with summary counts and candidate classification. |
| `go run ./cmd/brevity doctor` | Native read-only diagnostics | Go diagnostic checks | Read-only | Implemented | Checks Git availability, repo and `.brevity` readability, config, provider health, task metadata, runs, locks, vault/worktree paths, and task worktree paths without PowerShell. |
| `go run ./cmd/brevity doctor --json` | Native read-only diagnostics | Go diagnostic checks | Read-only | Implemented | Emits `brevity.command-result.v1` with a stable `brevity.doctor.v1` payload; exits non-zero when error diagnostics exist. |
| `.\brevity.ps1 tui` | PowerShell TUI scaffold | PowerShell runtime state JSON | Read-only | Implemented | Original lightweight operator scaffold; useful as a reference, not the active future frontend direction. |
| Native Go `.brevity` reader | Runtime migration | Go state readers | Read-only | Implemented for current read slice | Provider health, tasks, runs, and worktree scans are available for native runtime state; provider health and task metadata use `internal/state`. |
| Interactive mutation UI | Frontend mutation UI | Future command-result actions | Mutating | Planned/deferred | No interactive mutation UI exists yet in the PowerShell TUI or Go dashboard. |
| `go run ./cmd/brevity task merge <slug>` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Planned/deferred | PowerShell has merge behavior; Go command surface does not expose it yet. |
| Orphan cleanup execute | PowerShell-backed action | PowerShell command-result JSON | Mutating | Planned/deferred | Go does not expose mutating orphan cleanup execution yet. |
| `go run ./cmd/brevity task runs compact --execute --archive` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Planned/deferred | Go exposes only dry-run compaction planning. |
| Background worker lifecycle | Runtime migration | PowerShell today, future Go TBD | Mutating | Planned/deferred | Worker lifecycle remains PowerShell-owned. |

## Documentation Notes

- Go owns provider health read/write, `.brevity/tasks.json` reading and locked
  task-start and prompt/context refresh writes,
  `.brevity/runs.jsonl` run-history reading, native runtime-state building,
  native task status, task runtime/detail inspection, doctor diagnostics,
  cleanup/orphan inspection reports, task mutation preflight gates, and the
  Bubble Tea native source.
- PowerShell remains the authority for task new/run/merge/cleanup execution,
  worker/provider execution, and legacy compatibility. Its prompt/context
  refresh behavior is now reference/fallback rather than the Go CLI path.
- Every Go task mutation must pass native preflight first. The
  `brevity.task-preflight.v1` JSON payload is the contract shared by CLI, TUI,
  and operator flows.
- Native Start task does not create/delete branches, create/delete worktrees, or
  run providers/workers. Native Refresh context writes only the task prompt,
  bounded worktree context files, and task refresh metadata.
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
