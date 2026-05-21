# Brevity Concepts

Brevity is a repo scaffold and command vocabulary for AI-assisted development on a
Windows machine. It keeps the useful parts of the original bootstrap script
while moving from many generated helper scripts to one CLI entry point.

## From Bootstrap Script To Brevity

The bootstrap script created a full workspace and wrote helper scripts under
`.system`. Brevity keeps the same operating model, but consolidates repo-local
runtime state under `.brevity`:

| Bootstrap concept | Brevity concept |
| --- | --- |
| `.system` | `.brevity` |
| bootstrap onboard helper | `Brevity onboard` |
| bootstrap task helper | `Brevity task new` |
| bootstrap status helper | `Brevity status` |
| AI-Vault | AI-Vault remains supported |
| `worktrees\active`, `paused`, `completed` | first-class Brevity task state |

## Dev Root

The dev root is the top-level workspace folder that contains repos, worktrees,
vaults, scratch files, and Brevity orchestration state.

```text
<dev-root>\
  .brevity\
  repos\
  worktrees\
  vaults\
  scratch\
```

Brevity commands should accept an explicit `-DevRoot` when useful and otherwise
default to `C:\dev`.

## .brevity

`.brevity` is the orchestration area. It replaces `.system` from the bootstrap
script. `Brevity init` creates it in the current Git repository and adds:

- `tasks.json`
- `config.json`

`tasks.json` starts as an empty array. It is runtime state only: Brevity uses it
to track worktrees, branches, prompts, statuses, and cleanup state for task
work that Brevity has already created. Durable planned work belongs in vault task
specs, not in `.brevity\tasks.json`.

`provider-health.json` records lightweight runtime health metadata for configured
AI providers. New workspaces start each provider as `unknown`:

```json
{
  "codex": {
    "status": "unknown",
    "note": "",
    "updatedAt": null
  },
  "gemini": {
    "status": "unknown",
    "note": "",
    "updatedAt": null
  }
}
```

Provider health is scheduler metadata, not task lifecycle state. It must not
change task status, worktree state, branch state, merge state, or cleanup state.
The supported health states are:

- `healthy`
- `capacity-degraded`
- `quota-constrained`
- `unavailable`
- `unknown`

For example, Gemini `MODEL_CAPACITY_EXHAUSTED` can be represented as
`capacity-degraded`, while Codex low quota can be represented as
`quota-constrained`. Brevity v0 does not automatically route, retry, or fall back
based on this file; it only provides a clear local place to record the state for
future routing decisions.

`Brevity provider status` reads `.brevity\provider-health.json` and prints each
provider's status, update timestamp, and note.

`Brevity provider set <provider> <status> [-Note <note>]` updates one provider
record in `.brevity\provider-health.json`. The provider must already exist in
the health file, and the status must be one of the supported health states. The
command refreshes `updatedAt` with a UTC timestamp and stores the note when one
is supplied.

`Brevity logs recent` gives a concise operator view of recent runtime activity.
It tails the vault `runtime-log.md` file and lists the most recent worker log
files under `.brevity\logs` without changing either log format. Use
`--count <n>` to control both recent runtime-memory entries and worker-log
entries.

`Brevity logs task <slug>` prints the latest worker log path for one task and a
small tail of that log. Use `--tail <n>` to control the number of worker-log
lines shown. If no worker log exists for the task, it reports the expected log
folder cleanly. The command is read-only.

`Brevity task runs <slug>` lists recent worker runs for one task, most recent
first. It prefers the append-only `.brevity\runs.jsonl` index when available and
falls back to scanning `.brevity\logs\<slug>\*.log`. Worker logs remain the
source of detailed output. Use `--json` for structured output.

`Brevity task runs reconcile --dry-run` scans `.brevity\runs.jsonl` for stale
or incomplete worker run records and prints a conservative operator report. It
does not mutate the run index. Without `--dry-run`, the command refuses safely.
Future reconciliation may mark stale/incomplete runs after operator review, but
v1 is report-only.

`Brevity task runs retention --dry-run` reports read-only run index retention
signals before any compaction exists: path, size, record counts, parse failures,
oldest and newest timestamps, top tasks by record count, and stale/incomplete
record counts. It refuses without `--dry-run` and does not mutate
`.brevity\runs.jsonl`. Use `--json` for structured output.

`Brevity task runs compact --dry-run` reports a read-only compaction plan for
`.brevity\runs.jsonl`. It applies the retention policy conceptually, preserves
the latest 20 indexed runs per task, preserves stale or incomplete records,
preserves failed records, and reports older successful completed runs as
archive/summary candidates. It refuses without `--dry-run`, never mutates the
run index, and never touches worker logs. Use `--json` for structured output.

