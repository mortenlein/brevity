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
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $repoRoot = (& git rev-parse --show-toplevel 2>$null)
    $gitExitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference

    if ($gitExitCode -eq 0 -and -not [string]::IsNullOrWhiteSpace($repoRoot)) {
        return (Split-Path -Leaf $repoRoot)
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
    Write-Host ""
    Write-Host "Planned commands:"
    Write-Host "  lane init"
    Write-Host "  lane onboard"
    Write-Host "  lane task status"
    Write-Host "  lane task merge"
    Write-Host "  lane task cleanup"
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
    $repoName = Get-RepositoryName
    $worktreeName = "$repoName-$Slug"
    $targetPath = Join-Path $rootPath "worktrees\active\$worktreeName"
    $branchName = "task/$Slug"

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

    Write-Host "Created task worktree"
    Write-Host "Path: $targetPath"
    Write-Host "Branch: $branchName"
    Write-Host "Start worker: codex -C $targetPath"
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
            "status" { Write-NotImplemented "task status" }
            "merge" { Write-NotImplemented "task merge" }
            "cleanup" { Write-NotImplemented "task cleanup" }
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
