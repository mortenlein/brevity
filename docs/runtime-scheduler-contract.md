# Runtime Scheduler Contract

`brevity scheduler plan` is the first Go-native runtime scheduler contract.
It decides which single queue item the runtime would claim next, explains why,
and reports whether that item is eligible for reservation.

`brevity scheduler reserve-next` reserves exactly one selected item from that
same plan. It is reservation only. Neither command is provider execution.

## Command

```powershell
go run ./cmd/brevity scheduler plan
go run ./cmd/brevity scheduler plan --json
go run ./cmd/brevity scheduler reserve-next
```

The JSON contract uses schema `brevity.runtime-scheduler-plan.v1`.

## Queue Plan vs Scheduler Plan

`brevity queue plan` answers: what queue items are runnable?

`brevity scheduler plan` answers: what one queue item would the runtime claim
next, and why?

The scheduler reuses the queue planner for tolerant queue reading, validation,
runnable classification, skipped item reasons, duplicate detection, reservation
visibility, and queue-order preservation. It does not duplicate queue planning
rules.

## Selection Rules

Scheduler v1 intentionally keeps selection simple:

- The queue file must be missing or valid.
- Missing queue means no selected item.
- Runnable candidates come from the existing queue plan.
- The selected item is the first runnable item in queue-file order.
- Runnable means queued, unreserved, valid, supported by the queue planner, and
  carrying a valid task slug.

Skipped items remain skipped when they are invalid, duplicate ids, reserved,
cancelled, use unsupported statuses, contain invalid reservations, appear in a
corrupted queue file, or belong to a future unsupported queue version.

## Reservation Relationship

Scheduler planning reports reservation eligibility for the selected item. A
selected item is eligible when it is queued, runnable, unreserved, and has a
valid task slug.

Planning does not reserve the item. Reservations remain explicit orchestration
ownership intent and do not imply execution.

`brevity scheduler reserve-next` computes the scheduler plan, reserves the
selected item through the existing queue reservation path, and prints the queue
item id, task slug, and reservation id. If no item is selected, it fails without
mutating queue or task state.

## Non-Goals

Scheduler v1 does not introduce:

- provider execution
- worker execution
- runtime supervisor startup
- queue draining
- task lifecycle mutation
- run history creation
- priority
- provider concurrency
- provider cooldowns
- retries
- dependency graphs
- parallel scheduling
- fairness
- distributed leases

## Future Execution Relationship

Future execution may consume a scheduler decision, reserve the selected item,
and then hand off to a separate execution contract. That future layer must keep
reservation, task state transitions, run history, provider execution, and
cleanup boundaries explicit.

Scheduler planning itself remains read-only.

## Safety Invariants

`brevity scheduler plan` must never:

- launch providers
- launch workers
- start or stop the supervisor
- mutate task execution state
- mutate queue state
- reserve queue items
- drain the queue
- create run history
- imply a task has started

If an explicit reservation command is added later, reservation must still not
imply provider execution or task start.

`brevity scheduler reserve-next` may mutate only queue reservation metadata. It
must never launch providers, launch workers, start the supervisor, drain the
queue, mutate task execution state, create run history, or imply a task has
started.
