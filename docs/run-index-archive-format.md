# Run Index Archive Format v1

This document defines the planned archive/summary record shape for future
`.brevity\runs.jsonl` compaction. It is a contract for later implementation;
Brevity does not currently mutate, rewrite, truncate, archive, or delete run
index records.

The matching JSON schema lives at
[`docs/run-index-archive.schema.json`](run-index-archive.schema.json).

## Purpose

Archive records are additive summaries for old indexed worker runs. They make
future compaction auditable by recording what was summarized, when it was
summarized, and which safety counts were preserved.

An archive record is not permission to delete raw logs. Worker logs remain the
source of detailed worker output, and v1 compaction must not delete them
automatically.

## Record Shape

Archive records are JSON objects intended to be written as JSON Lines records by
a future explicit compaction command.

Required fields:

- `schema` - contract identifier, currently `brevity.run-index-archive.v1`.
- `recordType` - `run-index-archive`.
- `slug` - task slug whose runs are summarized.
- `archivedAt` - UTC archive timestamp.
- `preservedRunCount` - number of raw run records represented by this summary.
- `oldestRun` - boundary summary for the oldest archived run.
- `newestRun` - boundary summary for the newest archived run.
- `successCount` - summarized succeeded run count.
- `failureCount` - summarized failed run count.
- `staleCount` - summarized stale run count.
- `incompleteCount` - summarized incomplete run count.
- `archivedRunIds` or `archivedRunRanges` - explicit run identity preservation.

Boundary run summaries use this shape:

```json
{
  "runId": "20260519T080000Z-example",
  "startedAt": "2026-05-19T08:00:00Z",
  "finishedAt": "2026-05-19T08:03:00Z",
  "workerStatus": "succeeded",
  "exitCode": 0
}
```

Run ranges are only valid when a future implementation can prove the ordering is
stable and unambiguous. Otherwise, it should prefer explicit `archivedRunIds`.

## Example

```json
{
  "schema": "brevity.run-index-archive.v1",
  "recordType": "run-index-archive",
  "slug": "example-task",
  "archivedAt": "2026-05-19T12:00:00Z",
  "preservedRunCount": 12,
  "oldestRun": {
    "runId": "20260501T090000Z-example-task",
    "startedAt": "2026-05-01T09:00:00Z",
    "finishedAt": "2026-05-01T09:04:00Z",
    "workerStatus": "succeeded",
    "exitCode": 0
  },
  "newestRun": {
    "runId": "20260510T140000Z-example-task",
    "startedAt": "2026-05-10T14:00:00Z",
    "finishedAt": "2026-05-10T14:07:00Z",
    "workerStatus": "failed",
    "exitCode": 1
  },
  "successCount": 10,
  "failureCount": 2,
  "staleCount": 0,
  "incompleteCount": 0,
  "archivedRunIds": [
    "20260501T090000Z-example-task",
    "20260502T090000Z-example-task"
  ],
  "archivedRunRanges": []
}
```

## Safety Rules

Future mutating compaction must:

- Preserve at least the latest configured N indexed runs per task.
- Treat archive records as additive summaries, not silent deletion.
- Keep raw worker logs untouched in v1.
- Refuse or warn before summarizing stale or incomplete runs.
- Preserve stale and incomplete counts when such records are intentionally
  summarized after reconciliation.
- Run dry-run-first and take the run-index lock before writing.

Until mutation exists, `Brevity task runs compact --dry-run` remains plan-only
and should only report archive/summary candidates.

## Planned Mutation Command

Mutating run-index compaction is not implemented yet. The planned explicit
command shape is:

```powershell
.\brevity.ps1 task runs compact --execute
.\brevity.ps1 task runs compact --execute --archive
```

The `--execute` flag is the future mutation boundary. Without it, compaction
must remain read-only. The optional `--archive` flag would allow the command to
write additive archive records using this format while compacting old raw run
records from `.brevity\runs.jsonl`.

Compaction must never run automatically. A TUI or automation consumer must show
the exact action, target path, retention effect, archive behavior, and backup
path before invoking an executing command.

## Planned Mutation Sequence

A future implementation must perform mutation in this order:

1. Acquire `.brevity\runs.lock`.
2. Reread `.brevity\runs.jsonl` while holding the lock.
3. Recompute the compaction plan from the locked read, ignoring any stale
   pre-lock dry-run plan.
4. Create a timestamped backup of the current `.brevity\runs.jsonl`.
5. Write the compacted run index to a temporary file in the same directory.
6. Validate that the compacted temporary file parses as JSON Lines.
7. If archive records are produced, validate every archive record against
   `brevity.run-index-archive.v1`.
8. Atomically replace `.brevity\runs.jsonl` with the temporary file when
   possible on Windows.
9. Preserve the backup path in the command result.
10. Release `.brevity\runs.lock` in a `finally` block.

The locked reread and recomputed plan are required because `.brevity\runs.jsonl`
is append-only worker runtime state. A dry-run report is advisory; it is not a
write plan that can safely be replayed later.

## Rollback And Failure Behavior

Future mutating compaction must fail closed:

- Never delete backups automatically.
- If validation fails, abort before replacing `.brevity\runs.jsonl`.
- If replacement fails, the original `.brevity\runs.jsonl` should remain in
  place.
- If archive validation fails, abort before replacing `.brevity\runs.jsonl`.
- Preserve the backup path and failure details in the command result whenever a
  backup was created.
- Leave worker logs untouched; logs are never deleted by v1 compaction.

Stale and incomplete records must be preserved in the live run index unless a
future reconciliation flow has explicitly resolved them. Failed runs are
retained longer than successful runs, and v1 compaction should only compact old
successful completed runs that the retention plan can safely summarize.