`Brevity task preflight <new|start|run|merge|cleanup> <slug>` is the native Go
safety gate for future task mutations. It reads repo state, task metadata,
provider health, lock state, worktree/branch signals, and cleanup/orphan
inspection data, then reports whether the requested mutation would be allowed,
warned, or blocked. `--json` emits the stable `brevity.task-preflight.v1`
contract used by CLI, TUI, and operator flows.

Preflight never executes the mutation it describes. It does not create/delete
worktrees, create/delete branches, write `tasks.json`, or launch providers or
workers. PowerShell still owns task mutation execution until a later migration
explicitly moves execution into Go.

### Run Index Retention

`.brevity\runs.jsonl` remains an append-only worker run index for now. It is the
fast source for recent per-task run history and latest-run runtime summaries,
while worker logs remain the source of detailed output. Brevity v1 should never
delete worker logs automatically.

The default retention policy for future run-index compaction is:

- Keep at least the latest 20 indexed runs per task.
- Keep every incomplete or stale record until reconciliation has reviewed it.
- Keep failed runs longer than successful runs.
- Prefer archival or summary records for old completed runs instead of silent
  discard.
- Treat retention warnings in a future TUI as advisory until an operator chooses
  an explicit action.

Only dry-run compaction planning exists. Future mutating compaction must be
explicit, dry-run-first, and protected by locking before it mutates
`.brevity\runs.jsonl`.

The planned future mutation command shape is:

```powershell
.\brevity.ps1 task runs compact --execute
.\brevity.ps1 task runs compact --execute --archive
```

These commands are not implemented yet. Before any future implementation writes
the run index, it must acquire `.brevity\runs.lock`, reread
`.brevity\runs.jsonl` under that lock, recompute the compaction plan, create a
backup, write and validate a compacted temporary file, validate archive records
when produced, replace `.brevity\runs.jsonl` atomically when possible on
Windows, report the backup path, and release the lock in a `finally` block.
Failed validation must abort before replacement, failed replacement must leave
the original run index in place, and backups must never be deleted
automatically. Compaction remains explicit rather than automatic, and a future
TUI must show the exact action before executing it.

### Run Index Archive Format

Future run-index compaction should write additive archive records instead of
silently discarding old run records. The v1 archive contract is described in
[`docs/run-index-archive-format.md`](run-index-archive-format.md), with a JSON
schema at
[`docs/run-index-archive.schema.json`](run-index-archive.schema.json).

Archive records summarize a bounded set of old runs for one task slug. They
must include a schema/version marker, the task slug, an archive timestamp, the
number of preserved runs summarized, oldest and newest run boundaries, outcome
counts, stale/incomplete counts, and either explicit archived run IDs or compact
ranges. Future compaction should still preserve the latest N indexed runs per
task in `.brevity\runs.jsonl`.

Archive summaries are not deletion receipts for worker logs. Brevity v1 must not
delete raw worker logs automatically, and stale or incomplete run records must
not be compacted away silently. They should remain indexed until reconciliation
has reviewed them or the archive record explicitly preserves their status for
operator inspection.

### Runtime State JSON

`Brevity runtime state --json` emits the read-only runtime inspection contract
for future TUI and automation consumers. The human dashboard may change for
operator readability, but JSON consumers should target the machine contract
instead of parsing console text.

The top-level `schema` field identifies the contract version. The current value
is `brevity.runtime-state.v1`. Contract evolution should be additive whenever
possible: consumers should tolerate unknown fields, and Brevity should avoid
removing or renaming existing fields within v1 unless there is a deliberate
schema break. Breaking changes should move to a new schema value, for example
`brevity.runtime-state.v2`.

The matching schema lives at
[`docs/runtime-state.schema.json`](runtime-state.schema.json). It reflects
`brevity.runtime-state.v1` for TUI and automation consumers and should evolve
additively when possible.

The centralized contract index lives at [`docs/contracts.md`](contracts.md). It
indexes runtime/TUI contracts and schemas for automation and TUI consumers,
including runtime-state and command-result contracts.

### TUI MVP

`Brevity tui` is the original minimal PowerShell runtime/operator dashboard
scaffold. It is read-only and polls `Brevity runtime state --json` on a
conservative interval instead of owning orchestration state itself.

The v1 TUI renders compact runtime summary, provider health, task counts by
normalized state, recent tasks, cleanup warnings, and stale or incomplete run
indicators. It deliberately avoids mutations, task execution, background
workers, command palettes, event streaming, embedded Git UI, and embedded
editing.

