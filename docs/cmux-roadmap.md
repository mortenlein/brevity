# CMUX Operator Roadmap

This roadmap defines the phased rollout for the CMUX operator interface. Each
phase has a bounded scope, clear entry criteria, and explicit non-goals. Phases
are sequential by default; a later phase must not begin until the previous phase
is stable and its contracts are validated.

See [`docs/cmux-operator.md`](cmux-operator.md) for the full design specification,
ownership boundaries, and risk register.

## Phase 0 — Contract Readiness (Pre-CMUX)

**Goal:** Confirm that the Brevity runtime contracts CMUX depends on are stable
and complete enough to build a read-only view.

**Entry criteria:**
- `brevity runtime state --json` emits `brevity.runtime-state.v1` with task
  summaries, provider health, queue/scheduler plan, run history summary, and
  cleanup candidates.
- `brevity scheduler plan --json` emits `brevity.runtime-scheduler-plan.v1` with
  runnable count, skipped count, reserved count, next runnable item, and skip
  reason.
- `brevity.command-result.v1` is emitted by at least one mutating command (for
  example, `task start`).

**Work in this phase:**
- Identify any gaps in the `brevity.runtime-state.v1` contract that CMUX needs.
- Extend the contract additively if needed (new fields only, no breaking changes).
- Document the contract fields CMUX will consume in this doc.

**Non-goals for this phase:**
- No CMUX code.
- No mutation contracts beyond what already exists.
- No new runtime behavior.

---

## Phase 1 — Read-Only MVP

**Goal:** A functional terminal-native read-only CMUX dashboard that surfaces
Brevity runtime state, queue, scheduler plan, and task/provider details.

**Entry criteria:**
- Phase 0 complete.
- Brevity contracts for runtime state and scheduler plan are stable.

**Scope:**

1. **Multi-panel layout.** Render a compact header, provider health summary,
   task count bar by normalized state, task list, and queue/scheduler summary
   from `brevity.runtime-state.v1` and `brevity.runtime-scheduler-plan.v1`.

2. **Task detail view.** Show task normalized state, worktree path, prompt path,
   last run summary (provider, profile, exit code, start/end time, log path),
   and any cleanup warnings from the runtime-state contract.

3. **Provider detail view.** Show provider name, health status, update timestamp,
   and operator note from the runtime-state contract.

4. **Keyboard navigation.** Move selection between items, open/close detail views,
   manual refresh (`r`), help toggle (`?`), quit (`q`). Line-oriented input for
   Windows console compatibility.

5. **Watch mode.** Poll at a configurable refresh interval. Redraw only when
   stable content changes. Suppress redraws for timestamps alone.

6. **Suggested next actions.** Surface `suggestedNextActions` from the runtime-
   state contract. Display them as operator guidance, not as executable buttons.

**Contracts consumed:**
- `brevity.runtime-state.v1` via `brevity runtime state --json`
- `brevity.runtime-scheduler-plan.v1` via `brevity scheduler plan --json`

**Constraints:**
- No mutation keys in this phase.
- No worker log streaming.
- No vault reads beyond already-materialized worktree context.
- No TUI framework unless an explicit framework decision approves one.
- No new `.brevity` file reads beyond existing contracts.

**Exit criteria:**
- Dashboard renders stable, accurate, low-noise output from real runtime state.
- Watch mode suppresses unnecessary redraws.
- Navigation and detail views work in standard Windows terminals.
- No hidden mutations observed during extended watch mode operation.

---

## Phase 2 — Keyboard Navigation and Detail Expansion

**Goal:** Richer read-only detail views without any mutation keys.

**Entry criteria:**
- Phase 1 stable and validated.

**Scope:**

1. **Cleanup candidate details.** Show severity, category, worktree path, branch,
   dirty state, safety flags, and suggested inspection commands from
   `orphanedTaskWorktrees` and stale task fields in runtime state.

2. **Run history list.** Navigate recent run records for a selected task from
   the runtime-state contract. Show run id, provider, profile, exit code, start
   time, duration, and log path.

3. **Vault task spec display (read-only).** For a selected task with a known slug,
   invoke `Brevity task spec <slug>` (read-only CLI call) and render the spec
   contents in a detail pane. No direct vault file reads.

4. **Queue item details.** For each queue item in the scheduler plan, show id,
   slug, status, reserved flag, reservation owner, and skip reason when present.

5. **Improved help view.** Show available keys, current panel context, and explicit
   note that no mutations are possible in this phase.

**Constraints:**
- No mutation keys.
- No additional Brevity contracts introduced without an explicit contract extension.
- Vault spec display is strictly delegated to `Brevity task spec`.

**Exit criteria:**
- All detail views render correct data from contracts.
- Help correctly describes available actions.
- No CMUX-local state that does not come from a Brevity contract.

---

## Phase 3 — Low-Risk Confirmed Actions

**Goal:** Let the operator dispatch a small set of low-blast-radius mutating
commands through a confirmed action flow.

**Entry criteria:**
- Phase 2 stable and validated.
- `brevity.command-result.v1` is emitted by all candidate commands.
- Confirmation UI, command display, and refresh-after-command behavior are
  implemented and reviewed.

**Candidate actions (lowest risk first):**

1. `Brevity provider set <provider> <status> [-Note <note>]` — Update provider
   health status. Low blast radius; does not change task state.
2. `Brevity provider reset <provider>` — Reset provider health to `unknown`.
3. `go run ./cmd/brevity task refresh-context <slug>` — Refresh worktree context
   from vault memory. Does not mutate task status or launch workers.

