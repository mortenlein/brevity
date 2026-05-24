# Go Frontend Support Matrix

Brevity's Go command under `cmd\brevity` is a frontend/runtime client plus the
native runtime authority slices. PowerShell remains the legacy reference and
fallback for orchestration behavior, but Go now owns provider health state
mutation, task creation, task/runtime state reading, run-history inspection,
read-only doctor/detail diagnostics, cleanup/orphan inspection reports, native
orphan cleanup planning/execution, native
task mutation preflight gates, `task start <slug>` metadata mutation, native
task prompt/context refresh, native task-run planning, native task-run provider
execution, and native task-merge planning/execution.

The original PowerShell `.\brevity.ps1 tui` command is a lightweight read-only
runtime/operator scaffold. The Go dashboard and `--watch` mode are the active
frontend direction for the future operator UX. The default dashboard source
still consumes PowerShell-produced runtime-state data for compatibility, while
`runtime state --json`, `task status`, and `--json-source native` read runtime
state directly from Go. The Bubble Tea dashboard can run confirmed Start task,
Refresh context, and Run Worker through native Go. Run Worker uses the native
task-run plan/preflight envelope, then executes the provider command with
argv-style `os/exec`, writes logs, appends `.brevity/runs.jsonl`, and updates
task runtime metadata.

Runtime queue persistence, inspection, planning, and explicit reservations are
Go-native. `queue add`, `queue list`, `queue inspect`, `queue plan`, `queue
reserve`, `queue unreserve`, and `queue remove` manage or read
`.brevity\runtime-queue.json` as inert infrastructure state only; they do not
execute providers, spawn workers, start the supervisor, or mutate task lifecycle
state. The Bubble Tea dashboard now displays queue file health, item/status
counts, reserved counts, corruption warnings, and oldest queued age as
read-only visibility only.

Runtime scheduler planning is Go-native and read-only. `scheduler plan` consumes
the queue plan, selects the first eligible runnable queue item, explains the
selection or no-selection reason, and reports reservation eligibility without
reserving, executing providers, spawning workers, starting the supervisor,
creating run history, draining the queue, or mutating task state.

Runtime execution records and explicit manual launch are Go-native.
`execution list` and `execution inspect` read
`.brevity\runtime-executions.json`, `execution plan-from-reservation
<queue-item-id>` creates one planned execution record from an already reserved
queue item, and `execution launch <execution-id>` runs exactly one ready
execution in the foreground. It invokes one provider process with argv-style
`os/exec`, streams output, captures the exit code, and updates only execution
state. It does not start scheduler loops, drain the queue, create retries, run
in the background, mutate task workflow state, or create provider pools.

Go-owned `.brevity` writes must go through `internal/state` and the advisory
`.brevity/state.lock` protocol. Provider execution and worker execution for
`task run --execute`, `task merge`, and explicit cleanup commands are
implemented natively. Cleanup execution is still separate and not part of the
native merge path.

Native preflight is the safety contract for Go-owned task mutation.
Preflight is read-only: it does not create/delete worktrees, create/delete
branches, write `tasks.json`, or launch providers/workers. PowerShell remains a
legacy reference/fallback for merge semantics and other historical orchestration
behavior, while cleanup execution remains separate.

The dashboard UX and interactive action roadmap is documented in
[`docs/go-dashboard-ux-plan.md`](go-dashboard-ux-plan.md).

This matrix is intentionally conservative. Go is the primary runtime authority
where a capability is marked migrated. PowerShell remains available as
legacy/reference/fallback unless the row says the capability is still
PowerShell-owned. "Mutating" means the command can change runtime state,
worktrees, branches, logs, or provider metadata.

The machine-readable companion lives at
[`docs/brevity-support-matrix.json`](brevity-support-matrix.json). The CLI can
render the same native support data with:

```powershell
go run ./cmd/brevity support matrix
go run ./cmd/brevity support matrix --json
```

## Ownership Matrix