Future interactive features should keep a clear boundary between the runtime
client, renderer, and action system. Mutating actions should use structured
command-result contracts and remain explicit.

### Go Runtime Client

The Go CLI under `cmd\brevity` is currently a frontend/runtime client over the
PowerShell backend. It can render the dashboard and dispatch selected
PowerShell-backed action runners, but it does not own runtime state and does
not write `.brevity` metadata directly.

The Go dashboard and watch mode are the active direction for the future
operator UX. The PowerShell TUI remains useful as a lightweight reference
scaffold, and both dashboard paths currently consume PowerShell-produced
runtime-state style data.

The supported Go command surface is intentionally smaller than the PowerShell
surface. The authoritative command list and implementation status live in the
[Go frontend support matrix](go-support-matrix.md).

PowerShell JSON contracts are the source of truth for both read-only runtime
state and mutating command results. There is no interactive mutation UI yet in
either the PowerShell TUI or the Go dashboard, and native Go `.brevity` state
ownership remains a future migration step.

The v1 snapshot includes these major sections:

- `providers` - provider health totals and per-provider health records.
- `taskCounts` - tracked, runnable, blocked, stale, provider-gated, and review
  task counts.
- `tasks` - runtime task summaries sorted by slug.
  Each task summary includes compact worker lifecycle fields such as
  `workerStatus`, `lastRunStartedAt`, `lastRunFinishedAt`, `lastExitCode`,
  `lastFailureType`, `lastLogPath`, `lastProvider`, and `lastProfile`.
  Latest run summary is read from `.brevity\runs.jsonl` when available, with
  worker log scanning retained as a fallback.
- `groups` - runtime task slug groups such as runnable, blocked, stale,
  provider-gated, and review.
- `orphanedTaskWorktrees` - active task-like worktrees missing from
  `.brevity\tasks.json`.
- `lock` - task metadata lock status, lock path, and lock age in minutes.
- `runtimeMemory` - runtime log metadata plus recent runtime-memory entries.
- `suggestedNextActions` - suggested operator actions based on the snapshot.

`config.json` records:

- `projectName`
- `devRoot`
- `vaultPath`
- `worktreesRoot`
- `defaultProvider`
- `providers`

For new configs, `worktreesRoot` points at:

```text
<dev-root>\worktrees\active
```

New configs also include worker run settings:

```json
{
  "defaultProvider": "gemini",
  "providers": {
    "codex": {
      "command": "codex",
      "mode": "exec",
      "sandbox": "workspace-write",
      "model": null,
      "profile": null,
      "executionPolicy": "Bypass",
      "autoExecute": false
    },
    "gemini": {
      "command": "gemini",
      "model": "gemini-3-flash-preview",
      "approvalMode": "yolo",
      "skipTrust": true,
      "env": {
        "GEMINI_API_KEY": "GEMINI_API_KEY"
      }
    },
    "copilot": {
      "command": "copilot",
      "allowAllTools": true,
      "allowAllPaths": true,
      "noAskUser": true
    }
  }
}
```

Existing files are left unchanged by normal init.

Set `defaultProvider` or pass `--profile <name>` to select a worker provider.
With Gemini, Brevity passes the task prompt text with `-p` and passes
`-m <model>` when configured. Set `providers.gemini.skipTrust` to `true` to pass
`--approval-mode yolo`. Set `providers.gemini.env` to an object of environment
variables, such as `GEMINI_API_KEY`, when Gemini authentication should be scoped
to the worker process. Dry runs print configured variable names but mask values.

`Brevity init --repair [-DevRoot <path>]` is the corrective init mode. It
re-detects `projectName` from the Git repository root folder and recomputes:

- `vaultPath` as `<dev-root>\vaults\AI-Vault\10-Projects\<project-name>`
- `worktreesRoot` as `<dev-root>\worktrees\active`

If `.brevity\config.json` is missing, repair mode creates it. If it exists, repair
mode updates the known Brevity fields only when they are wrong and preserves
unknown or custom fields. It also creates the same missing `.brevity` files,
folders, and vault project memory paths as normal init. Existing vault memory
files are not overwritten. Repair also adds missing Codex run settings without
removing custom config fields.

Repair output reports repaired fields, unchanged fields, created paths, and
already-existing paths.

`Brevity plan` reads `config.json` and writes:

```text
<repo>\.brevity\planner-prompt.md
```

The planner prompt is a manual handoff prompt for Codex. It tells Codex to read
`AGENTS.md`, read the configured `vaultPath` project memory, select exactly one
small high-value task, and return:

- task title
- task slug
- worker prompt

