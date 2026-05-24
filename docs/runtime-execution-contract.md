# Runtime Execution Record Contract

`.brevity\runtime-executions.json` is the durable runtime execution contract.
It records that the runtime intends to execute a reserved queue item and now
tracks the first explicit manual foreground provider launch path.

An execution record is not automatic scheduling. A record does not imply the
scheduler will run, the queue will drain, task state will change, run history
will be created, retries will occur, or background orchestration exists.

## File

```json
{
  "version": 1,
  "executions": [
    {
      "id": "exec-20260522T120000-abc123ef",
      "queueItemId": "queue-abc123",
      "task": "some-task",
      "reservationId": "res-abc123",
      "status": "planned",
      "createdAt": "2026-05-22T12:00:00Z",
      "updatedAt": "2026-05-22T12:00:00Z"
    }
  ]
}
```

The current version is `1`. The file lives under `.brevity` next to the runtime
queue and supervisor state.

## Commands

```powershell
go run ./cmd/brevity execution list
go run ./cmd/brevity execution inspect
go run ./cmd/brevity execution inspect --json
go run ./cmd/brevity execution plan-from-reservation <queue-item-id>
go run ./cmd/brevity execution mark-ready <execution-id>
go run ./cmd/brevity execution mark-planned <execution-id>
go run ./cmd/brevity execution preflight <execution-id>
go run ./cmd/brevity execution preflight <execution-id> --json
go run ./cmd/brevity execution launch-dry-run <execution-id>
go run ./cmd/brevity execution launch-dry-run <execution-id> --json
go run ./cmd/brevity execution launch <execution-id>
go run ./cmd/brevity execution launch <execution-id> --json
go run ./cmd/brevity scheduler plan-execution
```

`execution list` reads `.brevity\runtime-executions.json`, tolerates a missing
file as an empty set, reports corrupted JSON safely, and remains read-only.

`execution inspect` reports the execution file path, file health, version, total
execution count, status counts, duplicate ids, invalid record warnings, and
parse/version errors. It remains read-only.

`execution plan-from-reservation <queue-item-id>` creates exactly one planned
execution record for an already reserved queue item. It requires the queue item
to exist, requires reservation metadata with a `reservationId`, rejects a
duplicate execution for the same queue item and reservation, and writes
`.brevity\runtime-executions.json` atomically.

`scheduler plan-execution` reaches the same execution planning path through the
scheduler layer. It computes the scheduler plan, requires a reserved scheduler
candidate, creates one planned execution record, prints the queue item id, task
slug, reservation id, and execution id, and rejects duplicate planning for the
same reservation.

`execution mark-ready <execution-id>` transitions one execution record from
`planned` to `ready`, updates `updatedAt`, and writes
`.brevity\runtime-executions.json` atomically. It prints the execution id, task,
old status, and new status. It is metadata-only pre-execution eligibility.

`execution mark-planned <execution-id>` transitions one execution record from
`ready` back to `planned`, updates `updatedAt`, and writes atomically. It is a
metadata-only rollback and does not affect queue or task state.

`execution preflight <execution-id>` is the final read-only check layer before a
future provider launch contract. It loads `.brevity\runtime-executions.json`,
finds the execution record, requires `status: ready`, loads
`.brevity\runtime-queue.json`, verifies the referenced queue item still exists,
verifies the queue item is still reserved, verifies the reservation id matches
the execution record, verifies the queue task still matches the execution task,
and reports whether the ready execution is launch-eligible.

Preflight does not write any runtime file. It does not start providers, spawn
workers, start the supervisor, drain the queue, mutate task state, create run
history, or mark an execution running, succeeded, or failed.

`execution launch-dry-run <execution-id>` is the provider launch preparation
contract. It first runs the same read-only execution preflight checks, then
loads task metadata, resolves provider/profile configuration, resolves the
expected worktree and prompt paths, and builds the argv-style provider launch
intent that a later launch contract may consume.

Launch dry-run is payload preparation only. It prints what would be launched,
including provider, profile, worktree, prompt, command argv, and dry-run mode.
It does not call provider binaries, create subprocesses, start workers, start
the supervisor, mutate queue/task/execution state, create run history, or add
running/succeeded/failed statuses.

`execution launch <execution-id>` is the first real provider execution path. It
is manual, foreground-only, and single-execution-only. It loads the execution
record, requires `status: ready`, runs execution preflight, resolves the same
provider/profile/worktree/prompt context as launch dry-run, builds the same
argv-style provider launch payload, starts exactly one provider process, streams
provider stdout/stderr live to the console, captures the exit code, and prints
the final execution state.

Launch uses argv-style process execution. It must not shell-concatenate
commands, use `Invoke-Expression`, use `cmd /c` as a general execution wrapper,
start scheduler loops, drain the queue, mutate queue status, mutate task
workflow state, add retries, launch providers in parallel, or create daemon,
background, or distributed orchestration.