| Capability | Go support | PowerShell support | Current authority | Safety class | Test coverage | TUI support | Migration status | Next action |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Dashboard/watch console | Implemented; default PowerShell source, native source optional | Implemented legacy TUI scaffold | Go for future operator UX | read-only | unit tests | Go dashboard and Bubble Tea | migrated frontend direction | Keep improving native source |
| Runtime state JSON | Native reader implemented | Implemented legacy producer | Go | read-only | unit and contract tests | native source supported | migrated | Keep compatibility schema additive |
| Runtime queue persistence, inspection, planning, and reservations | Native add, list, inspect, plan, reserve, unreserve, remove for `.brevity\runtime-queue.json`; Bubble Tea displays read-only queue health | Not PowerShell-owned | Go | inert runtime metadata mutation / read-only diagnostics and planning | unit tests | Bubble Tea read-only visibility | migrated foundation | Keep queue execution out of v1 |
| Runtime scheduler planning and reservation | Native scheduler plan selects first eligible runnable queue item from queue plan; `reserve-next` reserves that one item through queue reservation | Not PowerShell-owned | Go | read-only scheduling decision / inert reservation metadata | unit tests | none | migrated contract | Keep execution out of scheduler v1 |
| Runtime execution records and manual launch | Native list, inspect, plan-from-reservation, readiness transitions, preflight, launch-dry-run, and single foreground provider launch for `.brevity\runtime-executions.json`; native runtime state and Bubble Tea expose compact execution visibility | Not PowerShell-owned | Go | execution metadata / manual provider-executing mutation | unit and fake-provider tests | Bubble Tea read-only visibility | migrated execution contract | Keep scheduler loops and queue draining out of execution launch |
| Provider health read/write | Native read, set, reset | Implemented legacy compatibility | Go | mutating metadata | unit tests | actions available | migrated | Deprecate PowerShell-first docs |
| Task metadata/runtime reads | Native task status, detail, runtime-info | Implemented legacy views | Go | read-only | unit tests | native TUI source | migrated | Keep Go as authority |
| Task new worktree creation | Native implementation | Implemented legacy compatibility | Go | mutating git and metadata | fixture tests | action available | migrated | Keep PowerShell as fallback only |
| Task start metadata transition | Native implementation | Legacy manual start helper | Go | mutating metadata | unit tests | action available | migrated | Clarify PowerShell legacy semantics |
| Task run planning | Native plan envelope | Implemented legacy plan | Go | read-only provider plan | unit tests | action available | migrated | Keep plan contract stable |
| Task run provider execution | Native argv execution | Implemented legacy execution | Go | provider-executing mutation | fake-provider tests | action available | migrated | Keep no-real-provider test rule |
| Task context refresh | Native implementation | Implemented legacy compatibility | Go | mutating prompt/context metadata | unit tests | action available | migrated | Keep alias `task context refresh` |
| Task merge | Native plan and execute | Implemented legacy merge | Go | mutating git and metadata | fixture tests | planned TUI enrichment | migrated | Add merge confirmation UI later |
| Selected task cleanup | Native plan and explicit force execute | Implemented legacy cleanup | Go | destructive git cleanup | fixture tests | planned TUI enrichment | migrated | Keep cleanup separate from merge |
| Orphan cleanup | Native inspect, plan, execute | Implemented legacy orphan helpers | Go | destructive cleanup | fixture tests | inspection in TUI | migrated | Prefer native candidate IDs |
| Doctor diagnostics | Native read-only diagnostics | Implemented with repair paths | Go for read-only doctor; PowerShell for repair | read-only / mutating repair | unit tests | not direct | partially migrated | Migrate repair only if still needed |
| Run history reads | Native runs and runtime summaries | Implemented legacy reads | Go | read-only | unit tests | native source supported | migrated | Keep JSONL contract stable |
| Run history maintenance | Native runs inspect and compact | PowerShell task runs reconcile/retention/compact remains | Go for native compact; PowerShell for reconcile/retention | read-only / mutating compact | unit tests | not direct | partially migrated | Migrate reconcile and retention next |
| Workspace init/repair | Native init and repair | Implemented legacy compatibility | Go | mutating skeleton/config | unit and fixture tests | none | migrated | Keep PowerShell as reference/fallback |
| Task activate/spec | Native activate and read-only spec | Implemented legacy compatibility | Go | mutating git/metadata for activate; read-only for spec | fixture tests | none | migrated | Keep no-provider execution boundary |
| Planner prompt generation/application | Not native | Implemented | PowerShell | mutating prompt/tasks/worktrees depending on flags | PowerShell manual coverage | none | not migrated | Split prompt generation from task creation |
| Memory notes, logs, session summary | Not native or partial via run reads | Implemented | PowerShell | read-only and memory mutation | PowerShell manual coverage | dashboard reads runtime memory | not migrated | Migrate small read-only views when needed |
| Provider profiles/profile aliases | Duplicated in task-run planning | Implemented source matrix | Mixed | read-only config | unit tests for Go planning | indirect | partially migrated | Move profile matrix to shared native metadata |

