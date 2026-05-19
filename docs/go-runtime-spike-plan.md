# Go Runtime Spike Plan

This plan defines how Brevity can explore a Go runtime without replacing the
current PowerShell implementation. The goal is coexistence first, migration
only after contracts and parity are proven.

## Strategy

Brevity should not do a big-bang rewrite. The current PowerShell runtime remains
the executable reference implementation for behavior, state interpretation, and
operator-facing command results.

Go starts as a frontend/runtime client and spike. Its first responsibility is
to consume existing runtime contracts, render the same operational view that
PowerShell already exposes, and dispatch selected actions through the
PowerShell JSON command-result surface. Compatibility is driven by documented
JSON contracts, smoke validation, and observable parity with PowerShell
behavior.

The PowerShell runtime should continue to own `.brevity` mutation, worker
lifecycle, cleanup, and branch integration until a Go behavior has a clear
reference behavior, a stable contract, and a rollback path. Go must not mutate
`.brevity` state directly in the current spike.

## Current Go Scope

The current Go runtime spike is a PowerShell-backed client. The dashboard reads
PowerShell-produced runtime state, and the action runners invoke PowerShell
commands that return command-result JSON. Native Go runtime state ownership has
not started.

Initial inputs:

- `.brevity\config.json`
- `.brevity\tasks.json`
- `.brevity\provider-health.json`
- `.brevity\runs.jsonl`

The dashboard consumes:

```powershell
.\brevity.ps1 runtime state --json
```

Reading PowerShell-produced runtime state first keeps the spike anchored to the
reference implementation while the Go data model and rendering path mature.
Native `.brevity` file reading can follow once the JSON contract path is stable.

## Current Go Command Surface

The Go client requires Go on `PATH` and runs from the repository root:

```powershell
go run ./cmd/brevity
go run ./cmd/brevity --once
```

The dashboard invokes `.\brevity.ps1 runtime state --json`, validates the
`brevity.runtime-state.v1` schema, and renders a plain dashboard from that
PowerShell-produced snapshot.

The currently supported Go action commands are:

```powershell
go run ./cmd/brevity doctor
go run ./cmd/brevity provider set <provider> <status>
go run ./cmd/brevity provider reset <provider>
go run ./cmd/brevity task new <slug>
go run ./cmd/brevity task cleanup <slug> --force
go run ./cmd/brevity task context refresh <slug>
go run ./cmd/brevity task runtime-info <slug>
go run ./cmd/brevity task runs <slug>
go run ./cmd/brevity task run <slug> --execute [--profile <profile>] [--smoke]
go run ./cmd/brevity task runs reconcile --dry-run
go run ./cmd/brevity task runs retention --dry-run
go run ./cmd/brevity task runs compact --dry-run
```

These commands route to PowerShell JSON contracts. They may cause PowerShell to
perform the requested mutation or execution, but the Go client itself does not
write `.brevity` metadata or own runtime state.

PowerShell remains the reference runtime for state interpretation,
orchestration behavior, mutations, worker lifecycle, cleanup, and branch
integration.

## Non-Goals

- No TUI mutation UI yet.
- No native Go runtime state ownership yet.
- No direct Go writes to `.brevity` runtime metadata yet.
- No replacing `brevity.ps1` yet.
- No planner automation in the Go spike.

## Proposed Folder Layout

```text
/cmd/brevity
/internal/runtime
/internal/state
/internal/tui
/internal/contracts
/docs
```

Responsibilities:

- `/cmd/brevity` - CLI entry point and command wiring.
- `/internal/runtime` - runtime-state loading and orchestration-facing read
  models.
- `/internal/state` - native `.brevity` file readers after the JSON-first phase.
- `/internal/tui` - read-only terminal rendering.
- `/internal/contracts` - typed representations of documented JSON contracts.
- `/docs` - contract and migration documentation.

## Migration Phases

### Phase 0: Docs and Spike Plan

Document coexistence boundaries, runtime ownership, contract expectations, and
the migration path before adding Go code.

### Phase 1: Go Reads PowerShell Runtime State JSON

Add a Go CLI spike that shells out to:

```powershell
.\brevity.ps1 runtime state --json
```

It should parse `brevity.runtime-state.v1`, tolerate unknown fields, and report
contract or parsing failures without mutating repository state.

### Phase 2: Go Renders TUI from Runtime State

Render a read-only dashboard from the PowerShell-produced runtime state JSON.
The dashboard should match the current TUI scaffold's information hierarchy:
provider health, task counts, task groups, orphaned task worktrees, runtime
memory, cleanup candidates, and suggested next actions where available.

### Phase 3: Go Reads Native `.brevity` Files

Add native readers for `.brevity\config.json`, `.brevity\tasks.json`,
`.brevity\provider-health.json`, and `.brevity\runs.jsonl`. Compare native Go
interpretation against PowerShell runtime state output before trusting it as a
source for UI or command behavior.

### Phase 4: Go Implements Read-Only Commands

Move selected read-only commands into Go after their JSON contracts and
PowerShell reference behavior are stable. Candidate commands include runtime
state inspection and task status views.

### Phase 5: Selected Mutating Commands After Command-Result Parity

Only move mutating commands after command-result parity is proven. Each moved
command must return the same contract shape, preserve existing operator
semantics, and have an explicit rollback path to the PowerShell implementation.

## Parity Rules

Every behavior moved toward Go must satisfy these rules:

- A PowerShell reference behavior exists.
- A JSON contract exists for machine-readable output.
- Smoke validation exists for the behavior.
- A rollback path exists to continue using PowerShell.
- Unknown JSON fields are tolerated by consumers.
- Runtime state remains observational unless an explicit mutating command is
  invoked.

## Go and Rust

Go is favored for the first implementation because it supports fast iteration on
CLI and TUI workflows, produces straightforward single-binary tools, and is
easier for agents to maintain during an incremental migration.

Rust remains a possible later choice for performance-critical components, but it
should not block the first compatibility spike. The immediate problem is
contract parity and coexistence, not maximum runtime performance.

## Validation

Documentation-only changes for this plan should validate with:

```powershell
git diff --check
```

Also verify that the new Markdown file uses CRLF line endings and that no
runtime behavior changed. The spike plan should not require edits to
`brevity.ps1` or any `.brevity` runtime state files.

## Recommended First Go Implementation Task

Create a minimal Go CLI spike under `/cmd/brevity` that runs
`.\brevity.ps1 runtime state --json`, parses the top-level
`brevity.runtime-state.v1` document, and renders a plain read-only dashboard
from that JSON without reading native `.brevity` files or mutating state.
