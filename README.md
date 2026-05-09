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
.\lane.ps1 status [-DevRoot <path>]
.\lane.ps1 task new <slug> [-DevRoot <path>]
.\lane.ps1 task start <slug>
.\lane.ps1 task status
.\lane.ps1 task merge <slug>
.\lane.ps1 task cleanup <slug> [--force]
```

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
lane init
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
- Markdown remains the durable memory layer.
- Git remains the source of truth for code.