## PowerShell Command Surface

| Command | Class | Go equivalent | PowerShell status | Risk | Recommended disposition |
| --- | --- | --- | --- | --- | --- |
| `help` | read-only | `--help` | compatibility | low | keep wrapper |
| `status` | read-only | partial via `doctor`, `runtime state` | PowerShell-only | low | migrate later |
| `init [--repair]` | mutating skeleton/config | `go run ./cmd/brevity init [--repair]` | Go-owned | medium | keep PowerShell as reference |
| `plan`, `plan backlog`, `plan workers` | mutating prompt files / read-only profile docs | partial profile logic in Go | PowerShell-owned | medium | migrate later |
| `plan apply` | mutating tasks/worktrees, optional provider start | none | PowerShell-owned legacy | high | deprecate or redesign before migrating |
| `board` | read-only | `task status`, dashboard | legacy view | low | deprecate duplicate |
| `doctor [--json]` | read-only | `doctor [--json]` | legacy/fallback | low | keep fallback |
| `doctor --repair` | mutating repair | none | PowerShell-owned | medium | migrate next if repair remains |
| `doctor execution-policy` | read-only helper | none | PowerShell helper | low | keep permanently as Windows helper |
| `memory note` | mutating memory log | none | PowerShell-owned | low | migrate later or keep helper |
| `logs recent`, `logs task` | read-only | partial via run history | PowerShell-owned | low | migrate later |
| `runtime state [--json]` | read-only | `runtime state --json` | legacy/fallback | low | keep fallback |
| `runtime start|stop|status` | runtime lifecycle metadata | native equivalent | delegated compatibility | medium | delegate only |
| `queue add|list|inspect|plan|reserve|unreserve|remove` | runtime queue metadata, diagnostics, read-only planning, and explicit reservations | native equivalent | Go-owned | medium | keep PowerShell out of queue authority |
| `scheduler plan|reserve-next` | read-only scheduler decision / explicit reservation | native equivalent | Go-owned | low | keep execution out of scheduler contract |
| `execution list|inspect|plan-from-reservation|launch` | runtime execution metadata, diagnostics, and explicit single provider launch | native equivalent | Go-owned | high for launch | keep launch manual and foreground |
| `tui` | read-only | dashboard/Bubble Tea | reference scaffold | low | deprecate later |
| `session summary` | read-only | none | PowerShell-owned | low | migrate later |
| `onboard` | not implemented | none | planned | low | implement in Go when requested |
| `provider status/set/reset` | read-only/mutating metadata | native equivalent | legacy/fallback | medium | keep wrapper/fallback |
| `provider docs/profiles` | read-only | partial profile planning | PowerShell-owned reference | low | migrate profile metadata next |
| `task new` | mutating git/metadata | native equivalent | legacy/fallback | high | keep wrapper now, deprecate later |
| `task activate` | mutating git/metadata from vault spec | `go run ./cmd/brevity task activate <slug>` | Go-owned | high | keep PowerShell as reference |
| `task spec` | read-only | `go run ./cmd/brevity task spec <slug>` | Go-owned | low | keep PowerShell as reference |
| `task start` | read-only manual handoff | native metadata transition | legacy divergent behavior | medium | deprecate or rename legacy helper |
| `task run` | read-only plan or provider-executing mutation | native equivalent | legacy/fallback | high | keep fallback with no new authority |
| `task status` | read-only | native equivalent | legacy/fallback | low | keep wrapper |
| `task runtime-info` | read-only | native equivalent | legacy/fallback | low | keep wrapper |
| `task runs <slug>` | read-only | native equivalent | legacy/fallback | low | keep wrapper |
| `task runs reconcile/retention/compact` | dry-run read-only or compact mutation | partial native runs compact | mixed legacy | medium | migrate reconcile/retention next |
| `task context refresh/status` | mutating refresh / read-only status | native refresh, no native status | mixed legacy | medium | keep alias, migrate status |
| `task merge` | mutating git/metadata | native equivalent | legacy/fallback | high | keep fallback |
| `task cleanup` | destructive git/metadata | native equivalent | legacy/fallback | high | keep fallback |
| `task cleanup-orphans` | destructive cleanup | native cleanup execute | duplicate legacy | high | deprecate after operator confidence |
| `task cleanup-orphan-branches` | destructive cleanup | native cleanup execute | duplicate legacy | high | deprecate after operator confidence |

