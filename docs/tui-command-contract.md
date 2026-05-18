# TUI Command Contract

This contract describes how a future Brevity TUI should cross the boundary
between read-only inspection and workspace mutation.

## State Source

The TUI should read runtime state with:

```powershell
.\brevity.ps1 runtime state --json
```

The JSON output is the read model. It is safe to render, filter, group, and
refresh, but it is not a mutation API.

The TUI must not edit Brevity runtime files or worktrees directly. In
particular, it must not write:

- `.brevity\tasks.json`
- `.brevity\provider-health.json`
- `runtime-log.md`
- task worktrees under `worktrees\active`, `worktrees\paused`, or
  `worktrees\completed`

All mutations should go through explicit Brevity CLI commands so validation,
locking, Git behavior, logging, and future policy checks stay centralized.

## Command Categories

Read-only commands inspect state and must not change files, worktrees, branches,
provider health, or runtime memory. Examples:

- `.\brevity.ps1 runtime state --json`
- `.\brevity.ps1 doctor`
- `.\brevity.ps1 logs recent`
- `.\brevity.ps1 task runtime-info <slug>`

Safe dry-run commands compute intended changes without applying them. They may
read runtime files and Git state, but they should leave the workspace unchanged.
Example:

- `.\brevity.ps1 task cleanup-orphans --dry-run`

Mutating commands intentionally change Brevity-managed state. They should be
invoked only after the operator chooses the action in the TUI. Examples:

- `.\brevity.ps1 task new <slug>`
- `.\brevity.ps1 task cleanup <slug> --force`
- `.\brevity.ps1 provider set <provider> <status>`
- `.\brevity.ps1 provider reset <provider>`
- `.\brevity.ps1 memory note <text>`

Destructive commands remove worktrees, delete branches, discard runtime records,
or otherwise make recovery depend on Git history or external backups. Force
cleanup is destructive even though it is also a mutating command.

## Confirmation Expectations

Destructive actions require explicit operator confirmation. A future TUI should
show the exact command it is about to execute before running it, including flags
such as `--force`.

After any mutating or destructive command completes, the TUI should refresh
state by running:

```powershell
.\brevity.ps1 runtime state --json
```

The post-command view should be rendered from that refreshed snapshot rather
than from optimistic local edits to runtime files.

## Safety Boundary

The TUI may offer buttons, menus, and confirmations, but Brevity CLI commands
remain the write boundary. Runtime JSON is an observation contract; command
execution is the mutation contract.
