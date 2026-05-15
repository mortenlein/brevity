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

    Write-Host "Brevity must be run inside a Git repository." -ForegroundColor Red
    exit 1
}

function Get-MainRepositoryRoot {
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $repoRoot = (& git rev-parse --main-worktree 2>$null)
    $gitExitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference

    if ($gitExitCode -ne 0 -or [string]::IsNullOrWhiteSpace($repoRoot) -or ([string]$repoRoot).Trim().StartsWith("-")) {
        return Get-RepositoryRoot
    }

    return ([string]$repoRoot).Trim()
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

function ConvertTo-BrevityBoolean {
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
        model = "gemini-3-flash-preview"
        approvalMode = "yolo"
        skipTrust = $true
        env = New-Object PSObject -Property ([ordered]@{
            GEMINI_API_KEY = "GEMINI_API_KEY"
        })
    }))
}

function Get-DefaultCopilotConfig {
    return (New-Object PSObject -Property ([ordered]@{
        command = "copilot"
        allowAllTools = $true
        allowAllPaths = $true
        noAskUser = $true
    }))
}

function Get-DefaultProvidersConfig {
    return (New-Object PSObject -Property ([ordered]@{
        codex = Get-DefaultCodexConfig
        gemini = Get-DefaultGeminiConfig
        copilot = Get-DefaultCopilotConfig
    }))
}

function Get-DefaultProviderHealthState {
    return (New-Object PSObject -Property ([ordered]@{
        codex = New-Object PSObject -Property ([ordered]@{
            status = "unknown"
            note = ""
            updatedAt = $null
        })
        gemini = New-Object PSObject -Property ([ordered]@{
            status = "unknown"
            note = ""
            updatedAt = $null
        })
        copilot = New-Object PSObject -Property ([ordered]@{
            status = "unknown"
            note = ""
            updatedAt = $null
        })
    }))
}

function Ensure-ProviderHealthFile {
    param(
        [string]$Path,
        [object[]]$Results
    )

    $healthLines = @(ConvertTo-Json -InputObject (Get-DefaultProviderHealthState) -Depth 10)
    return Ensure-File -Path $Path -Lines $healthLines -Results $Results
}

function Get-SupportedProviderHealthStatuses {
    return @("healthy", "capacity-degraded", "quota-constrained", "unavailable", "unknown")
}

function Read-ProviderHealth {
    $repoRoot = Get-RepositoryRoot
    $healthPath = Join-Path $repoRoot ".brevity\provider-health.json"

    if (-not (Test-Path -LiteralPath $healthPath)) {
        Write-Host "Provider health file not found: $healthPath" -ForegroundColor Red
        Write-Host "Run .\brevity.ps1 init to create Brevity runtime metadata."
        exit 1
    }

    $rawHealth = Get-Content -LiteralPath $healthPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawHealth)) {
        Write-Host "Provider health file is empty: $healthPath" -ForegroundColor Red
        exit 1
    }

    try {
        $health = $rawHealth | ConvertFrom-Json
    }
    catch {
        Write-Host "Provider health file is not valid JSON: $healthPath" -ForegroundColor Red
        exit 1
    }

    return (New-Object PSObject -Property ([ordered]@{
        path = $healthPath
        health = $health
    }))
}

function Test-ProviderHealthStatus {
    param([string]$Status)

    $normalizedStatus = ([string]$Status).ToLowerInvariant()
    return @(Get-SupportedProviderHealthStatuses) -contains $normalizedStatus
}

function Show-ProviderStatus {
    $providerHealth = Read-ProviderHealth
    $health = $providerHealth.health

    Write-Host "Provider health"
    Write-Host "Path: $($providerHealth.path)"
    Write-Host ""

    foreach ($providerName in @($health.PSObject.Properties.Name | Sort-Object)) {
        $provider = $health.$providerName
        $status = ""
        $note = ""
        $updatedAt = ""

        if ($null -ne $provider) {
            if (Get-Member -InputObject $provider -Name "status" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
                $status = [string]$provider.status
            }
            if (Get-Member -InputObject $provider -Name "note" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
                $note = [string]$provider.note
            }
            if (Get-Member -InputObject $provider -Name "updatedAt" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
                $updatedAt = [string]$provider.updatedAt
            }
        }

        if ([string]::IsNullOrWhiteSpace($status)) {
            $status = "unknown"
        }
        if ([string]::IsNullOrWhiteSpace($updatedAt)) {
            $updatedAt = "-"
        }
        if ([string]::IsNullOrWhiteSpace($note)) {
            $note = "-"
        }

        Write-Host "$providerName`t$status`t$updatedAt`t$note"
    }
}

