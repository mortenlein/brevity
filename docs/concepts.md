# Lane Concepts

Lane is a repo scaffold and command vocabulary for AI-assisted development on a
Windows machine. It keeps the useful parts of the original bootstrap script
while moving from many generated helper scripts to one CLI entry point.

## From Bootstrap Script To Lane

The bootstrap script created a full workspace and wrote helper scripts under
`.system`. Lane keeps the same operating model, but renames and consolidates it:

| Bootstrap concept | Lane concept |
| --- | --- |
| `.system` | `.lane` |
| `.system\scripts\onboard-ai-repo.ps1` | `lane onboard` |
| `.system\scripts\new-agent-task.ps1` | `lane task new` |
| `.system\scripts\workspace-status.ps1` | `lane status` |
| AI-Vault | AI-Vault remains supported |
| `worktrees\active`, `paused`, `completed` | first-class Lane task state |

## Dev Root

The dev root is the top-level workspace folder that contains repos, worktrees,
vaults, scratch files, and Lane orchestration state.

```text
<dev-root>\
  .lane\
  repos\
  worktrees\
  vaults\
  scratch\
```

Lane commands should accept an explicit `-DevRoot` when useful and otherwise
default to `C:\dev`.

## .lane

`.lane` is the orchestration area. It replaces `.system` from the bootstrap
script. `lane init` creates it in the current Git repository and adds:

- `tasks.json`
- `config.json`

`tasks.json` starts as an empty array. It is runtime state only: Lane uses it
to track worktrees, branches, prompts, statuses, and cleanup state for task
work that Lane has already created. Durable planned work belongs in vault task
specs, not in `.lane\tasks.json`.

`config.json` records:

- `projectName`
- `devRoot`
- `vaultPath`
- `worktreesRoot`

For new configs, `worktreesRoot` points at:

```text
<dev-root>\worktrees\active
```

Existing files are left unchanged by normal init.

`lane init --repair [-DevRoot <path>]` is the corrective init mode. It
re-detects `projectName` from the Git repository root folder and recomputes:

- `vaultPath` as `<dev-root>\vaults\AI-Vault\10-Projects\<project-name>`
- `worktreesRoot` as `<dev-root>\worktrees\active`

If `.lane\config.json` is missing, repair mode creates it. If it exists, repair
mode updates the known Lane fields only when they are wrong and preserves
unknown or custom fields. It also creates the same missing `.lane` files,
folders, and vault project memory paths as normal init. Existing vault memory
files are not overwritten.

Repair output reports repaired fields, unchanged fields, created paths, and
already-existing paths.

`lane plan` reads `config.json` and writes:

```text
<repo>\.lane\planner-prompt.md
```

The planner prompt is a manual handoff prompt for Codex. It tells Codex to read
`AGENTS.md`, read the configured `vaultPath` project memory, select exactly one
small high-value task, and return:

- task title
- task slug
- worker prompt

The planner prompt also tells Codex not to implement code, create a worktree,
call Codex automatically, propose autonomous planning, or use placeholders.
Lane prints the prompt path and tells the operator to open Codex in the repo and
paste the planner prompt.

`lane plan backlog` reads the same config and writes:

```text
<repo>\.lane\planner-backlog-prompt.md
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
where possible, avoid placeholders, and not implement code. Lane prints the
backlog prompt path and tells the operator to open Codex in the repo and paste
the backlog planner prompt. Lane does not parse planner output, create tasks
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

`lane init` creates the project memory folder and these child directories when
missing:

- `session-notes\`
- `tasks\`

It also creates these starter Markdown files when missing:

- `project.md`
- `architecture.md`
- `decisions.md`

If `AGENTS.md` is missing in the repository, `lane init` creates one that tells
Codex to read the project vault memory before doing work. If `AGENTS.md`
already exists, Lane leaves it unchanged.

Vault task specs are durable planned work. Each planned task can be stored as:

```text
<vaultPath>\tasks\<slug>.md
```

`lane task spec <slug>` reads `.lane\config.json`, uses `vaultPath`, and prints
the matching task spec Markdown file when it exists. If the spec is missing,
Lane reports the expected path. The command is read-only: it does not create
worktrees from specs, parse backlog planner output, or change
`.lane\tasks.json`.

## Repos

Repos live under the dev root by area:

```text
repos\
  active\
  experiments\
  archive\
  clients\
