# Command Result Contract

This contract describes the intended shape of machine-readable command results
for future Brevity TUI and runtime consumers. It is architectural guidance only;
it does not define implemented behavior yet.

## Why Structured Results Matter

Human console output is useful for operators, but it is a poor automation
boundary. Text written for people changes naturally as commands become clearer,
friendlier, or more detailed. A TUI that parses those strings would inherit that
fragility and could mistake wording changes for state changes.

Future orchestration needs a stable command result contract that describes what
happened, whether it succeeded, which warnings or errors matter, and what a
consumer should refresh or offer next. Console text may remain, but structured
results should become the automation contract.

## Desired Characteristics

Command results should be:

- Machine-readable, with a predictable JSON shape.
- Designed for additive evolution so new fields can be added without breaking
  older consumers.
- Stable in their success, warning, and error semantics.
- Explicit about the command outcome instead of requiring text interpretation.
- Able to include optional structured payloads for command-specific data.

Consumers should be able to distinguish a successful no-op, a successful
mutation with warnings, and a failed command without parsing human prose.

## Result Shape Concepts

A future result object could include fields like:

```json
{
  "schema": "brevity.commandResult",
  "version": 1,
  "command": "task cleanup",
  "success": true,
  "severity": "info",
  "warnings": [],
  "errors": [],
  "suggestedNextActions": [
    "refresh-runtime-state"
  ],
  "payload": {
    "changedResources": []
  }
}
```

The exact field names may evolve before implementation, but the core concepts
should stay stable:

- `schema` identifies the document kind.
- `version` identifies the contract version.
- `command` identifies the Brevity command that produced the result.
- `success` gives a simple machine-readable outcome.
- `severity` communicates the highest result severity, such as `info`,
  `warning`, or `error`.
- `warnings` contains non-fatal issues that should be visible to the operator.
- `errors` contains fatal issues that prevented completion.
- `suggestedNextActions` gives consumers safe follow-up hints.
- `payload` or `result` contains optional command-specific structured data.

## Human And Machine Output

Human-readable console output may still exist for direct CLI use. Operators
should continue to get clear messages when running Brevity by hand.

Machine-readable output should be treated as the contract for automation,
including future TUI behavior. A TUI should prefer structured command results
over parsing stdout text, and it should refresh its read model from
`runtime state --json` after mutations when the command result recommends or
requires it.

## Future CLI Patterns

Likely implementation patterns include:

- A `--json` flag on commands that need structured result output.
- Structured stdout for machine-readable JSON.
- Human diagnostics, progress, or verbose detail on stderr when appropriate.
- Exit code semantics that align with `success` and `severity`.

For example, a command could return exit code `0` for success, a non-zero exit
code for fatal failure, and still include warnings in a successful JSON result.
The exact exit code map should be defined when implementation begins.

## Initial Adoption Candidates

The first commands to adopt structured results should be the commands a TUI is
most likely to invoke after operator confirmation:

- `task new`
- `task cleanup`
- `task cleanup-orphans`
- `provider set`
- `provider reset`
- `memory note`

These commands cross the read-to-mutate boundary and would benefit most from a
stable result that says what changed, which warnings were raised, and which
state should be refreshed next.

## Non-Goals

This document does not implement structured command output, change runtime
state, alter command behavior, or require planner automation. It defines the
contract direction so future implementation can be incremental and compatible
with TUI orchestration.
