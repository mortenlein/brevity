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

    Write-Host "lane task new must be run inside a Git repository." -ForegroundColor Red
    exit 1
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
    Write-Host "  .\lane.ps1 status [-DevRoot <path>]"
    Write-Host "  .\lane.ps1 task new <slug> [-DevRoot <path>]"
    Write-Host "  .\lane.ps1 task status"
    Write-Host "  .\lane.ps1 task merge <slug>"
    Write-Host "  .\lane.ps1 task cleanup <slug>"
    Write-Host ""
    Write-Host "Planned commands:"
    Write-Host "  lane init"
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

function Remove-TaskWorktree {
    param([string]$Slug)

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\lane.ps1 task cleanup <slug>"
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

    git worktree remove $task.worktreePath
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Cleanup failed while removing worktree. Metadata was not changed." -ForegroundColor Red
        exit $LASTEXITCODE
    }

    git branch -d $task.branch
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Cleanup failed while deleting branch. Metadata was not changed." -ForegroundColor Red
        exit $LASTEXITCODE
    }

    $remainingTasks = @($tasks | Where-Object { $_.slug -ne $Slug })
    if ($remainingTasks.Count -eq 0) {
        Set-Content -LiteralPath $tasksPath -Value "[]" -Encoding ASCII
    }
    else {
        ConvertTo-Json -InputObject $remainingTasks -Depth 4 | Set-Content -LiteralPath $tasksPath -Encoding ASCII
    }

    Write-Host "Removed task metadata: $Slug"
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
        Write-NotImplemented "init"
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
                if ($null -ne $RemainingArgs) {
                    foreach ($taskArg in $RemainingArgs) {
                        $taskSlug = [string]$taskArg
                        break
                    }
                }

                Remove-TaskWorktree -Slug $taskSlug
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
