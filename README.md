# Brevity

Brevity is a Windows-first command line scaffold for AI-assisted repository work.
It extracts the useful shape of `bootstrap-ai-system-complete-v4.ps1` into a
small repo-owned tool:

- `.system` concepts are consolidated into repo-local `.brevity`
- `onboard-ai-repo.ps1` becomes `Brevity onboard`
- `new-agent-task.ps1` becomes `Brevity task new`
- `workspace-status.ps1` becomes `Brevity status`
- AI-Vault remains supported
- Git worktrees remain first-class

Brevity v0 is intentionally small. It documents the workflow and provides a thin
PowerShell CLI surface, but it does not implement planner automation.

## Files

- `brevity.ps1` - Windows PowerShell CLI entry point.
- `AGENTS.md` - working instructions for agents modifying Brevity.
- `docs/concepts.md` - concepts carried forward from the bootstrap script.

## Usage

From this repository:

```powershell
.\brevity.ps1 help
```

## Planning Workflow

Brevity is designed for an AI-assisted planning workflow. This allows you to use a planner (like an AI agent) to break down a large goal into smaller, runnable tasks that can be stored durably in the project vault.

The workflow is:

1.  **Generate a Plan:** Use `plan backlog` to generate a prompt for your AI planner.

    ```powershell
    .\brevity.ps1 plan backlog
    ```

    Paste the generated prompt into your AI agent.

2.  **Save the Planner Output:** The planner will return a list of tasks in a specific Markdown format. Save this output to a file, for example, `C:\temp\my-plan.md`.

    *Example Planner Output (`my-plan.md`):*
    ```markdown
    - title: Add execution policy support
      slug: execution-policy
      status: planned
      dependencies: []
      workerPrompt: |
        Read AGENTS.md.
        Implement execution policy configuration support.
        Ensure the new config field is documented in README.md.
        Stop after patch + summary.

    - title: Improve board command output
      slug: improve-board-output
      status: planned
      dependencies: [execution-policy]
      workerPrompt: |
        Read AGENTS.md.
        Refactor the `Show-Board` function in `brevity.ps1`.
        The output should be a table instead of a list.
        Columns: Slug, Status, Branch, Worktree.
        Stop after patch + summary.
    ```

3.  **Apply the Plan:** Use `plan apply` to create durable task specs in your AI-Vault from the planner's output file.

    ```powershell
    .\brevity.ps1 plan apply C:\temp\my-plan.md
    ```

    Brevity will parse the file and create:
    - `<vaultPath>\tasks\execution-policy.md`
    - `<vaultPath>\tasks\improve-board-output.md`

4.  **Activate a Task:** Now that the task specs exist in the vault, you can activate one to create a worktree and prepare it for the worker.

    ```powershell
    .\brevity.ps1 task activate execution-policy
    ```

This process bridges the planning phase with the worker loop. Once a task is activated, you can use the fast loop below to execute it.

## Economic Model Selection

Brevity encourages selecting the cheapest sufficient model for each task to manage
costs and quotas effectively. Planners should assign a **complexity tier** to
each task, which maps to a stable **worker profile**.

-   **Low Complexity:** Documentation, unit tests, simple fixes. Use
    `codex-fast`, `gemini-lite`, or `gemini-flash`.
-   **Medium Complexity:** Feature work, refactoring, integration tests. Use
    `codex-balanced`, `gemini-flash`, or `gemini-pro`.
-   **High Complexity:** Architecture, deep debugging, complex logic. Use
    `codex-deep` or `gemini-pro`.

