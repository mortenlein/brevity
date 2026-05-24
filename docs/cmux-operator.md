# CMUX Operator Interface

CMUX is a planned operator layer over the Brevity runtime. It surfaces existing
Brevity primitives — runtime state, queue, scheduler, task metadata, provider
health, and worker run history — in a unified terminal-native view. It is not a
second orchestration system.

## Purpose

CMUX exists to reduce operator context switching. Today an operator running
multiple tasks in parallel must manually correlate `Brevity board`, provider
health, queue plan, scheduler plan, and worker logs to understand what is
happening and what to do next. CMUX assembles that picture from existing
Brevity contracts and makes it navigable.

CMUX is a **consumer**, not an owner. It reads Brevity's existing state contracts,
dispatches existing Brevity CLI commands, and refreshes from those contracts after
each mutation. It does not carry its own task store, scheduler, provider registry,
merge engine, or worker lifecycle.

## What CMUX Owns

CMUX owns:

- **Rendering** — how runtime state, queue, scheduler, provider health, task
  metadata, worker run history, and vault context are displayed to the operator.
- **Navigation** — how the operator moves between views, selects items, and
  drills into detail.
- **Command dispatch** — converting operator intent (keypress, selection, confirm)
  into an explicit Brevity CLI invocation.
- **Refresh coordination** — polling or refreshing the Brevity runtime-state
  contract after a command completes and presenting the updated view.
- **Operator confidence** — showing the exact command that will run, the expected
  effect, and any destructive or execution flags before the operator confirms.

