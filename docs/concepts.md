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
script. Future versions may store Lane templates, prompts, workflow documents,
and local configuration here.

Lane v0 documents this directory but does not require it to exist for `status`.

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

The original onboarding helper created:

- `overview.md`
- `architecture.md`
- `tasks.md`
- `adr\`
- `session-notes\`

Lane should preserve that model when `lane onboard` is implemented.

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

Lane should keep that convention for `lane task new`.

## Command Model

Lane is designed around these commands:

- `lane init` creates the workspace skeleton.
- `lane onboard` prepares an existing repo and AI-Vault project memory.
- `lane status` reports repos, worktrees, and vault presence.
- `lane task new` creates an isolated worktree and task branch.
- `lane task status` reports task worktree state.
- `lane task merge` merges a completed task branch back to its base.
- `lane task cleanup` removes task worktrees after merge.

Lane v0 only provides the CLI scaffold and status behavior. Planner automation
is deliberately out of scope.
