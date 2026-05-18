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

The v1 snapshot includes these major sections:

- `providers` - provider health totals and per-provider health records.
- `taskCounts` - tracked, runnable, blocked, stale, provider-gated, and review
  task counts.
- `tasks` - runtime task summaries sorted by slug.
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

`Brevity task start <slug>` reads the same metadata file and finds the matching
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

Before printing the command, Brevity refreshes `prompt.md` from the matching
vault task spec when one exists and refreshes `.brevity\context` in the
worktree from selected project memory files. The prompt keeps the worker bounded
by including the task slug, embedded spec contents, local context guidance,
constraints, acceptance checks, and stop-after-summary instructions.

`Brevity task run <slug> [--execute] [--profile <name>] [--smoke] [--force-provider]` reads the same metadata file and finds the matching task
record. It prints:

- task slug
- worktree path
- prompt path
- headless worker command

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
change the user's machine policy. By default, Brevity prints the command only.
With `--execute`, Brevity runs the generated command. It does not update task
status, record metrics, or run planner automation. Unsupported worker providers
return a clear unsupported-provider error.

Before a worker handoff, Brevity materializes only selected durable project
memory files into the task worktree at `.brevity\context\`: `project.md`,
`architecture.md`, `decisions.md`, `current-state.md`, and `roadmap.md`. Missing
files are skipped. Workers read those local files instead of external vault
paths, preserving the split between vault durable memory and bounded worktree
execution context.

Normal operator flow:

```powershell
.\brevity.ps1 task new my-task
.\brevity.ps1 task runtime-info my-task
.\brevity.ps1 task context status my-task
.\brevity.ps1 task context refresh my-task
```

`task runtime-info` exposes the current context state for the task. `task
context status` reports the managed context files in the worktree, while `task
context refresh` re-materializes them from vault memory. If a selected vault
memory file does not exist, Brevity skips it safely. Refresh is useful when an
operator wants to restore Brevity-managed context files before handing the task
to a worker.

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
