param(
    [Parameter(Position = 0)]
    [string]$Command = "help",

    [Parameter(Position = 1)]
    [string]$Subcommand,

    [string]$DevRoot = "C:\dev",

    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$RemainingArgs
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Resolve-DevRoot {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        throw "DevRoot does not exist: $Path"
    }

    return (Resolve-Path -LiteralPath $Path).Path
}

function Get-RepositoryName {
    $repoRoot = Get-RepositoryRoot
    return (Split-Path -Leaf $repoRoot)
}

function Get-RepositoryRoot {
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $repoRoot = (& git rev-parse --show-toplevel 2>$null)
    $gitExitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference

    if ($gitExitCode -eq 0 -and -not [string]::IsNullOrWhiteSpace($repoRoot)) {
        return $repoRoot
    }

    Write-Host "Lane must be run inside a Git repository." -ForegroundColor Red
    exit 1
}

function Add-InitResult {
    param(
        [object[]]$Results,
        [string]$Status,
        [string]$Path
    )

    return @($Results) + (New-Object PSObject -Property ([ordered]@{
        status = $Status
        path = $Path
    }))
}

function Ensure-Directory {
    param(
        [string]$Path,
        [object[]]$Results
    )

    if (Test-Path -LiteralPath $Path) {
        return Add-InitResult -Results $Results -Status "existing" -Path $Path
    }

    New-Item -ItemType Directory -Path $Path | Out-Null
    return Add-InitResult -Results $Results -Status "created" -Path $Path
}

function Ensure-File {
    param(
        [string]$Path,
        [string[]]$Lines,
        [object[]]$Results
    )

    if (Test-Path -LiteralPath $Path) {
        return Add-InitResult -Results $Results -Status "existing" -Path $Path
    }

    $parent = Split-Path -Parent $Path
    if (-not (Test-Path -LiteralPath $parent)) {
        New-Item -ItemType Directory -Path $parent | Out-Null
    }

    Set-Content -LiteralPath $Path -Value $Lines -Encoding ASCII
    return Add-InitResult -Results $Results -Status "created" -Path $Path
}

function Write-InitResults {
    param([object[]]$Results)

    $created = @($Results | Where-Object { $_.status -eq "created" })
    $existing = @($Results | Where-Object { $_.status -eq "existing" })

    Write-Section "Created"
    if ($created.Count -eq 0) {
        Write-Host "No files or directories created."
    }
    else {
        $created | ForEach-Object { Write-Host $_.path }
    }

    Write-Section "Already Existed"
    if ($existing.Count -eq 0) {
        Write-Host "No existing files or directories detected."
    }
    else {
        $existing | ForEach-Object { Write-Host $_.path }
    }
}

function Write-Section {
    param([string]$Title)

    Write-Host ""
    Write-Host "=== $Title ===" -ForegroundColor Cyan
}

function Write-DirectoryChildren {
    param(
        [string]$Title,
        [string]$Path
    )

    Write-Section $Title

    if (-not (Test-Path -LiteralPath $Path)) {
        Write-Host "Missing: $Path" -ForegroundColor DarkYellow
        return
    }

    $children = Get-ChildItem -LiteralPath $Path -Directory -ErrorAction SilentlyContinue
    if (-not $children) {
        Write-Host "No entries: $Path" -ForegroundColor DarkGray
        return
    }

    $children | ForEach-Object {
        Write-Host $_.FullName
    }
}

function Show-Help {
    Write-Host "Lane v0"
    Write-Host ""
    Write-Host "Usage:"
    Write-Host "  .\lane.ps1 help"
    Write-Host "  .\lane.ps1 init [-DevRoot <path>]"
    Write-Host "  .\lane.ps1 plan"
    Write-Host "  .\lane.ps1 board"
    Write-Host "  .\lane.ps1 status [-DevRoot <path>]"
    Write-Host "  .\lane.ps1 task new <slug> [-DevRoot <path>]"
    Write-Host "  .\lane.ps1 task start <slug>"
    Write-Host "  .\lane.ps1 task status"
    Write-Host "  .\lane.ps1 task merge <slug>"
    Write-Host "  .\lane.ps1 task cleanup <slug> [--force]"
    Write-Host ""
    Write-Host "Planned commands:"
    Write-Host "  lane onboard"
}

