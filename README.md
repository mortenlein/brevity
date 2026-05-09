# Lane

Lane is a Windows-first command line scaffold for AI-assisted repository work.
It extracts the useful shape of `bootstrap-ai-system-complete-v4.ps1` into a
small repo-owned tool:

- `.system` becomes `.lane`
- `onboard-ai-repo.ps1` becomes `lane onboard`
- `new-agent-task.ps1` becomes `lane task new`
- `workspace-status.ps1` becomes `lane status`
- AI-Vault remains supported
- Git worktrees remain first-class

Lane v0 is intentionally small. It documents the workflow and provides a thin
PowerShell CLI surface, but it does not implement planner automation.

## Files

- `lane.ps1` - Windows PowerShell CLI entry point.
- `AGENTS.md` - working instructions for agents modifying Lane.
- `docs/concepts.md` - concepts carried forward from the bootstrap script.

## Usage

From this repository:

```powershell
.\lane.ps1 help
```

Lane v0 supports:

```powershell
.\lane.ps1 help
.\lane.ps1 init [-DevRoot <path>]
.\lane.ps1 init --repair [-DevRoot <path>]
.\lane.ps1 plan
.\lane.ps1 plan backlog
.\lane.ps1 board
.\lane.ps1 status [-DevRoot <path>]
.\lane.ps1 task new <slug> [-DevRoot <path>]
.\lane.ps1 task activate <slug>
.\lane.ps1 task spec <slug>
.\lane.ps1 task start <slug>
.\lane.ps1 task run <slug> [--execute]
.\lane.ps1 task status
.\lane.ps1 task merge <slug>
.\lane.ps1 task cleanup <slug> [--force]
```

The init command prepares the current Git repository for Lane. It creates
repo-local Lane state when missing:

```text
<repo>\.lane\
<repo>\.lane\tasks.json
<repo>\.lane\config.json
```

`config.json` records the project name, dev root, AI-Vault project path,
worktrees root, and Codex run settings. The project name is the Git repository
root folder name.

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

Use repair mode when an existing `.lane\config.json` points at the wrong
project, vault, or worktree location:

```powershell
.\lane.ps1 init --repair [-DevRoot <path>]
```

Repair mode re-detects the project name from the Git repository root folder,
recomputes `vaultPath` as
`<dev-root>\vaults\AI-Vault\10-Projects\<project-name>`, and recomputes
`worktreesRoot` as `<dev-root>\worktrees\active`. It creates `config.json` if
missing, updates only the known Lane fields when they are wrong, and preserves
unknown or custom fields. It also creates the same missing `.lane` files,
folders, and AI-Vault project memory paths as normal init. Existing vault
memory files are not overwritten. Repair also adds missing Codex run settings
without removing custom config fields.

Repair mode prints repaired config fields, unchanged config fields, created
paths, and already-existing paths.

The plan command reads:

```text
<repo>\.lane\config.json
```

It writes a planner prompt to:

```text
<repo>\.lane\planner-prompt.md
```

The generated prompt tells Codex to read `AGENTS.md`, read the configured
AI-Vault project memory, select exactly one small high-value task, and return a
task title, task slug, and worker prompt. It also tells Codex not to implement
code, create a worktree, call Codex automatically, or use placeholders.

After writing the prompt, Lane prints the prompt path and:

```text
Open Codex in this repo and paste the planner prompt.
```

Lane does not automatically launch Codex or run autonomous planning.

The backlog plan mode reads the same config and writes a backlog planner prompt
to:

```text
<repo>\.lane\planner-backlog-prompt.md
```

The generated backlog prompt tells Codex to read `AGENTS.md`, read the
configured AI-Vault project memory, and plan a larger body of work as 5-10
small tasks. Each task must include a title, slug, `status: planned`,
`dependencies: []`, and a concrete `workerPrompt`. The prompt tells Codex to
keep tasks small and independently executable where possible, avoid
placeholders, and not implement code.

After writing the backlog prompt, Lane prints the prompt path and:

```text
Open Codex in this repo and paste the backlog planner prompt.
```

Lane does not parse planner output, create tasks from the backlog, implement a
TUI, or launch Codex. Planned backlog work belongs in Markdown files under:

```text
<vaultPath>\tasks\
```

The board command reads:

```text
<repo>\.lane\tasks.json
```

