# Brevity

Brevity is a Windows-first command line scaffold for AI-assisted repository work.
It extracts the useful shape of `bootstrap-ai-system-complete-v4.ps1` into a
small repo-owned tool:

- `.system` becomes `.Brevity`
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
.\brevity.ps1 plan apply <file>
.\brevity.ps1 board
.\brevity.ps1 status [-DevRoot <path>]
.\brevity.ps1 task new <slug> [-DevRoot <path>]
.\brevity.ps1 task activate <slug>
.\brevity.ps1 task spec <slug>
.\brevity.ps1 task start <slug>
.\brevity.ps1 task run <slug> [--execute]
.\brevity.ps1 task status
.\brevity.ps1 task merge <slug>
.\brevity.ps1 task cleanup <slug> [--force]
```

The init command prepares the current Git repository for Brevity. It creates
repo-local Brevity state when missing:

```text
<repo>\.brevity\
<repo>\.brevity\tasks.json
<repo>\.brevity\config.json
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
unknown or custom fields. It also creates the same missing `.Brevity` files,
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

and records task metadata in the source repository at:

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

Brevity copies the vault task spec contents into:

```text
<worktreePath>\prompt.md
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

The task start command reads the matching record from:

```text
<repo>\.brevity\tasks.json
```

It prints the task slug, worktree path, prompt path, and exact Codex command to
run manually:

```text
codex -C <worktreePath> -a never -s workspace-write
```

It also prints `Read prompt.md and follow it exactly.` Brevity does not
automatically launch Codex.

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
`-m <model>` and `-p <profile>` when configured. For `gemini`, Brevity builds a
non-interactive command that runs from the task worktree and passes the
`prompt.md` contents to `-p`. It includes `-m <model>` when configured, and
includes `-s` when sandbox is not blank or `none`. Set
`providers.gemini.skipTrust` to `true` to pass `--approval-mode yolo` to Gemini.
Set `providers.gemini.env` to an object of environment variables, such as
`GOOGLE_API_KEY`, when Gemini authentication should be scoped to the worker
process. Dry runs print configured variable names but mask values.
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

Brevity does not implement metrics or other AI providers yet. Setting another
provider returns a clear unsupported-provider error.

The task status command reads:

```text
<repo>\.brevity\tasks.json
```

When task metadata exists, it prints the slug, branch, status, worktree path,
and prompt path for each task. When no task metadata exists, it prints
`No Brevity tasks found.`

The task merge command reads the matching record from:

```text
<repo>\.brevity\tasks.json
```

It merges the recorded branch into the current Git branch with
`git merge <branch>`. When the merge succeeds, Brevity updates the task status to
`merged`. It does not remove the worktree, delete the branch, or remove task
metadata. If the merge fails, Brevity leaves metadata unchanged.

The task cleanup command reads the matching record from:

```text
<repo>\.brevity\tasks.json
```

It removes the recorded Git worktree, deletes the recorded Git branch with
`git branch -d`, and then removes the task record from metadata. This is the
default safe cleanup behavior. If Git cannot remove the worktree or delete the
branch, Brevity leaves the metadata unchanged.

Use `--force` only when you explicitly want Git's forced cleanup behavior:

```powershell
.\brevity.ps1 task cleanup <slug> --force
```

With `--force`, Brevity removes the worktree with
`git worktree remove --force <worktreePath>` and deletes the branch with
`git branch -D <branch>`. If the recorded worktree path is missing or is not a
registered Git worktree, Brevity prints a warning and continues to branch removal.
When branch removal succeeds, or the branch is already missing, Brevity removes the
task metadata record. If branch removal fails for another reason, Brevity keeps the
metadata unchanged.

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
  "codex": {
    "provider": "gemini",
    "command": "gemini",
    "env": {
      "GEMINI_API_KEY": "$env:GEMINI_API_KEY"
    }
  }
}
```

Brevity will expand `$env:GEMINI_API_KEY` to its value when running the worker.
This keeps the secret out of the repository.

### Troubleshooting

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

`Brevity status` is the successor to the bootstrap script's
`.system\scripts\workspace-status.ps1`.

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
- Codex is the only configured worker provider in v0.
- Markdown remains the durable memory layer.
- Git remains the source of truth for code.
