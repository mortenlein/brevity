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

Provider execution will be introduced by a later contract. That future layer can
consume planned execution records, but it must add its own explicit state
transition and safety checks.

## Lifecycle

The v1 lifecycle is intentionally tiny:

- `planned`: the runtime intends to execute the reserved queue item later.
- `cancelled`: the planned intent was cancelled before provider execution was
  introduced or started.

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

`execution list` and `execution inspect` are read-only. `execution
plan-from-reservation` writes only `.brevity\runtime-executions.json` and its
advisory lock file.

## Non-Goals

This contract does not implement planner automation, provider scheduling,
worker lifecycle, retries, queue draining, task mutation, run history, or TUI
controls.

## Future Scheduler Relationship

The scheduler can use this contract as the durable boundary between reservation
ownership and actual execution. A future scheduler execution command can reserve
a queue item, create a planned execution record, and then explicitly transition
into provider execution through a separate contract.

Until that future contract exists, planned execution records are inert runtime
metadata.
