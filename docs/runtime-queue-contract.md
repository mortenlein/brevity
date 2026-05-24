# Runtime Queue Contract

`.brevity\runtime-queue.json` is Brevity's first durable runtime queue state
file. It records operator intent to queue work for a future runtime, but it does
not execute that work.

The queue is infrastructure state. It is not task state, and queue membership
must not be interpreted as task lifecycle progress.

## File Shape

```json
{
  "version": 1,
  "items": [
    {
      "id": "20260522-abc123",
      "task": "some-task-slug",
      "provider": "gemini",
      "profile": "default",
      "status": "queued",
      "createdAt": "2026-05-22T12:00:00Z",
      "updatedAt": "2026-05-22T12:00:00Z",
      "reservation": {
        "owner": "runtime-supervisor",
        "reservedAt": "2026-05-22T12:01:00Z",
        "reservationId": "res-20260522T120100-abc12345"
      }
    }
  ]
}
```

## Fields

- `version` is the file contract version. The current version is `1`.
- `items` is an ordered list of queued runtime items.
- `id` is a stable queue item identifier generated when the item is added.
- `task` is the task slug supplied by the operator.
- `provider` is the provider selected for future execution metadata. It does
  not imply provider execution.
- `profile` is the future worker profile metadata. The initial default is
  `default`.
- `status` is the queue item status.
- `createdAt` and `updatedAt` are UTC timestamps.
- `reservation` is optional orchestration metadata for explicit execution
  ownership intent. Missing `reservation` means the item is unreserved.
- `reservation.owner` identifies the reserving component.
- `reservation.reservedAt` is the UTC reservation timestamp.
- `reservation.reservationId` is a stable id for this reservation event.

## Statuses

The v1 contract allows only:

- `queued`
- `cancelled`

Do not add `running`, `completed`, or `failed` until queue execution exists and
has a separate contract update.

Manual execution launch finalization does not add queue statuses. When
`brevity execution launch <execution-id>` reaches terminal execution state, a
completed launch removes the corresponding queue item. A failed launch keeps the
queue item present with `status: queued` and clears reservation metadata so an
operator can explicitly re-reserve or retry later.

## Lifecycle

`brevity queue add <task>` appends an item with status `queued` and persists the
file atomically. `brevity queue reserve <id>` adds optional reservation metadata
to one item. `brevity queue unreserve <id>` clears reservation metadata from one
item and tolerates items that are already unreserved. `brevity queue list` reads
and prints the queue. `brevity queue inspect` reads queue infrastructure
diagnostics. `brevity queue plan` explains read-only runnable/skipped candidate
ordering. `brevity queue remove <id>` removes one queue item by queue item id.

These commands do not drain the queue, run providers, spawn workers, start the
supervisor, imply execution started, or mutate task execution state. Reserve and
unreserve mutate only queue reservation metadata.

`brevity execution launch <execution-id>` may also finalize one corresponding
queue item after the execution reaches `completed` or `failed`. This is
infrastructure cleanup only: it does not mark tasks done or failed, does not
auto-retry, does not launch another item, and does not start scheduler or
supervisor loops.

## Inspection

`brevity queue inspect` is read-only operator visibility. It reports:

- queue file path
- queue file state: `missing`, `valid`, `corrupted`, or `invalid`
- queue version and supported version
- total item count
- count by status
- reserved item count
- oldest and newest queued item age
- duplicate queue item ids
- invalid item fields
- invalid reservation metadata
- unsupported future version warning

`brevity queue inspect --json` emits the same diagnostics as a compact
machine-readable object. The JSON shape is intentionally small and diagnostic;
operators should not treat it as a scheduling contract.

Inspection must tolerate a missing file, an empty queue, corrupted JSON,
unsupported future versions, duplicate ids, and invalid item fields. It must not
repair, normalize, rewrite, or otherwise mutate `.brevity\runtime-queue.json`.
Invalid queue infrastructure state is not task failure.

## Planning

`brevity queue plan` is read-only candidate planning. It determines which items
would be considered runnable under the intentionally small v1 rules and which
items are skipped with a reason. Runnable items require status `queued`, a valid
task slug, a present non-duplicated queue id, valid timestamp fields, and no
reservation. Reserved items are skipped with a reason such as `reserved by
runtime-supervisor`.

Planning does not introduce scheduling policy. It preserves queue-file order and
does not apply priorities, provider concurrency, retries, dependency graphs, or
provider cooldowns.

`brevity queue plan --json` emits a compact machine-readable
`brevity.runtime-queue-plan.v1` summary. The planning semantics and non-goals
are documented in
[`docs/runtime-queue-planning.md`](runtime-queue-planning.md).

## Ownership And Locking

The Go runtime owns `.brevity\runtime-queue.json`. Mutating queue commands must
use a native advisory queue lock and atomic JSON replacement through the native
state store. Read-only listing and inspection do not acquire the mutation lock
and must not rewrite the file.

PowerShell may remain a compatibility reference, but new queue behavior must be
Go-native.

## Relationship To Supervisor

The runtime queue can exist while the supervisor is stopped or missing. The
supervisor must not be required for `queue add`, `queue list`, `queue inspect`,
`queue plan`, `queue reserve`, `queue unreserve`, or `queue remove`.

The current supervisor foundation is observational and must not drain
`.brevity\runtime-queue.json`.

## Safety Invariants

- Queue state is infrastructure state, not task state.
- Adding to the queue must not execute anything.
- Listing the queue must be read-only.
- Inspecting the queue must be read-only.
- Inspecting the queue is not scheduling.
- Inspecting the queue is not execution.
- Inspecting the queue must not start the runtime supervisor.
- Inspecting the queue must not repair or rewrite queue files automatically.
- Planning the queue is not execution.
- Planning the queue must not reserve ownership.
- Planning the queue must not mutate or drain the queue.
- Reserving a queue item is not execution.
- Reserving a queue item must not launch providers or workers.
- Reserving a queue item must not mutate task execution state.
- Reserving a queue item must not imply successful scheduling.
- Unreserving a queue item must mutate only queue reservation metadata.
- The queue does not require the supervisor to be running.
- Missing `.brevity\runtime-queue.json` is an empty queue.
- Corrupted queue JSON must be reported safely with a clear error.
- Invalid queue infrastructure state is not task failure.
- Dry-run paths must never drain the queue.
- Provider failures are not queue corruption.
- Queue remove deletes only the matching queue item id.
- Completed execution launch removes only the matching queue item id.
- Failed execution launch keeps the matching item queued and unreserved.
- Queue finalization must not mutate task workflow state.

## Non-Goals

Runtime queue v1 does not implement scheduling, background execution,
supervisor draining, worker spawning, provider retries, provider fallback,
task-state mutation, task completion, or automatic cleanup.
