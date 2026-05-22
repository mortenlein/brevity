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
      "updatedAt": "2026-05-22T12:00:00Z"
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

## Statuses

The v1 contract allows only:

- `queued`
- `cancelled`

Do not add `running`, `completed`, or `failed` until queue execution exists and
has a separate contract update.

## Lifecycle

`brevity queue add <task>` appends an item with status `queued` and persists the
file atomically. `brevity queue list` reads and prints the queue. `brevity queue
remove <id>` removes one queue item by queue item id.

These commands do not drain the queue, run providers, spawn workers, start the
supervisor, or mutate task execution state.

## Ownership And Locking

The Go runtime owns `.brevity\runtime-queue.json`. Mutating queue commands must
use a native advisory queue lock and atomic JSON replacement through the native
state store. Read-only listing does not acquire the mutation lock and must not
rewrite the file.

PowerShell may remain a compatibility reference, but new queue behavior must be
Go-native.

## Relationship To Supervisor

The runtime queue can exist while the supervisor is stopped or missing. The
supervisor must not be required for `queue add`, `queue list`, or `queue
remove`.

The current supervisor foundation is observational and must not drain
`.brevity\runtime-queue.json`.

## Safety Invariants

- Queue state is infrastructure state, not task state.
- Adding to the queue must not execute anything.
- Listing the queue must be read-only.
- The queue does not require the supervisor to be running.
- Missing `.brevity\runtime-queue.json` is an empty queue.
- Corrupted queue JSON must be reported safely with a clear error.
- Dry-run paths must never drain the queue.
- Provider failures are not queue corruption.
- Queue remove deletes only the matching queue item id.

## Non-Goals

Runtime queue v1 does not implement scheduling, background execution,
supervisor draining, worker spawning, provider retries, provider fallback,
task-state mutation, task completion, or automatic cleanup.
