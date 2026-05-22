# Runtime Queue Reservations

Queue reservations are optional metadata on `.brevity\runtime-queue.json`
items. A reservation means:

```text
This queue item is intended for execution ownership.
```

It does not mean the item is executing.

## Shape

```json
{
  "reservation": {
    "owner": "runtime-supervisor",
    "reservedAt": "2026-05-22T12:00:00Z",
    "reservationId": "res-20260522T120000-abc12345"
  }
}
```

The `reservation` field is optional. Existing queue files without it remain
valid.

## Lifecycle

`brevity queue reserve <id>` adds reservation metadata to one valid queue item,
updates `updatedAt`, and writes the queue atomically under the queue lock. It
rejects missing items, invalid queue files, and items that already have a
reservation.

`brevity queue unreserve <id>` clears reservation metadata from one queue item,
updates `updatedAt` only when metadata was present, and writes atomically. It
tolerates items that are already unreserved.

## Ownership Model

`owner` names the orchestration component claiming intended execution ownership.
The current explicit owner is `runtime-supervisor`. Ownership is advisory
metadata for the future scheduler; it is not a process lock, worker handle, run
record, or task lifecycle state.

`reservationId` is stable for the reservation event. Clearing and reserving
again creates a new reservation event.

## Planning And Visibility

`brevity queue plan` remains read-only. Reserved items are excluded from the
runnable list and reported in the skipped list with a reason such as `reserved
by runtime-supervisor`.

`brevity queue inspect` reports reserved counts and invalid reservation
metadata. The Bubble Tea dashboard surfaces the reserved count and reservation
skip reason through its compact queue summary.

## Non-Goals

Reservations do not:

- execute providers
- launch workers
- start the supervisor
- drain the queue
- mutate `.brevity\tasks.json`
- write `.brevity\runs.jsonl`
- imply execution started
- imply successful scheduling

## Scheduler Relationship

A future scheduler may use reservations to make queue ownership explicit before
execution. That scheduler still needs a separate execution contract before any
provider or worker process is launched.

Until that contract exists, reservations are orchestration intent only.
