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

    New-Item -ItemType Directory -Path $Path -Force | Out-Null
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

function Write-RepairFieldResults {
    param([object[]]$Results)

    $repaired = @($Results | Where-Object { $_.status -eq "repaired" })
    $unchanged = @($Results | Where-Object { $_.status -eq "unchanged" })

    Write-Section "Repaired Fields"
    if ($repaired.Count -eq 0) {
        Write-Host "No config fields repaired."
    }
    else {
        $repaired | ForEach-Object {
            Write-Host "$($_.name): $($_.oldValue) -> $($_.newValue)"
        }
    }

    Write-Section "Unchanged Fields"
    if ($unchanged.Count -eq 0) {
        Write-Host "No config fields were already correct."
    }
    else {
        $unchanged | ForEach-Object {
            Write-Host "$($_.name): $($_.newValue)"
        }
    }
}

function Add-RepairFieldResult {
    param(
        [object[]]$Results,
        [string]$Status,
        [string]$Name,
        [object]$OldValue,
        [object]$NewValue
    )

    return @($Results) + (New-Object PSObject -Property ([ordered]@{
        status = $Status
        name = $Name
        oldValue = $OldValue
        newValue = $NewValue
    }))
}

function Set-ConfigField {
    param(
        [object]$Config,
        [string]$Name,
        [object]$Value
    )

    $member = Get-Member -InputObject $Config -Name $Name -MemberType NoteProperty -ErrorAction SilentlyContinue
    if ($null -eq $member) {
        Add-Member -InputObject $Config -MemberType NoteProperty -Name $Name -Value $Value
        return
    }

    $Config.$Name = $Value
}

function ConvertTo-LaneBoolean {
    param([object]$Value)

    if ($null -eq $Value) {
        return $false
    }

    if ($Value -is [bool]) {
        return $Value
    }

    $text = ([string]$Value).Trim()
    return [string]::Equals($text, "true", [System.StringComparison]::OrdinalIgnoreCase)
}

function Get-DefaultCodexConfig {
    return (New-Object PSObject -Property ([ordered]@{
        command = "codex"
        mode = "exec"
        sandbox = "workspace-write"
        model = $null
        profile = $null
        executionPolicy = "Bypass"
        autoExecute = $false
    }))
}

function Get-DefaultGeminiConfig {
    return (New-Object PSObject -Property ([ordered]@{
        command = "gemini"
        model = $null
        approvalMode = $null
        skipTrust = $false
    }))
}

function Get-DefaultProvidersConfig {
    return (New-Object PSObject -Property ([ordered]@{
        codex = Get-DefaultCodexConfig
        gemini = Get-DefaultGeminiConfig
    }))
}

function Repair-ConfigField {
    param(
        [object]$Config,
        [object[]]$Results,
        [string]$Name,
        [string]$ExpectedValue
    )

    $member = Get-Member -InputObject $Config -Name $Name -MemberType NoteProperty -ErrorAction SilentlyContinue
    $oldValue = $null
    if ($null -ne $member) {
        $oldValue = $Config.$Name
    }

    if ($null -ne $member -and [string]::Equals([string]$oldValue, $ExpectedValue, [System.StringComparison]::OrdinalIgnoreCase)) {
        return Add-RepairFieldResult -Results $Results -Status "unchanged" -Name $Name -OldValue $oldValue -NewValue $ExpectedValue
    }

    Set-ConfigField -Config $Config -Name $Name -Value $ExpectedValue
    return Add-RepairFieldResult -Results $Results -Status "repaired" -Name $Name -OldValue $oldValue -NewValue $ExpectedValue
}

function Repair-ConfigObjectField {
    param(
        [object]$Config,
        [object[]]$Results,
        [string]$Name
    )

    $member = Get-Member -InputObject $Config -Name $Name -MemberType NoteProperty -ErrorAction SilentlyContinue
    $oldValue = $null
    if ($null -ne $member) {
        $oldValue = $Config.$Name
    }

    if ($null -eq $member -or $null -eq $oldValue -or $oldValue -isnot [System.Management.Automation.PSCustomObject]) {
        Set-ConfigField -Config $Config -Name $Name -Value (New-Object PSObject)
        return Add-RepairFieldResult -Results $Results -Status "repaired" -Name $Name -OldValue $oldValue -NewValue "[object]"
    }

    return Add-RepairFieldResult -Results $Results -Status "unchanged" -Name $Name -OldValue $oldValue -NewValue "[object]"
}