**Confirmation model for every action:**
- Show the exact argv-style command that will run.
- Show the expected `brevity.command-result.v1` schema fields.
- Show affected resources (provider name, task slug, worktree path).
- Require explicit confirmation before dispatch.
- On completion (success or failure), parse the command result and refresh runtime
  state from `brevity runtime state --json`.
- Display failures from the command result, not from raw stderr parsing.

**Constraints:**
- No destructive actions in this phase (`--force`, task cleanup, branch deletion).
- No worker execution in this phase.
- No actions that mutate task status, worktree state, or branch state.

**Exit criteria:**
- Each action dispatched, parsed, and refreshed correctly.
- Failures surface the structured command-result error, not a raw error message.
- Dashboard refreshes from fresh state after every action.
- No action dispatched without explicit operator confirmation.

---

## Phase 4 — Destructive Actions

**Goal:** Add confirmed cleanup flows with clear destruction warnings and dry-run
preview.

**Entry criteria:**
- Phase 3 stable and validated.
- Dashboard reliably shows exact commands, affected resources, and failure states.

**Candidate actions:**

1. `go run ./cmd/brevity task cleanup <slug> --plan --json` — Show dry-run
   cleanup plan before confirmation.
2. `go run ./cmd/brevity task cleanup <slug> --force` — Execute cleanup after
   dry-run display and explicit confirmation.
3. `go run ./cmd/brevity cleanup inspect` + `cleanup execute <id> --force` —
   Orphan cleanup after inspection and confirmation.

**Constraints:**
- Every destructive action must show the dry-run plan before the confirmation
  prompt.
- Destructive actions must show the `--force` flag explicitly in the confirmation.
- No destructive action is dispatched on a single keypress; at minimum two
  explicit confirmation steps are required.

**Exit criteria:**
- Dry-run plan is displayed and correct before every destructive action.
- Destructive actions fail safely if Brevity refuses them (dirty worktree,
  unmerged branch, missing task metadata).
- No task data lost due to unexpected CMUX behavior.

---

## Phase 5 — Task Execution Actions

**Goal:** Let the operator launch a task worker through CMUX with full confirmation
and result handling.

**Entry criteria:**
- Phase 4 stable and validated.
- `Brevity task run --plan --json` emits a stable execution envelope.
- Scheduler plan and preflight contracts are stable.

**Candidate actions:**

1. `go run ./cmd/brevity task run <slug> --plan --json` — Show planned execution
   envelope before confirmation: task state, provider, profile, model, prompt
   freshness, planned run id, planned log paths, argv-style command, blockers.
2. `go run ./cmd/brevity task run <slug> --execute` — Execute after plan display
   and explicit confirmation.

**Constraints:**
- No automatic execution; the operator must confirm the plan output.
- Log paths are surfaced for the operator to tail in a separate terminal. CMUX
  must not stream or embed raw worker output.
- After execution starts, CMUX refreshes runtime state and shows updated task
  status. It does not hold a reference to the worker process.

**Exit criteria:**
- Execution envelope rendered correctly from `--plan --json` before dispatch.
- Worker launched and log path surfaced to operator.
- Runtime state refreshes correctly after launch.
- No hidden worker process management in CMUX.

---

## Phase 6 — Native Go Runtime Ownership and Framework Evaluation

**Goal:** Migrate runtime authority from PowerShell to native Go where parity is
proven, and evaluate a TUI framework only if interaction complexity justifies it.

**Entry criteria:**
- Phases 1-5 stable.
- Go CLI has explicit parity for all mutating commands used by CMUX.
- PowerShell and Go contracts produce equivalent outputs for all CMUX consumers.

**Scope:**
- Switch CMUX from PowerShell-backed command dispatch to native Go command dispatch
  where Go owns the authoritative implementation.
- Document which PowerShell commands are retired and which remain as
  compatibility/legacy behavior.
- Evaluate Bubble Tea or another TUI framework if keyboard interaction has grown
  complex enough to justify the dependency. If adopted, document the framework
  decision and keep the existing line-oriented input as a fallback for
  non-interactive flows.

**Constraints:**
- No framework adoption without a documented framework decision.
- No new PowerShell-first runtime behavior added after this phase starts.
- Go CLI parity must be validated with smoke tests before PowerShell dispatch
  is removed for any command.

---

## Contract Dependencies

| CMUX capability | Contract | Schema |
| --- | --- | --- |
| Runtime state, tasks, provider health | `brevity runtime state --json` | `brevity.runtime-state.v1` |
| Queue plan and scheduler decision | `brevity scheduler plan --json` | `brevity.runtime-scheduler-plan.v1` |
| Mutation results | All mutating CLI commands | `brevity.command-result.v1` |
| Task execution envelope | `brevity task run --plan --json` | (see runtime-scheduler-contract.md) |
| Task spec (read-only) | `brevity task spec <slug>` | Markdown output |

All contracts must evolve additively. CMUX must tolerate unknown fields. Breaking
contract changes must use a new schema version before CMUX is updated.

## Non-Goals (All Phases)

- A second orchestration runtime or task data model.
- Autonomous task execution without operator confirmation.
- Vault write authority.
- Worker process supervision or embedded log streaming.
- Automatic retry, fallback, or routing logic.
- Multi-machine or distributed orchestration.
- A web interface.
- A daemon that executes tasks unattended.
- Any behavior that mutates `.brevity` files without going through the Brevity CLI.
