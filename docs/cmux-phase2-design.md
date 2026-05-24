# CMUX Phase 2 Design

This document defines the design direction for CMUX Phase 2, building on the
read-only Phase 1 foundation. It is a planning and constraints document, not an
implementation spec. No Go code changes, new CLI flags, or behavior changes are
implied by this document alone.

See [`docs/cmux-operator.md`](cmux-operator.md) for the full CMUX design spec,
[`docs/cmux-runtime-reader.md`](cmux-runtime-reader.md) for the Phase 1
implementation, and [`docs/cmux-roadmap.md`](cmux-roadmap.md) for the phased
rollout.

---

## 1. Phase 2 Purpose

Phase 1 established CMUX as a **read-only operator report layer**: fetch
two runtime contracts, parse them into a `Snapshot`, render in text, markdown,
or JSON.

Phase 2 elevates CMUX into an **operator cognition layer**: structured,
deterministic artifacts that answer specific operational questions and can be
handed off directly to a human reviewer or an AI agent without further
interpretation.

The Phase 1 question is: _"What is the current runtime state?"_

Phase 2 answers focused questions:

- _"What is ready to merge and what is not — and why?"_
- _"What is blocking the queue right now?"_
- _"Which tasks have become stale and need attention?"_
- _"What should an AI agent do next with this task?"_
- _"Is the runtime healthy enough to run the next queue item?"_

Phase 2 outputs are still one-shot, deterministic, and pipe-safe. The
improvement is **semantic focus**: each report mode answers one specific
operational question rather than dumping all available state.

---

## 2. Non-Goals

The following are explicitly out of scope for Phase 2 (and should be revisited
only after Phase 2 exit criteria are met):

| Non-goal | Reason deferred |
|---|---|
| Watch mode / live redraw | Adds interactive complexity before report semantics are stable |
| Terminal redraw loops | Couples output to terminal capabilities; breaks remote/AI/CI usage |
| Bubble Tea or TUI framework adoption | Framework lock-in risk; report semantics must be validated first |
| Web UI | Premature; adds a deployment surface before operator workflow is proven |
| Worker execution | Outside CMUX scope; CMUX is an observer, not an executor |
| Mutation commands (merge, cleanup, run) | CMUX reads contracts; it does not dispatch actions |
| Direct `.brevity` file reads | Bypasses normalization logic; breaks contract boundary |
| AI calls or LLM invocations | CMUX produces AI-ingestable artifacts; it does not call AI |
| Autonomous orchestration | CMUX informs; it does not decide |
| Polling or scheduling | One-shot execution only; callers own the schedule |

These are not permanent exclusions. They are deferred until Phase 2 exit
criteria are met and the interaction model is validated.

---

## 3. Candidate Phase 2 Workflows

Each workflow is a **new render mode** or **focused report flag** that reuses
the existing `Snapshot`, `Fetcher`, and `RenderOptions` infrastructure. All
workflows are read-only, deterministic, and output text/markdown/JSON.

### 3.1 Review Packet Improvements

**What:** Extend the existing `--review <slug>` mode (Phase 1) with richer
merge-blocking detail: which checklist items are blocking, what the last run
produced, and which provider or profile was involved.

**Value:** The review packet is already the highest-information artifact for a
single task. Making it more actionable for AI handoff is low-effort, high-return.

**New fields needed:** Last run start/end timestamps, last run log path (see
§5). No contract changes required if those fields are already present but
currently unused.

---

### 3.2 Merge Readiness Report

**What:** A fleet-level report of all tasks, grouped by merge readiness:
`merge-ready`, `reviewing`, `blocked`, `needs-run`, `merged`, and `other`.
Filters to only in-flight tasks by default.

**Usage sketch:**
```powershell
brevity cmux --merge-report
brevity cmux --merge-report --output markdown
brevity cmux --merge-report --output json
```

**Value:** A single command that answers _"what can be merged today?"_ is the
most common operator question after `brevity cmux`. It replaces repeated
`--state` queries with a structured, grouped view.