```

Lane should not own project source files except when a command explicitly
onboards a repo or creates a requested file.

## Worktrees

Worktrees are first-class Lane task workspaces:

```text
worktrees\
  active\
  paused\
  completed\
```

`lane task new <slug>` creates a branch named:

```text
task/<slug>
```

and a worktree named:

```text
worktrees\active\<repo-name>-<slug>
```

Lane should keep that convention for `lane task new`. The command also writes
a simple `prompt.md` file into the new worktree so a worker has a durable task
starting point without launching any automation.

Task metadata lives in the source repository:

```text
<repo>\.lane\tasks.json
```

Each record includes:

- `slug`
- `branch`
- `worktreePath`
- `promptPath`
- `status`
- `createdAt`

New task records use `ready-for-worker` status.

`lane board` reads the same metadata file and groups tasks by status. It shows
groups when matching tasks are present, including:

- `planned`
- `ready-for-worker`
- `running`
- `merged`
- `done`
- `blocked`

For each task, it reports:

- `slug`
- `branch`
- `worktreePath`

If no task metadata exists, Lane reports that no Lane tasks were found. The
board is read-only and does not start task work, run planner automation, merge
branches, clean up worktrees, or otherwise change task lifecycle state.

`lane task status` reads the same metadata file and reports:

- `slug`
- `branch`
- `status`
- `worktreePath`
- `promptPath`

If the metadata file does not exist, Lane reports that no Lane tasks were
found.

`lane task start <slug>` reads the same metadata file and finds the matching
task record. It prints:

- task slug
- worktree path
- prompt path
- exact Codex start command

The Codex command format is:

```text
codex -C <worktreePath> -a never -s workspace-write
```

The command also tells the operator to read `prompt.md` and follow it exactly.
It does not automatically launch Codex or run planner automation.

`lane task cleanup <slug> [--force]` reads the same metadata file and finds the
matching task record.

Without `--force`, Lane keeps the safe cleanup behavior: it removes the
recorded Git worktree, deletes the recorded Git branch with `git branch -d`,
and removes the task record only after cleanup succeeds.

With `--force`, Lane removes the worktree with
`git worktree remove --force <worktreePath>` and deletes the branch with
`git branch -D <branch>`. If the recorded worktree path is already missing or
is not registered with Git, Lane prints a warning and continues to branch
removal. Lane removes the task record when branch removal succeeds or the
branch is already missing. If branch removal fails for another reason, the
metadata stays in place so the task can be inspected or retried explicitly.

`lane task merge <slug>` reads the same metadata file, finds the matching task
record, and merges the recorded branch into the current Git branch with
`git merge <branch>`. When the merge succeeds, Lane updates the task status to
`merged`. It does not clean up the worktree, delete the branch, or remove task
metadata. If the merge fails, the metadata stays unchanged.

## Command Model

Lane is designed around these commands:

- `lane init` creates the repo-local Lane skeleton and AI-Vault project memory.
- `lane init --repair` repairs known config paths and recreates missing
  skeleton files without overwriting existing vault memory.
- `lane plan` writes a manual Codex planner prompt from repo-local Lane config.
- `lane plan backlog` writes a manual Codex backlog planner prompt.
- `lane board` groups Lane task metadata by status.
- `lane onboard` prepares an existing repo and AI-Vault project memory.
- `lane status` reports repos, worktrees, and vault presence.
- `lane task new` creates an isolated worktree and task branch.
- `lane task spec` prints a vault-backed task spec by slug.
- `lane task start` prints the manual Codex start command for a task worktree.
- `lane task status` reports task worktree state.
- `lane task merge` merges a completed task branch back to its base.
- `lane task cleanup` removes task worktrees after merge.

Lane v0 provides the CLI scaffold, repository initialization, planner prompt
generation, workspace status, task creation, task status start instructions,
task reporting, task merge, and task cleanup. Planner automation is deliberately
out of scope.