Stable profiles decouple task planning from volatile provider model names.
Provider model IDs are internal implementation details. Brevity keeps these
profiles in one script-local capability matrix that records provider,
cost tier, capability tier, complexity fit, intended use, and optional
provider-native settings such as `model`. Brevity also keeps a script-local,
planner-only complexity default map that gives future planners a clear preferred
profile order without changing worker execution, automatic routing, fallback, or
explicit `--profile` behavior. For more details on available profiles and
fallback strategies, see
[Worker Profiles](docs/concepts.md#worker-profiles) in the concepts
documentation.

Canonical profiles are the source of truth: `gemini-lite`, `gemini-flash`,
`gemini-pro`, `codex-fast`, `codex-balanced`, `codex-deep`, and `copilot`.
Brevity also accepts operator-friendly aliases for common choices. Aliases are
convenience names that resolve to canonical profiles before worker settings are
selected; they do not create separate profiles.

Examples:

```text
gemini-fast -> gemini-flash
codex-default -> codex-balanced
```

Use either canonical names or aliases with `task run`:

```powershell
.\brevity.ps1 task run my-task --profile gemini-fast --execute
.\brevity.ps1 task run my-task --profile codex-default --execute
```

## Worker Fast Loop

The recommended fast iteration loop for a Gemini worker on an **activated** task is:

1.  `.\brevity.ps1 task spec <slug>` - review the task spec.
2.  `.\brevity.ps1 task run <slug> --execute` - run the worker.
3.  `.\brevity.ps1 task merge <slug>` - merge the completed work.
4.  `.\brevity.ps1 task cleanup <slug>` - remove the worktree and branch.

Brevity v0 supports:

```powershell
.\brevity.ps1 help
.\brevity.ps1 init [-DevRoot <path>]
.\brevity.ps1 init --repair [-DevRoot <path>]
.\brevity.ps1 plan
.\brevity.ps1 plan backlog
.\brevity.ps1 plan workers
.\brevity.ps1 plan apply <file>
.\brevity.ps1 board
.\brevity.ps1 doctor [--repair]
.\brevity.ps1 doctor execution-policy
.\brevity.ps1 memory note <message>
.\brevity.ps1 logs recent [--count <n>]
.\brevity.ps1 logs task <slug> [--tail <n>]
.\brevity.ps1 session summary [--json]
.\brevity.ps1 runtime state [--json]
.\brevity.ps1 runtime start
.\brevity.ps1 runtime status
.\brevity.ps1 runtime stop
.\brevity.ps1 tui
.\brevity.ps1 status [-DevRoot <path>]
.\brevity.ps1 provider status
.\brevity.ps1 provider docs
.\brevity.ps1 provider profiles [--profile <name>] [--json]
.\brevity.ps1 provider reset <provider>
.\brevity.ps1 provider set <provider> <status> [-Note <note>]
.\brevity.ps1 task new <slug> [-DevRoot <path>]
.\brevity.ps1 task activate <slug>
.\brevity.ps1 task spec <slug>
.\brevity.ps1 task start <slug>
.\brevity.ps1 task runtime-info <slug>
.\brevity.ps1 task runs <slug> [--json]
.\brevity.ps1 task runs reconcile --dry-run
.\brevity.ps1 task runs retention --dry-run
.\brevity.ps1 task runs compact --dry-run [--json]
.\brevity.ps1 task run <slug> [--execute] [--profile <name>] [--smoke] [--force-provider]
.\brevity.ps1 task context refresh <slug>
.\brevity.ps1 task context status <slug>
.\brevity.ps1 task status
.\brevity.ps1 task merge <slug>
.\brevity.ps1 task cleanup <slug> [--force]
.\brevity.ps1 task cleanup-orphan-branches --dry-run
```

## TUI

`.\brevity.ps1 tui` starts the original PowerShell runtime/operator dashboard
scaffold. It is read-only, polls `.\brevity.ps1 runtime state --json` every few
seconds, and renders a compact view of repo/runtime summary, provider health,
task counts by normalized state, recent tasks, cleanup warnings, and stale or
incomplete run indicators. Press `q` or `Esc` to quit.

The TUI is intentionally a thin runtime-state consumer. It does not start
workers, mutate task metadata, merge branches, edit files, stream events, or
embed Git operations. Future mutation support should go through explicit
command-result contracts rather than duplicating orchestration logic in a view
layer. The PowerShell TUI remains useful as a lightweight reference scaffold,
but it is not the primary direction for the future operator UX.

## Go Runtime Client

The experimental Go layer requires Go to be installed and available on `PATH`.
Run it from the repository root. It is currently a frontend/runtime client over
the PowerShell backend, not a replacement runtime. The Go dashboard, especially
watch mode, is the active direction for the future operator UX:

```powershell
go run ./cmd/brevity
go run ./cmd/brevity --once
go run ./cmd/brevity --watch
go run ./cmd/brevity --watch --refresh 5s
go run ./cmd/brevity --watch --no-clear
```

The dashboard reads the PowerShell-produced runtime-state contract:

```powershell
.\brevity.ps1 runtime state --json
```

`--watch` keeps the read-only dashboard open and periodically polls
`.\brevity.ps1 runtime state --json`; `--refresh` accepts a Go duration such as
`5s` and controls the polling interval. Watch mode suppresses redraws when the
stable dashboard content is unchanged, so runtime `GeneratedAt` and poll
timestamps do not force a redraw by themselves. `--no-clear` disables
clear-screen behavior on changed renders. The current watch UI uses
line-oriented input for Windows-friendly, dependency-free consoles: type `j` or
`k` and press Enter to move the selection, type `d` and press Enter or press
Enter alone to toggle details, type `r` then Enter to refresh, type `?` then
Enter to toggle help, and type `q` then Enter to quit. Ctrl+C also stops watch
mode. The Go client exits cleanly without mutating `.brevity` state.

Watch detail panes are read-only inspection views. They currently cover
provider details, task details, cleanup candidate details, and suggested action
details. Suggested actions are guidance from runtime state, not executable
dashboard commands.

Go is the runtime authority for migrated orchestration behavior. PowerShell is
legacy compatibility/reference behavior and should delegate instead of adding
new runtime-owned logic. Current native Go actions own task metadata, provider
health, cleanup, run-history inspection, and the first runtime supervisor
foundation.

Raw terminal input and a framework such as Bubble Tea are deferred. The current
dashboard mode stays dependency-free and Windows-friendly while the operator
frontend stabilizes.

The supported Go command surface, including implemented and deferred commands,
is tracked in [`docs/go-support-matrix.md`](docs/go-support-matrix.md). There is
no interactive mutation UI in either the PowerShell TUI or the Go dashboard yet,
and the Go surface should stay conservative as behavior moves out of legacy
PowerShell compatibility.

The future operator UX roadmap for the Go dashboard is documented in
[`docs/go-dashboard-ux-plan.md`](docs/go-dashboard-ux-plan.md).

## Runtime State Contract

`.\brevity.ps1 runtime state --json` prints the machine-readable runtime state
snapshot for the current repository. It is intended for future TUI and
automation consumers that need read-only orchestration state without scraping the
human dashboard.

The JSON includes `schema`, currently `brevity.runtime-state.v1`. Consumers
should check this value before depending on the shape. The v1 contract should
evolve additively where practical: new fields may be added, but existing fields
should not be removed or renamed casually. If Brevity needs a breaking contract,
it should publish a new schema such as `brevity.runtime-state.v2`.
The discoverable schema for TUI and automation consumers is
[`docs/runtime-state.schema.json`](docs/runtime-state.schema.json), which
reflects `brevity.runtime-state.v1` and should evolve additively when possible.

Future machine-readable mutation results are documented in
[`docs/command-result-contract.md`](docs/command-result-contract.md), with the
schema at [`docs/command-result.schema.json`](docs/command-result.schema.json).
That contract is intended for TUI and automation consumers, should evolve
additively where practical, and consumers should tolerate unknown fields.

The centralized contract index is [`docs/contracts.md`](docs/contracts.md). It
indexes the runtime/TUI contract surface and schemas for automation and TUI
consumers, including the runtime-state and command-result contracts.

## Runtime Supervisor

`go run ./cmd/brevity runtime start`, `runtime status`, and `runtime stop` are
the first Go-native persistent supervisor foundation. The supervisor writes
`.brevity\runtime.json`, owns `.brevity\runtime.lock`, updates a heartbeat, and
responds to graceful stop requests.

This layer is intentionally small: it does not execute providers, drain queues,
spawn workers, mutate task execution state, or expose network APIs. PowerShell
wrappers may call these Go commands for compatibility.

Major sections include:

- `providers` - provider health summary and per-provider health records.
- `taskCounts` - aggregate counts for tracked, runnable, blocked, stale,
  provider-gated, and review tasks.
- `tasks` - sorted task summaries from `.brevity\tasks.json`.
  Each task summary includes compact worker lifecycle fields such as
  `workerStatus`, `lastRunStartedAt`, `lastRunFinishedAt`, `lastExitCode`,
  `lastFailureType`, `lastLogPath`, `lastProvider`, and `lastProfile`.
  Latest run summary is read from `.brevity\runs.jsonl` when available, with
  worker log scanning retained as a fallback.
- `groups` - task slug lists grouped by runtime classification.
- `orphanedTaskWorktrees` - task-like active worktrees not tracked in runtime
  task metadata.
- `lock` - task metadata lock presence, path, and age in minutes.
- `runtimeMemory` - runtime log path, existence, recent entry count, and recent
  entries.
- `suggestedNextActions` - operator guidance derived from the current snapshot.

## Run Index Retention Policy

`.brevity\runs.jsonl` is the append-only worker run index. Brevity uses it for
recent run history, runtime-state summaries, stale/incomplete run detection, and
future TUI inspection. Worker logs remain the durable detailed output for each
run, and Brevity v1 must not delete those logs automatically.

The default retention policy for future run-index compaction is conservative:

- Preserve at least the latest 20 indexed runs for each task.
- Preserve all incomplete or stale run records until reconciliation has reviewed
  them.
- Preserve failed runs longer than successful runs, because failures are usually
  more useful for diagnosis.
- Archive or summarize older completed run records before removal from the hot
  index; do not silently discard history.
- Treat retention warnings in a future TUI as advisory operator signals, not as
  automatic cleanup instructions.

PowerShell run maintenance remains legacy/reference behavior:
`.\brevity.ps1 task runs retention --dry-run` and
`.\brevity.ps1 task runs compact --dry-run` are report-only operator views.

Native Go now owns run-history maintenance. `go run ./cmd/brevity runs inspect`
is read-only and reports malformed rows, duplicate run IDs, stale incomplete
runs, missing log references, and whether compaction would rewrite the hot run
index. `go run ./cmd/brevity runs compact --plan --json` emits the same plan in
a command-result envelope without mutation.

`go run ./cmd/brevity runs compact --force [--json]` is the explicit native
rewrite path. It uses the native state lock, preserves the latest valid record
for duplicate run IDs deterministically, quarantines malformed rows to
`.brevity\runs-malformed.jsonl`, rewrites `.brevity\runs.jsonl` atomically, and
does not delete worker logs by default. Log deletion is intentionally unsupported
in v0.

The init command prepares the current Git repository for Brevity. It creates
repo-local Brevity state when missing:

```text
<repo>\.brevity\
<repo>\.brevity\tasks.json
<repo>\.brevity\provider-health.json
<repo>\.brevity\config.json
```

`config.json` records the project name, dev root, AI-Vault project path,
worktrees root, and Codex run settings. The project name is the Git repository
root folder name.

`provider-health.json` is lightweight runtime metadata for AI provider health.
It starts Codex and Gemini as `unknown` and supports `healthy`,
`capacity-degraded`, `quota-constrained`, `unavailable`, and `unknown`.
Use `.\brevity.ps1 provider status` to inspect current provider state. Use
`.\brevity.ps1 provider set <provider> <status> [-Note <note>]` to update one
provider, refresh `updatedAt`, and optionally store an operator note.
Provider health is not task status: it must not change task lifecycle,
worktree, merge, or cleanup state. Brevity v0 does not automatically route or
fall back based on provider health.

It also creates project memory under AI-Vault:

```text
<dev-root>\vaults\AI-Vault\10-Projects\<project-name>\
  project.md
  architecture.md
  decisions.md
  session-notes\
  tasks\
```

If `AGENTS.md` is missing, init creates one that instructs Codex to read the
project vault memory before doing work. Existing files are never overwritten;
init prints what it created and what already existed.

Use repair mode when an existing `.brevity\config.json` points at the wrong
project, vault, or worktree location:

```powershell
.\brevity.ps1 init --repair [-DevRoot <path>]
```

Repair mode re-detects the project name from the Git repository root folder,
recomputes `vaultPath` as
`<dev-root>\vaults\AI-Vault\10-Projects\<project-name>`, and recomputes
`worktreesRoot` as `<dev-root>\worktrees\active`. It creates `config.json` if
missing, updates only the known Brevity fields when they are wrong, and preserves
unknown or custom fields. It also creates the same missing `.brevity` files,
folders, and AI-Vault project memory paths as normal init. Existing vault
memory files are not overwritten. Repair also adds missing Codex run settings
without removing custom config fields.

Repair mode prints repaired config fields, unchanged config fields, created
paths, and already-existing paths.

The plan command reads:

```text
<repo>\.brevity\config.json
```

It writes a planner prompt to:

```text
<repo>\.brevity\planner-prompt.md
```

The generated prompt tells Codex to read `AGENTS.md`, read the configured
AI-Vault project memory, select exactly one small high-value task, and return a
task title, task slug, and worker prompt. It also tells Codex not to implement
code, create a worktree, call Codex automatically, or use placeholders.

After writing the prompt, Brevity prints the prompt path and:

```text
Open Codex in this repo and paste the planner prompt.
```

Brevity does not automatically launch Codex or run autonomous planning.

The backlog plan mode reads the same config and writes a backlog planner prompt
to:

```text
<repo>\.brevity\planner-backlog-prompt.md
```

The generated backlog prompt tells Codex to read `AGENTS.md`, read the
configured AI-Vault project memory, and plan a larger body of work as 5-10
small tasks. Each task must include a title, slug, `status: planned`,
`dependencies: []`, and a concrete `workerPrompt`. The prompt tells Codex to
keep tasks small and independently executable where possible, avoid
placeholders, and not implement code.

After writing the backlog prompt, Brevity prints the prompt path and:

```text
Open Codex in this repo and paste the backlog planner prompt.
```

The backlog prompt command does not create tasks from the backlog, implement a
TUI, or launch Codex. Planned backlog work belongs in Markdown files under:

```text
<vaultPath>\tasks\
```

The plan apply command reads a structured planner output Markdown file and
creates durable task specs under:

```text
<vaultPath>\tasks\
```

Planner output tasks must use these fields:

```text
- title: Example task
- slug: example-task
- status: planned
- dependencies: []
- workerPrompt: Read AGENTS.md.
  Do one small bounded task.
  Stop after patch + summary.
```

Brevity validates required fields, requires `status: planned`, writes readable
Markdown task specs, and refuses to overwrite existing task specs. It does not
activate worktrees, launch Codex, merge branches, or change runtime task
metadata.

The board command reads:

```text
<repo>\.brevity\tasks.json
```

`.brevity\tasks.json` is runtime state only. It tracks task worktrees, branches,
prompts, statuses, and cleanup state for task work that Brevity has already
created. It is not the durable planning backlog.

Vault task specs are durable planned work. They live as Markdown files under:

```text
<vaultPath>\tasks\<slug>.md
```

The board command groups runtime task metadata by status and prints the task
slug, branch, and worktree path for each task. It shows status groups when
matching tasks are present, including:

- `planned`
- `ready-for-worker`
- `running`
- `merged`
- `done`
- `blocked`

Runtime task state is also exposed as `normalizedState` for TUI and automation
consumers. This does not replace existing task metadata `status`; it is a
derived compatibility layer over metadata, runtime health, and worker history.
Canonical normalized states are:

- `planned` - durable task spec exists but no runtime worktree has been started.
- `ready-for-worker` - task metadata is present and the task can be offered to a worker.
- `running` - the latest worker run appears active or incomplete.
- `succeeded` - work is complete under a legacy completed/done status.
- `failed` - the latest worker run failed.
- `reviewing` - a worker succeeded and the task has not been merged yet.
- `merged` - the task branch has been merged and awaits cleanup.
- `stale` - required worktree, branch, prompt, or registration facts are missing.
- `blocked` - provider or metadata state prevents safe worker execution.
- `orphaned` - runtime facts exist without matching task metadata.

When no task metadata exists, it prints `No Brevity tasks found.` The board is
read-only; it does not start work, run planner automation, merge branches, or
change task lifecycle state.

The status command lists the standard Brevity workspace locations when they exist:

- `repos\active`
- `worktrees\active`
- `worktrees\paused`
- `worktrees\completed`
- `vaults\AI-Vault`

The task new command creates a Git worktree at:

```text
<dev-root>\worktrees\active\<repo-name>-<slug>
```

and creates the matching branch:

```text
task/<slug>
```

It also writes a placeholder worker prompt to:

```text
<dev-root>\worktrees\active\<repo-name>-<slug>\prompt.md
```

and copies selected project memory into:

```text
<dev-root>\worktrees\active\<repo-name>-<slug>\.brevity\context\
```

The local context folder may include `project.md`, `architecture.md`,
`decisions.md`, `current-state.md`, and `roadmap.md`. Missing files are skipped.
Workers should read these materialized files instead of external vault paths.
The vault remains durable memory; the worktree remains the bounded execution
context.

Example operator check:

```powershell
.\brevity.ps1 task new my-task
.\brevity.ps1 task runtime-info my-task
.\brevity.ps1 task context status my-task
.\brevity.ps1 task context refresh my-task
```

`task runtime-info` shows the task's worktree, prompt, provider, context state,
existing `status`, derived `normalizedState`, and last known worker lifecycle
state. `task context status` inspects the managed files under
`.brevity\context`, and `task context refresh` restores those managed files from
vault memory. Missing vault files are skipped safely, so the worker always sees
only the local bounded context that exists for that task.

The command records task metadata in the source repository at:

```text
<repo>\.brevity\tasks.json
```

Each task record includes the slug, branch, worktree path, prompt path, status,
and creation timestamp. New tasks start with `ready-for-worker` status.

The task activate command reads:

```text
<repo>\.brevity\config.json
```

It uses `vaultPath`, `worktreesRoot`, and `projectName` from that config. For
the requested slug, Brevity reads the durable vault task spec from:

```text
<vaultPath>\tasks\<slug>.md
```

Then it creates a Git worktree at:

```text
<worktreesRoot>\<projectName>-<slug>
```

and creates the matching branch:

```text
task/<slug>
```

Brevity embeds the vault task spec contents in a bounded worker prompt at:

```text
<worktreePath>\prompt.md
```

It also materializes selected project memory into:

```text
<worktreePath>\.brevity\context\
```

The original vault task spec is not modified or deleted. Brevity records runtime
metadata in:

```text
<repo>\.brevity\tasks.json
```

Each activated task record includes the slug, branch, worktree path, prompt
path, spec path, status, and creation timestamp. Activated tasks start with
`ready-for-worker` status. This command does not launch Codex or run the task.

The task spec command reads:

```text
<repo>\.brevity\config.json
```

It uses `vaultPath` from that config and looks for:

```text
<vaultPath>\tasks\<slug>.md
```

When the spec exists, Brevity prints the task slug, spec path, and Markdown file
contents. When the spec is missing, Brevity prints a clear not-found message and
the expected path. This command is read-only; it does not create worktrees,
parse backlog planner output, or change `.brevity\tasks.json`.

The native Go task start command reads the matching record from:

```text
<repo>\.brevity\tasks.json
```

It runs native mutation preflight, takes the advisory `.brevity\state.lock`,
updates the task record to `ready-for-worker`, sets `updatedAt`, sets
`startedAt` when absent, preserves unrelated and unknown task fields, and emits
`brevity.command-result.v1` for JSON consumers. It does not launch Codex, run a
provider/worker, create or delete branches, create or delete worktrees, or
materialize prompt/context files. PowerShell task start remains available as
legacy/reference behavior.

The task run command reads the matching record from:

```text
<repo>\.brevity\tasks.json
```

It reads Codex settings from `.brevity\config.json` and prints the task slug,
worktree path, prompt path, and headless Codex command:

```text
codex exec -C <worktreePath> -s <sandbox> prompt.md
```

The configured provider may be `codex` or `gemini`. For `codex`, Brevity includes
`-m <model>` when a model is configured by provider config or by the selected
worker profile. It includes `-p <profile>` only when a native Codex provider
profile is explicitly configured; Brevity worker profile names such as
`codex-balanced` are not passed to Codex `-p`. For `gemini`, Brevity builds a
non-interactive command that runs from the task worktree and passes the
`prompt.md` contents to `-p`. It includes `-m <model>` when configured, and
includes `-s` when sandbox is not blank or `none`. Set
`providers.gemini.skipTrust` to `true` to pass `--approval-mode yolo` to Gemini.
Set `providers.gemini.env` to an object of environment variables, such as
`GOOGLE_API_KEY`, when Gemini authentication should be scoped to the worker
process. Dry runs print configured variable names but mask values.
Before printing or executing the worker command, Brevity refreshes `prompt.md`
from the vault task spec when available and refreshes `.brevity\context` from
selected project memory files.
By default, this is a dry run and does not execute the worker, change task
status, or record metrics.
When `--execute` is used, Brevity applies `codex.executionPolicy` from
`.brevity\config.json` to the worker process only. The default is `Bypass`, which
helps PowerShell run script shims such as globally installed npm commands
without changing the user's machine policy.
Set `codex.executionPolicy` to another PowerShell execution policy name, such
as `RemoteSigned`, if a repository needs a stricter worker process policy.

Use `--execute` to run the generated command:

```powershell
.\brevity.ps1 task run <slug> --execute
```

Brevity does not implement metrics yet. Unsupported worker providers return a
clear unsupported-provider error.

The task status command reads:

```text
<repo>\.brevity\tasks.json
```

When task metadata exists, it prints the slug, branch, status, normalized state,
worktree path, and prompt path for each task. When no task metadata exists, it prints
`No Brevity tasks found.`

The native task merge command reads the matching record from:

```text
<repo>\.brevity\tasks.json
```

`go run .\cmd\brevity task merge <slug> --plan --json` builds a read-only merge
plan with the source branch, current target branch, worktree dirty state,
expected Git argv commands, blockers, warnings, and expected metadata mutation.

`go run .\cmd\brevity task merge <slug> [--json]` refuses blocked plans, checks
out the target branch, and runs `git merge <branch>` without shell string
concatenation. When the merge succeeds, Brevity updates the task status to
`merged` through the native state store and advisory lock. It does not remove
the worktree, delete the branch, or remove task metadata. If the merge fails,
Brevity leaves metadata unchanged. Cleanup remains explicit and separate.

The native Go task cleanup command reads the matching record from:

```text
<repo>\.brevity\tasks.json
```

Plan first when you want the exact cleanup decision without mutation:

```powershell
go run .\cmd\brevity task cleanup <slug> --plan --json
```

The plan reports the worktree path, branch, dirty flag, branch merged flag when
available, whether cleanup is removable/destructive, expected Git argv commands,
expected metadata mutation, blockers, warnings, and the generated timestamp.

Execution is always gated by explicit `--force`:

```powershell
go run .\cmd\brevity task cleanup <slug> --force --json
```

Native cleanup refuses dirty worktrees and unmerged branches. It removes the
recorded Git worktree with `git worktree remove`, deletes the recorded branch
with safe `git branch -d`, and then removes the selected task record through the
native state store and advisory lock. It does not force-delete branches, does
not clean up orphan records, and never runs implicitly after merge. PowerShell
cleanup remains present as legacy/reference behavior.

Orphan cleanup is separate from normal task cleanup. It only considers registered
Git worktrees under the active worktree root, on `task/*` branches, with no
matching `.brevity\tasks.json` metadata. `--dry-run` reports the candidates
without changing anything:

```powershell
.\brevity.ps1 task cleanup-orphans --dry-run
```

To remove those orphaned task worktrees and then delete their `task/*` branches,
run the explicit execute form:

```powershell
.\brevity.ps1 task cleanup-orphans --execute
```

Brevity re-checks each candidate immediately before removal and skips anything
that is no longer registered, no longer under the active worktree root, no longer
on a `task/*` branch, now has task metadata, or has dirty Git status. Dirty
orphaned worktrees are not force deleted; Brevity prints inspection commands and
safe next-step guidance instead.

Orphan branch cleanup is dry-run only. It reports local `task/*` branches that
have no `.brevity\tasks.json` metadata and are not checked out in any registered
Git worktree:

```powershell
.\brevity.ps1 task cleanup-orphan-branches --dry-run
```

The report shows whether each branch appears merged into the current `HEAD` and
prints the suggested manual `git branch -D <branch>` command. Without
`--dry-run`, the command refuses safely.

## Workspace Lifecycle Hygiene

Brevity promotes a high-velocity, high-hygiene lifecycle for AI-assisted work.
Because AI workers can iterate rapidly, a workspace can quickly accumulate
stale branches and worktrees if cleanup is treated as optional maintenance.

### Short-lived Worktrees

Worktrees are intended to be ephemeral. A worktree should exist only for the
duration of a single task. Once the task is merged, it should be removed to keep
the workspace manageable.

### Persistent Vault Memory

While worktrees and branches are short-lived, project knowledge is durable.
Brevity uses the AI-Vault to store task specs, architecture notes, and
decisions. Context and intent remain available in the vault even after a
worktree is deleted.

### Aggressive Cleanup

Cleanup is a core part of the Brevity task loop, not optional maintenance.
The standard flow for every task ends with `Brevity task cleanup`. This:
1.  Removes the Git worktree.
2.  Deletes the local Git branch.
3.  Clears the runtime metadata from `.brevity\tasks.json`.

Maintaining a clean `Brevity board` is essential for reasoning about the current
state of the project.

### Recoverable Execution

AI workers or model providers may fail during execution (e.g., due to timeouts,
crashes, or capacity errors). These failures can leave a task in a partial state
with runtime metadata still present.

Brevity's lifecycle model is designed to be recoverable:
-   **If a worker fails:** The worktree and branch remain intact. You can
    re-run the task with `Brevity task run <slug> --execute`.
-   **If a task is abandoned:** Use `Brevity task cleanup <slug> --force` to
    reset the workspace state.

## Gemini Worker Setup

To use Gemini as a worker, you need to configure trust and authentication.

For more information on provider capabilities and how to write prompts that work across different providers, see the "Provider Capabilities" section in `docs/concepts.md`.

### Trust Root

Gemini CLI uses a parent-folder trust model. It looks for a `.gemini` folder in
the parent directory of the script it's running. For Brevity, this means your
dev root must contain a `.gemini` folder.

```text
<dev-root>\
  .gemini\
  repos\
  worktrees\
```

Create this folder manually if it doesn't exist.

### API Key

Gemini requires a `GEMINI_API_KEY`. For security, this key should not be stored
in repository configuration. Brevity loads it from an environment variable.

You can set this in your PowerShell profile:

```powershell
[System.Environment]::SetEnvironmentVariable('GEMINI_API_KEY', 'your-api-key', 'User')
```

Or you can configure Brevity to pass it to the worker process. In your
repository's `.brevity\config.json`, set the `env` property under `gemini`:

```json
{
  "defaultProvider": "gemini",
  "providers": {
    "gemini": {
      "command": "gemini",
      "env": {
        "GEMINI_API_KEY": "$env:GEMINI_API_KEY"
      }
    }
  }
}
```

Brevity will expand `$env:GEMINI_API_KEY` to its value when running the worker.
This keeps the secret out of the repository.

### Troubleshooting

- **Capacity and Quota:** If a worker fails with a `429` error, such as `QUOTA_EXHAUSTED` or `MODEL_CAPACITY_EXHAUSTED`, this should be treated as a **worker infrastructure failure**, not a task failure.

  Capacity errors are transient. When they occur:
  1.  **Retry later:** The provider may have temporary capacity limits.
  2.  **Switch profiles:** Use the `--profile` flag to manually route the task
      to a different worker profile or provider that may have available capacity.

  *Examples:*
  ```powershell
  # Switch to a different Gemini profile
  .\brevity.ps1 task run <slug> --execute --profile gemini-flash
  .\brevity.ps1 task run <slug> --execute --profile gemini-lite

  # Switch to a different provider profile
  .\brevity.ps1 task run <slug> --execute --profile codex-balanced
  ```

  See `docs/concepts.md` for more on available profiles and fallback strategies.

- **`ripgrep` not found:** Gemini may warn that `rg.exe` (ripgrep) is not in your path. This is a non-blocking warning. Gemini will fall back to its internal search tool if ripgrep is not available, which may be slower.

  For better performance, we recommend installing ripgrep.

  - **Windows:**
    ```powershell
    winget install BurntSushi.ripgrep.MSVC
    ```
    Or install with Chocolatey:
    ```powershell
    choco install ripgrep
    ```

  - **macOS:**
    ```bash
    brew install ripgrep
    ```

  - **Linux (Debian/Ubuntu):**
    ```bash
    sudo apt-get install ripgrep
    ```

  After installation, ensure `rg` is available in your system's PATH.

## Planned Commands

These commands are part of Brevity's public design, but are not implemented in v0:

```powershell
Brevity onboard
```

`Brevity status` is the current workspace inspection command for repos,
worktrees, and vault presence.

## Workspace Layout

Brevity keeps orchestration separate from project source:

```text
<dev-root>\
  .brevity\
  repos\
    active\
    experiments\
    archive\
    clients\
  worktrees\
    active\
    paused\
    completed\
  vaults\
    AI-Vault\
      00-Inbox\
      01-Global\
      02-Ideas\
      10-Projects\
      90-Archive\
  scratch\
```

## Design Constraints

- Windows-first, PowerShell-first.
- No runtime dependencies beyond PowerShell and Git for future worktree flows.
- No web app.
- No planner automation in v0.
- Planner prompt generation is manual and does not create worktrees.
- Codex, Gemini, and Copilot worker profiles are configured in v0.
- Markdown remains the durable memory layer.
- Git remains the source of truth for code.