**Implementation notes:** Pure rendering logic over the existing `Snapshot`.
No new contracts needed. A new `renderMergeReport` function grouped by
normalized state.

---

### 3.3 Cleanup Readiness Report

**What:** A focused report of tasks that are eligible for cleanup — merged tasks
with worktrees present, orphaned worktrees, and stale branches.

**Usage sketch:**
```powershell
brevity cmux --cleanup-report
brevity cmux --cleanup-report --output json
```

**Value:** Cleanup candidates are currently scattered across the runtime-state
`orphanedTaskWorktrees`, `activeWorktrees`, and individual task worktree
presence fields. A unified cleanup readiness report surfaces actionable
candidates in one view.

**Implementation notes:** Reads `orphanedTaskWorktrees` and `activeWorktrees`
from the runtime-state contract alongside per-task worktree presence. No new
contracts required.

---

### 3.4 Provider Health Drilldown

**What:** An extended provider health view that surfaces degradation history,
time since last update, and whether a degraded provider is blocking any
currently queued tasks.

**Usage sketch:**
```powershell
brevity cmux --section providers --output json
# existing — Phase 2 enriches the providers block
```

**Value:** When a provider is degraded, the operator needs to know: _how long,
which tasks are affected, and whether the queue is stalled because of it?_ The
Phase 1 providers section shows status; Phase 2 shows impact.

**Implementation notes:** Cross-referencing provider status against queued
task providers requires joining the runtime-state providers block with queue
items. This may require a new composite view in the `Snapshot` or an additional
fetch from the runtime-state contract if provider-task assignment is exposed.

---

### 3.5 Blocked Task Report

**What:** A focused list of blocked tasks with the reason each is blocked,
grouped by block type: provider-gated, queue-reserved, manually-blocked, and
unknown.

**Usage sketch:**
```powershell
brevity cmux --blocked-report
brevity cmux --blocked-report --output json
```

**Value:** The existing `--state blocked` filter shows blocked tasks but not
_why_ they are blocked. A dedicated blocked report groups by block type and
surfaces actionable resolution steps for each.

**Implementation notes:** Currently the runtime-state contract exposes
`taskCounts.providerGated` but individual tasks do not carry a `blockedReason`
field. A blocked report at Phase 2 can infer block type from state + provider
availability heuristics. Proper `blockedReason` fields on tasks would improve
fidelity (see §5).

---

### 3.6 Stale Task Report

**What:** A list of tasks that have not progressed in an extended period —
defined as tasks whose last run, last status change, or worktree modification
falls outside a configurable staleness threshold.

**Usage sketch:**
```powershell
brevity cmux --stale-report
brevity cmux --stale-report --output json
```

**Value:** Stale tasks accumulate silently. A stale report surfaces them
before they become cleanup candidates or block queue throughput.

**Implementation notes:** Staleness detection requires timestamps on tasks:
last run time, last state transition, or worktree mtime. The runtime-state
contract currently exposes `latestRunWorkerStatus` and provider/profile fields
but not timestamps (see §5). Phase 2 can approximate staleness from
`generatedAt` minus inferred last-active signals.

---

### 3.7 Queue Readiness Report

**What:** A structured view of the scheduler queue: how many items are runnable,
which items are blocked and why, what the next candidate would be, and whether
reservation eligibility is met.

**Usage sketch:**
```powershell
brevity cmux --queue-report
brevity cmux --queue-report --output json
```

**Value:** The Phase 1 Queue / Scheduler section shows a summary. A dedicated
queue readiness report exposes the full scheduler plan — skipped items, skip
reasons, reservation status — in a focused, filterable view.

**Implementation notes:** The scheduler-plan contract (`brevity.runtime-scheduler-plan.v1`)
already exposes `skipped`, `safetyChecks`, and `reservationEligibility`. A
queue readiness report is a richer rendering of data already in the `Snapshot`.
No new contracts required.

