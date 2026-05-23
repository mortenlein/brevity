# Runtime Execution Record Contract

`.brevity\runtime-executions.json` is the first durable runtime execution
intent contract. It records that the runtime intends to execute a reserved queue
item later.

An execution record is not provider execution. It does not mean a provider has
started, a worker has been spawned, task state has changed, run history has been
created, or work has succeeded or failed.

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

Execution records do not launch Codex, Gemini, Copilot, or any worker process.
They do not create logs, append `.brevity\runs.jsonl`, change task metadata, or
mark a task as running, succeeded, or failed.

Launch dry-run is still not provider execution. It is the final preview layer
between `ready` execution records and a later provider launch contract. Provider
execution will be introduced by a separate contract with its own explicit state
transition and safety checks.

## Lifecycle

The v1 lifecycle is intentionally tiny:

- `planned`: the runtime intends to execute the reserved queue item later, but
  the record has not yet passed pre-execution eligibility checks.
- `ready`: the planned execution record has passed pre-execution checks and is
  eligible for a future provider launch.
- `cancelled`: the planned intent was cancelled before provider execution was
  introduced or started.

`ready` is not provider running. It does not mean a provider process has
started, a worker exists, logs have been created, or task state has changed.
`ready` also does not mean preflight has passed. A ready execution is only
launch-eligible after `execution preflight <execution-id>` confirms the queue
reservation and task linkage still match at the moment of checking.

No other statuses are valid in v1.

Future provider execution work may add statuses such as running, succeeded, or
failed through a new or expanded contract. They are deliberately absent here.

## Safety Invariants

Execution records must never:

- launch providers
- spawn workers
- start the supervisor implicitly
- drain the queue
- mutate task execution state
- create run history
- mark success or failure
- introduce running, completed, succeeded, or failed statuses

`execution list`, `execution inspect`, `execution preflight`, and
`execution launch-dry-run` are read-only. `execution plan-from-reservation`,
`execution mark-ready`, and `execution mark-planned` write only
`.brevity\runtime-executions.json` and its advisory lock file.

## Non-Goals

This contract does not implement planner automation, provider scheduling,
worker lifecycle, retries, queue draining, task mutation, run history, or TUI
controls.

## Future Scheduler Relationship

The scheduler can use this contract as the durable boundary between reservation
ownership and actual execution. `scheduler plan-execution` now connects
scheduler reserved-item selection to planned execution metadata. A future
scheduler execution command can reserve a queue item, create a planned execution
record, and then explicitly transition into provider execution through a
separate contract.

Until that future contract exists, planned and ready execution records are inert
runtime metadata.