function Show-Status {
    param([string]$Root)

    $rootPath = Resolve-DevRoot $Root

    Write-Host "Lane workspace status"
    Write-Host "DevRoot: $rootPath"

    $laneRoot = Join-Path $rootPath ".lane"
    $vaultRoot = Join-Path $rootPath "vaults\AI-Vault"

    Write-Section "LANE"
    if (Test-Path -LiteralPath $laneRoot) {
        Write-Host $laneRoot -ForegroundColor Green
    }
    else {
        Write-Host "Missing: $laneRoot" -ForegroundColor DarkYellow
    }

    Write-Section "AI-VAULT"
    if (Test-Path -LiteralPath $vaultRoot) {
        Write-Host $vaultRoot -ForegroundColor Green
    }
    else {
        Write-Host "Missing: $vaultRoot" -ForegroundColor DarkYellow
    }

    Write-DirectoryChildren "ACTIVE REPOS" (Join-Path $rootPath "repos\active")
    Write-DirectoryChildren "ACTIVE WORKTREES" (Join-Path $rootPath "worktrees\active")
    Write-DirectoryChildren "PAUSED WORKTREES" (Join-Path $rootPath "worktrees\paused")
    Write-DirectoryChildren "COMPLETED WORKTREES" (Join-Path $rootPath "worktrees\completed")
}

function Write-NotImplemented {
    param([string]$Name)

    Write-Host "lane $Name is planned but not implemented in Lane v0." -ForegroundColor Yellow
    Write-Host "See docs\concepts.md for the command design."
}

function Get-TaskField {
    param(
        [object]$Task,
        [string]$Name
    )

    $member = Get-Member -InputObject $Task -Name $Name -MemberType NoteProperty -ErrorAction SilentlyContinue
    if ($null -eq $member) {
        return ""
    }

    $value = $Task.$Name
    if ($null -eq $value) {
        return ""
    }

    return [string]$value
}

function Read-LaneTasks {
    $repoRoot = Get-RepositoryRoot
    $tasksPath = Join-Path $repoRoot ".lane\tasks.json"

    if (-not (Test-Path -LiteralPath $tasksPath)) {
        return @()
    }

    $rawTasks = Get-Content -LiteralPath $tasksPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawTasks)) {
        return @()
    }

    $parsedTasks = $rawTasks | ConvertFrom-Json
    if ($null -eq $parsedTasks) {
        return @()
    }

    return @($parsedTasks)
}

function Show-Board {
    $tasks = @(Read-LaneTasks)

    if ($tasks.Count -eq 0) {
        Write-Host "No Lane tasks found."
        return
    }

    $knownStatuses = @(
        "planned",
        "ready-for-worker",
        "running",
        "merged",
        "done",
        "blocked"
    )

    $statuses = @()
    foreach ($knownStatus in $knownStatuses) {
        $matchingTasks = @($tasks | Where-Object { (Get-TaskField -Task $_ -Name "status") -eq $knownStatus })
        if ($matchingTasks.Count -gt 0) {
            $statuses += $knownStatus
        }
    }

    $otherStatuses = @(
        $tasks |
            ForEach-Object { Get-TaskField -Task $_ -Name "status" } |
            Where-Object { -not [string]::IsNullOrWhiteSpace($_) -and ($knownStatuses -notcontains $_) } |
            Sort-Object -Unique
    )

    $statuses += $otherStatuses

    $tasksWithoutStatus = @($tasks | Where-Object { [string]::IsNullOrWhiteSpace((Get-TaskField -Task $_ -Name "status")) })
    if ($tasksWithoutStatus.Count -gt 0) {
        $statuses += "unknown"
    }

    foreach ($status in $statuses) {
        if ($status -eq "unknown") {
            $groupTasks = $tasksWithoutStatus
        }
        else {
            $groupTasks = @($tasks | Where-Object { (Get-TaskField -Task $_ -Name "status") -eq $status })
        }

        if ($groupTasks.Count -eq 0) {
            continue
        }

        Write-Section $status
        foreach ($task in $groupTasks) {
            Write-Host "slug: $((Get-TaskField -Task $task -Name "slug"))"
            Write-Host "branch: $((Get-TaskField -Task $task -Name "branch"))"
            Write-Host "worktreePath: $((Get-TaskField -Task $task -Name "worktreePath"))"
            Write-Host ""
        }
    }
}