---

### 3.8 AI Handoff Packet

**What:** A single structured artifact that gives an AI agent everything it
needs to reason about the current Brevity runtime state and decide what to do
next. Includes: runtime summary, task fleet state, per-reviewing-task review
packets, queue status, provider health, and suggested next actions — all in one
JSON or Markdown document.

**Usage sketch:**
```powershell
brevity cmux --handoff
brevity cmux --handoff --output markdown
brevity cmux --handoff --output json
```

**Value:** The highest-leverage Phase 2 artifact. An AI agent currently needs
to issue multiple `brevity cmux` commands to build context. The handoff packet
collapses that into a single deterministic artifact that can be injected
directly into an AI context window.

**Implementation notes:** Composite rendering that assembles the existing
section blocks (providers, tasks, queue, actions) with inline review packets
for all tasks in `reviewing` or `ready-for-merge` state. Bounded by a
`--limit` parameter to keep output within AI context budget. No new contracts
required. New JSON schema: `brevity.cmux-handoff.v1`.

---

### 3.9 AI-Vault Export Packet

**What:** A structured export of the CMUX runtime snapshot formatted for
persistent AI context storage — a Vault entry. Includes schema metadata,
generation timestamp, repo root, and a complete fleet state suitable for
diffing across time.

**Usage sketch:**
```powershell
brevity cmux --vault-export
brevity cmux --vault-export --output json
```

**Value:** AI agents that maintain a Vault for runtime context can use this
artifact to track fleet state over time, detect regressions, and compare
pre/post-run states without re-fetching live contracts.

**Implementation notes:** Structurally similar to the handoff packet but
optimized for storage and diffing rather than immediate action. Fixed schema
version for stable deserialization. New JSON schema: `brevity.cmux-vault-export.v1`.

---

## 4. Recommended Implementation Order

The following five slices are ranked by the ratio of operator/AI value to
implementation risk, given the Phase 1 codebase.

| Rank | Slice | Rationale |
|---|---|---|
| **1** | AI Handoff Packet (`--handoff`) | Highest unique value; consolidates multi-command context-gathering into one artifact; no new contracts; directly enables AI-agent workflows; builds on all existing render infrastructure. |
| **2** | Merge Readiness Report (`--merge-report`) | Most common operator question after the default report; pure rendering over existing `Snapshot`; no new contracts; validates grouped-report rendering pattern before generalizing. |
| **3** | Blocked Task Report (`--blocked-report`) | Queue stalls are caused by blocked tasks; surfacing block type and impact unblocks operators; current contract has enough data for a useful heuristic view. |
| **4** | Queue Readiness Report (`--queue-report`) | The scheduler-plan contract already has skip reasons and safety checks; a focused queue report makes that data actionable without new fetches. |
| **5** | Cleanup Readiness Report (`--cleanup-report`) | Cleanup candidates are currently diffuse; a unified cleanup view closes the loop between review → merge → cleanup and reduces orphaned worktree accumulation. |

**Deferred to later in Phase 2:**
- Review Packet Improvements (depends on timestamp fields in contracts — see §5)
- Stale Task Report (depends on timestamp fields — see §5)
- Provider Health Drilldown (depends on provider-task assignment data — see §5)
- AI-Vault Export Packet (build after handoff packet is validated)

---

## 5. Data Contract Needs

Phase 2 can be implemented with the contracts available in Phase 1 for the
top-ranked slices. However, the following additional fields would improve
fidelity for lower-ranked slices. **No contract changes are required now;**
this section documents what to request from the contract owners when Phase 2
work begins.

### 5.1 `brevity.runtime-state.v1` — TaskSummary fields