function Set-ProviderStatus {
    param(
        [string]$ProviderName,
        [string]$Status,
        [string]$Note = ""
    )

    if ([string]::IsNullOrWhiteSpace($ProviderName)) {
        Write-Host "Missing provider name." -ForegroundColor Red
        Write-Host "Usage: .\brevity.ps1 provider set <provider> <status> [-Note <note>]"
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($Status)) {
        Write-Host "Missing provider status." -ForegroundColor Red
        Write-Host "Usage: .\brevity.ps1 provider set <provider> <status> [-Note <note>]"
        exit 1
    }

    $normalizedProviderName = ([string]$ProviderName).ToLowerInvariant()
    $normalizedStatus = ([string]$Status).ToLowerInvariant()

    if (-not (Test-ProviderHealthStatus -Status $normalizedStatus)) {
        Write-Host "Invalid provider status: $Status" -ForegroundColor Red
        Write-Host "Supported statuses: $((Get-SupportedProviderHealthStatuses) -join ', ')"
        exit 1
    }

    $providerHealth = Read-ProviderHealth
    $health = $providerHealth.health
    if (-not (Get-Member -InputObject $health -Name $normalizedProviderName -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
        Write-Host "Invalid provider: $ProviderName" -ForegroundColor Red
        Write-Host "Known providers: $((@($health.PSObject.Properties.Name) | Sort-Object) -join ', ')"
        exit 1
    }

    $provider = $health.$normalizedProviderName
    if ($null -eq $provider -or $provider -isnot [System.Management.Automation.PSCustomObject]) {
        $provider = New-Object PSObject
        $health.$normalizedProviderName = $provider
    }

    Set-ConfigField -Config $provider -Name "status" -Value $normalizedStatus
    Set-ConfigField -Config $provider -Name "note" -Value ([string]$Note)
    Set-ConfigField -Config $provider -Name "updatedAt" -Value ([DateTime]::UtcNow.ToString("o"))

    ConvertTo-Json -InputObject $health -Depth 10 | Set-Content -LiteralPath $providerHealth.path -Encoding ASCII
    Write-Host "Updated provider health: $normalizedProviderName -> $normalizedStatus"
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

    $defaultProvider = "gemini"
    if (Get-Member -InputObject $Config -Name "codex" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $legacyConfigForProvider = $Config.codex
        if ($null -ne $legacyConfigForProvider -and (Get-Member -InputObject $legacyConfigForProvider -Name "provider" -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
            $legacyProvider = ([string]$legacyConfigForProvider.provider).ToLowerInvariant()
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
        $legacyConfig = $Config.codex
        if ($null -ne $legacyConfig -and $legacyConfig -is [System.Management.Automation.PSCustomObject]) {
            foreach ($legacyField in @($legacyConfig.PSObject.Properties.Name)) {
                if ($legacyField -eq "provider") {
                    continue
                }

                if (-not (Get-Member -InputObject $Config.providers.codex -Name $legacyField -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
                    Set-ConfigField -Config $Config.providers.codex -Name $legacyField -Value $legacyConfig.$legacyField
                }
            }
        }
    }

    $codexDefaults = Get-DefaultCodexConfig
    $geminiDefaults = Get-DefaultGeminiConfig
    $copilotDefaults = Get-DefaultCopilotConfig

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
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.gemini -Results $Results -ProviderName "gemini" -Name "env" -ExpectedValue $geminiDefaults.env
    $Results = Repair-ConfigObjectField -Config $Config.providers -Results $Results -Name "copilot"
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.copilot -Results $Results -ProviderName "copilot" -Name "command" -ExpectedValue $copilotDefaults.command
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.copilot -Results $Results -ProviderName "copilot" -Name "allowAllTools" -ExpectedValue $copilotDefaults.allowAllTools
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.copilot -Results $Results -ProviderName "copilot" -Name "allowAllPaths" -ExpectedValue $copilotDefaults.allowAllPaths
    $Results = Repair-ProviderConfigField -ProviderConfig $Config.providers.copilot -Results $Results -ProviderName "copilot" -Name "noAskUser" -ExpectedValue $copilotDefaults.noAskUser

    return $Results
}

function Write-Section {
    param([string]$Title)

    Write-Host ""
    Write-Host "=== $Title ===" -ForegroundColor Cyan
}

function Write-DirectoryChildren {
    param(
        [string]$Path,
        [string]$Title
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
    Write-Host "Brevity v0"
    Write-Host ""
    Write-Host "Usage:"
    Write-Host "  .\brevity.ps1 help"
    Write-Host "  .\brevity.ps1 init [-DevRoot <path>]"
    Write-Host "  .\brevity.ps1 init --repair [-DevRoot <path>]"
    Write-Host "  .\brevity.ps1 plan"
    Write-Host "  .\brevity.ps1 plan backlog"
    Write-Host "  .\brevity.ps1 plan apply <file>"
    Write-Host "  .\brevity.ps1 board"
    Write-Host "  .\brevity.ps1 status [-DevRoot <path>]"
    Write-Host "  .\brevity.ps1 provider status"
    Write-Host "  .\brevity.ps1 provider set <provider> <status> [-Note <note>]"
    Write-Host "  .\brevity.ps1 task new <slug> [-DevRoot <path>]"
    Write-Host "  .\brevity.ps1 task activate <slug>"
    Write-Host "  .\brevity.ps1 task spec <slug>"
    Write-Host "  .\brevity.ps1 task start <slug>"
    Write-Host "  .\brevity.ps1 task run <slug> [--execute] [--profile <name>]"
    Write-Host "  .\brevity.ps1 task status"
    Write-Host "  .\brevity.ps1 task merge <slug>"
    Write-Host "  .\brevity.ps1 task cleanup <slug> [--force]"
    Write-Host ""
    Write-Host "Planned commands:"
    Write-Host "  brevity onboard"
}

function Show-Status {
    param([string]$Root)

    $rootPath = Resolve-DevRoot $Root

    Write-Host "Brevity workspace status"
    Write-Host "DevRoot: $rootPath"

    $brevityRoot = Join-Path $rootPath ".brevity"
    $vaultRoot = Join-Path $rootPath "vaults\AI-Vault"

    Write-Section "BREVITY"
    if (Test-Path -LiteralPath $brevityRoot) {
        Write-Host $brevityRoot -ForegroundColor Green
    }
    else {
        Write-Host "Missing: $brevityRoot" -ForegroundColor DarkYellow
    }

    Write-Section "AI-VAULT"
    if (Test-Path -LiteralPath $vaultRoot) {
        Write-Host $vaultRoot -ForegroundColor Green
    }
    else {
        Write-Host "Missing: $vaultRoot" -ForegroundColor DarkYellow
    }

    Write-DirectoryChildren -Title "ACTIVE REPOS" -Path (Join-Path $rootPath "repos\active")
    Write-DirectoryChildren -Title "ACTIVE WORKTREES" -Path (Join-Path $rootPath "worktrees\active")
    Write-DirectoryChildren -Title "PAUSED WORKTREES" -Path (Join-Path $rootPath "worktrees\paused")
    Write-DirectoryChildren -Title "COMPLETED WORKTREES" -Path (Join-Path $rootPath "worktrees\completed")
}

function Write-NotImplemented {
    param([string]$Name)

    Write-Host "brevity $Name is planned but not implemented in Brevity v0." -ForegroundColor Yellow
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

function Read-BrevityTasks {
    $repoRoot = Get-RepositoryRoot
    $tasksPath = Join-Path $repoRoot ".brevity\tasks.json"

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
    $tasks = @(Read-BrevityTasks)

    if ($tasks.Count -eq 0) {
        Write-Host "No Brevity tasks found."
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

function Read-BrevityConfig {
    $repoRoot = Get-MainRepositoryRoot
    $configPath = Join-Path $repoRoot ".brevity\config.json"

    if (-not (Test-Path -LiteralPath $configPath)) {
        Write-Host "Brevity config not found: $configPath" -ForegroundColor Red
        Write-Host "Run .\brevity.ps1 init first."
        exit 1
    }

    $rawConfig = Get-Content -LiteralPath $configPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawConfig)) {
        Write-Host "Brevity config is empty: $configPath" -ForegroundColor Red
        exit 1
    }

    $config = $rawConfig | ConvertFrom-Json
    if ($null -eq $config) {
        Write-Host "Brevity config could not be read: $configPath" -ForegroundColor Red
        exit 1
    }

    if (-not (Get-Member -InputObject $config -Name "vaultPath" -MemberType NoteProperty)) {
        Write-Host "Brevity config is missing vaultPath: $configPath" -ForegroundColor Red
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($config.vaultPath)) {
        Write-Host "Brevity config vaultPath is empty: $configPath" -ForegroundColor Red
        exit 1
    }

    if (-not (Get-Member -InputObject $config -Name "worktreesRoot" -MemberType NoteProperty)) {
        Write-Host "Brevity config is missing worktreesRoot: $configPath" -ForegroundColor Red
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($config.worktreesRoot)) {
        Write-Host "Brevity config worktreesRoot is empty: $configPath" -ForegroundColor Red
        exit 1
    }

    if (-not (Get-Member -InputObject $config -Name "projectName" -MemberType NoteProperty)) {
        Write-Host "Brevity config is missing projectName: $configPath" -ForegroundColor Red
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($config.projectName)) {
        Write-Host "Brevity config projectName is empty: $configPath" -ForegroundColor Red
        exit 1
    }

    return $config
}

function New-PlanPrompt {
    $repoRoot = Get-RepositoryRoot
    $config = Read-BrevityConfig
    $promptPath = Join-Path $repoRoot ".brevity\planner-prompt.md"

    $promptLines = @(
        "Read AGENTS.md.",
        "",
        "You are planning one Brevity worker task for this repository.",
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
        "Task complexity: (low, medium, high)",
        "Worker profile: (e.g., gemini-flash, codex-balanced)",
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
        "Worker Profile Guidance:",
        "",
        "- Choose the cheapest profile sufficient for the task complexity.",
        "- low: Use gemini-lite or codex-fast.",
        "- medium: Use gemini-flash or codex-balanced.",
        "- high: Use gemini-pro or codex-deep.",
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
    $config = Read-BrevityConfig
    $promptPath = Join-Path $repoRoot ".brevity\planner-backlog-prompt.md"

    $promptLines = @(
        "Read AGENTS.md.",
        "",
        "You are planning a backlog of Brevity worker tasks for this repository.",
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

    if ($Line -match '^\s*(?:[-*]\s*)?(title|slug|complexity|profile|status|dependencies|workerPrompt)\s*:\s*(.*)$') {
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
        Write-Host "Usage: .\brevity.ps1 plan apply <file>"
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
                    complexity = ""
                    profile = ""
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

    $config = Read-BrevityConfig
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
        [string]$Slug,
        [string]$SpecContents = "",
        [string]$SpecPath = ""
    )

    $normalizedSpec = ""
    if (-not [string]::IsNullOrWhiteSpace($SpecContents)) {
        $normalizedSpec = $SpecContents.Trim()
    }

    $promptLines = @(
        "Read AGENTS.md first.",
        "",
        "You are a bounded implementation worker in this Brevity task worktree.",
        "",
        "# Task",
        "",
        "Slug: $Slug",
        "",
        "# Task Spec",
        ""
    )

    if ([string]::IsNullOrWhiteSpace($normalizedSpec)) {
        $promptLines += @(
            "No vault task spec was materialized for this task.",
            "Use the task slug and repository instructions only. Do not invent unrelated scope."
        )
    }
    else {
        if (-not [string]::IsNullOrWhiteSpace($SpecPath)) {
            $promptLines += "Source: $SpecPath"
            $promptLines += ""
        }
        $promptLines += $normalizedSpec
    }

    $promptLines += @(
        "",
        "# Constraints",
        "",
        "- Keep changes small and focused on this task.",
        "- Stay inside this task worktree.",
        "- Do not merge branches.",
        "- Do not clean up or remove worktrees.",
        "- Do not add package managers, dependencies, generated projects, or web apps unless the task explicitly requires it.",
        "- Prefer straightforward PowerShell and existing repository patterns.",
        "",
        "# Acceptance Checks",
        "",
        "- The requested behavior is implemented.",
        "- Relevant local checks have been run, or any checks that could not be run are called out.",
        "- The final summary names changed files and verification performed.",
        "",
        "# Worker Behavior",
        "",
        "- Inspect only the context needed to complete the task.",
        "- Make the patch directly.",
        "- Preserve unrelated user or repository changes.",
        "- Stop after patch and concise summary."
    )

    Set-Content -LiteralPath $Path -Value $promptLines -Encoding ASCII
}

function Initialize-BrevityRepository {
    param(
        [string]$Root,
        [bool]$Repair = $false
    )

    $rootPath = Resolve-DevRoot $Root
    $repoRoot = Get-RepositoryRoot
    $projectName = Split-Path -Leaf $repoRoot
    $brevityRoot = Join-Path $repoRoot ".brevity"
    $tasksPath = Join-Path $brevityRoot "tasks.json"
    $providerHealthPath = Join-Path $brevityRoot "provider-health.json"
    $configPath = Join-Path $brevityRoot "config.json"
    $vaultPath = Join-Path $rootPath "vaults\AI-Vault\10-Projects\$projectName"
    $worktreesRoot = Join-Path $rootPath "worktrees\active"
    $agentsPath = Join-Path $repoRoot "AGENTS.md"

    $results = @()
    $results = Ensure-Directory -Path $brevityRoot -Results $results
    $results = Ensure-File -Path $tasksPath -Lines @("[]") -Results $results
    $results = Ensure-ProviderHealthFile -Path $providerHealthPath -Results $results

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
            defaultProvider = "gemini"
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
        "Project memory for Brevity-assisted work."
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
        Write-Host "Repaired Brevity project: $projectName"
    }
    else {
        Write-Host "Initialized Brevity project: $projectName"
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

    $brevityRoot = Join-Path $RepoRoot ".brevity"
    if (-not (Test-Path -LiteralPath $brevityRoot)) {
        New-Item -ItemType Directory -Path $brevityRoot | Out-Null
    }

    $tasksPath = Join-Path $brevityRoot "tasks.json"
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
    $tasksPath = Join-Path $repoRoot ".brevity\tasks.json"

    if (-not (Test-Path -LiteralPath $tasksPath)) {
        Write-Host "No Brevity tasks found."
        return
    }

    $rawTasks = Get-Content -LiteralPath $tasksPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawTasks)) {
        Write-Host "No Brevity tasks found."
        return
    }

    $parsedTasks = $rawTasks | ConvertFrom-Json
    if ($null -eq $parsedTasks) {
        Write-Host "No Brevity tasks found."
        return
    }

    $tasks = @($parsedTasks)
    if ($tasks.Count -eq 0) {
        Write-Host "No Brevity tasks found."
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
        Write-Host "Usage: .\brevity.ps1 task spec <slug>"
        exit 1
    }

    $config = Read-BrevityConfig
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

function Get-VaultTaskSpecPath {
    param([string]$Slug)

    $config = Read-BrevityConfig
    return (Join-Path (Join-Path $config.vaultPath "tasks") "$Slug.md")
}

function Update-TaskPromptFromSpec {
    param(
        [string]$PromptPath,
        [string]$Slug,
        [string]$SpecPath = ""
    )

    $resolvedSpecPath = $SpecPath
    if ([string]::IsNullOrWhiteSpace($resolvedSpecPath)) {
        $resolvedSpecPath = Get-VaultTaskSpecPath -Slug $Slug
    }
    elseif (-not (Test-Path -LiteralPath $resolvedSpecPath)) {
        $defaultSpecPath = Get-VaultTaskSpecPath -Slug $Slug
        if (Test-Path -LiteralPath $defaultSpecPath) {
            $resolvedSpecPath = $defaultSpecPath
        }
    }

    if (Test-Path -LiteralPath $resolvedSpecPath) {
        $specContents = Get-Content -LiteralPath $resolvedSpecPath -Raw
        Write-TaskPrompt -Path $PromptPath -Slug $Slug -SpecContents $specContents -SpecPath $resolvedSpecPath
        return $resolvedSpecPath
    }

    Write-TaskPrompt -Path $PromptPath -Slug $Slug
    return ""
}

function Start-TaskWork {
    param([string]$Slug)

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\brevity.ps1 task start <slug>"
        exit 1
    }

    $repoRoot = Get-RepositoryRoot
    $tasksPath = Join-Path $repoRoot ".brevity\tasks.json"

    if (-not (Test-Path -LiteralPath $tasksPath)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Brevity task metadata exists at: $tasksPath"
        exit 1
    }

    $rawTasks = Get-Content -LiteralPath $tasksPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawTasks)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Brevity task metadata exists at: $tasksPath"
        exit 1
    }

    $parsedTasks = $rawTasks | ConvertFrom-Json
    $tasks = @($parsedTasks)
    $task = $tasks | Where-Object { $_.slug -eq $Slug } | Select-Object -First 1

    if ($null -eq $task) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "Use .\brevity.ps1 task status to list known tasks."
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

    $specPath = ""
    if (Get-Member -InputObject $task -Name "specPath" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $specPath = [string]$task.specPath
    }

    $materializedSpecPath = Update-TaskPromptFromSpec -PromptPath $task.promptPath -Slug $Slug -SpecPath $specPath
    $workerCommand = "codex -C $($task.worktreePath) -a never -s workspace-write"

    Write-Host "Task: $($task.slug)"
    Write-Host "Worktree: $($task.worktreePath)"
    Write-Host "Prompt: $($task.promptPath)"
    if (-not [string]::IsNullOrWhiteSpace($materializedSpecPath)) {
        Write-Host "Spec: $materializedSpecPath"
    }
    Write-Host "Worker: $workerCommand"
    Write-Host "Read prompt.md and follow it exactly."
}

function Get-BrevityProfileMatrix {
    return [ordered]@{
        "gemini-lite" = @{
            provider = "gemini"
            costTier = "low"
            capabilityTier = "lite"
            complexityFit = @("low")
            intendedUse = "Low-latency worker for simple edits, documentation, and small tests."
            model = $null
            providerConfig = @{}
        }
        "gemini-flash" = @{
            provider = "gemini"
            costTier = "low"
            capabilityTier = "fast"
            complexityFit = @("low", "medium")
            intendedUse = "Default Gemini worker for everyday implementation and review tasks."
            model = "gemini-3-flash-preview"
            providerConfig = @{}
        }
        "gemini-pro" = @{
            provider = "gemini"
            costTier = "medium"
            capabilityTier = "pro"
            complexityFit = @("medium", "high")
            intendedUse = "Higher-capability Gemini worker for complex design and refactoring."
            model = $null
            providerConfig = @{}
        }
        "codex-fast" = @{
            provider = "codex"
            costTier = "low"
            capabilityTier = "fast"
            complexityFit = @("low")
            intendedUse = "Quick Codex worker for straightforward edits and focused fixes."
            model = $null
            providerConfig = @{}
        }
        "codex-balanced" = @{
            provider = "codex"
            costTier = "medium"
            capabilityTier = "balanced"
            complexityFit = @("medium")
            intendedUse = "Default Codex worker for everyday development tasks."
            model = $null
            providerConfig = @{}
        }
        "codex-deep" = @{
            provider = "codex"
            costTier = "high"
            capabilityTier = "deep"
            complexityFit = @("high")
            intendedUse = "Deep Codex worker for difficult bug fixes and refactors."
            model = $null
            providerConfig = @{}
        }
        "copilot" = @{
            provider = "copilot"
            costTier = "low"
            capabilityTier = "default"
            complexityFit = @("low", "medium", "high")
            intendedUse = "GitHub Copilot CLI worker for all complexity levels."
            model = $null
            providerConfig = @{}
        }
    }
}

function Get-BrevityComplexityProfileDefaults {
    return [ordered]@{
        low = @(
            "codex-fast",
            "gemini-lite",
            "gemini-flash"
        )
        medium = @(
            "codex-balanced",
            "gemini-flash",
            "gemini-pro"
        )
        high = @(
            "codex-deep",
            "gemini-pro"
        )
    }
}

function Get-BrevityProfileConfig {
    param([string]$Name)

    $profileMatrix = Get-BrevityProfileMatrix
    $normalizedName = $Name.ToLowerInvariant()

    if (-not $profileMatrix.Contains($normalizedName)) {
        throw "Unknown worker profile: $Name. Brevity v0 supports: gemini-lite, gemini-flash, gemini-pro, codex-fast, codex-balanced, codex-deep, copilot."
    }

    return $profileMatrix[$normalizedName]
}

function Get-WorkerConfig {
    param(
        [object]$Config,
        [string]$ProfileName
    )

    $provider = "codex"
    if (Get-Member -InputObject $Config -Name "defaultProvider" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $provider = $Config.defaultProvider
    }
    elseif (Get-Member -InputObject $Config -Name "codex" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $legacyConfig = $Config.codex
        if ($null -ne $legacyConfig -and (Get-Member -InputObject $legacyConfig -Name "provider" -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
            $provider = $legacyConfig.provider
        }
    }

    if ([string]::IsNullOrWhiteSpace($provider)) {
        $provider = "codex"
    }

    $normalizedProvider = ([string]$provider).ToLowerInvariant()

    # Apply profile overrides if requested
    $profileOverrides = $null
    if (-not [string]::IsNullOrWhiteSpace($ProfileName)) {
        $profileOverrides = Get-BrevityProfileConfig -Name $ProfileName
        $normalizedProvider = ([string]$profileOverrides.provider).ToLowerInvariant()
    }

    $providerDefaults = $null
    if ($normalizedProvider -eq "gemini") {
        $providerDefaults = Get-DefaultGeminiConfig
    }
    elseif ($normalizedProvider -eq "codex") {
        $providerDefaults = Get-DefaultCodexConfig
    }
    elseif ($normalizedProvider -eq "copilot") {
        $providerDefaults = Get-DefaultCopilotConfig
    }
    else {
        throw "Unsupported worker provider: $normalizedProvider. Brevity v0 supports providers 'codex', 'gemini', and 'copilot'."
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
    $env = New-Object PSObject

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
        $skipTrust = ConvertTo-BrevityBoolean -Value $providerDefaults.skipTrust
    }
    if (Get-Member -InputObject $providerDefaults -Name "env" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $env = $providerDefaults.env
    }

    foreach ($fieldName in @("command", "mode", "sandbox", "model", "profile", "executionPolicy", "approvalMode")) {
        if (Get-Member -InputObject $providerConfig -Name $fieldName -MemberType NoteProperty -ErrorAction SilentlyContinue) {
            Set-Variable -Name $fieldName -Value $providerConfig.$fieldName
        }
    }

    if (Get-Member -InputObject $providerConfig -Name "skipTrust" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $skipTrust = ConvertTo-BrevityBoolean -Value $providerConfig.skipTrust
    }
    if (Get-Member -InputObject $providerConfig -Name "env" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        if ($null -ne $providerConfig.env -and $providerConfig.env -is [System.Management.Automation.PSCustomObject]) {
            $env = $providerConfig.env
        }
    }

    # Profile providerConfig contains provider-native settings only.
    if ($null -ne $profileOverrides -and $profileOverrides.ContainsKey("providerConfig")) {
        $nativeOverrides = $profileOverrides.providerConfig
        if ($null -ne $nativeOverrides) {
            foreach ($fieldName in @("command", "mode", "sandbox", "model", "profile", "executionPolicy", "approvalMode")) {
                if ($nativeOverrides.ContainsKey($fieldName)) {
                    Set-Variable -Name $fieldName -Value $nativeOverrides[$fieldName]
                }
            }

            if ($nativeOverrides.ContainsKey("skipTrust")) {
                $skipTrust = ConvertTo-BrevityBoolean -Value $nativeOverrides["skipTrust"]
            }
            if ($nativeOverrides.ContainsKey("env")) {
                $env = $nativeOverrides["env"]
            }
        }
    }

    # Profile model is provider-native execution metadata; profile names are not passed as Codex -p.
    if ($null -ne $profileOverrides -and $profileOverrides.ContainsKey("model") -and $null -ne $profileOverrides.model) {
        $model = $profileOverrides.model
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
        env = $env
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

function ConvertTo-WorkerEnvironmentMap {
    param([object]$EnvironmentConfig)

    $environment = [ordered]@{}
    if ($null -eq $EnvironmentConfig -or $EnvironmentConfig -isnot [System.Management.Automation.PSCustomObject]) {
        return $environment
    }

    foreach ($property in @($EnvironmentConfig.PSObject.Properties)) {
        if ([string]::IsNullOrWhiteSpace($property.Name) -or $property.Name -match '=') {
            throw "Invalid worker environment variable name: $($property.Name)"
        }

        if ($null -eq $property.Value) {
            continue
        }

        $sourceName = [string]$property.Value
        if ([string]::IsNullOrWhiteSpace($sourceName) -or $sourceName -match '=') {
            throw "Invalid source environment variable name for $($property.Name): $sourceName"
        }

        $sourceValue = [Environment]::GetEnvironmentVariable($sourceName, "Process")
        if ([string]::IsNullOrWhiteSpace($sourceValue)) {
            $errorMessage = "Worker environment variable '$($property.Name)' maps to '$sourceName', but '$sourceName' is not set in the current process environment."
            if ([string]::Equals($sourceName, "GEMINI_API_KEY", [System.StringComparison]::OrdinalIgnoreCase)) {
                $errorMessage += "`nPlease set it using the command: `n`$env:GEMINI_API_KEY='your-key-here'`nOr configure it in Brevity's credential manager."
            }
            throw $errorMessage
        }

        $environment[$property.Name] = $sourceValue
    }

    return $environment
}

function Format-EnvironmentDisplay {
    param([System.Collections.IDictionary]$Environment)

    if ($null -eq $Environment -or $Environment.Count -eq 0) {
        return ""
    }

    $assignments = @()
    foreach ($name in @($Environment.Keys)) {
        $assignments += "`$env:$name='<configured>'"
    }

    return ($assignments -join "; ") + "; "
}

function New-WorkerCommand {
    param(
        [object]$Config,
        [string]$WorktreePath,
        [string]$ProfileName
    )

    $workerConfig = Get-WorkerConfig -Config $Config -ProfileName $ProfileName

    if ([string]::Equals([string]$workerConfig.provider, "gemini", [System.StringComparison]::OrdinalIgnoreCase)) {
        $arguments = @()
        $displayArguments = @()
        $environment = ConvertTo-WorkerEnvironmentMap -EnvironmentConfig $workerConfig.env

        if (-not [string]::IsNullOrWhiteSpace($workerConfig.sandbox) -and -not [string]::Equals([string]$workerConfig.sandbox, "none", [System.StringComparison]::OrdinalIgnoreCase)) {
            $arguments += "-s"
            $displayArguments += "-s"
        }

        if (-not [string]::IsNullOrWhiteSpace($workerConfig.model)) {
            $arguments += "-m"
            $arguments += [string]$workerConfig.model
            $displayArguments += "-m"
            $displayArguments += [string]$workerConfig.model
        }

        $geminiApprovalMode = $workerConfig.approvalMode
        if ([string]::IsNullOrWhiteSpace($geminiApprovalMode) -and $workerConfig.skipTrust) {
            $geminiApprovalMode = "yolo"
        }

        if (-not [string]::IsNullOrWhiteSpace($geminiApprovalMode)) {
            $arguments += "--approval-mode"
            $arguments += [string]$geminiApprovalMode
            $displayArguments += "--approval-mode"
            $displayArguments += [string]$geminiApprovalMode
        }
        
        if ($workerConfig.skipTrust) {
            $arguments += "--skip-trust"
            $displayArguments += "--skip-trust"
        }

        $arguments += "-p"
        $arguments += Get-TaskPromptText -WorktreePath $WorktreePath
        $displayArguments += "-p"
        $displayCommand = Format-CommandLine -Parts (@([string]$workerConfig.command) + $displayArguments)
        $environmentDisplay = Format-EnvironmentDisplay -Environment $environment
        $display = "Set-Location -LiteralPath $(Format-PowerShellLiteral -Value $WorktreePath); $environmentDisplay$displayCommand (Get-Content -LiteralPath 'prompt.md' -Raw)"

        return (New-Object PSObject -Property ([ordered]@{
            provider = [string]$workerConfig.provider
            command = [string]$workerConfig.command
            arguments = $arguments
            executionPolicy = [string]$workerConfig.executionPolicy
            workingDirectory = $WorktreePath
            environment = $environment
            display = $display
        }))
    }

    if ([string]::Equals([string]$workerConfig.provider, "copilot", [System.StringComparison]::OrdinalIgnoreCase)) {
        $arguments = @(
            "-C",
            $WorktreePath,
            "--allow-all-tools",
            "--allow-all-paths",
            "--no-ask-user"
        )

        $displayArguments = @(
            "-C",
            $WorktreePath,
            "--allow-all-tools",
            "--allow-all-paths",
            "--no-ask-user"
        )

        $displayCommand = Format-CommandLine -Parts (@([string]$workerConfig.command) + $displayArguments)
        $promptPath = Join-Path $WorktreePath "prompt.md"
        $display = "Get-Content -Raw $(Format-PowerShellLiteral -Value $promptPath) | $displayCommand"

        return (New-Object PSObject -Property ([ordered]@{
            provider = [string]$workerConfig.provider
            command = [string]$workerConfig.command
            arguments = $arguments
            promptPath = $promptPath
            executionPolicy = ""
            workingDirectory = $WorktreePath
            environment = [ordered]@{}
            display = $display
            useStdin = $true
        }))
    }

    $arguments = @(
        [string]$workerConfig.mode,
        "-C",
        $WorktreePath,
        "-s",
        [string]$workerConfig.sandbox
    )

    if (-not [string]::IsNullOrWhiteSpace($workerConfig.model)) {
        $arguments += "-m"
        $arguments += [string]$workerConfig.model
    }

    if (-not [string]::IsNullOrWhiteSpace($workerConfig.profile)) {
        $arguments += "-p"
        $arguments += [string]$workerConfig.profile
    }

    $arguments += "prompt.md"
    $parts = @([string]$workerConfig.command) + $arguments

    return (New-Object PSObject -Property ([ordered]@{
        provider = [string]$workerConfig.provider
        command = [string]$workerConfig.command
        arguments = $arguments
        executionPolicy = [string]$workerConfig.executionPolicy
        workingDirectory = $WorktreePath
        environment = [ordered]@{}
        display = Format-CommandLine -Parts $parts
    }))
}

function Show-TaskRun {
    param(
        [string]$Slug,
        [bool]$Execute = $false,
        [string]$ProfileName
    )

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\brevity.ps1 task run <slug> [--execute] [--profile <name>]"
        exit 1
    }

    $repoRoot = Get-RepositoryRoot
    $config = Read-BrevityConfig
    $tasksPath = Join-Path $repoRoot ".brevity\tasks.json"

    if (-not (Test-Path -LiteralPath $tasksPath)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Brevity task metadata exists at: $tasksPath"
        exit 1
    }

    $rawTasks = Get-Content -LiteralPath $tasksPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawTasks)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Brevity task metadata exists at: $tasksPath"
        exit 1
    }

    $parsedTasks = $rawTasks | ConvertFrom-Json
    $tasks = @($parsedTasks)
    $task = $tasks | Where-Object { $_.slug -eq $Slug } | Select-Object -First 1

    if ($null -eq $task) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "Use .\brevity.ps1 task status to list known tasks."
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
        $workerCommand = New-WorkerCommand -Config $config -WorktreePath $task.worktreePath -ProfileName $ProfileName
    }
    catch {
        Write-Host $_.Exception.Message -ForegroundColor Red
        exit 1
    }

    Write-Host "Task: $($task.slug)"
    Write-Host "Worktree: $($task.worktreePath)"
    Write-Host "Prompt: $($task.promptPath)"
    Write-Host "Provider: $($workerCommand.provider)"
    Write-Host "Worker: $($workerCommand.display)"
    if (-not [string]::IsNullOrWhiteSpace($workerCommand.executionPolicy)) {
        Write-Host "ExecutionPolicy (worker process): $($workerCommand.executionPolicy)"
    }

    if (-not $Execute) {
        Write-Host "Dry run. Pass --execute to run the worker non-interactively."
        return
    }

    Write-Host "Executing $($workerCommand.provider) worker..."
    $previousExecutionPolicyPreference = $env:PSExecutionPolicyPreference
    $previousEnvironment = [ordered]@{}
    if (-not [string]::IsNullOrWhiteSpace($workerCommand.executionPolicy)) {
        $env:PSExecutionPolicyPreference = $workerCommand.executionPolicy
    }

    try {
        foreach ($name in @($workerCommand.environment.Keys)) {
            $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
            [Environment]::SetEnvironmentVariable($name, $workerCommand.environment[$name], "Process")
        }

        Push-Location -LiteralPath $workerCommand.workingDirectory
        try {
            if (Get-Member -InputObject $workerCommand -Name "useStdin" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
                if ($workerCommand.useStdin) {
                    $promptContent = Get-Content -LiteralPath $workerCommand.promptPath -Raw
                    $argsForWorker = [string[]]@($workerCommand.arguments)
                    $workerOutput = @($promptContent | & $workerCommand.command @argsForWorker 2>&1)
                    $exitCode = $LASTEXITCODE
                }
                else {
                    $argsForWorker = [string[]]@($workerCommand.arguments)
                    $workerOutput = @(& $workerCommand.command @argsForWorker 2>&1)
                    $exitCode = $LASTEXITCODE
                }
            }
            else {
                $argsForWorker = [string[]]@($workerCommand.arguments)
                $workerOutput = @(& $workerCommand.command @argsForWorker 2>&1)
                $exitCode = $LASTEXITCODE
            }

            foreach ($line in $workerOutput) {
                Write-Host $line
            }

            if ($exitCode -ne 0) {
                $renderedOutput = ($workerOutput | ForEach-Object { [string]$_ }) -join [Environment]::NewLine

                Write-Host ""
                Write-Host "Worker failed with exit code $exitCode." -ForegroundColor Yellow

                if ($renderedOutput -match "\[object Object\]") {
                    Write-Host "Worker returned an object-shaped error rendered as [object Object]." -ForegroundColor Yellow
                    Write-Host "The provider CLI likely lost structured error details before Brevity could display them." -ForegroundColor Gray
                    Write-Host "Provider: $($workerCommand.provider)" -ForegroundColor Gray
                    Write-Host "Command: $($workerCommand.command)" -ForegroundColor Gray
                }
                Write-Host "If the output contains 'QUOTA_EXHAUSTED', 'MODEL_CAPACITY_EXHAUSTED', or 'exhausted your capacity'," -ForegroundColor Gray
                Write-Host "this is an infrastructure failure, not a task failure. Consider retrying later or switching" -ForegroundColor Gray
                Write-Host "to a different worker profile." -ForegroundColor Gray
                exit $exitCode
            }
        }
        finally {
            Pop-Location
        }
    }
    finally {
        foreach ($name in @($previousEnvironment.Keys)) {
            if ($null -eq $previousEnvironment[$name]) {
                [Environment]::SetEnvironmentVariable($name, $null, "Process")
            }
            else {
                [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], "Process")
            }
        }

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
        Write-Host "Usage: .\brevity.ps1 task cleanup <slug> [--force]"
        exit 1
    }

    $repoRoot = Get-RepositoryRoot
    $tasksPath = Join-Path $repoRoot ".brevity\tasks.json"

    $tasks = @()
    if (Test-Path -LiteralPath $tasksPath) {
        $rawTasks = Get-Content -LiteralPath $tasksPath -Raw
        if (-not [string]::IsNullOrWhiteSpace($rawTasks)) {
            $parsedTasks = $rawTasks | ConvertFrom-Json
            $tasks = @($parsedTasks)
        }
    }

    $task = $tasks | Where-Object { $_.slug -eq $Slug } | Select-Object -First 1
    $taskFoundInMetadata = $null -ne $task

    if (-not $taskFoundInMetadata) {
        if ($Force) {
            Write-Host "Warning: metadata not found for task '$Slug'. Attempting orphaned cleanup." -ForegroundColor Yellow
            $inferredBranch = "task/$Slug"
            $inferredWorktreePath = $null

            # Try to find the worktree from Git list
            $worktreeList = (& git worktree list --porcelain 2>$null)
            foreach ($line in $worktreeList) {
                if ($line.StartsWith("worktree ")) {
                    $path = $line.Substring("worktree ".Length)
                    if ($path.EndsWith("-$Slug", [System.StringComparison]::OrdinalIgnoreCase)) {
                        $inferredWorktreePath = $path
                        break
                    }
                }
            }

            # Fallback to default path calculation
            if ([string]::IsNullOrWhiteSpace($inferredWorktreePath)) {
                $config = Read-BrevityConfig
                $inferredWorktreePath = Join-Path $config.worktreesRoot "$($config.projectName)-$Slug"
            }

            $task = [PSCustomObject]@{
                slug         = $Slug
                branch       = $inferredBranch
                worktreePath = $inferredWorktreePath
            }
        }
        else {
            Write-Host "Task not found: $Slug" -ForegroundColor Red
            if (-not (Test-Path -LiteralPath $tasksPath)) {
                Write-Host "No Brevity task metadata exists at: $tasksPath"
            }
            else {
                Write-Host "Use .\brevity.ps1 task status to list known tasks."
            }
            exit 1
        }
    }

    if ([string]::IsNullOrWhiteSpace($task.worktreePath)) {
        Write-Host "Task metadata is missing worktreePath for: $Slug" -ForegroundColor Red
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($task.branch)) {
        Write-Host "Task metadata is missing branch for: $Slug" -ForegroundColor Red
        exit 1
    }

    Write-Host "Cleaning up Brevity task: $Slug"
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
        if ($taskFoundInMetadata) {
            Remove-TaskMetadataRecord -TasksPath $tasksPath -Tasks $tasks -Slug $Slug
        }
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

    if ($taskFoundInMetadata) {
        Remove-TaskMetadataRecord -TasksPath $tasksPath -Tasks $tasks -Slug $Slug
    }
}

function Merge-TaskBranch {
    param([string]$Slug)

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\brevity.ps1 task merge <slug>"
        exit 1
    }

    $repoRoot = Get-RepositoryRoot
    $tasksPath = Join-Path $repoRoot ".brevity\tasks.json"

    if (-not (Test-Path -LiteralPath $tasksPath)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Brevity task metadata exists at: $tasksPath"
        exit 1
    }

    $rawTasks = Get-Content -LiteralPath $tasksPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawTasks)) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "No Brevity task metadata exists at: $tasksPath"
        exit 1
    }

    $parsedTasks = $rawTasks | ConvertFrom-Json
    $tasks = @($parsedTasks)
    $task = $tasks | Where-Object { $_.slug -eq $Slug } | Select-Object -First 1

    if ($null -eq $task) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "Use .\brevity.ps1 task status to list known tasks."
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($task.branch)) {
        Write-Host "Task metadata is missing branch for: $Slug" -ForegroundColor Red
        exit 1
    }

    Write-Host "Merging Brevity task: $Slug"
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
        Write-Host "Usage: .\brevity.ps1 task new <slug> [-DevRoot <path>]"
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

    $specPath = Update-TaskPromptFromSpec -PromptPath $promptPath -Slug $Slug
    Add-TaskMetadata -RepoRoot $repoRoot -Slug $Slug -Branch $branchName -WorktreePath $targetPath -PromptPath $promptPath -SpecPath $specPath

    Write-Host "Created task worktree"
    Write-Host "Path: $targetPath"
    Write-Host "Branch: $branchName"
    Write-Host "Prompt: $promptPath"
    Write-Host "Metadata: $(Join-Path $repoRoot ".brevity\tasks.json")"
}