function Read-LaneConfig {
    $repoRoot = Get-RepositoryRoot
    $configPath = Join-Path $repoRoot ".lane\config.json"

    if (-not (Test-Path -LiteralPath $configPath)) {
        Write-Host "Lane config not found: $configPath" -ForegroundColor Red
        Write-Host "Run .\lane.ps1 init first."
        exit 1
    }

    $rawConfig = Get-Content -LiteralPath $configPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawConfig)) {
        Write-Host "Lane config is empty: $configPath" -ForegroundColor Red
        exit 1
    }

    $config = $rawConfig | ConvertFrom-Json
    if ($null -eq $config) {
        Write-Host "Lane config could not be read: $configPath" -ForegroundColor Red
        exit 1
    }

    if (-not (Get-Member -InputObject $config -Name "vaultPath" -MemberType NoteProperty)) {
        Write-Host "Lane config is missing vaultPath: $configPath" -ForegroundColor Red
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($config.vaultPath)) {
        Write-Host "Lane config vaultPath is empty: $configPath" -ForegroundColor Red
        exit 1
    }

    return $config
}

function New-PlanPrompt {
    $repoRoot = Get-RepositoryRoot
    $config = Read-LaneConfig
    $promptPath = Join-Path $repoRoot ".lane\planner-prompt.md"

    $promptLines = @(
        "Read AGENTS.md.",
        "",
        "You are planning one Lane worker task for this repository.",
        "",
        "Project memory:",
        "",
        '```text',
        $config.vaultPath,
        '```',
        "",
        "Read the configured vaultPath project memory before selecting work. Use the Markdown files and task notes there as durable project context.",
        "",
        "Select exactly ONE small, high-value task that a worker can complete safely in a focused patch.",
        "",
        "Return only these fields:",
        "",
        "Task title:",
        "Task slug:",
        "Worker prompt:",
        "",
        "Worker prompt requirements:",
        "",
        "- Tell the worker to read AGENTS.md.",
        "- Give a concrete, bounded implementation task.",
        "- Include the relevant context from project memory.",
        "- Include concise verification steps.",
        "- End with: Stop after patch + summary.",
        "",
        "Constraints:",
        "",
        "- Do not implement code.",
        "- Do not create a worktree.",
        "- Do not call Codex automatically.",
        "- Do not propose autonomous planning.",
        "- Avoid placeholders such as TODO, TBD, <fill in>, or examples that must be replaced.",
        "- Choose a task that is small enough for one worker turn."
    )

    Set-Content -LiteralPath $promptPath -Value $promptLines -Encoding ASCII

    Write-Host "Planner prompt: $promptPath"
    Write-Host "Open Codex in this repo and paste the planner prompt."
}

function Write-TaskPrompt {
    param(
        [string]$Path,
        [string]$Slug
    )

    $promptLines = @(
        "Read AGENTS.md.",
        "",
        "Task: $Slug",
        "",
        "Keep changes small.",
        "",
        "Stop after patch + summary."
    )

    Set-Content -LiteralPath $Path -Value $promptLines -Encoding ASCII
}