The planner prompt also tells Codex not to implement code, create a worktree,
call Codex automatically, propose autonomous planning, or use placeholders.
Brevity prints the prompt path and tells the operator to open Codex in the repo and
paste the planner prompt.

`Brevity plan backlog` reads the same config and writes:

```text
<repo>\.brevity\planner-backlog-prompt.md
```

The backlog planner prompt is also a manual handoff prompt for Codex. It tells
Codex to read `AGENTS.md`, read the configured `vaultPath` project memory, and
plan a larger body of work as 5-10 small tasks. Each returned task must include:

- `title`
- `slug`
- `status: planned`
- `dependencies: []`
- `workerPrompt`

The backlog prompt tells Codex to keep tasks small and independently executable
where possible, avoid placeholders, and not implement code. Brevity prints the
backlog prompt path and tells the operator to open Codex in the repo and paste
the backlog planner prompt. Brevity does not parse planner output, create tasks
from the backlog, implement a TUI, or launch Codex. Planned backlog work belongs
in Markdown files under:

```text
<vaultPath>\tasks\
```

## AI-Vault

AI-Vault remains the durable knowledge store for project memory and global
workflow notes. The expected location is:

```text
vaults\AI-Vault\
  00-Inbox\
  01-Global\
  02-Ideas\
  10-Projects\
  90-Archive\
```

Project-specific memory belongs under:

```text
vaults\AI-Vault\10-Projects\<project-name>\
```

`Brevity init` creates the project memory folder and these child directories when
missing:

- `session-notes\`
- `tasks\`

It also creates these starter Markdown files when missing:

- `project.md`
- `architecture.md`
- `decisions.md`

If `AGENTS.md` is missing in the repository, `Brevity init` creates one that tells
Codex to read the project vault memory before doing work. If `AGENTS.md`
already exists, Brevity leaves it unchanged.

Vault task specs are durable planned work. Brevity supports vault-backed task
specs for planned task handoff. Each planned task can be stored as:

```text
<vaultPath>\tasks\<slug>.md
```

`Brevity task spec <slug>` reads `.brevity\config.json`, uses `vaultPath`, and prints
the matching task spec Markdown file when it exists. If the spec is missing,
Brevity reports the expected path. The command is read-only: it does not create
worktrees from specs, parse backlog planner output, or change
`.brevity\tasks.json`.

## Repos

Repos live under the dev root by area:

```text
repos\
  active\
  experiments\
  archive\
  clients\
```

Brevity should not own project source files except when a command explicitly
onboards a repo or creates a requested file.

## Worktrees

Worktrees are first-class Brevity task workspaces:

```text
worktrees\
  active\
  paused\
  completed\
