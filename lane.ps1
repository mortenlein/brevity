param(
    [Parameter(Position = 0)]
    [string]$Command = "help",

    [Parameter(Position = 1)]
    [string]$Subcommand,

    [string]$DevRoot = (Get-Location).Path
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
    Write-Host ""
    Write-Host "Planned commands:"
    Write-Host "  lane init"
    Write-Host "  lane onboard"
    Write-Host "  lane task new"
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
            "new" { Write-NotImplemented "task new" }
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