function Initialize-LaneRepository {
    param([string]$Root)

    $rootPath = Resolve-DevRoot $Root
    $repoRoot = Get-RepositoryRoot
    $projectName = Split-Path -Leaf $repoRoot
    $laneRoot = Join-Path $repoRoot ".lane"
    $tasksPath = Join-Path $laneRoot "tasks.json"
    $configPath = Join-Path $laneRoot "config.json"
    $vaultPath = Join-Path $rootPath "vaults\AI-Vault\10-Projects\$projectName"
    $worktreesRoot = Join-Path $rootPath "worktrees"
    $agentsPath = Join-Path $repoRoot "AGENTS.md"

    $results = @()
    $results = Ensure-Directory -Path $laneRoot -Results $results
    $results = Ensure-File -Path $tasksPath -Lines @("[]") -Results $results

    $config = New-Object PSObject -Property ([ordered]@{
        projectName = $projectName
        devRoot = $rootPath
        vaultPath = $vaultPath
        worktreesRoot = $worktreesRoot
    })
    $configLines = @(ConvertTo-Json -InputObject $config -Depth 4)
    $results = Ensure-File -Path $configPath -Lines $configLines -Results $results

    $results = Ensure-Directory -Path $vaultPath -Results $results
    $results = Ensure-Directory -Path (Join-Path $vaultPath "session-notes") -Results $results
    $results = Ensure-Directory -Path (Join-Path $vaultPath "tasks") -Results $results

    $results = Ensure-File -Path (Join-Path $vaultPath "project.md") -Lines @(
        "# $projectName",
        "",
        "Project memory for Lane-assisted work."
    ) -Results $results
    $results = Ensure-File -Path (Join-Path $vaultPath "architecture.md") -Lines @(
        "# Architecture",
        "",
        "Record durable architecture context here."
    ) -Results $results
    $results = Ensure-File -Path (Join-Path $vaultPath "decisions.md") -Lines @(
        "# Decisions",
        "",
        "Record durable project decisions here."
    ) -Results $results

    $results = Ensure-File -Path $agentsPath -Lines @(
        "# Agent Instructions",
        "",
        "Before doing work in this repository, read the project memory in:",
        "",
        '```text',
        $vaultPath,
        '```',
        "",
        "Use the vault memory for durable project context. Do not overwrite existing repository files unless the task explicitly requires it."
    ) -Results $results

    Write-Host "Initialized Lane project: $projectName"
    Write-Host "Repo: $repoRoot"
    Write-Host "DevRoot: $rootPath"
    Write-Host "Vault: $vaultPath"
    Write-InitResults -Results $results
}

function Add-TaskMetadata {
    param(
        [string]$RepoRoot,
        [string]$Slug,
        [string]$Branch,
        [string]$WorktreePath,
        [string]$PromptPath
    )

    $laneRoot = Join-Path $RepoRoot ".lane"
    if (-not (Test-Path -LiteralPath $laneRoot)) {
        New-Item -ItemType Directory -Path $laneRoot | Out-Null
    }

    $tasksPath = Join-Path $laneRoot "tasks.json"
    $tasks = @()
    if (Test-Path -LiteralPath $tasksPath) {
        $rawTasks = Get-Content -LiteralPath $tasksPath -Raw
        if (-not [string]::IsNullOrWhiteSpace($rawTasks)) {
            $parsedTasks = $rawTasks | ConvertFrom-Json
            if ($null -ne $parsedTasks) {
                $tasks = @($parsedTasks)
            }
        }
    }

    $taskRecord = New-Object PSObject -Property ([ordered]@{
        slug = $Slug
        branch = $Branch
        worktreePath = $WorktreePath
        promptPath = $PromptPath
        status = "ready-for-worker"
        createdAt = (Get-Date).ToUniversalTime().ToString("o")
    })

    $tasks = @($tasks) + $taskRecord
    ConvertTo-Json -InputObject $tasks -Depth 4 | Set-Content -LiteralPath $tasksPath -Encoding ASCII
}

function Show-TaskStatus {
    $repoRoot = Get-RepositoryRoot
    $tasksPath = Join-Path $repoRoot ".lane\tasks.json"

    if (-not (Test-Path -LiteralPath $tasksPath)) {
        Write-Host "No Lane tasks found."
        return
    }

    $rawTasks = Get-Content -LiteralPath $tasksPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawTasks)) {
        Write-Host "No Lane tasks found."
        return
    }

    $parsedTasks = $rawTasks | ConvertFrom-Json
    if ($null -eq $parsedTasks) {
        Write-Host "No Lane tasks found."
        return
    }

    $tasks = @($parsedTasks)
    if ($tasks.Count -eq 0) {
        Write-Host "No Lane tasks found."
        return
    }

    $tasks |
        Select-Object slug, branch, status, worktreePath, promptPath |
        Format-List
}