Launch mutates `.brevity\runtime-executions.json` through the execution lock and
appends one observational row to `.brevity\runs.jsonl` after the launch reaches
a terminal execution status. Queue semantics remain separate.

The launch run-history row is not task workflow state. It records the execution
id, queue item id, task slug, provider/profile, argv-style command when safely
available, timestamps, exit code, final execution status, and a short error
summary for failed launches or nonzero provider exits. It must not mark tasks
done or failed, clear reservations, dequeue items, or imply scheduler progress.
If preflight fails before a provider launch begins, no launch history row is
written.

Human output reports each check clearly:

```text
EXECUTION PREFLIGHT
Execution: exec-20260522T120000-abc123ef
Task: some-task
Status: ready

Checks:
- execution exists: ok
- status ready: ok
- queue item exists: ok
- queue item has reservation: ok
- reservation matches: ok
- task matches: ok
- execution status is launch-eligible: ok

Result: passed
```

Compact JSON output is available with `--json`:

```json
{"executionId":"exec-20260522T120000-abc123ef","task":"some-task","status":"ready","passed":true,"checks":[{"name":"execution exists","passed":true}]}
```

Launch dry-run JSON is compact and uses argv-style command arrays:

```json
{"executionId":"exec-20260522T120000-abc123ef","task":"some-task","status":"ready","launchEligible":true,"provider":"gemini","profile":"default","command":["gemini","-p","C:\\worktrees\\active\\some-task\\prompt.md"],"checks":[{"name":"execution exists","passed":true}]}
```

## Reservation vs Execution Plan

A queue reservation records orchestration ownership intent on
`.brevity\runtime-queue.json`. It means the scheduler or operator has claimed a
queue item so another scheduler pass should not choose it.

An execution plan records a second, explicit runtime intent:

> the runtime intends to execute this reserved queue item

The reservation is still not execution. The execution plan is also still not
execution. Together they only create durable handoff state for a future scheduler
execution loop.

## Execution Plan vs Provider Execution

Execution planning does not launch Codex, Gemini, Copilot, or any worker
process. It does not create logs, append `.brevity\runs.jsonl`, change task
metadata, or mark a task as running, succeeded, or failed.

Launch dry-run is still not provider execution. It is the final preview layer
between `ready` execution records and `execution launch`.

Provider execution occurs only through `execution launch <execution-id>`, and
only for that one named ready execution record.

## Lifecycle

The v1 lifecycle is intentionally tiny:

- `planned`: the runtime intends to execute the reserved queue item later, but
  the record has not yet passed pre-execution eligibility checks.
- `ready`: the planned execution record has passed pre-execution checks and is
  eligible for a future provider launch.
- `launching`: the provider launch path has started and the provider process is
  being invoked for this single foreground execution.
- `completed`: the provider process exited successfully with exit code `0`.
- `failed`: provider launch failed or the provider process exited nonzero.
- `cancelled`: the planned intent was cancelled before provider execution
  started.

`ready` is not provider running. It does not mean a provider process has
started, a worker exists, logs have been created, or task state has changed.
`ready` also does not mean preflight has passed. A ready execution is only
launch-eligible after `execution preflight <execution-id>` confirms the queue
reservation and task linkage still match at the moment of checking.

The launch state transition is:

```text
ready -> launching -> completed
ready -> launching -> failed
```

Every transition updates `updatedAt`, writes atomically, and uses the execution
lock. A failed preflight leaves the record unchanged. A provider launch failure
or nonzero provider exit marks the execution `failed`.

No other statuses are valid in v1. Do not add retries, paused, abandoned,
resumed, timeout, killed, or provider-pool states.

## Safety Invariants

Execution launch must never:

- start the supervisor implicitly
- drain the queue
- mutate task execution state
- treat launch history as task workflow state
- add retries
- run multiple executions
- introduce background or daemon orchestration
- introduce distributed coordination

`execution list`, `execution inspect`, `execution preflight`, and
`execution launch-dry-run` are read-only. `execution plan-from-reservation`,
`execution mark-ready`, `execution mark-planned`, and `execution launch` write
only their documented runtime metadata. `execution launch` additionally appends
observational run history; it does not clear or change queue reservations.

## Non-Goals

This contract does not implement planner automation, provider scheduling,
worker lifecycle management, retries, queue draining, task mutation, run
history, TUI controls, provider pools, parallel execution, daemon execution, or
distributed execution.

## Future Scheduler Relationship

The scheduler uses this contract as the durable boundary between reservation
ownership and execution metadata. `scheduler plan-execution` connects scheduler
reserved-item selection to planned execution metadata. It still does not call
`execution launch`, start loops, or auto-drain the queue.
