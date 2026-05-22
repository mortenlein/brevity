# Runtime Queue Planning

`brevity queue plan` is Brevity's first read-only planning view over
`.brevity\runtime-queue.json`.

Planning answers:

- what queue items would be considered next
- why each item is runnable or skipped
- the queue-file order Brevity would observe today

Planning is not execution. It does not run providers, spawn workers, start the
runtime supervisor, drain the queue, update task state, reserve ownership, or
rewrite queue files.

## Command

```powershell
go run ./cmd/brevity queue plan
go run ./cmd/brevity queue plan --json
```

The human output is intended for operators. The JSON output emits the compact
`brevity.runtime-queue-plan.v1` shape for tools that need the same read-only
summary.

## Runnable Semantics

A queue item is runnable when all of these are true:

- `status` is `queued`
- `task` is a valid Brevity task slug
- `id` is present
- the queue id is not duplicated by another item
- timestamps are valid enough to trust the item record
- no reservation metadata is present

Runnable items are reported in queue-file order. Brevity v1 does not apply
priority, provider concurrency, retry, dependency, cooldown, or scheduling
rules.

## Skipped Semantics

A queue item is skipped when it cannot be safely considered runnable during this
observational phase. Current skip reasons include:

- missing id
- duplicate queue id
- invalid task slug
- unsupported status
- invalid timestamp fields
- invalid reservation metadata
- valid reservation metadata, reported as `reserved by <owner>`

Skipped queue items are infrastructure warnings, not task failures.
Reserved queue items are not considered runnable because another orchestration
component has explicitly claimed intended execution ownership. That claim is
metadata only; it does not mean execution has started.

## Corrupted Or Future Queue Files

If the queue file is missing, the plan is an empty queue. If the queue file is
empty, malformed JSON, or unreadable, the plan reports the file as corrupted or
invalid and returns no runnable items.

If the queue file has an unsupported future version, Brevity refuses to infer
execution order from it. The plan reports the version mismatch and returns no
runnable items.

## Read-Only Guarantees

`queue plan` does not acquire execution ownership and does not mutate:

- `.brevity\runtime-queue.json`
- `.brevity\runtime.json`
- `.brevity\tasks.json`
- `.brevity\runs.jsonl`
- task worktrees or branches
- provider state

It is safe to run while the supervisor is stopped or missing.

## Non-Goals

Runtime queue planning v1 deliberately does not implement:

- scheduler loops
- queue draining
- provider execution
- worker spawning
- provider concurrency
- retries
- dependency graphs
- distributed orchestration
- provider cooldown logic

## Future Scheduler Relationship

A future scheduler may consume the planning layer as one input, but that future
scheduler must have its own explicit execution contract. Until then, queue
planning remains observational only.