function Start-TaskWork {
    param([string]$Slug)

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\lane.ps1 task start <slug>"
        exit 1
    }

    $repoRoot = Get-RepositoryRoot
    $tasksPath = Join-Path $repoRoot ".lane\tasks.json"

    if (-not (Test-Path -LiteralPath $tasksPath)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Lane task metadata exists at: $tasksPath"
        exit 1
    }

    $rawTasks = Get-Content -LiteralPath $tasksPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawTasks)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Lane task metadata exists at: $tasksPath"
        exit 1
    }

    $parsedTasks = $rawTasks | ConvertFrom-Json
    $tasks = @($parsedTasks)
    $task = $tasks | Where-Object { $_.slug -eq $Slug } | Select-Object -First 1

    if ($null -eq $task) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "Use .\lane.ps1 task status to list known tasks."
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($task.worktreePath)) {
        Write-Host "Task metadata is missing worktreePath for: $Slug" -ForegroundColor Red
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($task.promptPath)) {
        Write-Host "Task metadata is missing promptPath for: $Slug" -ForegroundColor Red
        exit 1
    }

    $codexCommand = "codex -C $($task.worktreePath) -a never -s workspace-write"

    Write-Host "Task: $($task.slug)"
    Write-Host "Worktree: $($task.worktreePath)"
    Write-Host "Prompt: $($task.promptPath)"
    Write-Host "Codex: $codexCommand"
    Write-Host "Read prompt.md and follow it exactly."
}

function Test-GitWorktreeRegistered {
    param([string]$WorktreePath)

    $resolvedExpectedPath = $WorktreePath
    if (Test-Path -LiteralPath $WorktreePath) {
        $resolvedExpectedPath = (Resolve-Path -LiteralPath $WorktreePath).Path
    }

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $worktreeList = (& git worktree list --porcelain 2>$null)
    $gitExitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference

    if ($gitExitCode -ne 0) {
        throw "Unable to list Git worktrees."
    }

    foreach ($line in $worktreeList) {
        if (-not $line.StartsWith("worktree ")) {
            continue
        }

        $registeredPath = $line.Substring("worktree ".Length)
        if (Test-Path -LiteralPath $registeredPath) {
            $registeredPath = (Resolve-Path -LiteralPath $registeredPath).Path
        }

        if ([string]::Equals($registeredPath, $resolvedExpectedPath, [System.StringComparison]::OrdinalIgnoreCase)) {
            return $true
        }
    }

    return $false
}

function Test-GitBranchExists {
    param([string]$Branch)

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    & git show-ref --verify --quiet "refs/heads/$Branch"
    $gitExitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference

    return ($gitExitCode -eq 0)
}

function Remove-TaskMetadataRecord {
    param(
        [string]$TasksPath,
        [object[]]$Tasks,
        [string]$Slug
    )

    $remainingTasks = @($Tasks | Where-Object { $_.slug -ne $Slug })
    if ($remainingTasks.Count -eq 0) {
        Set-Content -LiteralPath $TasksPath -Value "[]" -Encoding ASCII
    }
    else {
        ConvertTo-Json -InputObject $remainingTasks -Depth 4 | Set-Content -LiteralPath $TasksPath -Encoding ASCII
    }

    Write-Host "Removed task metadata: $Slug"
}

