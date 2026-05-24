# Execution Operator Guide

This guide describes the manual, foreground execution pipeline for one queued
Brevity task. The pipeline connects the Go-native queue, scheduler, execution
record, preflight, launch preview, real provider launch, run-history append,
and queue finalization steps without introducing automatic orchestration.

Use this flow when an operator wants to run exactly one queued task through the
runtime execution path and observe every boundary before launch. It is useful
for validating queue state, scheduler selection, execution planning, provider
argv construction, and terminal launch behavior.

Do not use this flow as a scheduler loop, daemon, queue drainer, retry system,
merge system, or task-status workflow. Each command is explicit, foreground,
and bounded to one selected item.

## Full Pipeline

```powershell
brevity queue add <task>
brevity queue inspect
brevity queue plan
brevity scheduler plan
brevity scheduler reserve-next
brevity scheduler plan-execution
brevity execution list
brevity execution mark-ready <execution-id>
brevity execution preflight <execution-id>
brevity execution launch-dry-run <execution-id>
brevity execution launch <execution-id>
brevity execution inspect
brevity queue inspect
```

## Command Steps

`brevity queue add <task>` appends one queued item for the task. It writes
`.brevity\runtime-queue.json`. It does not reserve, plan, or launch anything.

`brevity queue inspect` reads queue file health, counts, reservations, and
warnings. It is read-only.

`brevity queue plan` reads the queue and reports runnable and skipped items in
queue-file order. It is read-only and does not reserve ownership.

`brevity scheduler plan` consumes the queue plan and selects the single first
eligible runnable item the runtime would claim next. It is read-only.

`brevity scheduler reserve-next` reserves that one selected queue item. It
mutates only reservation metadata in `.brevity\runtime-queue.json`.

`brevity scheduler plan-execution` creates one execution record for the
reserved scheduler item. It writes `.brevity\runtime-executions.json` and does
not launch the provider.

`brevity execution list` reads current execution records. It is read-only and
helps the operator copy the execution id for later steps.

`brevity execution mark-ready <execution-id>` transitions one execution record
from `planned` to `ready`. It writes `.brevity\runtime-executions.json`.

`brevity execution preflight <execution-id>` checks that the ready execution
still points at a matching reserved queue item and task. It is read-only and
does not create run history.

`brevity execution launch-dry-run <execution-id>` runs the same preflight checks
and prints the provider/profile/worktree/prompt/argv launch intent. It is
read-only and does not start a provider process.

`brevity execution launch <execution-id>` launches exactly one ready execution
in the foreground. It starts one provider process with argv-style execution,
streams output, records the terminal execution state, appends run history, and
finalizes the matching queue item.

`brevity execution inspect` reads execution file health and status counts after
launch. It is read-only.

`brevity queue inspect` verifies the final queue boundary. Completed launches
remove the item. Failed launches leave it queued and unreserved.

## State Boundaries

Queue state lives in:

- `.brevity\runtime-queue.json`

Queue add and reservation commands may mutate this file. Queue inspect and
queue plan must not mutate it. Execution launch may finalize only the matching
queue item after terminal execution state.

Execution state lives in:

- `.brevity\runtime-executions.json`

Execution planning, readiness transitions, and launch status transitions may
mutate this file. List, inspect, preflight, and launch dry-run are read-only.

Run history lives in:

- `.brevity\runs.jsonl`

Execution launch appends one observational run-history entry after the provider
launch reaches terminal state. Failed preflight before provider launch does not
append a launch-history row.

Task metadata is separate runtime workflow state. Execution launch should not
mutate task metadata, task workflow status, task branches, task worktree
classification, merge state, or cleanup state.

## Success Behavior

When `brevity execution launch <execution-id>` succeeds:

- the execution becomes `completed`
- one run-history entry is appended to `.brevity\runs.jsonl`
- the completed queue item is removed from `.brevity\runtime-queue.json`
- task workflow state is unchanged

## Failure Behavior

When `brevity execution launch <execution-id>` fails after launch begins:

- the execution becomes `failed`
- one run-history entry is appended to `.brevity\runs.jsonl`
- the queue item remains `queued`
- the queue reservation is cleared
- task workflow state is unchanged
- no automatic retry occurs

If preflight fails before provider launch begins, launch does not start the
provider process and does not append a launch-history row.

## Intentionally Separate

The manual execution pipeline is deliberately separate from:

- scheduler loops
- queue auto-drain
- daemon or background execution
- parallel provider execution
- automatic retries
- auto-merge
- auto-cleanup
- task status mutation
- task workflow transitions
- branch integration
- worktree cleanup

Those behaviors require their own explicit contracts before they can exist in
Brevity.

## Safety Invariants

The pipeline must preserve these invariants:

- no scheduler loop
- no queue auto-drain
- no daemon/background execution
- no parallelism
- no auto-merge
- no auto-cleanup
- no task status mutation

Manual launch is a single explicit foreground provider run for one ready
execution id. The queue, execution record, run history, and task workflow state
remain separate boundaries.