| Field | Type | Purpose |
|---|---|---|
| `blockedReason` | `string` | Human-readable reason a task is in `blocked` state. Enables grouped blocked report without heuristics. |
| `lastStateTransitionAt` | `string` (ISO 8601) | When the task last changed normalized state. Enables staleness detection. |
| `latestRunStartedAt` | `string` (ISO 8601) | When the latest run began. Enables staleness and run-duration reporting. |
| `latestRunCompletedAt` | `string` (ISO 8601) | When the latest run completed. Complements `latestRunStartedAt`. |
| `latestRunLogPath` | `string` | Path to the latest run log file. Enriches review packets. |
| `cleanupEligible` | `bool` | Whether the runtime considers this task a cleanup candidate. Simplifies cleanup report logic. |

### 5.2 `brevity.runtime-state.v1` — Provider health fields

| Field | Type | Purpose |
|---|---|---|
| `degradedSince` | `string` (ISO 8601) | When the provider entered a degraded/unavailable state. Enables duration-of-degradation reporting. |
| `affectedQueueItems` | `[]string` | Queue item IDs blocked by this provider's degradation. Enables impact analysis in health drilldown. |

### 5.3 `brevity.runtime-scheduler-plan.v1` — SkippedItem fields

| Field | Type | Purpose |
|---|---|---|
| `blockType` | `string` | Categorical block type (`provider-gated`, `reserved`, `no-provider`, `stale`). Enables grouped queue readiness display. |

All of the above are **additive** fields. Existing consumers tolerate unknown
fields via `json.Unmarshal` into typed structs, so additions are non-breaking.

---

## 6. UI / Frontend Guidance

### Why Phase 2 should remain one-shot and report-oriented

The Phase 1 design deliberately excluded interactive UI. Phase 2 should
maintain this constraint for the following reasons:

**Report semantics must be stable before interaction is layered on top.**
An interactive frontend (watch mode, Bubble Tea, web dashboard) forces
decisions about refresh intervals, key bindings, redraw suppression, and
terminal capabilities. These decisions are hard to reverse once made. The
Phase 2 report modes are the primitives that any future frontend would
display — getting those right first means the frontend is a thin rendering
layer, not a coupled component.

**One-shot output is universally composable; interactive output is not.**
`brevity cmux --handoff --output json | jq ...` works in a shell pipeline,
a CI script, an AI agent context, and a log capture without modification.
A live-redraw dashboard does not. Phase 2 report modes should remain
pipe-safe and log-safe.

**AI agent consumption is the primary Phase 2 use case.**
AI agents cannot use interactive TUI output. The handoff packet and
vault export are explicitly designed to be injected into AI context windows.
Coupling Phase 2 to a terminal interaction model would exclude the highest-value
consumer.

**The correct path to interaction:**
1. Validate Phase 2 report semantics through human and AI use.
2. Define a stable `Snapshot` shape (with Phase 2 data contract additions).
3. If interactive UI is needed, adopt Bubble Tea with `Snapshot` as the model
   and `Render` as a pure view — the architecture already supports this (see
   `docs/cmux-runtime-reader.md` § Future Extension Points).

### When to consider a frontend

Adopt an interactive frontend when **all** of the following are true:
- Phase 2 report modes are in stable use.
- The `Snapshot` shape is stable (no pending additive contract changes in flight).
- A concrete operator workflow has been identified that requires less than
  three keystrokes rather than a single command invocation.
- The frontend's test surface (rendering, keyboard, state) can be isolated
  from the report rendering surface.

---

## 7. Safety Model

CMUX Phase 2 inherits and extends the Phase 1 safety model. Every Phase 2
artifact must satisfy all five properties:

### Read-only
CMUX never writes files, spawns workers, mutates task state, reserves queue
items, or calls any mutating CLI command. Every output is derived exclusively
from the two runtime contracts. This must remain true regardless of how rich
Phase 2 reports become.

### Explicit
No Phase 2 mode takes implicit action. A report that says _"this task is ready
to merge"_ does not trigger a merge. A report that lists cleanup candidates
does not initiate cleanup. The operator or AI agent reads the report and
decides what to do next. CMUX informs; it does not act.