function Remove-TaskWorktree {
    param(
        [string]$Slug,
        [bool]$Force = $false
    )

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\lane.ps1 task cleanup <slug> [--force]"
        exit 1
    }

    $repoRoot = Get-RepositoryRoot
    $tasksPath = Join-Path $repoRoot ".lane\tasks.json"

    if (-not (Test-Path -LiteralPath $tasksPath)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Lane task metadata exists at: $tasksPath"
        exit 1
    }

    $rawTasks = Get-Content -LiteralPath $tasksPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawTasks)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Lane task metadata exists at: $tasksPath"
        exit 1
    }

    $parsedTasks = $rawTasks | ConvertFrom-Json
    $tasks = @($parsedTasks)
    $task = $tasks | Where-Object { $_.slug -eq $Slug } | Select-Object -First 1

    if ($null -eq $task) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "Use .\lane.ps1 task status to list known tasks."
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($task.worktreePath)) {
        Write-Host "Task metadata is missing worktreePath for: $Slug" -ForegroundColor Red
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($task.branch)) {
        Write-Host "Task metadata is missing branch for: $Slug" -ForegroundColor Red
        exit 1
    }

    Write-Host "Cleaning up Lane task: $Slug"
    Write-Host "Worktree: $($task.worktreePath)"
    Write-Host "Branch: $($task.branch)"
    if ($Force) {
        Write-Host "Force: enabled"
    }

    $worktreeExists = Test-Path -LiteralPath $task.worktreePath
    $worktreeRegistered = Test-GitWorktreeRegistered -WorktreePath $task.worktreePath

    if ((-not $worktreeExists) -or (-not $worktreeRegistered)) {
        Write-Host "Warning: recorded worktree is missing or not registered with Git: $($task.worktreePath)" -ForegroundColor Yellow
        Write-Host "Continuing to branch removal."
    }
    elseif ($Force) {
        git worktree remove --force $task.worktreePath
        if ($LASTEXITCODE -ne 0) {
            Write-Host "Cleanup failed while force-removing worktree. Metadata was not changed." -ForegroundColor Red
            exit $LASTEXITCODE
        }
    }
    else {
        git worktree remove $task.worktreePath
        if ($LASTEXITCODE -ne 0) {
            Write-Host "Cleanup failed while removing worktree. Metadata was not changed." -ForegroundColor Red
            exit $LASTEXITCODE
        }
    }

    if (-not (Test-GitBranchExists -Branch $task.branch)) {
        Write-Host "Warning: recorded branch is already missing: $($task.branch)" -ForegroundColor Yellow
        Remove-TaskMetadataRecord -TasksPath $tasksPath -Tasks $tasks -Slug $Slug
        return
    }

    if ($Force) {
        git branch -D $task.branch
    }
    else {
        git branch -d $task.branch
    }

    if ($LASTEXITCODE -ne 0) {
        Write-Host "Cleanup failed while deleting branch. Metadata was not changed." -ForegroundColor Red
        exit $LASTEXITCODE
    }

    Remove-TaskMetadataRecord -TasksPath $tasksPath -Tasks $tasks -Slug $Slug
}

function Merge-TaskBranch {
    param([string]$Slug)

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\lane.ps1 task merge <slug>"
        exit 1
    }

    $repoRoot = Get-RepositoryRoot
    $tasksPath = Join-Path $repoRoot ".lane\tasks.json"

    if (-not (Test-Path -LiteralPath $tasksPath)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Lane task metadata exists at: $tasksPath"
        exit 1
    }

    $rawTasks = Get-Content -LiteralPath $tasksPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawTasks)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Lane task metadata exists at: $tasksPath"
        exit 1
    }

    $parsedTasks = $rawTasks | ConvertFrom-Json
    $tasks = @($parsedTasks)
    $task = $tasks | Where-Object { $_.slug -eq $Slug } | Select-Object -First 1

    if ($null -eq $task) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "Use .\lane.ps1 task status to list known tasks."
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($task.branch)) {
        Write-Host "Task metadata is missing branch for: $Slug" -ForegroundColor Red
        exit 1
    }

    Write-Host "Merging Lane task: $Slug"
    Write-Host "Branch: $($task.branch)"
    Write-Host "Target: current Git branch"

    git merge $task.branch
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Merge failed. Task metadata was not changed." -ForegroundColor Red
        exit $LASTEXITCODE
    }

    foreach ($taskRecord in $tasks) {
        if ($taskRecord.slug -eq $Slug) {
            $taskRecord.status = "merged"
        }
    }

    ConvertTo-Json -InputObject $tasks -Depth 4 | Set-Content -LiteralPath $tasksPath -Encoding ASCII
    Write-Host "Updated task status to merged: $Slug"
}

