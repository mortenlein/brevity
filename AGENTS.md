# Agent Instructions

Brevity is a Windows-first PowerShell CLI scaffold extracted from
`bootstrap-ai-system-complete-v4.ps1`.

## Scope

For Brevity v0, keep the repository small:

- Maintain only `README.md`, `AGENTS.md`, `brevity.ps1`, and `docs/concepts.md`.
- Do not add package managers, dependencies, generated projects, or web apps.
- Do not implement planner automation yet.
- Prefer straightforward PowerShell over framework abstractions.

## Design Rules

- `.brevity` is the orchestration directory name. Do not reintroduce `.system`.
- AI-Vault remains a supported knowledge store under `vaults\AI-Vault`.
- Worktrees are first-class and live under `worktrees\active`,
  `worktrees\paused`, and `worktrees\completed`.
- Markdown is the durable memory format.
- Git is the source of truth for source code and branch integration.

## Command Direction

The intended command map is:

- `brevity init` creates the Brevity workspace skeleton.
- `brevity onboard` prepares an existing repo for Brevity.
- `brevity status` replaces `workspace-status.ps1`.
- `brevity task new` replaces `new-agent-task.ps1`.
- `brevity task status` reports task worktree state.
- `brevity task merge` merges completed task branches.
- `brevity task cleanup` removes completed task worktrees after merge.

Only implement commands when explicitly requested. Until then, commands may be
documented as planned and return a clear "not implemented" message.

## Editing Guidance

- Keep changes minimal and locally test `brevity.ps1` with PowerShell.
- Preserve compatibility with Windows PowerShell where practical.
- Use plain ASCII unless an existing file clearly requires otherwise.
- End work with a concise summary and verification notes.
