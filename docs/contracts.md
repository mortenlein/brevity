# Contracts

Brevity exposes machine-readable runtime and operator contracts for future TUI
and automation consumers. Contracts should evolve additively where practical so
older consumers can keep working as new fields are introduced.

## Runtime State

- [`docs/runtime-state-contract.md`](runtime-state-contract.md) - describes
  `.\brevity.ps1 runtime state --json` as the primary read-only runtime
  inspection contract.
- [`docs/runtime-state.schema.json`](runtime-state.schema.json) - JSON schema
  for `brevity.runtime-state.v1`.

Runtime state is observational and read-only. Consumers should use it to render
provider health, task state, worktree findings, runtime memory, and suggested
next actions without mutating Brevity state.

## Command Results

- [`docs/command-result-contract.md`](command-result-contract.md) - describes
  the future structured result shape for CLI commands.
- [`docs/command-result.schema.json`](command-result.schema.json) - JSON schema
  for `brevity.command-result.v1`.

Command results are the intended machine-readable boundary for command
outcomes, warnings, errors, refresh hints, and command-specific payloads.

## TUI Guidance

- [`docs/tui-consumer-guide.md`](tui-consumer-guide.md) - guidance for reading
  runtime state and rendering it safely.
- [`docs/tui-command-contract.md`](tui-command-contract.md) - guidance for
  invoking explicit CLI commands from a TUI.
- [`docs/tui-readiness-checklist.md`](tui-readiness-checklist.md) - checklist
  for deciding when Brevity is ready for a real TUI.

## Consumer Rules

Runtime state is not a mutation channel. Consumers that need to change Brevity
state should invoke explicit CLI commands, then refresh runtime state after the
command completes.

Consumers should tolerate unknown fields, ignore JSON object field ordering,
and prefer additive contract handling where practical.
