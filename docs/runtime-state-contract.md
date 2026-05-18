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