`.lane\tasks.json` is runtime state only. It tracks task worktrees, branches,
prompts, statuses, and cleanup state for task work that Lane has already
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

When no task metadata exists, it prints `No Lane tasks found.` The board is
read-only; it does not start work, run planner automation, merge branches, or
change task lifecycle state.

The status command lists the standard Lane workspace locations when they exist:

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

and records task metadata in the source repository at:

```text
<repo>\.lane\tasks.json
```

Each task record includes the slug, branch, worktree path, prompt path, status,
and creation timestamp. New tasks start with `ready-for-worker` status.

The task activate command reads:

```text
<repo>\.lane\config.json
```

It uses `vaultPath`, `worktreesRoot`, and `projectName` from that config. For
the requested slug, Lane reads the durable vault task spec from:

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

Lane copies the vault task spec contents into:

```text
<worktreePath>\prompt.md
```

The original vault task spec is not modified or deleted. Lane records runtime
metadata in:

```text
<repo>\.lane\tasks.json
```

Each activated task record includes the slug, branch, worktree path, prompt
path, spec path, status, and creation timestamp. Activated tasks start with
`ready-for-worker` status. This command does not launch Codex or run the task.

The task spec command reads:

```text
<repo>\.lane\config.json
```

It uses `vaultPath` from that config and looks for:

```text
<vaultPath>\tasks\<slug>.md
```

When the spec exists, Lane prints the task slug, spec path, and Markdown file
contents. When the spec is missing, Lane prints a clear not-found message and
the expected path. This command is read-only; it does not create worktrees,
parse backlog planner output, or change `.lane\tasks.json`.

The task start command reads the matching record from:

```text
<repo>\.lane\tasks.json
```

It prints the task slug, worktree path, prompt path, and exact Codex command to
run manually:

```text
codex -C <worktreePath> -a never -s workspace-write
```

It also prints `Read prompt.md and follow it exactly.` Lane does not
automatically launch Codex.

The task run command reads the matching record from:

```text
<repo>\.lane\tasks.json
```

It reads Codex settings from `.lane\config.json` and prints the task slug,
worktree path, prompt path, and headless Codex command:

```text
codex exec -C <worktreePath> -s <sandbox> prompt.md
```

If configured, Lane includes `-m <model>` and `-p <profile>`. By default, this
is a dry run and does not execute Codex, change task status, or record metrics.

Use `--execute` to run the generated command:

```powershell
.\lane.ps1 task run <slug> --execute
```

Lane does not implement metrics or other AI providers yet.

The task status command reads:

```text
<repo>\.lane\tasks.json
```

When task metadata exists, it prints the slug, branch, status, worktree path,
and prompt path for each task. When no task metadata exists, it prints
`No Lane tasks found.`

The task merge command reads the matching record from:

```text
<repo>\.lane\tasks.json
```

It merges the recorded branch into the current Git branch with
`git merge <branch>`. When the merge succeeds, Lane updates the task status to
`merged`. It does not remove the worktree, delete the branch, or remove task
metadata. If the merge fails, Lane leaves metadata unchanged.

The task cleanup command reads the matching record from:

```text
<repo>\.lane\tasks.json
```

It removes the recorded Git worktree, deletes the recorded Git branch with
`git branch -d`, and then removes the task record from metadata. This is the
default safe cleanup behavior. If Git cannot remove the worktree or delete the
branch, Lane leaves the metadata unchanged.

Use `--force` only when you explicitly want Git's forced cleanup behavior:

```powershell
.\lane.ps1 task cleanup <slug> --force
```

With `--force`, Lane removes the worktree with
`git worktree remove --force <worktreePath>` and deletes the branch with
`git branch -D <branch>`. If the recorded worktree path is missing or is not a
registered Git worktree, Lane prints a warning and continues to branch removal.
When branch removal succeeds, or the branch is already missing, Lane removes the
task metadata record. If branch removal fails for another reason, Lane keeps the
metadata unchanged.

## Planned Commands

These commands are part of Lane's public design, but are not implemented in v0:

```powershell
lane onboard
```

`lane status` is the successor to the bootstrap script's
`.system\scripts\workspace-status.ps1`.

## Workspace Layout

Lane keeps orchestration separate from project source:

```text
<dev-root>\
  .lane\
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
- Codex is the only configured worker provider in v0.
- Markdown remains the durable memory layer.
- Git remains the source of truth for code.
