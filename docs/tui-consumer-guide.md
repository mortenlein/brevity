# TUI Consumer Guide

This guide describes how future TUI and runtime consumers should read Brevity
orchestration state without taking ownership of orchestration behavior.

## Runtime State Source

Consumers should inspect runtime state with:

```powershell
.\brevity.ps1 runtime state --json
```

The command emits a point-in-time JSON snapshot for the current repository. It
is read-only inspection state and should not be treated as a mutation channel.
Future TUIs should execute explicit CLI commands, such as task or merge
commands, when they need to change Brevity state.

## Contract Expectations

The current runtime state schema is `brevity.runtime-state.v1`. Consumers should
check the top-level `schema` field before relying on the document shape.

The v1 contract should evolve additively where practical. Consumers should
tolerate unknown fields, and they should not depend on object field ordering.
Breaking changes should use a new schema value rather than silently changing the
meaning of existing v1 fields.

## Refresh Behavior

Runtime state is a snapshot, not a stream. A TUI can refresh after explicit user
actions, after known command completion, and at modest idle intervals when the
view is active. Avoid tight polling loops; they add noise without improving the
contract.

Consumers should handle partial or missing data defensively. Prefer showing an
empty section, a stale timestamp, or a conservative warning over failing the
whole interface when one optional section is absent or incomplete.

## Rendering Guidance

Render provider health from the `providers` section as operational status, not
as a command plan. Provider warnings should help explain why tasks may be gated.

Render task groups from `groups` as navigation shortcuts over the canonical
task records in `tasks`. If a task appears in multiple groups, keep the task
identity stable and let each group act as a filtered view.

Render `orphanedTaskWorktrees` separately from tracked tasks. Orphans are
inspection findings that may require operator cleanup, but they should not be
quietly promoted into tracked task state.

Render `suggestedNextActions` as advisory prompts. A future TUI may offer
buttons that run explicit CLI commands, but the runtime state document itself
must remain non-mutating.