```

`Brevity task new <slug>` creates a branch named:

```text
task/<slug>
```

and a worktree named:

```text
worktrees\active\<repo-name>-<slug>
```

Brevity should keep that convention for `Brevity task new`. The command also writes
a simple `prompt.md` file into the new worktree so a worker has a durable task
starting point without launching any automation.

Task metadata lives in the source repository:

```text
<repo>\.brevity\tasks.json
```

Each record includes:

- `slug`
- `branch`
- `worktreePath`
- `promptPath`
- `specPath`
- `status`
- `createdAt`

New task records use `ready-for-worker` status.

`Brevity task activate <slug>` is the vault-backed task creation flow. It reads
`.brevity\config.json` and uses:

- `vaultPath`
- `worktreesRoot`
- `projectName`

Brevity reads the planned task spec from:

```text
<vaultPath>\tasks\<slug>.md
```

It creates branch `task/<slug>` and a worktree at:

```text
<worktreesRoot>\<projectName>-<slug>
```

The task spec contents are embedded in a bounded worker prompt at:

```text
<worktreePath>\prompt.md
```

The vault task spec remains unchanged. Brevity records runtime metadata in
`.brevity\tasks.json`, including `specPath`, and sets the task status to
`ready-for-worker`. The command does not launch Codex and does not run worker
automation.

`Brevity board` reads the same metadata file and groups tasks by status. It shows
groups when matching tasks are present, including:

- `planned`
- `ready-for-worker`
- `running`
- `merged`
- `done`
- `blocked`

For runtime consumers, Brevity derives a canonical `normalizedState` beside the
existing metadata `status`. The metadata status is preserved for backward
compatibility; normalized state is the predictable grouping field intended for
the TUI.

Canonical task states:

- `planned` - durable task spec exists, but runtime task metadata has not been created.
- `ready-for-worker` - task metadata exists and the task is eligible for worker execution.
- `running` - the latest worker run is active, unknown-running, or incomplete.
- `succeeded` - task work is complete under a legacy completed/done status.
- `failed` - the latest worker run failed.
- `reviewing` - the latest worker run succeeded and the task branch has not been merged.
- `merged` - task branch integration succeeded and cleanup remains.
- `stale` - recorded worktree, branch, prompt, or Git registration facts are missing.
- `blocked` - provider health or task metadata prevents safe execution.
- `orphaned` - task-like runtime facts exist without matching task metadata.

The normalizer maps older statuses such as `done`, `completed`, and
`stale-*` runtime statuses into the canonical set. Unknown non-empty metadata
statuses are treated as `blocked` until a future explicit transition model
defines them.

For each task, it reports:

- `slug`
- `branch`
- `worktreePath`

If no task metadata exists, Brevity reports that no Brevity tasks were found. The
board is read-only and does not start task work, run planner automation, merge
branches, clean up worktrees, or otherwise change task lifecycle state.

`Brevity task status` reads the same metadata file and reports:

- `slug`
- `branch`
- `status`
- `worktreePath`
- `promptPath`

If the metadata file does not exist, Brevity reports that no Brevity tasks were
found.

`Brevity task start <slug>` is Go-owned task lifecycle mutation. It reads the
same metadata file, runs native mutation preflight, acquires the advisory
`.brevity\state.lock`, and updates the matching task record to
`ready-for-worker`.

Start updates `updatedAt`, sets `startedAt` when absent, preserves unrelated and
unknown task fields, and returns `brevity.command-result.v1` in JSON mode. It
does not launch Codex, run a provider/worker, create or delete branches, create
or delete worktrees, or materialize prompt/context files. PowerShell task start
remains available only as legacy/reference behavior.

The command also tells the operator to read `prompt.md` and follow it exactly.
It does not automatically launch Codex or run planner automation.

Before printing the command, Brevity refreshes `prompt.md` from the matching
vault task spec when one exists and refreshes `.brevity\context` in the
worktree from selected project memory files. The prompt keeps the worker bounded
by including the task slug, embedded spec contents, local context guidance,
constraints, acceptance checks, and stop-after-summary instructions.

`Brevity task run <slug> --plan --json [--profile <name>]` is now owned by
native Go. It reads the same metadata file and returns a materialized execution
envelope for operators and the TUI. The envelope includes task state,
provider/profile/model resolution, prompt freshness, worktree/prompt paths, a
planned run id, planned log/stdout/stderr paths, argv-style worker command
shape, expected future state mutations and files, warnings, blockers, and
`authority: native-go`.

Planning is read-only. It does not launch Codex, Gemini, Antigravity, or any
worker process. It does not write `.brevity\runs.jsonl`, update task state, or
materialize execution logs. Run Worker in the Bubble Tea dashboard consumes this
native plan and remains plan-only.

`Brevity task run <slug> --execute [--profile <name>] [--smoke] [--force-provider]`
remains the legacy PowerShell execution path. Native Go execution is not
implemented yet.

The task-run command reads the matching task record and plans:

- task slug
- worktree path
- prompt path
- headless worker command argv/display shape

The command is built from worker settings in `.brevity\config.json` or overridden by
the provided profile. The configured
provider may be `codex` or `gemini`. The headless Codex command format is:

```text
codex exec -C <worktreePath> -s <sandbox> prompt.md
```

If `model` is configured, Brevity includes `-m <model>`. If `profile` is
configured in the Codex provider config, Brevity includes `-p <profile>`.
Brevity worker profiles such as `codex-balanced` are orchestration metadata;
they select the Codex provider but are not passed to Codex as native `-p`
config profiles.

The headless Gemini command format is:

```text
Set-Location -LiteralPath <worktreePath>; gemini -s -p (Get-Content -LiteralPath 'prompt.md' -Raw)
```

If `model` is configured, Brevity includes `-m <model>`. Set `sandbox` to `none` or
blank to omit `-s`. Set `skipTrust` to `true` to include
`--approval-mode yolo`. With `--execute`, Brevity applies
`executionPolicy` to the worker process only before running the generated
command. The default `Bypass` value is scoped to the child process and does not
change the user's machine policy. With `--plan`, Brevity prints or returns the
native execution envelope only. With legacy `--execute`, PowerShell runs the
generated command. Unsupported worker providers return a clear
unsupported-provider error.

Before a worker handoff, Brevity materializes only selected durable project
memory files into the task worktree at `.brevity\context\`: `project.md`,
`architecture.md`, `decisions.md`, `current-state.md`, and `roadmap.md`. Missing
files are skipped. Workers read those local files instead of external vault
paths, preserving the split between vault durable memory and bounded worktree
execution context.

Normal operator flow:

```powershell
.\brevity.ps1 task new my-task
go run ./cmd/brevity task runtime-info my-task
go run ./cmd/brevity task refresh-context my-task
go run ./cmd/brevity task runtime-info my-task --json
```

`task runtime-info` exposes the current prompt path, prompt existence, prompt
refresh status, metadata `status`, derived `normalizedState`, and last known
worker lifecycle state for the task.

Go owns prompt/context refresh. `task refresh-context` runs native preflight,
loads task metadata from `.brevity\tasks.json`, reads configured vault memory
when available, rewrites the task `prompt.md`, refreshes selected bounded
context files under the worktree `.brevity\context\`, and records
`promptRefreshedAt` plus `promptRefreshStatus` in task metadata. The legacy
`task context refresh` command shape is kept as a compatibility alias in the Go
CLI.

The vault remains durable memory. Runtime state remains under `.brevity`.
Refresh copies only selected vault files into the task worktree:
`project.md`, `architecture.md`, `decisions.md`, `current-state.md`, and
`roadmap.md`. Missing vault files are optional and reported as missing context;
they do not make the vault a hard dependency. Workers read local materialized
context instead of external vault paths, preserving the authority split between
durable project memory and bounded execution context.

The generated prompt is deterministic apart from task/vault inputs. It includes
the task slug, state, embedded task spec when present, local context guidance,
provider/profile hints when present, execution constraints, and operator
acceptance guidance. Refresh does not launch providers or workers, does not
merge branches, and does not clean up or create worktrees.

## Gemini Trust

Gemini CLI uses a parent-folder trust model. When you run `gemini` from a
worktree, it checks for a `.gemini` folder in the parent directory. If found,
Gemini trusts the worktree and its contents.

This is necessary for Brevity's dynamic worktree workflow. Each task gets a
fresh worktree, but the trust root remains the same.

The dev root is the trust root:

```text
<dev-root>\
  .gemini\
  repos\
  worktrees\