### Deterministic
For a given `Snapshot` and `RenderOptions`, every render call must produce
identical output. This property is what makes watch-mode redraw suppression
possible (future), AI response caching reliable, and CI diffs stable. No
Phase 2 report mode may introduce randomness, wall-clock timestamps in output
(only in metadata), or non-deterministic ordering.

### Remote-safe
All Phase 2 outputs must work correctly over SSH, WinRM, CI pipelines, and
agent subprocess invocations without modification. This means:
- No ANSI escape sequences.
- No terminal clearing.
- No interactive prompts.
- No watch mode or blocking loops.
- Exit immediately after rendering.

### AI-ingestable
Phase 2 outputs — especially the handoff packet and vault export — are
explicitly designed for AI context injection. This means:
- `--output markdown` produces clean GFM with no HTML, no ANSI, no code fences
  wrapping the whole document.
- `--output json` produces typed, versioned JSON with a stable schema
  identifier, non-null slices, and forward-compatible additive evolution.
- Text output is plain ASCII with no visual noise (no box-drawing characters,
  no padded columns with excessive whitespace).
- Every report mode has a well-defined empty-state behavior (no crashes,
  no silent omissions).

---

## 8. Phase 2 Exit Criteria

Phase 2 is complete when all of the following are true:

### Functionality
- [ ] AI Handoff Packet (`--handoff`) implemented and tested in text, markdown,
  and JSON output modes.
- [ ] Merge Readiness Report (`--merge-report`) implemented and tested.
- [ ] Blocked Task Report (`--blocked-report`) implemented and tested.
- [ ] Queue Readiness Report (`--queue-report`) implemented and tested.
- [ ] Cleanup Readiness Report (`--cleanup-report`) implemented and tested.

### Quality
- [ ] All five new report modes have `TestXxx_*` test coverage including:
  empty-state, partial-snapshot (one contract unavailable), filter interaction,
  determinism, no-ANSI, and JSON schema validation.
- [ ] All new JSON output uses versioned schema identifiers.
- [ ] `docs/cmux-runtime-reader.md` is updated to document Phase 2 flags and
  output modes.
- [ ] No new ANSI sequences, watch mode, interactive input, or mutation code
  introduced.

### Validation
- [ ] `go test ./internal/cmux` passes with no new test failures.
- [ ] `go test ./cmd/brevity` passes with no new test failures.
- [ ] `brevity cmux --handoff --output json` produces valid, schema-tagged JSON
  that can be parsed by `jq` and injected into an AI context window without
  modification.
- [ ] `brevity cmux --help` documents all Phase 2 flags.
- [ ] `git diff --check` clean on all changed files.

### Architecture
- [ ] All new render modes follow the existing dispatch pattern:
  `Render → renderXxx → renderTextXxx / renderMarkdownXxx / renderJSONXxx`.
- [ ] No new direct `.brevity` file reads introduced anywhere in `internal/cmux`.
- [ ] `Fetcher` interface is unchanged or extended only with new optional
  contract methods (backward-compatible).
- [ ] `Snapshot` struct is extended only with additive fields (no removals,
  no renames).

---

## Appendix: Phase 2 Workflow Summary

| Workflow | Flag sketch | New contracts needed | Rank |
|---|---|---|---|
| AI Handoff Packet | `--handoff` | None | 1 |
| Merge Readiness Report | `--merge-report` | None | 2 |
| Blocked Task Report | `--blocked-report` | `blockedReason` (optional) | 3 |
| Queue Readiness Report | `--queue-report` | `blockType` on skipped items (optional) | 4 |
| Cleanup Readiness Report | `--cleanup-report` | None | 5 |
| Review Packet Improvements | `--review` (existing) | Timestamp fields | Deferred |
| Stale Task Report | `--stale-report` | Timestamp fields | Deferred |
| Provider Health Drilldown | `--section providers` (enriched) | `degradedSince`, `affectedQueueItems` | Deferred |
| AI-Vault Export Packet | `--vault-export` | None (after handoff) | Deferred |