CMUX may also surface vault context when it is already materialized in a task
worktree (for example, from `.brevity\context\`). It must not import vault files
directly or couple runtime execution to vault structure.

## What CMUX Must Not Own

CMUX must never own:

- **Runtime state** — `.brevity\tasks.json`, `.brevity\runtime-queue.json`,
  `.brevity\runtime.json`, `.brevity\runs.jsonl`, `.brevity\provider-health.json`,
  or any other file under `.brevity`. These belong to the Brevity runtime.
- **Task execution semantics** — CMUX must not decide when a task is ready, whether
  a task can run, or which provider to use. These decisions belong to Brevity
  preflight and the scheduler.
- **Worker process management** — CMUX must not launch, kill, or supervise worker
  processes directly. Worker execution belongs to `Brevity task run --execute`.
- **Queue or scheduler logic** — CMUX must not implement its own queue ordering,
  priority, reservation, or selection rules. These belong to `brevity queue` and
  `brevity scheduler`.
- **Merge or cleanup logic** — CMUX must not implement branch integration or
  worktree cleanup. These belong to `Brevity task merge` and `Brevity task cleanup`.
- **Vault write authority** — CMUX must not write durable vault memory. If a future
  vault surface is needed (for example, surfacing architecture notes alongside a
  task), it must be read-only and scoped to materialized worktree context only.
- **Provider credentials or configuration** — CMUX must not store or proxy API
  keys, provider model selections, or worker profiles. These belong to
  `.brevity\config.json` and the Brevity CLI.
- **A parallel task or queue data model** — CMUX must not invent its own task
  records, queue items, or scheduler decisions. It must consume Brevity primitives
  and surface them.

## Relationship to the Brevity Runtime

CMUX is a **frontend operator layer** over the Brevity runtime. The division is
analogous to the existing Go dashboard relationship:

| Layer | Authority |
| --- | --- |
| `.brevity\` files | Runtime ground truth |
| `Brevity` CLI (Go + PowerShell) | State mutation and command execution |
| `brevity runtime state --json` | Read-only runtime-state contract |
| `brevity.command-result.v1` | Structured mutation result contract |
| Go dashboard / CMUX | Frontend consumer and operator UI |

CMUX reads the `brevity.runtime-state.v1` contract for task state, provider
health, run history, cleanup candidates, and suggested next actions. It reads
`brevity.runtime-scheduler-plan.v1` for queue and scheduler state. It dispatches
Brevity CLI commands for mutations and parses `brevity.command-result.v1` results.
It does not write to `.brevity` files directly.

Runtime state is not a mutation channel. CMUX must always go through the CLI.

## Relationship to AI-Vault

AI-Vault is **durable project memory**, not runtime state. The vault stores
architecture decisions, roadmap notes, task specs, session notes, and historical
context. It is not a runtime task queue and must not become one.

CMUX may surface vault information in two limited, read-only ways:

1. **Materialized worktree context** — If a task worktree contains
   `.brevity\context\` files (materialized from the vault by `task refresh-context`
   or `task activate`), CMUX may display those files alongside task details. These
   are bounded copies, not live vault reads.
2. **Task spec display** — CMUX may invoke `Brevity task spec <slug>` to surface
   a vault task spec alongside a task detail view. This is a read-only CLI call.

CMUX must not:

- Write to vault files.
- Read vault files directly using file paths from `.brevity\config.json` unless
  it is delegating to an existing Brevity CLI command.
- Treat vault structure as runtime state or queue input.
- Create a parallel task planning system inside or alongside the vault.

## Relationship to External Workers

Brevity supports external AI worker providers: Claude Code, Codex, Gemini, and
Copilot. Workers are executed by the Brevity CLI (`Brevity task run --execute`),
not by CMUX.

CMUX's relationship to workers is:

- **Display** — surface worker run history, last run status, exit codes, log paths,
  and provider/profile information from `brevity.runtime-state.v1`.
- **Dispatch** — let the operator select a task and confirm a `Brevity task run`
  invocation. Show the exact argv-style command before confirming.
- **Log access** — surface worker log paths from task details so the operator can
  open or tail logs in a separate terminal. CMUX must not stream or embed raw
  worker output.

CMUX must not implement its own worker launch, process management, log streaming,
or provider health polling. Worker process state is visible only through the
Brevity runtime-state contract.

## Design Principles

- **Consume, don't duplicate.** Every piece of state CMUX shows must come from an
  existing Brevity contract. If CMUX needs new data, the right answer is to extend
  an existing Brevity contract, not to add a CMUX-private data source.
- **Mutations are explicit and confirmed.** CMUX must show the exact command,
  affected resources, and any destructive or execution flags before the operator
  confirms. No hidden mutations.
- **Refresh after command.** After any mutation, CMUX refreshes from the runtime-
  state contract and renders from the fresh snapshot. No optimistic local edits.
- **Operator visibility, not automation.** CMUX is a view into what Brevity is
  doing, not an autonomous planner or scheduler. Suggested next actions come from
  the runtime-state contract; CMUX renders them, it does not act on them without
  explicit operator confirmation.
- **Terminal-native and low-noise.** Output should be readable as plain text,
  copyable into issues or logs, and usable in standard Windows terminals without
  requiring ANSI color or raw terminal input.

## Initial Read-Only MVP Scope

The CMUX read-only MVP must:

1. **Consume `brevity runtime state --json`** and render the full runtime state
   contract in a terminal-native multi-panel view: header, provider health, task
   counts by normalized state, task list, cleanup warnings, run indicators, and
   suggested next actions.

2. **Consume `brevity scheduler plan --json`** and show the queue plan summary:
   queue depth, runnable/skipped/reserved counts, next runnable task, and first
   skip reason when no runnable item is available.

3. **Support task detail views** that surface task normalized state, worktree
   path, prompt path, last run summary, provider, profile, and log path from the
   runtime-state contract.

4. **Support provider detail views** that surface provider name, health status,
   update timestamp, and operator note from the runtime-state contract.

5. **Support keyboard navigation** between panels and items: move selection,
   open/close detail views, manual refresh, help, and quit.

6. **Support watch mode** with configurable refresh interval. Redraw only when
   stable content changes. Suppress redraws for polling timestamps alone.

The MVP must not include:

- Any mutation keys or confirmed action dispatch.
- Worker log streaming.
- Queue or scheduler write operations.
- Vault reads beyond materialized worktree context.
- A TUI framework unless explicitly approved.

## Risks

**Risk: CMUX becomes a second runtime.**
If CMUX caches state, maintains its own task index, or makes decisions based on
local state rather than the Brevity runtime-state contract, it will diverge from
ground truth and add operational complexity. Mitigation: CMUX must always refresh
from Brevity contracts. It must never write to `.brevity` files.

**Risk: Vault coupling.**
If CMUX adds direct vault reads or begins treating vault task specs as a runtime
queue, the boundary between durable memory and runtime execution will erode.
Mitigation: CMUX surfaces vault information only through existing Brevity CLI
commands (`task spec`) or materialized context already present in a worktree.

**Risk: Premature mutation UI.**
Adding mutation keys before command-result handling, confirmation model, and
refresh-after-command behavior are stable risks silent mutations and confusing
failure states. Mitigation: the MVP is strictly read-only. Mutations belong in
a later phase behind explicit confirmation, command display, and command-result
parsing.

**Risk: Framework lock-in.**
Adopting a TUI framework before CMUX's interaction model is stable couples the
project to framework-specific patterns. Mitigation: stay dependency-free during
the read-only MVP. Evaluate Bubble Tea only when interaction pressure clearly
justifies it, and document that decision explicitly.

**Risk: Authority drift.**
If CMUX adds its own provider health metadata, scheduling logic, or task status
derivation, it will conflict with Brevity's authoritative sources. Mitigation:
CMUX reads authority from Brevity contracts only. It does not compute normalized
state, scheduler decisions, or provider eligibility.

## Non-Goals

- A second orchestration runtime.
- Autonomous task execution without operator confirmation.
- Vault write authority.
- A parallel queue or task data model.
- Worker process supervision or log streaming.
- Automatic retry, fallback, or routing logic.
- Distributed or multi-machine orchestration.
- A replacement for the Brevity CLI.
- A web interface.
- A daemon that executes tasks unattended.
- Framework adoption without an explicit framework decision.