function New-TaskWorktree {
    param(
        [string]$Root,
        [string]$Slug
    )

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\lane.ps1 task new <slug> [-DevRoot <path>]"
        exit 1
    }

    $rootPath = Resolve-DevRoot $Root
    $repoRoot = Get-RepositoryRoot
    $repoName = Get-RepositoryName
    $worktreeName = "$repoName-$Slug"
    $targetPath = Join-Path $rootPath "worktrees\active\$worktreeName"
    $branchName = "task/$Slug"
    $promptPath = Join-Path $targetPath "prompt.md"

    if (Test-Path -LiteralPath $targetPath) {
        Write-Host "Task worktree already exists: $targetPath" -ForegroundColor Red
        exit 1
    }

    $activeRoot = Split-Path -Parent $targetPath
    if (-not (Test-Path -LiteralPath $activeRoot)) {
        New-Item -ItemType Directory -Path $activeRoot | Out-Null
    }

    git worktree add $targetPath -b $branchName
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }

    Write-TaskPrompt -Path $promptPath -Slug $Slug
    Add-TaskMetadata -RepoRoot $repoRoot -Slug $Slug -Branch $branchName -WorktreePath $targetPath -PromptPath $promptPath

    Write-Host "Created task worktree"
    Write-Host "Path: $targetPath"
    Write-Host "Branch: $branchName"
    Write-Host "Prompt: $promptPath"
    Write-Host "Metadata: $(Join-Path $repoRoot ".lane\tasks.json")"
}

switch ($Command.ToLowerInvariant()) {
    "help" {
        Show-Help
    }
    "status" {
        Show-Status -Root $DevRoot
    }
    "init" {
        Initialize-LaneRepository -Root $DevRoot
    }
    "plan" {
        New-PlanPrompt
    }
    "board" {
        Show-Board
    }
    "onboard" {
        Write-NotImplemented "onboard"
    }
    "task" {
        if ([string]::IsNullOrWhiteSpace($Subcommand)) {
            Write-NotImplemented "task"
            exit 0
        }

        switch ($Subcommand.ToLowerInvariant()) {
            "new" {
                $taskSlug = $null
                if ($null -ne $RemainingArgs) {
                    foreach ($taskArg in $RemainingArgs) {
                        $taskSlug = [string]$taskArg
                        break
                    }
                }

                New-TaskWorktree -Root $DevRoot -Slug $taskSlug
            }
            "start" {
                $taskSlug = $null
                if ($null -ne $RemainingArgs) {
                    foreach ($taskArg in $RemainingArgs) {
                        $taskSlug = [string]$taskArg
                        break
                    }
                }

                Start-TaskWork -Slug $taskSlug
            }
            "status" { Show-TaskStatus }
            "merge" {
                $taskSlug = $null
                if ($null -ne $RemainingArgs) {
                    foreach ($taskArg in $RemainingArgs) {
                        $taskSlug = [string]$taskArg
                        break
                    }
                }

                Merge-TaskBranch -Slug $taskSlug
            }
            "cleanup" {
                $taskSlug = $null
                $forceCleanup = $false
                if ($null -ne $RemainingArgs) {
                    foreach ($taskArg in $RemainingArgs) {
                        if ($taskArg -eq "--force") {
                            $forceCleanup = $true
                        }
                        elseif ([string]::IsNullOrWhiteSpace($taskSlug)) {
                            $taskSlug = [string]$taskArg
                        }
                        else {
                            Write-Host "Unknown argument for lane task cleanup: $taskArg" -ForegroundColor Red
                            Write-Host "Usage: .\lane.ps1 task cleanup <slug> [--force]"
                            exit 1
                        }
                    }
                }

                Remove-TaskWorktree -Slug $taskSlug -Force $forceCleanup
            }
            default {
                Write-Host "Unknown lane task command: $Subcommand" -ForegroundColor Red
                Show-Help
                exit 1
            }
        }
    }
    default {
        Write-Host "Unknown lane command: $Command" -ForegroundColor Red
        Show-Help
        exit 1
    }
}
