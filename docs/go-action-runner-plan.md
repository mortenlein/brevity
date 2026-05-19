# Go Action Runner Contract Plan

This plan defines how future Go and TUI actions can safely invoke Brevity
runtime mutations while PowerShell remains the authoritative implementation.
It is a contract boundary plan only; it does not introduce interactive
mutations or runtime behavior changes.

## Architecture

The Go CLI and TUI remain frontend/runtime clients. Their job is to render
state, collect operator intent, dispatch actions, and refresh the interface
from the runtime state contract.

PowerShell remains the initial authoritative runtime for mutations. It owns
validation, locking, Git behavior, branch integration, worktree lifecycle,
provider state, task execution, cleanup, logging, and durable `.brevity`
updates.

Go invokes mutation commands through structured command-result contracts rather
than editing runtime files directly. A successful action path should produce a
`brevity.command-result.v1` document that Go can parse, validate, summarize,
and use as the trigger for a state refresh.

## Future Action Flow

Future interactive actions should follow this flow:

```text
user action
-> Go action dispatcher
-> PowerShell command
-> brevity.command-result.v1
-> Go interprets result
-> UI refresh via runtime state
```

The action dispatcher is responsible for mapping a UI intent to an explicit
PowerShell command invocation. It should not infer mutations by writing
`.brevity` files or manipulating worktrees itself.

The UI should render post-action state from a fresh runtime snapshot, not from
optimistic local edits. After a mutation completes, Go should call the runtime
state read path again and redraw from the returned `brevity.runtime-state.v1`
document.

## Requirements

### Stdout Discipline

Mutation commands intended for Go or TUI consumption must keep stdout reserved
for the structured command-result JSON document when machine-readable output is
requested. Human-oriented progress, diagnostics, and trace output should go to
stderr or durable logs so stdout remains parseable.

### Structured Errors

Expected failures should be represented inside `brevity.command-result.v1` with
stable fields for status, error code, message, affected resources, and suggested
next actions where useful. Go should prefer the structured result over parsing
free-form text.

### Nonzero Exit Handling

Go must treat nonzero process exits as failed command executions, but it should
still attempt to parse stdout as `brevity.command-result.v1` when present.

This allows PowerShell to report a structured failure while still using process
exit status for shell compatibility. If the process exits nonzero and no valid
command-result document is available, Go should surface a transport or
execution error and then refresh runtime state if the command may have partially
mutated state.

### Confirmation Boundaries

The Go/TUI layer owns operator confirmation for interactive actions. It should
show the exact planned action, including destructive flags such as `--execute`
or force-style cleanup options, before dispatching a mutating command.

PowerShell should continue to own semantic validation and may still reject an
unsafe command even after Go has collected confirmation. Confirmation in Go is
not permission to bypass runtime safety checks.

### Lock-Aware Mutation Behavior

Mutating PowerShell commands should be lock-aware. If a runtime lock, task lock,
worktree lock, or Git state makes a mutation unsafe, the command-result should
report a structured locked or conflict status instead of relying only on
free-form stderr.

Go should display lock and conflict outcomes as actionable states, not as
crashes. The UI should avoid retry loops unless a future contract explicitly
marks an operation as retryable.

### Refresh-After-Mutation Behavior

Every completed mutation attempt should be followed by a runtime state refresh.
This applies to success, structured failure, and ambiguous transport failure.

Refreshing after failures matters because a command can validate, acquire a
lock, partially change durable state, or write logs before failing. The UI's
next render should always be grounded in the runtime's current source of truth.

## Likely First Actions

The first interactive Go/TUI actions should be small, explicit, and already
close to existing PowerShell runtime behavior.

- `task context refresh` - Refresh task context or runtime-derived task
  metadata through a PowerShell command-result path.
- `task cleanup --execute` - Promote dry-run cleanup candidates into an
  explicitly confirmed cleanup execution.
- `provider set` and `provider reset` - Change provider status through the
  runtime command boundary and refresh provider health afterward.
- `task run --execute` - Start a task run only after command-result behavior,
  locking, and refresh semantics are stable.

## Non-Goals

- No embedded PowerShell runtime inside Go.
- No direct `.brevity` mutation from Go yet.
- No bypassing `brevity.command-result.v1` for mutating actions.
- No optimistic UI writes to runtime state files.
- No migration of mutation ownership before contract behavior is stable.

## Future Evolution

Native Go runtime mutations may become appropriate later, but only after parity
with PowerShell behavior is proven and command-result contracts have
stabilized. Each moved mutation should preserve the same observable contract,
lock behavior, error semantics, and refresh expectations.

Until then, Go is the action dispatcher and UI client; PowerShell is the
mutation authority.

## Validation

Documentation-only changes for this plan should validate with:

```powershell
git diff --check
```

Also verify that this Markdown file uses CRLF line endings and that no runtime
or code files changed.

## Recommended First Interactive Action

The recommended first interactive action after this plan is `provider set` or
`provider reset`.

Provider actions are high leverage but relatively contained: they exercise the
Go action dispatcher, PowerShell mutation invocation, structured
`brevity.command-result.v1` parsing, nonzero exit handling, confirmation
boundaries, and refresh-after-mutation behavior without immediately touching
worktree cleanup or task execution.