```

To set this up, create a `.gemini` folder in your dev root.

Gemini also requires an API key, sourced from `GEMINI_API_KEY`. Brevity can
pass this to the worker process from its own secure configuration. See the main
README for details.

`Brevity task cleanup <slug> [--force]` reads the same metadata file and finds the
matching task record.

Without `--force`, Brevity keeps the safe cleanup behavior: it removes the
recorded Git worktree, deletes the recorded Git branch with `git branch -d`,
and removes the task record only after cleanup succeeds.

With `--force`, Brevity removes the worktree with
`git worktree remove --force <worktreePath>` and deletes the branch with
`git branch -D <branch>`. If the recorded worktree path is already missing or
is not registered with Git, Brevity prints a warning and continues to branch
removal. Brevity removes the task record when branch removal succeeds or the
branch is already missing. If branch removal fails for another reason, the
metadata stays in place so the task can be inspected or retried explicitly.

`Brevity task cleanup-orphans --dry-run` reports registered task-like worktrees
under the active worktree root that are missing from `.brevity\tasks.json`.
`Brevity task cleanup-orphans --execute` is the explicit mutating form. Before
each removal, Brevity re-checks that the worktree is still registered, still
under the active worktree root, still on a `task/*` branch, and still missing
matching task metadata. It skips uncertain candidates and only deletes a branch
after the worktree removal succeeds and the branch still exists as `task/*`.
Dirty orphaned worktrees are treated as unsafe: Brevity reports tracked and
untracked changes when detectable, prints inspection commands, and leaves the
worktree untouched.

`Brevity task cleanup-orphan-branches --dry-run` reports local `task/*` branches
that have no matching `.brevity\tasks.json` metadata and are not currently
checked out in any registered Git worktree. It prints whether each branch is
merged into the current `HEAD` when Git can report that easily, plus the manual
`git branch -D <branch>` command. Without `--dry-run`, it refuses safely.

`Brevity task merge <slug>` reads the same metadata file, finds the matching task
record, and merges the recorded branch into the current Git branch with
`git merge <branch>`. When the merge succeeds, Brevity updates the task status to
`merged`. It does not clean up the worktree, delete the branch, or remove task
metadata. If the merge fails, the metadata stays unchanged.

## Provider Capabilities

Brevity supports different AI providers, such as Codex and Gemini. These providers may expose different tools and capabilities. For example, some providers may offer file system tools like `replace` and `write_file`, while others may not.

When writing prompts for Brevity workers, avoid assuming the availability of provider-specific tools. Gemini may report that a tool is unavailable if it is not supported by the current provider. In such cases, re-phrase the prompt in provider-neutral terms that describe the goal, not the specific tool to achieve it.

## Worker Profiles

Brevity uses stable worker profiles to decouple task planning from volatile provider
model names. Provider model IDs are internal implementation details that may
change as new models are released or tiers are renamed.

Planners should select the cheapest profile sufficient for the task's complexity
tier.

Brevity stores profile metadata in a central capability matrix in `brevity.ps1`.
Each entry records:

- `provider`
- `costTier`
- `capabilityTier`
- `complexityFit`
- `intendedUse`
- optional `model`
- optional `providerConfig` overrides

Planner logic should reason about stable profile capabilities instead of
provider model IDs. The matrix is scheduler metadata for future routing, but it
can also supply provider-native execution settings when those settings are
explicitly known.

Canonical profile names are the source of truth. Brevity v0 defines:

- `gemini-lite`
- `gemini-flash`
- `gemini-pro`
- `codex-fast`
- `codex-balanced`
- `codex-deep`
- `copilot`

Brevity also accepts aliases for operator ergonomics. An alias is resolved to
its canonical profile before worker resolution, so the selected provider,
model, and provider-native settings still come from the canonical profile.
Aliases are convenience names only; they are not separate profiles.

Common alias examples:

```text
gemini-fast -> gemini-flash
codex-default -> codex-balanced
```

Practical usage:

```powershell
.\brevity.ps1 task run my-task --profile gemini-fast --execute
.\brevity.ps1 task run my-task --profile codex-default --execute
```

`brevity.ps1` also defines planner-only complexity defaults in
`Get-BrevityComplexityProfileDefaults`:

| Complexity | Preferred profiles |
| --- | --- |
| `low` | `codex-fast`, `gemini-lite`, `gemini-flash` |
| `medium` | `codex-balanced`, `gemini-flash`, `gemini-pro` |
| `high` | `codex-deep`, `gemini-pro` |

These defaults guide manual planners and future scheduler routing. They do not
execute workers, mutate task status, invoke provider health, fall back
automatically, or override an explicit `--profile` selection.

### Gemini Profiles

| Profile | Cost | Capability | Complexity fit | Model |
| --- | --- | --- | --- | --- |
| `gemini-lite` | low | lite | low | none by default |
| `gemini-flash` | low | fast | low, medium | `gemini-3-flash-preview` |
| `gemini-pro` | medium | pro | medium, high | none by default |

### Codex Profiles

| Profile | Cost | Capability | Complexity fit | Model |
| --- | --- | --- | --- | --- |
| `codex-fast` | low | fast | low | none by default |
| `codex-balanced` | medium | balanced | medium | none by default |
| `codex-deep` | high | deep | high | none by default |

### Other Canonical Profiles

| Profile | Cost | Capability | Complexity fit | Model |
| --- | --- | --- | --- | --- |
| `copilot` | low | default | low, medium, high | none by default |

These are Brevity worker profiles, not Codex CLI config profiles. Passing
`--profile codex-balanced` to `Brevity task run` must not become
`codex exec ... -p balanced`; Codex `-p` is reserved for an explicitly configured
native Codex provider profile.

If a selected worker profile has a non-empty `model`, Brevity passes it to the
provider using the supported native model flag, such as Codex `-m`. Codex profile
entries intentionally do not name a model until a safe cheaper or stronger Codex
model is explicitly known in config or docs.

## Task Complexity Tiers

- `low`: Simple changes, unit tests, documentation updates. Prefer `codex-fast`,
  `gemini-lite`, then `gemini-flash`.
- `medium`: Feature implementation, cross-file refactoring, integration tests.
  Prefer `codex-balanced`, `gemini-flash`, then `gemini-pro`.
- `high`: Architectural changes, deep debugging, complex migrations. Prefer
  `codex-deep`, then `gemini-pro`.

## Fallback Guidance

When a preferred profile is unavailable due to quota exhaustion or capacity limits:

1.  **Capability Fallback:** Move to the next more capable profile within the same
    provider (e.g., `gemini-flash` -> `gemini-pro`).
2.  **Cross-Provider Fallback:** Move to the equivalent tier in a different
    provider (e.g., `gemini-flash` -> `codex-balanced`).
### Graceful Degradation: As a last resort, fall back to a less capable profile.
    Note that this may require more manual intervention or iterative refinement.

## Capacity Handling

Brevity treats `429` errors (quota exhaustion or model capacity) as **worker
infrastructure failures**, not task failures. These errors indicate that the
provider cannot fulfill the request at this time, but the task itself remains
valid and unchanged.

When a capacity failure occurs, Brevity fails the command but provides guidance
on how to proceed.

### Capacity Error Types

-   `QUOTA_EXHAUSTED`: The API key has reached its usage limit for the current
    period.
-   `MODEL_CAPACITY_EXHAUSTED`: The provider is currently overloaded and cannot
    accept new requests for that specific model.
-   `No capacity available for model...`: Similar to capacity exhaustion, often
    localized to a region or tier.

### Strategies for Capacity Failures

1.  **Retry Strategy:** Since capacity errors are often transient, waiting a few
    minutes and retrying is the simplest first step.
2.  **Profile Switching:** If one profile is exhausted, another might still have
    capacity. For example, if `gemini-flash` is overloaded, `gemini-pro` might
    be available (or vice versa). Use the `--profile` flag to manually route
    to a different worker profile:

    ```powershell
    # Switch to a more capable profile
    .\brevity.ps1 task run <slug> --execute --profile gemini-pro

    # Switch to a lower-latency profile
    .\brevity.ps1 task run <slug> --execute --profile gemini-lite

    # Switch to a different provider profile
    .\brevity.ps1 task run <slug> --execute --profile codex-balanced
    ```

3.  **Provider Switching:** If a provider is completely down or exhausted,
    selecting a different worker profile, such as moving from a Gemini profile
    to `codex-balanced`, can unblock work.

Automatic retry and profile switching are planned for future versions of Brevity.
In v0, these actions must be performed manually by the operator.

## Workspace Lifecycle Hygiene

Brevity promotes a high-velocity, high-hygiene lifecycle for AI-assisted work.
Because AI workers can iterate rapidly, a workspace can quickly accumulate
stale branches and worktrees if cleanup is treated as optional maintenance.

### Short-lived Worktrees

Worktrees are intended to be ephemeral. A worktree should exist only for the
duration of a single task. Once the task is merged, the worktree has served
its purpose and should be removed.

### Persistent Vault Memory

While worktrees and branches are short-lived, project knowledge is durable.
Brevity uses the AI-Vault to store task specs, architecture notes, and
decisions. This ensures that even if a worktree is deleted, the context and
intent behind the work remain available for future tasks.

### Aggressive Cleanup

Cleanup is a core part of the Brevity task loop. The standard flow for every
task ends with `Brevity task cleanup`. This command removes the worktree,
deletes the branch, and clears the runtime metadata.

Maintaining a clean `Brevity board` is essential for reasoning about the current
state of the project. Tasks that are done or merged should not linger in the
runtime state.

### Recoverable Execution

AI workers or model providers may fail during execution (e.g., due to timeouts,
crashes, or capacity errors). These failures can leave a task in a partial state
with runtime metadata still present.

Brevity's lifecycle model is designed to be recoverable:
-   If a worker fails, the worktree and branch remain intact.
-   You can re-run the task with `Brevity task run <slug> --execute`.
-   If a task becomes unrecoverable or is abandoned, use
    `Brevity task cleanup <slug> --force` to reset the workspace state.

## Command Model


Brevity is designed around these commands:

- `Brevity init` creates the repo-local Brevity skeleton and AI-Vault project memory.
- `Brevity init --repair` repairs known config paths and recreates missing
  skeleton files without overwriting existing vault memory.
- `Brevity plan` writes a manual Codex planner prompt from repo-local Brevity config.
- `Brevity plan backlog` writes a manual Codex backlog planner prompt.
- `Brevity board` groups Brevity task metadata by status.
- `Brevity onboard` prepares an existing repo and AI-Vault project memory.
- `Brevity status` reports repos, worktrees, and vault presence.
- `Brevity logs recent [--count <n>]` shows recent vault runtime memory and
  recent worker log files.
- `Brevity logs task <slug> [--tail <n>]` shows the latest worker log path and
  a small tail for a task.
- `Brevity task new` creates an isolated worktree and task branch.
- `Brevity task activate` creates a task worktree from a vault task spec.
- `Brevity task spec` prints a vault-backed task spec by slug.
- `Brevity task start` prints the manual Codex start command for a task worktree.
- `Brevity task run` prints or executes the headless worker command for a task
  worktree, supporting optional `--profile <name>` overrides.
- `Brevity task status` reports task worktree state.
- `Brevity task merge` merges a completed task branch back to its base.
- `Brevity task cleanup` removes task worktrees after merge.

Brevity v0 provides the CLI scaffold, repository initialization, planner prompt
generation, workspace status, task creation, task status start instructions,
headless task run instructions, task reporting, task merge, and task cleanup.
Planner automation and metrics are deliberately out of scope.
