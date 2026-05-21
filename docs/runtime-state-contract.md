# Runtime State Contract

`.\brevity.ps1 runtime state --json` is Brevity's primary read-only runtime
inspection contract. It gives TUI and automation consumers a machine-readable
snapshot of the current orchestration state without requiring them to parse
human console output.

## Schema

The runtime state document uses the schema identifier
`brevity.runtime-state.v1`. The discoverable JSON schema lives at
[`docs/runtime-state.schema.json`](runtime-state.schema.json).

The v1 contract is intended to evolve additively where practical. Producers may
add fields over time, and consumers should tolerate unknown fields. Consumers
should also treat JSON field ordering as insignificant and should not depend on
properties appearing in a particular order.

## Major Sections

- `providers` - provider health summary and per-provider health records.
- `taskCounts` - aggregate task counts by runtime classification.
- `tasks` - runtime task summaries from `.brevity\tasks.json`.
- `groups` - task slug lists grouped by runtime classification.
- `orphanedTaskWorktrees` - task-like active worktrees not tracked in runtime
  task metadata.
- `lock` - task metadata lock status.
- `runtimeMemory` - runtime log metadata and recent runtime-memory entries.
- `suggestedNextActions` - suggested operator actions derived from the current
  snapshot.

## Observational Only

Runtime state is observational only. Reading
`.\brevity.ps1 runtime state --json` should not mutate runtime state, repair
metadata, create worktrees, update provider configuration, or perform planner
automation.

Consumers that need to change Brevity state should invoke explicit commands and
then refresh from `runtime state --json` after the command completes.

## Performance Budget

Runtime state is intended to be safe for repeated TUI polling. The producer keeps
runtime-log memory recent-only, emits compact run-history fields, and serializes
JSON through a bounded serializer. Current internal budgets are:

- recent runtime-memory entries: 5 lines
- JSON depth: 8 levels
- JSON object or array entries per value: 200 entries
- latest run-history scan per task summary: 1 latest run
- worker-log header read per summarized run: 40 lines

Cleanup candidates are read-only inspection records. They should remain concise;
future expensive cleanup detail should be summarized or moved behind an explicit
command instead of fully expanding inside runtime state.

## Native Cleanup Inspection

Go owns read-only cleanup/orphan inspection:

```powershell
go run ./cmd/brevity cleanup inspect
go run ./cmd/brevity cleanup inspect --json
```

The JSON form emits `brevity.cleanup-inspection.v1` with summary counts and a
stable candidate list. Candidates can describe tracked task worktree issues,
missing worktrees, orphan task worktrees, orphan task branches, dirty worktrees,
stale runs, or unknown inspection cases. Each record includes severity,
available task/branch/worktree identity, dirty/removable/destructive flags,
reason, suggested action, and source.

This command is observational only. It lists Git worktrees, local branches, path
existence, and dirty status where safe. It does not remove worktrees, delete
branches, run `git clean`, mutate task state, launch providers, or execute
cleanup. PowerShell remains the authority for cleanup execution.

## Native Run History Shape

Native Go reads `.brevity\runs.jsonl` as an append-only JSONL index. Each record
may include `runId`, `slug`, `provider`, `profile`, `startedAt`, `finishedAt`,
`exitCode`, `workerStatus`, `failureType`, `logPath`, `stdoutPath`,
`stderrPath`, `summary`, and `message`.

Missing `.brevity\runs.jsonl` means empty run history. Malformed JSONL rows are
reported as clear read errors with the line number. Latest run selection is
deterministic: records sort by `finishedAt`, then `startedAt`, then later JSONL
line. If `finishedAt` is absent, the run is incomplete; runs older than the
worker stale threshold are reported as stale.

## Native Task Run Inspection Commands

Go now owns the read-only task run-history inspection path:

```powershell
go run ./cmd/brevity task runs <slug>
go run ./cmd/brevity task runs <slug> --json
go run ./cmd/brevity task runtime-info <slug>
go run ./cmd/brevity task runtime-info <slug> --json
```

These commands read `.brevity\tasks.json` and `.brevity\runs.jsonl`. They do
not mutate task metadata, write provider state, start workers, merge branches,
or clean up worktrees. PowerShell remains present as legacy/reference behavior
for broader command coverage, especially mutation commands and provider/worker
execution.

`task runs <slug> --json` emits a `brevity.command-result.v1` envelope whose
payload includes `slug`, `count`, and a newest-first `runs` array. Each run may
include `runId`, `workerStatus`, `exitCode`, `provider`, `profile`,
`startedAt`, `finishedAt`, `failureType`, `logPath`, `incomplete`, `stale`,
`runAgeMinutes`, and `source`.

`runs` is an empty array when the task exists but has no run history. Missing
tasks return `success: false` with a `task-not-found` error. Malformed run
history returns a read error with the JSONL line number.

`task runtime-info <slug> --json` emits the same command-result envelope with a
payload containing task metadata, `runCount`, latest run metadata, stale and
incomplete booleans, log path, and a short operator interpretation when useful.