| Command | Category | Backend | Read-only / Mutating | Status | Notes |
| --- | --- | --- | --- | --- | --- |
| `go run ./cmd/brevity` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Polls `.\brevity.ps1 runtime state --json` and renders the dashboard. |
| `go run ./cmd/brevity --once` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Renders one runtime-state snapshot and exits. |
| `go run ./cmd/brevity --watch` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Keeps polling runtime state until interrupted; line-oriented input supports `j`/`k` then Enter for movement, `d` or Enter for details, `r` then Enter for refresh, `?` then Enter for help, and `q` then Enter to quit. |
| `go run ./cmd/brevity --watch --refresh 5s` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Uses a Go duration value for the watch-mode polling interval. |
| `go run ./cmd/brevity --watch --no-clear` | Dashboard/frontend | PowerShell runtime state JSON | Read-only | Implemented | Suppresses screen clearing on changed renders; unchanged stable dashboard content does not redraw. |
| `go run ./cmd/brevity --bubble --json-source native` | Bubble Tea dashboard | Go native runtime state builder | Read-only | Implemented | Reads provider health, tasks, latest runs, runtime queue/execution visibility, and worktree cleanup signals without PowerShell. |
| `go run ./cmd/brevity runtime state --json` | Native state inspection | Go native runtime state builder | Read-only | Implemented | Emits stable `brevity.runtime-state.v1` JSON from native Go, including compact queue and planned execution visibility; no PowerShell call. |
| `go run ./cmd/brevity runtime start` | Runtime supervisor lifecycle | Go supervisor + `.brevity/runtime.lock` + `.brevity/runtime.json` | Mutating runtime metadata | Implemented foundation | Starts a lightweight heartbeat supervisor only; it does not execute providers, drain queues, or spawn workers. |
| `go run ./cmd/brevity runtime status` | Runtime supervisor inspection | Go runtime state reader | Read-only | Implemented foundation | Reads `.brevity/runtime.json`, tolerates missing/corrupted/stale state, and reports pid, uptime, heartbeat freshness, worker count, and queue depth. |
| `go run ./cmd/brevity runtime stop` | Runtime supervisor lifecycle | Go stop request + supervisor heartbeat loop | Mutating runtime metadata | Implemented foundation | Requests graceful shutdown and persists stopped state; no forceful provider or task behavior is involved. |
| `go run ./cmd/brevity queue add <task>` | Runtime queue persistence | Go `.brevity\runtime-queue.json` writer + queue lock | Mutating inert runtime metadata | Implemented foundation | Validates the task slug, appends a queued item, writes atomically, and does not execute providers, spawn workers, start the supervisor, or mutate task state. |
| `go run ./cmd/brevity queue list` | Runtime queue inspection | Go `.brevity\runtime-queue.json` reader | Read-only | Implemented foundation | Tolerates a missing queue file as empty and reports corrupted queue JSON clearly without mutation. |
| `go run ./cmd/brevity queue inspect [--json]` | Runtime queue diagnostics | Go tolerant `.brevity\runtime-queue.json` inspector | Read-only | Implemented | Reports path, file state, version, item counts, status counts, queued item ages, duplicate ids, invalid items, and unsupported future versions without scheduling, execution, supervisor startup, repair, or queue-file rewrite. |
| `go run ./cmd/brevity queue plan [--json]` | Runtime queue planning | Go tolerant `.brevity\runtime-queue.json` planner | Read-only | Implemented | Reports runnable queue candidates in queue-file order and skipped items with reasons. It does not execute providers, spawn workers, start the supervisor, drain the queue, mutate task state, or reserve ownership. |
| `go run ./cmd/brevity queue reserve <id>` | Runtime queue reservation | Go `.brevity\runtime-queue.json` writer + queue lock | Mutating inert runtime metadata | Implemented foundation | Adds optional reservation ownership metadata to one valid queue item, writes atomically, rejects duplicates/missing items, and does not execute providers, spawn workers, start the supervisor, drain the queue, or mutate task state. |
| `go run ./cmd/brevity queue unreserve <id>` | Runtime queue reservation | Go `.brevity\runtime-queue.json` writer + queue lock | Mutating inert runtime metadata | Implemented foundation | Clears reservation metadata from one queue item, tolerates already-unreserved items, writes atomically, and does not execute providers, spawn workers, start the supervisor, drain the queue, or mutate task state. |
| `go run ./cmd/brevity queue remove <id>` | Runtime queue persistence | Go `.brevity\runtime-queue.json` writer + queue lock | Mutating inert runtime metadata | Implemented foundation | Removes only the matching queue item id, writes atomically, and does not mutate task state. |
| `go run ./cmd/brevity scheduler plan [--json]` | Runtime scheduler planning | Go queue planner consumer | Read-only | Implemented contract | Selects the first eligible runnable queue item, reports why it was selected or why none was selected, and reports reservation eligibility without reserving, executing providers, spawning workers, starting the supervisor, creating run history, draining the queue, or mutating task state. |
| `go run ./cmd/brevity scheduler reserve-next` | Runtime scheduler reservation | Go queue planner consumer + queue reservation writer | Mutating inert runtime metadata | Implemented contract | Computes the scheduler plan, reserves the selected item through queue reservation, prints id/task/reservation id, and does not execute providers, spawn workers, start the supervisor, create run history, drain the queue, or mutate task state. |
| `go run ./cmd/brevity execution list` | Runtime execution records | Go `.brevity\runtime-executions.json` reader | Read-only | Implemented contract | Lists planned, ready, launching, completed, and failed execution records with status counts, treats a missing file as empty, reports corrupted JSON safely, and does not execute providers, spawn workers, start the supervisor, create run history, drain the queue, or mutate task state. |
| `go run ./cmd/brevity execution inspect [--json]` | Runtime execution diagnostics | Go tolerant `.brevity\runtime-executions.json` inspector | Read-only | Implemented contract | Reports path, file health, version, total executions, status counts, duplicate ids, invalid records, and parse/version errors without mutation. |
| `go run ./cmd/brevity execution plan-from-reservation <queue-item-id>` | Runtime execution intent planning | Go queue reader + execution writer | Mutating inert runtime metadata | Implemented contract | Creates exactly one planned execution record for an already reserved queue item and rejects missing, unreserved, reservation-less, or duplicate queue item/reservation pairs. It does not execute providers, spawn workers, start the supervisor, create run history, drain the queue, or mutate task state. |
| `go run ./cmd/brevity execution mark-ready <execution-id>` | Runtime execution readiness transition | Go execution writer + execution lock | Mutating inert runtime metadata | Implemented contract | Transitions exactly one execution record from planned to ready, updates `updatedAt`, and does not execute providers, spawn workers, start the supervisor, create run history, drain the queue, or mutate task state. |
| `go run ./cmd/brevity execution mark-planned <execution-id>` | Runtime execution readiness rollback | Go execution writer + execution lock | Mutating inert runtime metadata | Implemented contract | Transitions exactly one execution record from ready back to planned, updates `updatedAt`, and does not execute providers, spawn workers, start the supervisor, create run history, drain the queue, or mutate task state. |
| `go run ./cmd/brevity execution launch <execution-id> [--json]` | Runtime execution provider launch | Go execution preflight + provider resolver + argv `os/exec` + execution lock | Manual provider-executing mutation | Implemented contract | Requires a ready execution, runs preflight, resolves provider/profile/worktree/prompt context, launches exactly one provider process in the foreground, streams output, captures exit code, and transitions ready -> launching -> completed/failed. It does not mutate queue or task workflow state, create retries, run scheduler loops, or background execution. |
| `go run ./cmd/brevity provider status` | Native state inspection | Go `.brevity/provider-health.json` reader | Read-only | Implemented | Reads provider health through `internal/state`; no PowerShell call. |
| `go run ./cmd/brevity provider set <provider> <status> [--note <note>]` | Native state action | Go state store + `.brevity/state.lock` | Mutating | Implemented | Updates provider health without PowerShell or provider execution. |
| `go run ./cmd/brevity provider reset <provider>` | Native state action | Go state store + `.brevity/state.lock` | Mutating | Implemented | Resets provider health to `unknown` without PowerShell or provider execution. |
| `go run ./cmd/brevity init [--repair] [--json]` | Native setup action | Go state store + `.brevity/state.lock` | Mutating | Implemented | Creates or repairs `.brevity`, config, provider health, task metadata, vault folders, and default memory files without PowerShell, providers, or workers. |
| `go run ./cmd/brevity task status` | Native state inspection | Go `.brevity/tasks.json` reader | Read-only | Implemented | Lists tracked task metadata through `internal/state`; no PowerShell call and no task mutation. |
| `go run ./cmd/brevity task preflight <new|start|run|merge|cleanup> <slug> [--json]` | Native mutation safety gate | Go state readers + read-only cleanup/provider checks | Read-only | Implemented | Emits human or stable `brevity.task-preflight.v1` JSON with status, checks, blockers, warnings, expected mutations, destructive/provider-execution flags, and suggested next action. |
| `go run ./cmd/brevity task start <slug> [--json]` | Native state action | Go preflight + state store + `.brevity/state.lock` | Mutating | Implemented | Transitions allowed task states to `ready-for-worker`, updates `updatedAt` and `startedAt` when absent, preserves unrelated and unknown task fields, emits `brevity.command-result.v1`, and launches no provider/worker. |
| `go run ./cmd/brevity task new <slug> [--json]` | Native state action | Go preflight + Git worktree + state store + `.brevity/state.lock` | Mutating | Implemented | Creates the branch/worktree required by current semantics, materializes prompt/context from the optional vault spec, appends task metadata, emits `brevity.command-result.v1`, and launches no provider/worker. |
| `go run ./cmd/brevity task activate <slug> [--json]` | Native state action | Go Git worktree + state store + `.brevity/state.lock` | Mutating | Implemented | Requires a vault task spec, creates the task branch/worktree, materializes prompt/context, appends metadata, and launches no provider/worker. |
| `go run ./cmd/brevity task spec <slug> [--json]` | Native inspection | Go config + vault task spec reader | Read-only | Implemented | Prints or returns the vault task spec and related task prompt/worktree metadata without mutation. |
| `go run ./cmd/brevity task merge <slug> --plan [--json]` | Native merge planning | Go state reader + read-only Git inspector | Read-only | Implemented | Builds a `brevity.task-merge-plan.v1` payload with source/target branches, worktree dirty state, expected argv git commands, state mutation, blockers, warnings, and cleanup-required signal. It does not mutate Git or task metadata. |
| `go run ./cmd/brevity task merge <slug> [--json]` | Native merge execution | Go merge plan + argv `git` + state store + `.brevity/state.lock` | Mutating | Implemented | Refuses blocked plans, checks out the target branch, runs `git merge <sourceBranch>` without shell concatenation, marks task metadata `merged` only on success, and never deletes branches/worktrees or runs cleanup. Tests use disposable Git fixtures. |
| `go run ./cmd/brevity task cleanup <slug> --plan [--json]` | Native cleanup planning | Go state reader + read-only Git inspector | Read-only | Implemented | Emits `brevity.task-cleanup-plan.v1` with worktree/branch facts, dirty and merge-state checks, expected argv Git commands, force requirement, blockers, and warnings. |
| `go run ./cmd/brevity task cleanup <slug> --force [--json]` | Native cleanup execution | Go cleanup plan + argv `git` + state store + `.brevity/state.lock` | Mutating | Implemented | Requires explicit `--force`, refuses blocked/dirty/unmerged plans, removes the selected Git worktree, deletes the selected branch with safe `git branch -d`, removes selected task metadata, and never runs implicitly after merge. |
| `go run ./cmd/brevity task refresh-context <slug> [--json]` | Native state action | Go preflight + vault loader + state store + `.brevity/state.lock` | Mutating | Implemented | Rewrites `prompt.md`, refreshes bounded `.brevity\context` files from configured vault memory, updates prompt refresh metadata, emits `brevity.command-result.v1`, and launches no provider/worker. |
| `go run ./cmd/brevity task context refresh <slug> [--json]` | Native state action | Go native refresh service | Mutating | Compatibility alias | Legacy command shape routed to the same Go service; PowerShell remains reference/fallback only. |
| `go run ./cmd/brevity task runtime-info <slug>` | Native read-only inspection | Go `.brevity/tasks.json` + `.brevity/runs.jsonl` reader | Read-only | Implemented | Displays task runtime details without PowerShell. |
| `go run ./cmd/brevity task detail <slug>` | Native read-only inspection | Go `.brevity/tasks.json` + `.brevity/runs.jsonl` reader | Read-only | Implemented | Alias-style focused task detail view with metadata, worktree, prompt, provider/profile, latest run, and operator interpretation. |
| `go run ./cmd/brevity task runs <slug>` | Native read-only inspection | Go `.brevity/runs.jsonl` reader | Read-only | Implemented | Displays recent run-index records for one task without PowerShell. |
| `go run ./cmd/brevity task run <slug> --plan --json [--profile <profile>]` | Native execution planning | Go task-run plan service | Read-only | Implemented | Emits `brevity.command-result.v1` with a native `brevity.task-run-plan.v1` execution envelope: provider/profile, prompt freshness, planned run/log paths, argv command shape, expected future mutations/files, warnings, and blockers. It never launches providers/workers and never writes `.brevity\runs.jsonl`. |
| `go run ./cmd/brevity task run <slug> --plan [--profile <profile>]` | Native execution planning | Go task-run plan service | Read-only | Implemented | Human rendering of the same native execution envelope; exits non-zero when plan blockers exist. |
| `go run ./cmd/brevity task run <slug> --execute [--profile <profile>] [--smoke]` | Native provider execution | Go preflight + task-run plan + `os/exec` + `.brevity/state.lock` | Mutating | Implemented | Requires the native plan to be unblocked, executes the planned provider argv without shell concatenation, writes `.brevity/logs/<slug>/`, appends `.brevity/runs.jsonl`, updates task runtime metadata, and emits `brevity.command-result.v1`. Tests use fake providers only. |
| `go run ./cmd/brevity task runs reconcile --dry-run` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Reports stale or incomplete run records; dry-run only. |
| `go run ./cmd/brevity task runs retention --dry-run` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Reports run-index retention signals; dry-run only. |
| `go run ./cmd/brevity task runs compact --dry-run` | Read-only inspection | PowerShell command-result JSON | Read-only | Implemented | Reports a compaction plan; dry-run only. |
| `go run ./cmd/brevity cleanup inspect` | Native cleanup inspection | Go task store + read-only Git/worktree inspector | Read-only | Implemented | Reports missing worktrees, orphan task worktrees, orphan task branches, dirty worktrees, and stale runs; explicitly executes no cleanup. |
| `go run ./cmd/brevity cleanup inspect --json` | Native cleanup inspection | Go task store + read-only Git/worktree inspector | Read-only | Implemented | Emits stable `brevity.cleanup-inspection.v1` JSON with summary counts and candidate classification. |
| `go run ./cmd/brevity cleanup plan <candidate-id> --json` | Native orphan cleanup planning | Go cleanup detector + read-only Git inspector | Read-only | Implemented | Emits `brevity.command-result.v1` with `brevity.orphan-cleanup-plan-set.v1` and per-candidate `brevity.orphan-cleanup-plan.v1` plans. Reports dirty, merged, removable, destructive, force, expected argv Git commands, blockers, and warnings without mutation. |
| `go run ./cmd/brevity cleanup plan --all --json` | Native orphan cleanup planning | Go cleanup detector + read-only Git inspector | Read-only | Implemented | Plans all orphan cleanup candidates returned by native inspection. Selected task cleanup remains a separate `task cleanup <slug>` path. |
| `go run ./cmd/brevity cleanup execute <candidate-id> --force --json` | Native orphan cleanup execution | Go cleanup plan + argv `git` | Mutating | Implemented | Requires explicit `--force`, refuses blocked/dirty/unmerged candidates, removes orphan worktrees with `git worktree remove`, deletes orphan branches only with safe `git branch -d`, and never removes selected task metadata. Tests use disposable Git fixtures. |
| `go run ./cmd/brevity cleanup execute --all --force --json` | Native orphan cleanup execution | Go cleanup plan + argv `git` | Mutating | Implemented | Executes only unblocked orphan candidates, skips blocked candidates in all mode, reports partial results, and does not use PowerShell. |
| `go run ./cmd/brevity doctor` | Native read-only diagnostics | Go diagnostic checks | Read-only | Implemented | Checks Git availability, repo and `.brevity` readability, config, provider health, task metadata, runs, locks, vault/worktree paths, and task worktree paths without PowerShell. |
| `go run ./cmd/brevity doctor --json` | Native read-only diagnostics | Go diagnostic checks | Read-only | Implemented | Emits `brevity.command-result.v1` with a stable `brevity.doctor.v1` payload; exits non-zero when error diagnostics exist. |
| `.\brevity.ps1 tui` | PowerShell TUI scaffold | PowerShell runtime state JSON | Read-only | Implemented | Original lightweight operator scaffold; useful as a reference, not the active future frontend direction. |
| Native Go `.brevity` reader | Runtime migration | Go state readers | Read-only | Implemented for current read slice | Provider health, tasks, runs, and worktree scans are available for native runtime state; provider health and task metadata use `internal/state`. |
| Interactive mutation UI | Frontend mutation UI | Future command-result actions | Mutating | Planned/deferred | Bubble Tea has native task actions for existing flows; merge confirmation is still a future UI enrichment. |
| `go run ./cmd/brevity task runs compact --execute --archive` | PowerShell-backed action | PowerShell command-result JSON | Mutating | Planned/deferred | Go exposes only dry-run compaction planning. |
| Background worker lifecycle | Runtime migration | PowerShell today, future Go TBD | Mutating | Planned/deferred | Worker lifecycle remains PowerShell-owned. |