function Repair-ProviderConfigField {
    param(
        [object]$ProviderConfig,
        [object[]]$Results,
        [string]$ProviderName,
        [string]$Name,
        [object]$ExpectedValue
    )

    $member = Get-Member -InputObject $ProviderConfig -Name $Name -MemberType NoteProperty -ErrorAction SilentlyContinue
    $oldValue = $null
    if ($null -ne $member) {
        $oldValue = $ProviderConfig.$Name
    }

    if ($null -ne $member) {
        return Add-RepairFieldResult -Results $Results -Status "unchanged" -Name "providers.$ProviderName.$Name" -OldValue $oldValue -NewValue $oldValue
    }

    Set-ConfigField -Config $ProviderConfig -Name $Name -Value $ExpectedValue
    return Add-RepairFieldResult -Results $Results -Status "repaired" -Name "providers.$ProviderName.$Name" -OldValue $oldValue -NewValue $ExpectedValue
}

function Repair-ProviderConfigDefaults {
    param(
        [object]$Config,
        [object[]]$Results
    )

    $defaultProvider = "codex"
    if (Get-Member -InputObject $Config -Name "codex" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $legacyCodexForProvider = $Config.codex
        if ($null -ne $legacyCodexForProvider -and (Get-Member -InputObject $legacyCodexForProvider -Name "provider" -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
            $legacyProvider = ([string]$legacyCodexForProvider.provider).ToLowerInvariant()
            if ($legacyProvider -eq "codex" -or $legacyProvider -eq "gemini") {
                $defaultProvider = $legacyProvider
            }
        }
    }

    $Results = Repair-ConfigField -Config $Config -Results $Results -Name "defaultProvider" -ExpectedValue $defaultProvider
    $Results = Repair-ConfigObjectField -Config $Config -Results $Results -Name "providers"
    $Results = Repair-ConfigObjectField -Config $Config.providers -Results $Results -Name "codex"
    $Results = Repair-ConfigObjectField -Config $Config.providers -Results $Results -Name "gemini"

    if (Get-Member -InputObject $Config -Name "codex" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $legacyCodex = $Config.codex
        if ($null -ne $legacyCodex -and $legacyCodex -is [System.Management.Automation.PSCustomObject]) {
            foreach ($legacyField in @($legacyCodex.PSObject.Properties.Name)) {
                if ($legacyField -eq "provider") {
                    continue
                }

                if (-not (Get-Member -InputObject $Config.providers.codex -Name $legacyField -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
                    Set-ConfigField -Config $Config.providers.codex -Name $legacyField -Value $legacyCodex.$legacyField
                }
            }
        }
    }

    $codexDefaults = Get-DefaultCodexConfig
    $geminiDefaults = Get-DefaultGeminiConfig

    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.codex -Results $Results -ProviderName "codex" -Name "command" -ExpectedValue $codexDefaults.command
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.codex -Results $Results -ProviderName "codex" -Name "mode" -ExpectedValue $codexDefaults.mode
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.codex -Results $Results -ProviderName "codex" -Name "sandbox" -ExpectedValue $codexDefaults.sandbox
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.codex -Results $Results -ProviderName "codex" -Name "model" -ExpectedValue $codexDefaults.model
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.codex -Results $Results -ProviderName "codex" -Name "profile" -ExpectedValue $codexDefaults.profile
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.codex -Results $Results -ProviderName "codex" -Name "executionPolicy" -ExpectedValue $codexDefaults.executionPolicy
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.codex -Results $Results -ProviderName "codex" -Name "autoExecute" -ExpectedValue $codexDefaults.autoExecute
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.gemini -Results $Results -ProviderName "gemini" -Name "command" -ExpectedValue $geminiDefaults.command
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.gemini -Results $Results -ProviderName "gemini" -Name "model" -ExpectedValue $geminiDefaults.model
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.gemini -Results $Results -ProviderName "gemini" -Name "approvalMode" -ExpectedValue $geminiDefaults.approvalMode
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.gemini -Results $Results -ProviderName "gemini" -Name "skipTrust" -ExpectedValue $geminiDefaults.skipTrust

    return $Results
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
    Write-Host "  .\lane.ps1 init --repair [-DevRoot <path>]"
    Write-Host "  .\lane.ps1 plan"
    Write-Host "  .\lane.ps1 plan backlog"
    Write-Host "  .\lane.ps1 plan apply <file>"
    Write-Host "  .\lane.ps1 board"
    Write-Host "  .\lane.ps1 status [-DevRoot <path>]"
    Write-Host "  .\lane.ps1 task new <slug> [-DevRoot <path>]"
    Write-Host "  .\lane.ps1 task activate <slug>"
    Write-Host "  .\lane.ps1 task spec <slug>"
    Write-Host "  .\lane.ps1 task start <slug>"
    Write-Host "  .\lane.ps1 task run <slug> [--execute]"
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

    if (-not (Get-Member -InputObject $config -Name "worktreesRoot" -MemberType NoteProperty)) {
        Write-Host "Lane config is missing worktreesRoot: $configPath" -ForegroundColor Red
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($config.worktreesRoot)) {
        Write-Host "Lane config worktreesRoot is empty: $configPath" -ForegroundColor Red
        exit 1
    }

    if (-not (Get-Member -InputObject $config -Name "projectName" -MemberType NoteProperty)) {
        Write-Host "Lane config is missing projectName: $configPath" -ForegroundColor Red
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($config.projectName)) {
        Write-Host "Lane config projectName is empty: $configPath" -ForegroundColor Red
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

function New-BacklogPlanPrompt {
    $repoRoot = Get-RepositoryRoot
    $config = Read-LaneConfig
    $promptPath = Join-Path $repoRoot ".lane\planner-backlog-prompt.md"

    $promptLines = @(
        "Read AGENTS.md.",
        "",
        "You are planning a backlog of Lane worker tasks for this repository.",
        "",
        "Project memory:",
        "",
        '```text',
        $config.vaultPath,
        '```',
        "",
        "Read the configured vaultPath project memory before planning. Use the Markdown files and task notes there as durable project context.",
        "",
        "Plan a larger body of work as multiple small tasks.",
        "",
        "Return 5-10 tasks.",
        "",
        "Each task must include exactly these fields:",
        "",
        "- title",
        "- slug",
        "- status: planned",
        "- dependencies: []",
        "- workerPrompt",
        "",
        "Task requirements:",
        "",
        "- Keep tasks small and independently executable where possible.",
        "- Make each workerPrompt concrete and bounded.",
        "- Include relevant context from project memory in each workerPrompt.",
        "- Include concise verification steps in each workerPrompt.",
        "- End each workerPrompt with: Stop after patch + summary.",
        "- Avoid placeholders such as TODO, TBD, <fill in>, or examples that must be replaced.",
        "",
        "Constraints:",
        "",
        "- Do not implement code.",
        "- Do not create worktrees.",
        "- Do not create tasks from the backlog.",
        "- Do not parse planner output.",
        "- Do not launch Codex.",
        "- Do not propose or implement a TUI."
    )

    Set-Content -LiteralPath $promptPath -Value $promptLines -Encoding ASCII

    Write-Host "Backlog planner prompt: $promptPath"
    Write-Host "Open Codex in this repo and paste the backlog planner prompt."
}

function Get-PlannerFieldValue {
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

    return ([string]$value).Trim()
}

function Test-PlannerFieldLine {
    param(
        [string]$Line,
        [ref]$Name,
        [ref]$Value
    )

    if ($Line -match '^\s*(?:[-*]\s*)?(title|slug|status|dependencies|workerPrompt)\s*:\s*(.*)$') {
        $Name.Value = $matches[1]
        $Value.Value = $matches[2]
        return $true
    }

    return $false
}

function Add-PlannerTask {
    param(
        [object[]]$Tasks,
        [object]$CurrentTask,
        [string]$CurrentField,
        [string[]]$FieldLines
    )

    if ($null -eq $CurrentTask) {
        return @($Tasks)
    }

    if (-not [string]::IsNullOrWhiteSpace($CurrentField)) {
        Set-ConfigField -Config $CurrentTask -Name $CurrentField -Value (($FieldLines -join "`r`n").Trim())
    }

    return @($Tasks) + $CurrentTask
}

function Read-PlannerOutputTasks {
    param([string]$Path)

    if ([string]::IsNullOrWhiteSpace($Path)) {
        Write-Host "Missing planner output file." -ForegroundColor Red
        Write-Host "Usage: .\lane.ps1 plan apply <file>"
        exit 1
    }

    if (-not (Test-Path -LiteralPath $Path)) {
        Write-Host "Planner output file not found: $Path" -ForegroundColor Red
        exit 1
    }

    $lines = Get-Content -LiteralPath $Path
    $tasks = @()
    $currentTask = $null
    $currentField = ""
    $fieldLines = @()

    foreach ($line in $lines) {
        $fieldName = ""
        $fieldValue = ""
        if (Test-PlannerFieldLine -Line $line -Name ([ref]$fieldName) -Value ([ref]$fieldValue)) {
            if ($fieldName -eq "title") {
                $tasks = Add-PlannerTask -Tasks $tasks -CurrentTask $currentTask -CurrentField $currentField -FieldLines $fieldLines
                $currentTask = New-Object PSObject -Property ([ordered]@{
                    title = ""
                    slug = ""
                    status = ""
                    dependencies = ""
                    workerPrompt = ""
                })
            }
            elseif ($null -eq $currentTask) {
                Write-Host "Planner output has field before title: $fieldName" -ForegroundColor Red
                exit 1
            }
            elseif (-not [string]::IsNullOrWhiteSpace($currentField)) {
                Set-ConfigField -Config $currentTask -Name $currentField -Value (($fieldLines -join "`r`n").Trim())
            }

            $currentField = $fieldName
            $fieldLines = @($fieldValue)
        }
        elseif ($null -ne $currentTask -and -not [string]::IsNullOrWhiteSpace($currentField)) {
            $fieldLines += $line
        }
    }

    $tasks = Add-PlannerTask -Tasks $tasks -CurrentTask $currentTask -CurrentField $currentField -FieldLines $fieldLines
    return @($tasks)
}

function Write-VaultTaskSpec {
    param(
        [string]$TasksRoot,
        [object]$Task
    )

    $title = Get-PlannerFieldValue -Task $Task -Name "title"
    $slug = Get-PlannerFieldValue -Task $Task -Name "slug"
    $status = Get-PlannerFieldValue -Task $Task -Name "status"
    $dependencies = Get-PlannerFieldValue -Task $Task -Name "dependencies"
    $workerPrompt = Get-PlannerFieldValue -Task $Task -Name "workerPrompt"

    if ([string]::IsNullOrWhiteSpace($title)) {
        throw "Planner task is missing title."
    }

    if ([string]::IsNullOrWhiteSpace($slug)) {
        throw "Planner task '$title' is missing slug."
    }

    if ($slug -notmatch '^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$') {
        throw "Planner task '$title' has invalid slug '$slug'. Use lowercase letters, numbers, and hyphens."
    }

    if ($status -ne "planned") {
        throw "Planner task '$slug' must have status: planned."
    }

    if ([string]::IsNullOrWhiteSpace($dependencies)) {
        throw "Planner task '$slug' is missing dependencies."
    }

    if ([string]::IsNullOrWhiteSpace($workerPrompt)) {
        throw "Planner task '$slug' is missing workerPrompt."
    }

    if ($workerPrompt.StartsWith("|")) {
        $workerPrompt = $workerPrompt.Substring(1).Trim()
    }

    if (-not (Test-Path -LiteralPath $TasksRoot)) {
        New-Item -ItemType Directory -Path $TasksRoot -Force | Out-Null
    }

    $specPath = Join-Path $TasksRoot "$slug.md"
    if (Test-Path -LiteralPath $specPath) {
        throw "Vault task spec already exists: $specPath"
    }

    $specLines = @(
        "# Task: $title",
        "",
        "## Slug",
        "",
        $slug,
        "",
        "## Status",
        "",
        $status,
        "",
        "## Dependencies",
        "",
        $dependencies,
        "",
        "## Worker Prompt",
        "",
        $workerPrompt.Trim()
    )

    Set-Content -LiteralPath $specPath -Value $specLines -Encoding ASCII
    return $specPath
}

function Apply-PlannerOutput {
    param([string]$Path)

    $config = Read-LaneConfig
    $tasksRoot = Join-Path $config.vaultPath "tasks"
    $tasks = @(Read-PlannerOutputTasks -Path $Path)

    if ($tasks.Count -eq 0) {
        Write-Host "Planner output did not contain any tasks." -ForegroundColor Red
        exit 1
    }

    $written = @()
    try {
        foreach ($task in $tasks) {
            $written += Write-VaultTaskSpec -TasksRoot $tasksRoot -Task $task
        }
    }
    catch {
        Write-Host $_.Exception.Message -ForegroundColor Red
        exit 1
    }

    Write-Host "Created vault task specs:"
    $written | ForEach-Object { Write-Host $_ }
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
    param(
        [string]$Root,
        [bool]$Repair = $false
    )

    $rootPath = Resolve-DevRoot $Root
    $repoRoot = Get-RepositoryRoot
    $projectName = Split-Path -Leaf $repoRoot
    $laneRoot = Join-Path $repoRoot ".lane"
    $tasksPath = Join-Path $laneRoot "tasks.json"
    $configPath = Join-Path $laneRoot "config.json"
    $vaultPath = Join-Path $rootPath "vaults\AI-Vault\10-Projects\$projectName"
    $worktreesRoot = Join-Path $rootPath "worktrees\active"
    $agentsPath = Join-Path $repoRoot "AGENTS.md"

    $results = @()
    $results = Ensure-Directory -Path $laneRoot -Results $results
    $results = Ensure-File -Path $tasksPath -Lines @("[]") -Results $results

    $fieldResults = @()
    if ($Repair) {
        if (Test-Path -LiteralPath $configPath) {
            $rawConfig = Get-Content -LiteralPath $configPath -Raw
            if ([string]::IsNullOrWhiteSpace($rawConfig)) {
                $config = New-Object PSObject
            }
            else {
                $config = $rawConfig | ConvertFrom-Json
                if ($null -eq $config) {
                    $config = New-Object PSObject
                }
            }
        }
        else {
            $config = New-Object PSObject
        }

        $fieldResults = Repair-ConfigField -Config $config -Results $fieldResults -Name "projectName" -ExpectedValue $projectName
        $fieldResults = Repair-ConfigField -Config $config -Results $fieldResults -Name "devRoot" -ExpectedValue $rootPath
        $fieldResults = Repair-ConfigField -Config $config -Results $fieldResults -Name "vaultPath" -ExpectedValue $vaultPath
        $fieldResults = Repair-ConfigField -Config $config -Results $fieldResults -Name "worktreesRoot" -ExpectedValue $worktreesRoot
        $fieldResults = Repair-ProviderConfigDefaults -Config $config -Results $fieldResults

        $configLines = @(ConvertTo-Json -InputObject $config -Depth 10)
        if (Test-Path -LiteralPath $configPath) {
            Set-Content -LiteralPath $configPath -Value $configLines -Encoding ASCII
            $results = Add-InitResult -Results $results -Status "existing" -Path $configPath
        }
        else {
            Set-Content -LiteralPath $configPath -Value $configLines -Encoding ASCII
            $results = Add-InitResult -Results $results -Status "created" -Path $configPath
        }
    }
    else {
        $config = New-Object PSObject -Property ([ordered]@{
            projectName = $projectName
            devRoot = $rootPath
            vaultPath = $vaultPath
            worktreesRoot = $worktreesRoot
            defaultProvider = "codex"
            providers = Get-DefaultProvidersConfig
        })
        $configLines = @(ConvertTo-Json -InputObject $config -Depth 10)
        $results = Ensure-File -Path $configPath -Lines $configLines -Results $results
    }

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

    if ($Repair) {
        Write-Host "Repaired Lane project: $projectName"
    }
    else {
        Write-Host "Initialized Lane project: $projectName"
    }
    Write-Host "Repo: $repoRoot"
    Write-Host "DevRoot: $rootPath"
    Write-Host "Vault: $vaultPath"
    if ($Repair) {
        Write-RepairFieldResults -Results $fieldResults
    }
    Write-InitResults -Results $results
}

function Add-TaskMetadata {
    param(
        [string]$RepoRoot,
        [string]$Slug,
        [string]$Branch,
        [string]$WorktreePath,
        [string]$PromptPath,
        [string]$SpecPath = ""
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
        specPath = $SpecPath
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

function Show-TaskSpec {
    param([string]$Slug)

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\lane.ps1 task spec <slug>"
        exit 1
    }

    $config = Read-LaneConfig
    $specPath = Join-Path (Join-Path $config.vaultPath "tasks") "$Slug.md"

    if (-not (Test-Path -LiteralPath $specPath)) {
        Write-Host "Vault task spec not found: $Slug" -ForegroundColor Red
        Write-Host "Expected path: $specPath"
        exit 1
    }

    Write-Host "Task: $Slug"
    Write-Host "Spec: $specPath"
    Write-Host ""
    Get-Content -LiteralPath $specPath
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

function Get-CodexRunConfig {
    param([object]$Config)

    $provider = "codex"
    if (Get-Member -InputObject $Config -Name "defaultProvider" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $provider = $Config.defaultProvider
    }
    elseif (Get-Member -InputObject $Config -Name "codex" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $legacyCodex = $Config.codex
        if ($null -ne $legacyCodex -and (Get-Member -InputObject $legacyCodex -Name "provider" -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
            $provider = $legacyCodex.provider
        }
    }

    if ([string]::IsNullOrWhiteSpace($provider)) {
        $provider = "codex"
    }

    $normalizedProvider = ([string]$provider).ToLowerInvariant()
    $providerDefaults = $null
    if ($normalizedProvider -eq "gemini") {
        $providerDefaults = Get-DefaultGeminiConfig
    }
    elseif ($normalizedProvider -eq "codex") {
        $providerDefaults = Get-DefaultCodexConfig
    }
    else {
        throw "Unsupported worker provider: $provider. Lane v0 supports providers 'codex' and 'gemini'."
    }

    $providerConfig = $null
    if (Get-Member -InputObject $Config -Name "providers" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        if ($null -ne $Config.providers -and (Get-Member -InputObject $Config.providers -Name $normalizedProvider -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
            $providerConfig = $Config.providers.$normalizedProvider
        }
    }

    if ($null -eq $providerConfig -and $normalizedProvider -eq "codex" -and (Get-Member -InputObject $Config -Name "codex" -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
        $providerConfig = $Config.codex
    }

    if ($null -eq $providerConfig) {
        $providerConfig = New-Object PSObject
    }

    $command = $providerDefaults.command
    $mode = $null
    $sandbox = $null
    $model = $null
    $profile = $null
    $executionPolicy = $null
    $approvalMode = $null
    $skipTrust = $false

    if (Get-Member -InputObject $providerDefaults -Name "mode" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $mode = $providerDefaults.mode
    }
    if (Get-Member -InputObject $providerDefaults -Name "sandbox" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $sandbox = $providerDefaults.sandbox
    }
    if (Get-Member -InputObject $providerDefaults -Name "profile" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $profile = $providerDefaults.profile
    }
    if (Get-Member -InputObject $providerDefaults -Name "executionPolicy" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $executionPolicy = $providerDefaults.executionPolicy
    }
    if (Get-Member -InputObject $providerDefaults -Name "approvalMode" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $approvalMode = $providerDefaults.approvalMode
    }
    if (Get-Member -InputObject $providerDefaults -Name "skipTrust" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $skipTrust = ConvertTo-LaneBoolean -Value $providerDefaults.skipTrust
    }

    foreach ($fieldName in @("command", "mode", "sandbox", "model", "profile", "executionPolicy", "approvalMode")) {
        if (Get-Member -InputObject $providerConfig -Name $fieldName -MemberType NoteProperty -ErrorAction SilentlyContinue) {
            Set-Variable -Name $fieldName -Value $providerConfig.$fieldName
        }
    }
    if (Get-Member -InputObject $providerConfig -Name "skipTrust" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $skipTrust = ConvertTo-LaneBoolean -Value $providerConfig.skipTrust
    }

    if ([string]::IsNullOrWhiteSpace($command)) {
        $command = $providerDefaults.command
    }

    if ([string]::IsNullOrWhiteSpace($mode)) {
        if (Get-Member -InputObject $providerDefaults -Name "mode" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
            $mode = $providerDefaults.mode
        }
    }

    if ([string]::IsNullOrWhiteSpace($sandbox)) {
        if (Get-Member -InputObject $providerDefaults -Name "sandbox" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
            $sandbox = $providerDefaults.sandbox
        }
    }

    if ([string]::IsNullOrWhiteSpace($executionPolicy)) {
        if (Get-Member -InputObject $providerDefaults -Name "executionPolicy" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
            $executionPolicy = $providerDefaults.executionPolicy
        }
    }

    return (New-Object PSObject -Property ([ordered]@{
        provider = $normalizedProvider
        command = $command
        mode = $mode
        sandbox = $sandbox
        model = $model
        profile = $profile
        executionPolicy = $executionPolicy
        approvalMode = $approvalMode
        skipTrust = $skipTrust
    }))
}

function Get-TaskPromptText {
    param([string]$WorktreePath)

    $promptPath = Join-Path $WorktreePath "prompt.md"
    if (-not (Test-Path -LiteralPath $promptPath)) {
        throw "Prompt file not found: $promptPath"
    }

    $promptText = Get-Content -LiteralPath $promptPath -Raw
    if ([string]::IsNullOrWhiteSpace($promptText)) {
        throw "Prompt file is empty: $promptPath"
    }

    return $promptText
}

function Format-CommandArgument {
    param([string]$Value)

    if ($Value -notmatch '[\s"`]') {
        return $Value
    }

    return '"' + ($Value -replace '"', '\"') + '"'
}

function Format-CommandLine {
    param([string[]]$Parts)

    return (($Parts | ForEach-Object { Format-CommandArgument -Value $_ }) -join " ")
}

function Format-PowerShellLiteral {
    param([string]$Value)

    return "'" + ($Value -replace "'", "''") + "'"
}

function New-CodexTaskRunCommand {
    param(
        [object]$Config,
        [string]$WorktreePath
    )

    $codexConfig = Get-CodexRunConfig -Config $Config

    if ([string]::Equals([string]$codexConfig.provider, "gemini", [System.StringComparison]::OrdinalIgnoreCase)) {
        $arguments = @()
        $displayArguments = @()

        if (-not [string]::IsNullOrWhiteSpace($codexConfig.sandbox) -and -not [string]::Equals([string]$codexConfig.sandbox, "none", [System.StringComparison]::OrdinalIgnoreCase)) {
            $arguments += "-s"
            $displayArguments += "-s"
        }

        if (-not [string]::IsNullOrWhiteSpace($codexConfig.model)) {
            $arguments += "-m"
            $arguments += [string]$codexConfig.model
            $displayArguments += "-m"
            $displayArguments += [string]$codexConfig.model
        }

        $geminiApprovalMode = $codexConfig.approvalMode
        if ([string]::IsNullOrWhiteSpace($geminiApprovalMode) -and $codexConfig.skipTrust) {
            $geminiApprovalMode = "yolo"
        }

        if (-not [string]::IsNullOrWhiteSpace($geminiApprovalMode)) {
            $arguments += "--approval-mode"
            $arguments += [string]$geminiApprovalMode
            $displayArguments += "--approval-mode"
            $displayArguments += [string]$geminiApprovalMode
        }
        
        if ($codexConfig.skipTrust) {
            $arguments += "--skip-trust"
            $displayArguments += "--skip-trust"
        }

        $arguments += "-p"
        $arguments += Get-TaskPromptText -WorktreePath $WorktreePath
        $displayArguments += "-p"
        $displayCommand = Format-CommandLine -Parts (@([string]$codexConfig.command) + $displayArguments)
        $display = "Set-Location -LiteralPath $(Format-PowerShellLiteral -Value $WorktreePath); $displayCommand (Get-Content -LiteralPath 'prompt.md' -Raw)"

        return (New-Object PSObject -Property ([ordered]@{
            provider = [string]$codexConfig.provider
            command = [string]$codexConfig.command
            arguments = $arguments
            executionPolicy = [string]$codexConfig.executionPolicy
            workingDirectory = $WorktreePath
            display = $display
        }))
    }

    $arguments = @(
        [string]$codexConfig.mode,
        "-C",
        $WorktreePath,
        "-s",
        [string]$codexConfig.sandbox
    )

    if (-not [string]::IsNullOrWhiteSpace($codexConfig.model)) {
        $arguments += "-m"
        $arguments += [string]$codexConfig.model
    }

    if (-not [string]::IsNullOrWhiteSpace($codexConfig.profile)) {
        $arguments += "-p"
        $arguments += [string]$codexConfig.profile
    }

    $arguments += "prompt.md"
    $parts = @([string]$codexConfig.command) + $arguments

    return (New-Object PSObject -Property ([ordered]@{
        provider = [string]$codexConfig.provider
        command = [string]$codexConfig.command
        arguments = $arguments
        executionPolicy = [string]$codexConfig.executionPolicy
        workingDirectory = $WorktreePath
        display = Format-CommandLine -Parts $parts
    }))
}

function Show-TaskRun {
    param(
        [string]$Slug,
        [bool]$Execute = $false
    )

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\lane.ps1 task run <slug> [--execute]"
        exit 1
    }

    $repoRoot = Get-RepositoryRoot
    $config = Read-LaneConfig
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

    try {
        $codexCommand = New-CodexTaskRunCommand -Config $config -WorktreePath $task.worktreePath
    }
    catch {
        Write-Host $_.Exception.Message -ForegroundColor Red
        exit 1
    }

    Write-Host "Task: $($task.slug)"
    Write-Host "Worktree: $($task.worktreePath)"
    Write-Host "Prompt: $($task.promptPath)"
    Write-Host "Provider: $($codexCommand.provider)"
    Write-Host "Worker: $($codexCommand.display)"
    if (-not [string]::IsNullOrWhiteSpace($codexCommand.executionPolicy)) {
        Write-Host "ExecutionPolicy (worker process): $($codexCommand.executionPolicy)"
    }

    if (-not $Execute) {
        Write-Host "Dry run. Pass --execute to run the worker non-interactively."
        return
    }

    Write-Host "Executing $($codexCommand.provider) worker..."
    $previousExecutionPolicyPreference = $env:PSExecutionPolicyPreference
    if (-not [string]::IsNullOrWhiteSpace($codexCommand.executionPolicy)) {
        $env:PSExecutionPolicyPreference = $codexCommand.executionPolicy
    }

    try {
        Push-Location -LiteralPath $codexCommand.workingDirectory
        try {
            & $codexCommand.command @($codexCommand.arguments)
            if ($LASTEXITCODE -ne 0) {
                exit $LASTEXITCODE
            }
        }
        finally {
            Pop-Location
        }
    }
    finally {
        $env:PSExecutionPolicyPreference = $previousExecutionPolicyPreference
    }
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

function Activate-TaskWorktree {
    param([string]$Slug)

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\lane.ps1 task activate <slug>"
        exit 1
    }

    $repoRoot = Get-RepositoryRoot
    $config = Read-LaneConfig
    $specPath = Join-Path (Join-Path $config.vaultPath "tasks") "$Slug.md"

    if (-not (Test-Path -LiteralPath $specPath)) {
        Write-Host "Vault task spec not found: $Slug" -ForegroundColor Red
        Write-Host "Expected path: $specPath"
        exit 1
    }

    $worktreePath = Join-Path $config.worktreesRoot "$($config.projectName)-$Slug"
    $branchName = "task/$Slug"
    $promptPath = Join-Path $worktreePath "prompt.md"

    if (Test-Path -LiteralPath $worktreePath) {
        Write-Host "Task worktree already exists: $worktreePath" -ForegroundColor Red
        exit 1
    }

    if (Test-GitBranchExists -Branch $branchName) {
        Write-Host "Task branch already exists: $branchName" -ForegroundColor Red
        exit 1
    }

    if (-not (Test-Path -LiteralPath $config.worktreesRoot)) {
        New-Item -ItemType Directory -Path $config.worktreesRoot -Force | Out-Null
    }

    git worktree add $worktreePath -b $branchName
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }

    $specContents = Get-Content -LiteralPath $specPath -Raw
    Set-Content -LiteralPath $promptPath -Value $specContents -Encoding ASCII
    Add-TaskMetadata -RepoRoot $repoRoot -Slug $Slug -Branch $branchName -WorktreePath $worktreePath -PromptPath $promptPath -SpecPath $specPath

    Write-Host "Activated task worktree"
    Write-Host "Slug: $Slug"
    Write-Host "Path: $worktreePath"
    Write-Host "Branch: $branchName"
    Write-Host "Prompt: $promptPath"
    Write-Host "Spec: $specPath"
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
        $repair = $false
        if (-not [string]::IsNullOrWhiteSpace($Subcommand)) {
            if ($Subcommand -eq "--repair") {
                $repair = $true
            }
            else {
                Write-Host "Unknown lane init argument: $Subcommand" -ForegroundColor Red
                Show-Help
                exit 1
            }
        }

        if ($null -ne $RemainingArgs) {
            foreach ($initArg in $RemainingArgs) {
                if ($initArg -eq "--repair") {
                    $repair = $true
                }
                else {
                    Write-Host "Unknown lane init argument: $initArg" -ForegroundColor Red
                    Show-Help
                    exit 1
                }
            }
        }

        Initialize-LaneRepository -Root $DevRoot -Repair $repair
    }
    "plan" {
        if ([string]::IsNullOrWhiteSpace($Subcommand)) {
            New-PlanPrompt
        }
        elseif ($Subcommand.ToLowerInvariant() -eq "backlog") {
            New-BacklogPlanPrompt
        }
        elseif ($Subcommand.ToLowerInvariant() -eq "apply") {
            $plannerOutputPath = $null
            if ($null -ne $RemainingArgs) {
                foreach ($planArg in $RemainingArgs) {
                    $plannerOutputPath = [string]$planArg
                    break
                }
            }

            Apply-PlannerOutput -Path $plannerOutputPath
        }
        else {
            Write-Host "Unknown lane plan command: $Subcommand" -ForegroundColor Red
            Show-Help
            exit 1
        }
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
            "activate" {
                $taskSlug = $null
                if ($null -ne $RemainingArgs) {
                    foreach ($taskArg in $RemainingArgs) {
                        $taskSlug = [string]$taskArg
                        break
                    }
                }

                Activate-TaskWorktree -Slug $taskSlug
            }
            "spec" {
                $taskSlug = $null
                if ($null -ne $RemainingArgs) {
                    foreach ($taskArg in $RemainingArgs) {
                        $taskSlug = [string]$taskArg
                        break
                    }
                }

                Show-TaskSpec -Slug $taskSlug
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
            "run" {
                $taskSlug = $null
                $executeTask = $false
                if ($null -ne $RemainingArgs) {
                    foreach ($taskArg in $RemainingArgs) {
                        if ($taskArg -eq "--execute") {
                            $executeTask = $true
                        }
                        elseif ([string]::IsNullOrWhiteSpace($taskSlug)) {
                            $taskSlug = [string]$taskArg
                        }
                        else {
                            Write-Host "Unknown argument for lane task run: $taskArg" -ForegroundColor Red
                            Write-Host "Usage: .\lane.ps1 task run <slug> [--execute]"
                            exit 1
                        }
                    }
                }

                Show-TaskRun -Slug $taskSlug -Execute $executeTask
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