function Activate-TaskWorktree {
    param([string]$Slug)

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\brevity.ps1 task activate <slug>"
        exit 1
    }

    $repoRoot = Get-RepositoryRoot
    $config = Read-BrevityConfig
    $specPath = Get-VaultTaskSpecPath -Slug $Slug

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

    Update-TaskPromptFromSpec -PromptPath $promptPath -Slug $Slug -SpecPath $specPath | Out-Null
    Add-TaskMetadata -RepoRoot $repoRoot -Slug $Slug -Branch $branchName -WorktreePath $worktreePath -PromptPath $promptPath -SpecPath $specPath

    Write-Host "Activated task worktree"
    Write-Host "Slug: $Slug"
    Write-Host "Path: $worktreePath"
    Write-Host "Branch: $branchName"
    Write-Host "Prompt: $promptPath"
    Write-Host "Spec: $specPath"
    Write-Host "Metadata: $(Join-Path $repoRoot ".brevity\tasks.json")"
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
                Write-Host "Unknown brevity init argument: $Subcommand" -ForegroundColor Red
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
                    Write-Host "Unknown brevity init argument: $initArg" -ForegroundColor Red
                    Show-Help
                    exit 1
                }
            }
        }

        Initialize-BrevityRepository -Root $DevRoot -Repair $repair
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
            Write-Host "Unknown brevity plan command: $Subcommand" -ForegroundColor Red
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
    "provider" {
        if ([string]::IsNullOrWhiteSpace($Subcommand)) {
            Write-Host "Missing brevity provider command." -ForegroundColor Red
            Write-Host "Usage: .\brevity.ps1 provider status"
            Write-Host "Usage: .\brevity.ps1 provider set <provider> <status> [-Note <note>]"
            exit 1
        }

        switch ($Subcommand.ToLowerInvariant()) {
            "status" { Show-ProviderStatus }
            "set" {
                $providerName = $null
                $providerStatus = $null
                $providerNote = ""

                if ($null -ne $RemainingArgs) {
                    $skipNext = $false
                    for ($i = 0; $i -lt $RemainingArgs.Length; $i++) {
                        if ($skipNext) {
                            $skipNext = $false
                            continue
                        }

                        $arg = $RemainingArgs[$i]
                        if ([string]::Equals($arg, "-Note", [System.StringComparison]::OrdinalIgnoreCase)) {
                            if ($i + 1 -lt $RemainingArgs.Length) {
                                $providerNote = [string]$RemainingArgs[$i + 1]
                                $skipNext = $true
                            }
                            else {
                                Write-Host "Missing value for -Note." -ForegroundColor Red
                                exit 1
                            }
                        }
                        elseif ([string]::IsNullOrWhiteSpace($providerName)) {
                            $providerName = [string]$arg
                        }
                        elseif ([string]::IsNullOrWhiteSpace($providerStatus)) {
                            $providerStatus = [string]$arg
                        }
                        else {
                            Write-Host "Unknown argument for brevity provider set: $arg" -ForegroundColor Red
                            Write-Host "Usage: .\brevity.ps1 provider set <provider> <status> [-Note <note>]"
                            exit 1
                        }
                    }
                }

                Set-ProviderStatus -ProviderName $providerName -Status $providerStatus -Note $providerNote
            }
            default {
                Write-Host "Unknown brevity provider command: $Subcommand" -ForegroundColor Red
                Show-Help
                exit 1
            }
        }
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
                $profileName = $null
                if ($null -ne $RemainingArgs) {
                    $skipNext = $false
                    for ($i = 0; $i -lt $RemainingArgs.Length; $i++) {
                        if ($skipNext) {
                            $skipNext = $false
                            continue
                        }

                        $arg = $RemainingArgs[$i]
                        if ($arg -eq "--execute") {
                            $executeTask = $true
                        }
                        elseif ($arg -eq "--profile") {
                            if ($i + 1 -lt $RemainingArgs.Length) {
                                $profileName = [string]$RemainingArgs[$i + 1]
                                $skipNext = $true
                            }
                            else {
                                Write-Host "Missing value for --profile." -ForegroundColor Red
                                exit 1
                            }
                        }
                        elseif ([string]::IsNullOrWhiteSpace($taskSlug)) {
                            $taskSlug = [string]$arg
                        }
                        else {
                            Write-Host "Unknown argument for brevity task run: $arg" -ForegroundColor Red
                            Write-Host "Usage: .\brevity.ps1 task run <slug> [--execute] [--profile <name>]"
                            exit 1
                        }
                    }
                }

                Show-TaskRun -Slug $taskSlug -Execute $executeTask -ProfileName $profileName
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
                            Write-Host "Unknown argument for brevity task cleanup: $taskArg" -ForegroundColor Red
                            Write-Host "Usage: .\brevity.ps1 task cleanup <slug> [--force]"
                            exit 1
                        }
                    }
                }

                Remove-TaskWorktree -Slug $taskSlug -Force $forceCleanup
            }
            default {
                Write-Host "Unknown brevity task command: $Subcommand" -ForegroundColor Red
                Show-Help
                exit 1
            }
        }
    }
    default {
        Write-Host "Unknown brevity command: $Command" -ForegroundColor Red
        Show-Help
        exit 1
    }
}
