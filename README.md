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
```

The status command lists the standard Lane workspace locations when they exist:

- `repos\active`
- `worktrees\active`
- `worktrees\paused`
- `worktrees\completed`
- `vaults\AI-Vault`

The task new command creates a Git worktree at:

```text
<dev-root>\worktrees\active\<slug>
```

and creates the matching branch:

```text
task/<slug>
```

## Planned Commands

These commands are part of Lane's public design, but are not implemented in v0:

```powershell
lane init
lane onboard
lane task status
lane task merge
lane task cleanup
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