## Documentation Notes

- Go owns init/repair, provider metadata/profile resolution, provider health read/write, runtime queue persistence, runtime scheduler planning, `.brevity/tasks.json` reading and locked
  task-new, task-start, and prompt/context refresh writes,
  `.brevity/runs.jsonl` run-history reading, native runtime-state building,
  native task status, task runtime/detail inspection, doctor diagnostics,
  cleanup/orphan inspection reports, orphan cleanup planning/execution,
  task activation, task spec inspection,
  task mutation preflight gates,
  `task run --plan` execution envelopes, native task merge
  planning/execution, and the Bubble Tea native source.
- PowerShell remains the legacy reference/fallback for historical cleanup
  behavior. Native Go owns selected task cleanup and orphan cleanup execution
  in the CLI; PowerShell is not removed.
- Every Go task mutation must pass native preflight first. The
  `brevity.task-preflight.v1` JSON payload is the contract shared by CLI, TUI,
  and operator flows.
- Native Start task does not create/delete branches, create/delete worktrees, or
  run providers/workers. Native Refresh context writes only the task prompt,
  bounded worktree context files, and task refresh metadata. Native Task run
  writes only provider logs, run-history records, and task runtime metadata.
  Native Task merge writes only Git merge state and task metadata on success; it
  does not cleanup, delete branches, or delete worktrees. Native cleanup is a
  separate explicit command and does not force-delete branches. Orphan cleanup
  also requires explicit `--force`, refuses dirty worktrees and unmerged
  branches, uses safe `git branch -d`, and does not remove selected task
  metadata.
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
