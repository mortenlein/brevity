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

function Get-PreferredHealthyProfile {
    param(
        [object]$Health,
        [string]$CurrentProvider
    )

    $preferredProfiles = @(
        @{ provider = "gemini"; profile = "gemini-pro" },
        @{ provider = "codex"; profile = "codex-balanced" },
        @{ provider = "copilot"; profile = "copilot" }
    )

    foreach ($candidate in $preferredProfiles) {
        if ($candidate.provider -eq $CurrentProvider) {
            continue
        }

        if (-not (Get-Member -InputObject $Health -Name $candidate.provider -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
            continue
        }

        $status = [string]$Health.$($candidate.provider).status
        if ([string]::IsNullOrWhiteSpace($status) -or $status -eq "healthy" -or $status -eq "unknown") {
            return $candidate.profile
        }
    }

    return $null
}
function Get-ProviderHealthSummary {
    param([object]$Health)

    $totalProviders = 0
    $degradedProviders = 0
    $unavailableProviders = 0

    foreach ($property in $Health.PSObject.Properties) {
        $totalProviders++

        $status = [string]$property.Value.status

        if ($status -eq "quota-constrained" -or
            $status -eq "capacity-degraded") {
            $degradedProviders++
        }

        if ($status -eq "unavailable") {
            $unavailableProviders++
        }
    }

    return [pscustomobject]@{
        total = $totalProviders
        degraded = $degradedProviders
        unavailable = $unavailableProviders
    }
}

function Show-ProviderStatus {
    $providerHealth = Read-ProviderHealth
    $health = $providerHealth.health

    Write-Host "Provider health"
    Write-Host "Path: $($providerHealth.path)"
    Write-Host ""

    $summary = Get-ProviderHealthSummary -Health $health
    Write-Host "Providers: $($summary.total) total, $($summary.degraded) degraded, $($summary.unavailable) unavailable"
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


function Show-ProviderDocs {
    Write-Host "Provider health lifecycle"
    Write-Host ""
    Write-Host "Brevity treats provider availability as runtime infrastructure state."
    Write-Host "Provider failures should not corrupt task state, worktrees, merge state, or cleanup semantics."
    Write-Host ""
    Write-Host "Statuses:"
    Write-Host "  healthy            Provider is available for normal worker execution."
    Write-Host "  capacity-degraded  Provider works, but may be slow, overloaded, or intermittently unavailable."
    Write-Host "  quota-constrained  Provider is blocked or risky because of quota, credits, or usage limits."
    Write-Host "  unavailable        Provider should not be selected automatically."
    Write-Host "  unknown            No recent health signal has been recorded."
    Write-Host ""
    Write-Host "Gating behavior:"
    Write-Host "  healthy and unknown providers are allowed by default."
    Write-Host "  degraded providers may warn before execution."
    Write-Host "  quota-constrained and unavailable providers should be gated unless explicitly overridden."
    Write-Host ""
    Write-Host "Override behavior:"
    Write-Host "  Use --force-provider when you intentionally want to run a task despite provider gating."
    Write-Host "  Forced runs should preserve task metadata and execution logs like normal runs."
    Write-Host ""
    Write-Host "Auto-recovery:"
    Write-Host "  Worker output may update provider health when infrastructure failures are detected."
    Write-Host "  Capacity and quota failures are provider/runtime failures, not implementation failures."
    Write-Host ""
    Write-Host "Operational philosophy:"
    Write-Host "  The human remains merge authority."
    Write-Host "  Provider health is advisory state for orchestration decisions."
    Write-Host "  Brevity should fail early with actionable messages instead of damaging task state."
    Write-Host ""
    Write-Host "Related commands:"
    Write-Host "  .\brevity.ps1 provider status"
    Write-Host "  .\brevity.ps1 provider set <provider> <status> [-Note <note>]"
    Write-Host "  .\brevity.ps1 provider reset <provider>"
}


function Get-ProviderProfileSummary {
    param(
        [string]$ProfileName = ""
    )

    $config = Read-BrevityConfig
    $profileMatrix = Get-BrevityProfileMatrix
    $profileAliases = Get-BrevityProfileAliases
    $complexityDefaults = Get-BrevityComplexityProfileDefaults
    $profiles = @()
    $resolvedProfileName = $ProfileName
    $matchedAlias = ""

    if (-not [string]::IsNullOrWhiteSpace($ProfileName)) {
        $resolvedProfile = Resolve-BrevityProfileName -Name $ProfileName
        $resolvedProfileName = $resolvedProfile.CanonicalName
        $matchedAlias = $resolvedProfile.AliasName
    }

    foreach ($matrixProfileName in $profileMatrix.Keys) {
        $profile = $profileMatrix[$matrixProfileName]
        $workerConfig = $null
        if ($null -ne $config) {
            $workerConfig = Get-WorkerConfig -Config $config -ProfileName $matrixProfileName
        }

        $providerName = [string]$profile.provider
        $modelName = $null
        if ($null -ne $workerConfig) {
            $providerName = [string]$workerConfig.provider
            if (-not [string]::IsNullOrWhiteSpace([string]$workerConfig.model)) {
                $modelName = [string]$workerConfig.model
            }
        }

        if ([string]::IsNullOrWhiteSpace($modelName) -and $profile.ContainsKey("model") -and $null -ne $profile.model) {
            $modelName = [string]$profile.model
        }
        if ([string]::IsNullOrWhiteSpace($modelName)) {
            $modelName = "(provider default)"
        }

        $fallbackCandidates = @()
        foreach ($complexity in @($profile.complexityFit)) {
            if ($complexityDefaults.Contains($complexity)) {
                $orderedProfiles = @($complexityDefaults[$complexity])
                $profileIndex = [array]::IndexOf($orderedProfiles, $matrixProfileName)
                if ($profileIndex -ge 0 -and $profileIndex + 1 -lt $orderedProfiles.Count) {
                    $fallbackCandidates += @($orderedProfiles[($profileIndex + 1)..($orderedProfiles.Count - 1)])
                }
            }
        }

        $executionStyleHints = @()
        if ($profile.capabilityTier -in @("fast", "balanced", "deep")) {
            $executionStyleHints += $profile.capabilityTier
        }
        if ($profile.capabilityTier -eq "lite" -or $profile.costTier -eq "low") {
            $executionStyleHints += "lightweight"
        }
        if (@($profile.complexityFit) -contains "low") {
            $executionStyleHints += "smoke"
        }
        if ([string]$profile.intendedUse -match "review") {
            $executionStyleHints += "review"
        }

        $profiles += [PSCustomObject]@{
            name = $matrixProfileName
            canonicalName = $matrixProfileName
            aliases = @($profileAliases.Keys | Where-Object { $profileAliases[$_] -eq $matrixProfileName })
            matchedAlias = $matchedAlias
            valid = $true
            provider = $providerName
            model = $modelName
            capabilityIntent = $profile.capabilityTier
            category = (@($profile.complexityFit) -join ", ")
            intendedUse = $profile.intendedUse
            executionStyleHints = @($executionStyleHints | Select-Object -Unique)
            fallbackCandidates = @($fallbackCandidates | Select-Object -Unique)
        }
    }

    if (-not [string]::IsNullOrWhiteSpace($ProfileName)) {
        $profiles = @($profiles | Where-Object { $_.name -eq $resolvedProfileName })
    }

    return @($profiles)
}

function Show-ProviderProfiles {
    param(
        [string]$ProfileName = "",
        [bool]$Json = $false
    )

    try {
        $profiles = @(Get-ProviderProfileSummary -ProfileName $ProfileName)
    }
    catch {
        if ($Json) {
            [PSCustomObject]@{
                valid = $false
                error = $_.Exception.Message
            } | ConvertTo-Json -Depth 10
            return
        }

        Write-Host $_.Exception.Message -ForegroundColor Red
        return
    }

    if ($Json) {
        $profiles | ConvertTo-Json -Depth 10
        return
    }

    if ($profiles.Count -eq 0) {
        Write-Host "No provider profile matched: $ProfileName" -ForegroundColor Yellow
        Write-Host "Valid profiles: $((Get-ProviderProfileSummary | ForEach-Object { $_.name }) -join ', ')"
        return
    }

    Write-Host "Provider profiles"
    Write-Host "Valid profiles: $((Get-ProviderProfileSummary | ForEach-Object { $_.name }) -join ', ')"
    Write-Host ""
    if (-not [string]::IsNullOrWhiteSpace($ProfileName) -and $profiles.Count -gt 0 -and $profiles[0].matchedAlias) {
        Write-Host "Resolved profile alias: $($profiles[0].matchedAlias) -> $($profiles[0].canonicalName)"
        Write-Host ""
    }
    foreach ($profile in $profiles) {
        Write-Host "name: $($profile.name)"
        Write-Host "canonicalName: $($profile.canonicalName)"
        Write-Host "aliases: $(@($profile.aliases) -join ', ')"
        Write-Host "valid: $($profile.valid)"
        Write-Host "provider: $($profile.provider)"
        Write-Host "model: $($profile.model)"
        Write-Host "capabilityIntent: $($profile.capabilityIntent)"
        Write-Host "category: $($profile.category)"
        Write-Host "executionStyleHints: $(@($profile.executionStyleHints) -join ', ')"
        Write-Host "fallbackCandidates: $(@($profile.fallbackCandidates) -join ', ')"
        Write-Host "intendedUse: $($profile.intendedUse)"
        Write-Host ""
    }
}

function Reset-ProviderStatus {
    param(
        [string]$ProviderName
    )

    Set-ProviderStatus `
        -ProviderName $ProviderName `
        -Status "unknown" `
        -Note "Provider state reset."
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
    Write-VaultRuntimeEvent -Type "provider" -Subject $normalizedProviderName -Message "Provider health set to $normalizedStatus."
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
    Write-Host "  .\brevity.ps1 plan workers"
    Write-Host "  .\brevity.ps1 plan apply <file>"
    Write-Host "  .\brevity.ps1 board"
    Write-Host "  .\brevity.ps1 doctor [--repair]"
    Write-Host "  .\brevity.ps1 doctor execution-policy"
    Write-Host "  .\brevity.ps1 memory note <message>"
    Write-Host "  .\brevity.ps1 logs recent [--count <n>]"
    Write-Host "  .\brevity.ps1 logs task <slug> [--tail <n>]"
    Write-Host "  .\brevity.ps1 session summary [--json]"
    Write-Host "  .\brevity.ps1 status [-DevRoot <path>]"
    Write-Host "  .\brevity.ps1 provider status"
    Write-Host "  .\brevity.ps1 provider docs"
    Write-Host "  .\brevity.ps1 provider reset <provider>"
    Write-Host "  .\brevity.ps1 provider set <provider> <status> [-Note <note>]"
    Write-Host "  .\brevity.ps1 task new <slug> [-DevRoot <path>]"
    Write-Host "  .\brevity.ps1 task activate <slug>"
    Write-Host "  .\brevity.ps1 task spec <slug>"
    Write-Host "  .\brevity.ps1 task start <slug>"
    Write-Host "  .\brevity.ps1 task run <slug> [--execute] [--profile <name>] [--smoke] [--force-provider]"
    Write-Host "  .\brevity.ps1 task context refresh <slug>"
    Write-Host "  .\brevity.ps1 task context status <slug>"
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

function Read-TaskMetadataFile {
    param([string]$TasksPath)

    if (-not (Test-Path -LiteralPath $TasksPath)) {
        return @()
    }

    $rawTasks = Get-Content -LiteralPath $TasksPath -Raw
    if ([string]::IsNullOrWhiteSpace($rawTasks)) {
        return @()
    }

    $parsedTasks = $rawTasks | ConvertFrom-Json
    if ($null -eq $parsedTasks) {
        return @()
    }

    return @($parsedTasks)
}

function Write-TaskMetadataFile {
    param(
        [string]$TasksPath,
        [object[]]$Tasks
    )

    if ($Tasks.Count -eq 0) {
        Set-Content -LiteralPath $TasksPath -Value "[]" -Encoding ASCII
    }
    else {
        ConvertTo-Json -InputObject $Tasks -Depth 4 | Set-Content -LiteralPath $TasksPath -Encoding ASCII
    }
}

function Invoke-TaskMetadataLock {
    param(
        [string]$TasksPath,
        [scriptblock]$ScriptBlock
    )

    $brevityRoot = Split-Path -Parent $TasksPath
    if (-not (Test-Path -LiteralPath $brevityRoot)) {
        New-Item -ItemType Directory -Path $brevityRoot | Out-Null
    }

    $lockPath = Join-Path $brevityRoot "tasks.lock"
    $lockStream = $null

    for ($attempt = 0; $attempt -lt 50; $attempt++) {
        try {
            $lockStream = [System.IO.File]::Open($lockPath, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
            $lockBytes = [System.Text.Encoding]::ASCII.GetBytes("pid=$PID utc=$((Get-Date).ToUniversalTime().ToString("o"))")
            $lockStream.Write($lockBytes, 0, $lockBytes.Length)
            $lockStream.Flush()
            break
        }
        catch [System.IO.IOException] {
            Start-Sleep -Milliseconds 100
        }
    }

    if ($null -eq $lockStream) {
        Write-Host "Unable to acquire task metadata lock: $lockPath" -ForegroundColor Red
        Write-Host "Another Brevity process may be updating .brevity\tasks.json. Retry shortly, or inspect/remove the lock file if no Brevity process is running." -ForegroundColor Yellow
        exit 1
    }

    try {
        & $ScriptBlock
    }
    finally {
        $lockStream.Dispose()
        Remove-Item -LiteralPath $lockPath -Force -ErrorAction SilentlyContinue
    }
}

function Find-BrevityTaskBySlug {
    param([string]$Slug)

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        return $null
    }

    $tasks = @(Read-BrevityTasks)
    return ($tasks | Where-Object { $_.slug -eq $Slug } | Select-Object -First 1)
}

function Write-VaultRuntimeEvent {
    param(
        [string]$Type,
        [string]$Message,
        [string]$Subject = ""
    )

    if ([string]::IsNullOrWhiteSpace($Type) -or [string]::IsNullOrWhiteSpace($Message)) {
        return
    }

    $config = Read-BrevityConfig
    if ([string]::IsNullOrWhiteSpace($config.vaultPath)) {
        return
    }

    if (-not (Test-Path -LiteralPath $config.vaultPath)) {
        New-Item -ItemType Directory -Path $config.vaultPath -Force | Out-Null
    }

    $logPath = Join-Path $config.vaultPath "runtime-log.md"
    if (-not (Test-Path -LiteralPath $logPath)) {
        Set-Content -LiteralPath $logPath -Value @("# Runtime Log", "", "Concise Brevity orchestration memory.") -Encoding ASCII
    }

    $timestamp = (Get-Date).ToUniversalTime().ToString("o")
    $cleanMessage = ([string]$Message).Replace("`r", " ").Replace("`n", " ").Trim()
    $cleanSubject = ([string]$Subject).Replace("`r", " ").Replace("`n", " ").Trim()
    $subjectText = ""
    if (-not [string]::IsNullOrWhiteSpace($cleanSubject)) {
        $subjectText = " subject=$cleanSubject"
    }

    Add-Content -LiteralPath $logPath -Value "- $timestamp [$Type]$subjectText $cleanMessage" -Encoding ASCII
}

function Add-MemoryNote {
    param([string]$Message)

    if ([string]::IsNullOrWhiteSpace($Message)) {
        Write-Host "Missing memory note message." -ForegroundColor Red
        Write-Host "Usage: .\brevity.ps1 memory note <message>"
        exit 1
    }

    Write-VaultRuntimeEvent -Type "note" -Message $Message
    $config = Read-BrevityConfig
    Write-Host "Wrote runtime memory: $(Join-Path $config.vaultPath "runtime-log.md")"
}

function Get-RecentWorkerLogs {
    param([int]$Count = 5)

    $repoRoot = Get-RepositoryRoot
    $logsRoot = Join-Path $repoRoot ".brevity\logs"

    if (-not (Test-Path -LiteralPath $logsRoot)) {
        return @()
    }

    return @(Get-ChildItem -LiteralPath $logsRoot -Recurse -Filter "*.log" -File -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First $Count)
}

function Get-LatestTaskWorkerLog {
    param([string]$Slug)

    $repoRoot = Get-RepositoryRoot
    $taskLogsRoot = Join-Path (Join-Path $repoRoot ".brevity\logs") $Slug

    if (-not (Test-Path -LiteralPath $taskLogsRoot)) {
        return $null
    }

    return (Get-ChildItem -LiteralPath $taskLogsRoot -Filter "*.log" -File -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1)
}

function Resolve-PositiveIntegerOption {
    param(
        [string]$Name,
        [string]$Value,
        [string]$Usage
    )

    if ([string]::IsNullOrWhiteSpace($Value) -or $Value -notmatch '^\d+$') {
        Write-Host "Invalid value for ${Name}: $Value" -ForegroundColor Red
        Write-Host $Usage
        exit 1
    }

    $parsed = 0
    if (-not [int]::TryParse($Value, [ref]$parsed) -or $parsed -le 0) {
        Write-Host "Invalid value for ${Name}: $Value" -ForegroundColor Red
        Write-Host $Usage
        exit 1
    }

    return $parsed
}

function Show-RecentLogs {
    param(
        [int]$RuntimeCount = 8,
        [int]$WorkerCount = 5
    )

    $config = Read-BrevityConfig
    $runtimeLogPath = Join-Path $config.vaultPath "runtime-log.md"

    Write-Host "Recent runtime activity"
    Write-Host ""

    Write-Section "Runtime Memory"
    if (Test-Path -LiteralPath $runtimeLogPath) {
        $runtimeLines = @(Get-Content -LiteralPath $runtimeLogPath -Tail $RuntimeCount)
        if ($runtimeLines.Count -eq 0) {
            Write-Host "No runtime memory entries found."
        }
        else {
            $runtimeLines | ForEach-Object { Write-Host $_ }
        }
    }
    else {
        Write-Host "Runtime log not found: $runtimeLogPath" -ForegroundColor DarkYellow
    }

    Write-Host ""
    Write-Section "Worker Logs"
    $logs = @(Get-RecentWorkerLogs -Count $WorkerCount)
    if ($logs.Count -eq 0) {
        Write-Host "No worker logs found."
        return
    }

    foreach ($log in $logs) {
        $slug = Split-Path -Leaf (Split-Path -Parent $log.FullName)
        Write-Host "$($log.LastWriteTime.ToString("yyyy-MM-dd HH:mm:ss"))  $slug  $($log.FullName)"
    }
}

function Show-TaskLogs {
    param(
        [string]$Slug,
        [int]$Tail = 20
    )

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\brevity.ps1 logs task <slug> [--tail <n>]"
        exit 1
    }

    $latestLog = Get-LatestTaskWorkerLog -Slug $Slug

    Write-Host "Task logs"
    Write-Host "Slug: $Slug"

    if ($null -eq $latestLog) {
        $repoRoot = Get-RepositoryRoot
        $taskLogsRoot = Join-Path (Join-Path $repoRoot ".brevity\logs") $Slug
        Write-Host "Latest worker log: none"
        Write-Host "No worker logs found under: $taskLogsRoot" -ForegroundColor DarkYellow
        return
    }

    Write-Host "Latest worker log: $($latestLog.FullName)"
    Write-Host ""
    Write-Section "Tail"
    Get-Content -LiteralPath $latestLog.FullName -Tail $Tail | ForEach-Object { Write-Host $_ }
}

function Show-Board {
    $tasks = @(Read-BrevityTasks)

    $providerHealth = Read-ProviderHealth
    $health = $providerHealth.health

    $summary = Get-ProviderHealthSummary -Health $health
    Write-Host "Providers: $($summary.total) total, $($summary.degraded) degraded, $($summary.unavailable) unavailable"
    Write-Host ""

    if ($tasks.Count -eq 0) {
        Write-Host "No Brevity tasks found."
        return
    }

    $runtimeTasks = @($tasks | ForEach-Object { Get-TaskRuntimeInfo -Task $_ })
    $totalTasks = $runtimeTasks.Count
    $readyTasks = @($runtimeTasks | Where-Object { $_.status -eq "ready-for-worker" }).Count
    $staleTasks = @($runtimeTasks | Where-Object { $_.runtime.stale }).Count

    Write-Host "Tasks: $totalTasks total, $readyTasks ready, $staleTasks stale"
    Write-Host ""

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
        $matchingTasks = @($runtimeTasks | Where-Object { $_.status -eq $knownStatus })
        if ($matchingTasks.Count -gt 0) {
            $statuses += $knownStatus
        }
    }

    $otherStatuses = @(
        $runtimeTasks |
            ForEach-Object { $_.status } |
            Where-Object { -not [string]::IsNullOrWhiteSpace($_) -and ($knownStatuses -notcontains $_) } |
            Sort-Object -Unique
    )

    $statuses += $otherStatuses

    $tasksWithoutStatus = @($runtimeTasks | Where-Object { [string]::IsNullOrWhiteSpace($_.status) })
    if ($tasksWithoutStatus.Count -gt 0) {
        $statuses += "unknown"
    }

    foreach ($status in $statuses) {
        if ($status -eq "unknown") {
            $groupTasks = $tasksWithoutStatus
        }
        else {
            $groupTasks = @($runtimeTasks | Where-Object { $_.status -eq $status })
        }

        if ($groupTasks.Count -eq 0) {
            continue
        }

        Write-Section $status
        foreach ($task in $groupTasks) {
            Write-Host "slug: $($task.slug)"
            Write-Host "branch: $($task.branch)"
            Write-Host "worktreePath: $($task.worktree.path)"
            if ($task.runtime.stale) {
                Write-Host "runtime: stale" -ForegroundColor Yellow
            }
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

function Test-PlannerApplyPreflight {
    param(
        [object[]]$Tasks,
        [string]$TasksRoot,
        [bool]$CreateWorktrees = $false
    )

    $errors = @()
    $seenSlugs = @()
    $repoName = Get-RepositoryName
    $rootPath = Resolve-DevRoot $DevRoot
    $existingTasks = @(Read-BrevityTasks)

    foreach ($task in $Tasks) {
        $title = Get-PlannerFieldValue -Task $task -Name "title"
        $slug = Get-PlannerFieldValue -Task $task -Name "slug"
        $status = Get-PlannerFieldValue -Task $task -Name "status"
        $dependencies = Get-PlannerFieldValue -Task $task -Name "dependencies"
        $workerPrompt = Get-PlannerFieldValue -Task $task -Name "workerPrompt"
        $label = $(if ([string]::IsNullOrWhiteSpace($title)) { "<missing title>" } else { $title })

        if ([string]::IsNullOrWhiteSpace($slug)) {
            $errors += "Planner task '$label' is missing slug."
            continue
        }

        if ($slug -notmatch '^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$') {
            $errors += "Planner task '$label' has invalid slug '$slug'. Use lowercase letters, numbers, and hyphens."
            continue
        }

        if ($seenSlugs -contains $slug) {
            $errors += "Planner output contains duplicate slug: $slug"
        }
        else {
            $seenSlugs += $slug
        }

        if ([string]::IsNullOrWhiteSpace($title)) {
            $errors += "Planner task '$slug' is missing title."
        }

        if ($status -ne "planned") {
            $errors += "Planner task '$slug' must have status: planned."
        }

        if ([string]::IsNullOrWhiteSpace($dependencies)) {
            $errors += "Planner task '$slug' is missing dependencies."
        }

        if ([string]::IsNullOrWhiteSpace($workerPrompt)) {
            $errors += "Planner task '$slug' is missing workerPrompt."
        }

        $specPath = Join-Path $TasksRoot "$slug.md"
        if (Test-Path -LiteralPath $specPath) {
            $errors += "Vault task spec already exists for '$slug': $specPath"
        }

        if ($CreateWorktrees) {
            $existingTask = $existingTasks | Where-Object { $_.slug -eq $slug } | Select-Object -First 1
            if ($null -ne $existingTask) {
                $errors += "Task metadata already exists for '$slug'."
            }

            $worktreePath = Join-Path $rootPath "worktrees\active\$repoName-$slug"
            if (Test-Path -LiteralPath $worktreePath) {
                $errors += "Task worktree path already exists for '$slug': $worktreePath"
            }

            $branchName = "task/$slug"
            if (Test-GitBranchExists -Branch $branchName) {
                $errors += "Task branch already exists for '$slug': $branchName"
            }
        }
    }

    if ($errors.Count -eq 0) {
        return $true
    }

    Write-Host "Planner apply preflight failed." -ForegroundColor Red
    Write-Host "No task specs or worktrees were created."
    $errors | ForEach-Object { Write-Host "- $_" -ForegroundColor Red }
    return $false
}

function Apply-PlannerOutput {
    param(
        [string]$Path,
        [bool]$CreateWorktrees = $false,
        [bool]$StartWorkers = $false
    )

    $config = Read-BrevityConfig
    $tasksRoot = Join-Path $config.vaultPath "tasks"
    $tasks = @(Read-PlannerOutputTasks -Path $Path)

    if ($tasks.Count -eq 0) {
        Write-Host "Planner output did not contain any tasks." -ForegroundColor Red
        exit 1
    }

    if (-not (Test-PlannerApplyPreflight -Tasks $tasks -TasksRoot $tasksRoot -CreateWorktrees ($CreateWorktrees -or $StartWorkers))) {
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

    if ($CreateWorktrees -or $StartWorkers) {
        Write-Host ""
        Write-Host "Materializing planner tasks:"
        foreach ($task in $tasks) {
            $slug = [string]$task.slug
            if ([string]::IsNullOrWhiteSpace($slug)) {
                Write-Host "Skipping planner task without slug." -ForegroundColor Yellow
                continue
            }

            New-TaskWorktree -Root $DevRoot -Slug $slug

            if ($StartWorkers) {
                Start-TaskWork -Slug $slug
            }
        }

        Write-VaultRuntimeEvent -Type "planner" -Message "Materialized $($tasks.Count) planner task(s)."
    }
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
        "# Local Context",
        "",
        "Brevity materializes selected durable project memory into this worktree before worker execution.",
        "Read local context files from `.brevity\context\` when they exist.",
        "Do not read external vault paths directly; the vault is durable memory, and this worktree is the bounded execution context.",
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
    $taskRecord = New-Object PSObject -Property ([ordered]@{
        slug = $Slug
        branch = $Branch
        worktreePath = $WorktreePath
        promptPath = $PromptPath
        specPath = $SpecPath
        status = "ready-for-worker"
        createdAt = (Get-Date).ToUniversalTime().ToString("o")
    })

    Invoke-TaskMetadataLock -TasksPath $tasksPath -ScriptBlock {
        $tasks = @(Read-TaskMetadataFile -TasksPath $tasksPath)
        $tasks = @($tasks) + $taskRecord
        Write-TaskMetadataFile -TasksPath $tasksPath -Tasks $tasks
    }
}

function Get-TaskRuntimeWorktreeInfo {
    param([object]$Task)

    $worktreePath = Get-TaskField -Task $Task -Name "worktreePath"
    $exists = (-not [string]::IsNullOrWhiteSpace($worktreePath)) -and (Test-Path -LiteralPath $worktreePath)
    $registered = $false
    $hasGit = $false
    $clean = $null
    $currentBranch = ""

    if ($exists) {
        try {
            $registered = Test-GitWorktreeRegistered -WorktreePath $worktreePath
        }
        catch {
            $registered = $false
        }

        $gitDir = Join-Path $worktreePath ".git"
        $hasGit = Test-Path -LiteralPath $gitDir

        $previousErrorActionPreference = $ErrorActionPreference
        $ErrorActionPreference = "Continue"
        $currentBranch = (& git -C $worktreePath rev-parse --abbrev-ref HEAD 2>$null)
        $branchExitCode = $LASTEXITCODE
        $statusOutput = @(& git -C $worktreePath status --porcelain 2>$null)
        $statusExitCode = $LASTEXITCODE
        $ErrorActionPreference = $previousErrorActionPreference

        if ($branchExitCode -ne 0) {
            $currentBranch = ""
        }

        if ($statusExitCode -eq 0) {
            $clean = ($statusOutput.Count -eq 0)
        }
    }

    return [pscustomobject]@{
        exists = $exists
        path = $worktreePath
        registered = $registered
        hasGit = $hasGit
        clean = $clean
        currentBranch = [string]$currentBranch
    }
}

function Get-TaskRuntimePromptInfo {
    param([object]$Task)

    $promptPath = Get-TaskField -Task $Task -Name "promptPath"
    $exists = (-not [string]::IsNullOrWhiteSpace($promptPath)) -and (Test-Path -LiteralPath $promptPath)

    return [pscustomobject]@{
        exists = $exists
        path = $promptPath
    }
}

function Get-TaskRuntimeContextInfo {
    param([object]$Task)

    $worktreePath = Get-TaskField -Task $Task -Name "worktreePath"
    $contextPath = ""
    if (-not [string]::IsNullOrWhiteSpace($worktreePath)) {
        $contextPath = Join-Path $worktreePath ".brevity\context"
    }

    $exists = (-not [string]::IsNullOrWhiteSpace($contextPath)) -and (Test-Path -LiteralPath $contextPath -PathType Container)
    $managedFiles = @(Get-ManagedContextFileNames)
    $materializedFiles = @()
    $missingFiles = @()
    $lastWriteTimeUtc = $null

    foreach ($fileName in $managedFiles) {
        $filePath = $(if ([string]::IsNullOrWhiteSpace($contextPath)) { "" } else { Join-Path $contextPath $fileName })
        if ($exists -and (Test-Path -LiteralPath $filePath -PathType Leaf)) {
            $fileItem = Get-Item -LiteralPath $filePath
            $materializedFiles += $fileName
            if ($null -eq $lastWriteTimeUtc -or $fileItem.LastWriteTimeUtc -gt $lastWriteTimeUtc) {
                $lastWriteTimeUtc = $fileItem.LastWriteTimeUtc
            }
        }
        else {
            $missingFiles += $fileName
        }
    }

    return [pscustomobject]@{
        exists = $exists
        path = $contextPath
        materializedFileCount = $materializedFiles.Count
        managedFiles = $managedFiles
        materializedFiles = $materializedFiles
        missingFiles = $missingFiles
        newestMaterializedFileWriteTimeUtc = $(if ($null -ne $lastWriteTimeUtc) { $lastWriteTimeUtc.ToString("o") } else { $null })
    }
}

function Get-TaskRuntimeProviderInfo {
    $repoRoot = Get-RepositoryRoot
    $configPath = Join-Path $repoRoot ".brevity\config.json"
    $healthPath = Join-Path $repoRoot ".brevity\provider-health.json"

    $resolvedProvider = ""
    if (Test-Path -LiteralPath $configPath) {
        try {
            $config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
            if ($null -ne $config -and (Get-Member -InputObject $config -Name "defaultProvider" -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
                $resolvedProvider = [string]$config.defaultProvider
            }
        }
        catch {
            $resolvedProvider = ""
        }
    }

    $healthStatus = "unknown"
    if ((-not [string]::IsNullOrWhiteSpace($resolvedProvider)) -and (Test-Path -LiteralPath $healthPath)) {
        try {
            $health = Get-Content -LiteralPath $healthPath -Raw | ConvertFrom-Json
            if ($null -ne $health -and (Get-Member -InputObject $health -Name $resolvedProvider -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
                $providerHealth = $health.$resolvedProvider
                if ($null -ne $providerHealth -and (Get-Member -InputObject $providerHealth -Name "status" -MemberType NoteProperty -ErrorAction SilentlyContinue)) {
                    $healthStatus = [string]$providerHealth.status
                }
            }
        }
        catch {
            $healthStatus = "unknown"
        }
    }

    if ([string]::IsNullOrWhiteSpace($healthStatus)) {
        $healthStatus = "unknown"
    }

    return [pscustomobject]@{
        resolved = $resolvedProvider
        health = $healthStatus
        available = ($healthStatus -ne "unavailable")
        gated = ($healthStatus -eq "unavailable" -or $healthStatus -eq "quota-constrained" -or $healthStatus -eq "capacity-degraded")
    }
}

function Get-TaskRuntimeExecutionInfo {
    param([string]$Slug)

    $repoRoot = Get-RepositoryRoot
    $taskLogsRoot = Join-Path (Join-Path $repoRoot ".brevity\logs") $Slug
    $latestLog = $null
    $lastExitCode = $null
    $lastFailureType = $null

    if (Test-Path -LiteralPath $taskLogsRoot) {
        $latestLog = Get-ChildItem -LiteralPath $taskLogsRoot -Filter "*.log" -File -ErrorAction SilentlyContinue |
            Sort-Object LastWriteTime -Descending |
            Select-Object -First 1

        if ($null -ne $latestLog) {
            $exitCodeLine = Get-Content -LiteralPath $latestLog.FullName -ErrorAction SilentlyContinue |
                Where-Object { $_ -like "ExitCode:*" } |
                Select-Object -First 1

            if (-not [string]::IsNullOrWhiteSpace($exitCodeLine)) {
                $lastExitCode = ($exitCodeLine -replace "^ExitCode:\s*", "")
                if ($lastExitCode -ne "0") {
                    $lastFailureType = "worker-failed"
                }
            }
        }
    }

    return [pscustomobject]@{
        hasLog = ($null -ne $latestLog)
        lastLogPath = $(if ($null -ne $latestLog) { $latestLog.FullName } else { "" })
        lastExitCode = $lastExitCode
        lastFailureType = $lastFailureType
    }
}

function Get-TaskRuntimeStateFlags {
    param(
        [object]$Task,
        [object]$Worktree,
        [object]$Prompt
    )

    $branch = Get-TaskField -Task $Task -Name "branch"
    $branchExists = (-not [string]::IsNullOrWhiteSpace($branch)) -and (Test-GitBranchExists -Branch $branch)

    $missingWorktree = -not $Worktree.exists
    $unregisteredWorktree = ($Worktree.exists -and (-not $Worktree.registered))
    $missingPrompt = -not $Prompt.exists
    $missingBranch = -not $branchExists
    $stale = ($missingWorktree -or $unregisteredWorktree -or $missingPrompt -or $missingBranch)

    $issues = @()
    if ($missingWorktree) { $issues += "missing-worktree" }
    if ($unregisteredWorktree) { $issues += "unregistered-worktree" }
    if ($missingPrompt) { $issues += "missing-prompt" }
    if ($missingBranch) { $issues += "missing-branch" }

    return [pscustomobject]@{
        stale = $stale
        merged = $false
        orphaned = $false
        branchExists = $branchExists
        missingWorktree = $missingWorktree
        unregisteredWorktree = $unregisteredWorktree
        missingPrompt = $missingPrompt
        missingBranch = $missingBranch
        issues = $issues
    }
}

function Get-TaskRuntimeInfo {
    param(
        [object]$Task = $null,
        [string]$Slug = ""
    )

    if ($null -eq $Task -and -not [string]::IsNullOrWhiteSpace($Slug)) {
        $Task = Find-BrevityTaskBySlug -Slug $Slug
    }

    if ($null -eq $Task) {
        if ([string]::IsNullOrWhiteSpace($Slug)) {
            $Slug = ""
        }

        $emptyTask = [pscustomobject]@{
            slug = $Slug
            branch = $(if ([string]::IsNullOrWhiteSpace($Slug)) { "" } else { "task/$Slug" })
            worktreePath = ""
            promptPath = ""
            status = "missing-metadata"
        }

        $worktree = Get-TaskRuntimeWorktreeInfo -Task $emptyTask
        $prompt = Get-TaskRuntimePromptInfo -Task $emptyTask
        $context = Get-TaskRuntimeContextInfo -Task $emptyTask
        $provider = Get-TaskRuntimeProviderInfo
        $execution = Get-TaskRuntimeExecutionInfo -Slug $Slug
        $runtime = Get-TaskRuntimeStateFlags -Task $emptyTask -Worktree $worktree -Prompt $prompt

        return [pscustomobject]@{
            slug = $Slug
            branch = Get-TaskField -Task $emptyTask -Name "branch"
            status = "missing-metadata"
            metadataStatus = ""
            taskExists = $false
            worktree = $worktree
            prompt = $prompt
            context = $context
            provider = $provider
            runtime = $runtime
            execution = $execution
        }
    }

    $metadataStatus = Get-TaskField -Task $Task -Name "status"
    $slug = Get-TaskField -Task $Task -Name "slug"
    $branch = Get-TaskField -Task $Task -Name "branch"
    $worktree = Get-TaskRuntimeWorktreeInfo -Task $Task
    $prompt = Get-TaskRuntimePromptInfo -Task $Task
    $context = Get-TaskRuntimeContextInfo -Task $Task
    $provider = Get-TaskRuntimeProviderInfo
    $execution = Get-TaskRuntimeExecutionInfo -Slug $slug
    $runtime = Get-TaskRuntimeStateFlags -Task $Task -Worktree $worktree -Prompt $prompt

    $runtimeStatus = $metadataStatus
    if ($runtime.missingWorktree) {
        $runtimeStatus = "stale-worktree"
    }
    elseif ($runtime.unregisteredWorktree) {
        $runtimeStatus = "stale-unregistered-worktree"
    }
    elseif ($runtime.missingPrompt) {
        $runtimeStatus = "stale-prompt"
    }
    elseif ($runtime.missingBranch) {
        $runtimeStatus = "stale-branch"
    }

    return [pscustomobject]@{
        slug = $slug
        branch = $branch
        status = $runtimeStatus
        metadataStatus = $metadataStatus
        taskExists = $true
        worktree = $worktree
        prompt = $prompt
        context = $context
        provider = $provider
        runtime = $runtime
        execution = $execution
    }
}

function Get-TaskRuntimeStatus {
    param([object]$Task)

    return (Get-TaskRuntimeInfo -Task $Task).status
}

function Get-StaleTasks {
    $tasks = @(Read-BrevityTasks)
    return @($tasks | ForEach-Object { Get-TaskRuntimeInfo -Task $_ } | Where-Object { $_.runtime.stale })
}

function Test-TaskRuntimeState {
    param([string]$Slug)

    $runtimeInfo = Get-TaskRuntimeInfo -Slug $Slug
    return (-not $runtimeInfo.runtime.stale)
}


function Show-TaskRuntimeInfoCommand {
    param(
        [string]$Slug
    )

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host "Usage: .\brevity.ps1 task runtime-info <slug>"
        exit 1
    }

    Get-TaskRuntimeInfo -Slug $Slug | ConvertTo-Json -Depth 10
}

function Get-RequiredTaskForContextCommand {
    param(
        [string]$Slug,
        [string]$Usage
    )

    if ([string]::IsNullOrWhiteSpace($Slug)) {
        Write-Host "Missing task slug." -ForegroundColor Red
        Write-Host $Usage
        exit 1
    }

    $task = Find-BrevityTaskBySlug -Slug $Slug
    if ($null -eq $task) {
        Write-Host "Task not found: $Slug" -ForegroundColor Red
        Write-Host "Use .\brevity.ps1 task status to list known tasks."
        exit 1
    }

    $worktreePath = Get-TaskField -Task $task -Name "worktreePath"
    if ([string]::IsNullOrWhiteSpace($worktreePath)) {
        Write-Host "Task metadata is missing worktreePath for: $Slug" -ForegroundColor Red
        exit 1
    }

    if (-not (Test-Path -LiteralPath $worktreePath -PathType Container)) {
        Write-Host "Task worktree path does not exist for: $Slug" -ForegroundColor Red
        Write-Host "Expected path: $worktreePath"
        exit 1
    }

    return $task
}

function Show-TaskContextStatus {
    param([string]$Slug)

    $task = Get-RequiredTaskForContextCommand -Slug $Slug -Usage "Usage: .\brevity.ps1 task context status <slug>"
    $context = Get-TaskRuntimeContextInfo -Task $task

    Write-Host "Task: $Slug"
    Write-Host "Context: $($context.path)"
    Write-Host "Exists: $($context.exists)"
    Write-Host "Materialized files: $($context.materializedFileCount)"
    Write-Host "Missing managed files: $(@($context.missingFiles).Count)"
}

function Refresh-TaskContext {
    param([string]$Slug)

    $task = Get-RequiredTaskForContextCommand -Slug $Slug -Usage "Usage: .\brevity.ps1 task context refresh <slug>"
    Copy-TaskWorkspaceContext -WorktreePath (Get-TaskField -Task $task -Name "worktreePath") | Out-Null
    $context = Get-TaskRuntimeContextInfo -Task $task

    Write-Host "Refreshed task context"
    Write-Host "Task: $Slug"
    Write-Host "Context: $($context.path)"
    Write-Host "Refreshed files: $($context.materializedFileCount)"
    Write-Host "Missing managed files: $(@($context.missingFiles).Count)"
}

function Show-TaskStatus {
    $tasks = @(Read-BrevityTasks)
    if ($tasks.Count -eq 0) {
        Write-Host "No Brevity tasks found."
        return
    }

    $runtimeTasks = @($tasks | ForEach-Object { Get-TaskRuntimeInfo -Task $_ })

    $totalTasks = $runtimeTasks.Count
    $readyTasks = @($runtimeTasks | Where-Object { $_.status -eq "ready-for-worker" }).Count
    $staleTasks = @($runtimeTasks | Where-Object { $_.runtime.stale }).Count

    Write-Host "Tasks: $totalTasks total, $readyTasks ready, $staleTasks stale"
    Write-Host ""

    $runtimeTasks |
        ForEach-Object {
            [pscustomobject]@{
                slug = $_.slug
                branch = $_.branch
                status = $_.status
                worktreePath = $_.worktree.path
                promptPath = $_.prompt.path
                provider = $_.provider.resolved
                providerHealth = $_.provider.health
                lastExitCode = $_.execution.lastExitCode
            }
        } |
        Format-List
}

function Show-WorkerPlan {
    $tasks = @(Read-BrevityTasks)
    Write-Host "Worker execution plan"

    if ($tasks.Count -eq 0) {
        Write-Host "No Brevity tasks found."
        return
    }

    $runtimeTasks = @($tasks | ForEach-Object { Get-TaskRuntimeInfo -Task $_ })
    $planItems = @($runtimeTasks | ForEach-Object {
        $provider = $_.provider.resolved
        if ([string]::IsNullOrWhiteSpace($provider)) {
            $provider = "unknown"
        }

        $readiness = "blocked"
        if ($_.metadataStatus -eq "merged") {
            $readiness = "review"
        }
        elseif (-not $_.runtime.stale -and -not $_.provider.gated -and $_.metadataStatus -eq "ready-for-worker") {
            $readiness = "runnable"
        }
        elseif ($_.runtime.stale) {
            $readiness = "stale"
        }
        elseif ($_.provider.gated) {
            $readiness = "provider-gated"
        }

        [pscustomobject]@{
            slug = $_.slug
            provider = $provider
            providerHealth = $_.provider.health
            status = $_.status
            worktreeStatus = $(if ($_.worktree.exists) { $(if ($_.worktree.registered) { "registered" } else { "unregistered" }) } else { "missing" })
            worktreePath = $_.worktree.path
            stale = $_.runtime.stale
            readiness = $readiness
            branch = $_.branch
        }
    })

    $runnable = @($planItems | Where-Object { $_.readiness -eq "runnable" })
    $review = @($planItems | Where-Object { $_.readiness -eq "review" })
    $blocked = @($planItems | Where-Object { $_.readiness -ne "runnable" -and $_.readiness -ne "review" })

    Write-Host "Tasks: $($planItems.Count) total, $($runnable.Count) runnable, $($blocked.Count) blocked, $($review.Count) ready for review"

    Write-Section "Execution groups"
    Write-Host "Runnable group: $($runnable.Count) task(s)"
    Write-Host "Blocked group: $($blocked.Count) task(s)"
    Write-Host "Review queue: $($review.Count) task(s)"

    Write-Section "Provider capacity"
    if ($planItems.Count -eq 0) {
        Write-Host "No provider load."
    }
    else {
        $planItems |
            Group-Object provider |
            Sort-Object Name |
            ForEach-Object {
                $providerHealth = ($_.Group | Select-Object -First 1).providerHealth
                $providerRunnable = @($_.Group | Where-Object { $_.readiness -eq "runnable" }).Count
                Write-Host "$($_.Name): $providerRunnable runnable / $($_.Count) total task(s), health=$providerHealth"
                if ($providerHealth -eq "capacity-degraded" -or $providerHealth -eq "quota-constrained" -or $providerHealth -eq "unavailable") {
                    Write-Host "  Warning: provider health may limit parallel execution." -ForegroundColor Yellow
                }
                elseif ($providerRunnable -gt 1) {
                    Write-Host "  Candidate parallel group; provider capacity is not enforced yet."
                }
            }
    }

    Write-Section "Provider groups"
    $planItems |
        Sort-Object provider, readiness, slug |
        Group-Object provider |
        ForEach-Object {
            Write-Host "$($_.Name): $($_.Count) task(s)"
            $_.Group | ForEach-Object {
                Write-Host "  $($_.slug) [$($_.readiness)] worktree=$($_.worktreeStatus) stale=$($_.stale) health=$($_.providerHealth)"
            }
        }

    Write-Section "Proposed execution order"
    if ($runnable.Count -eq 0) {
        Write-Host "No runnable tasks."
    }
    else {
        $index = 1
        $runnable | Sort-Object provider, slug | ForEach-Object {
            Write-Host "$index. $($_.slug) provider=$($_.provider) branch=$($_.branch)"
            $index++
        }
    }

    Write-Section "Parallel safety"
    if ($runnable.Count -lt 2) {
        Write-Host "No runnable task pairs to compare."
    }
    else {
        $conflicts = @()
        for ($i = 0; $i -lt $runnable.Count; $i++) {
            for ($j = $i + 1; $j -lt $runnable.Count; $j++) {
                $left = $runnable[$i]
                $right = $runnable[$j]
                $leftPath = ConvertTo-DoctorComparablePath -Path $left.worktreePath
                $rightPath = ConvertTo-DoctorComparablePath -Path $right.worktreePath

                if ([string]::IsNullOrWhiteSpace($leftPath) -or [string]::IsNullOrWhiteSpace($rightPath)) {
                    continue
                }

                if ($leftPath -eq $rightPath -or $leftPath.StartsWith("$rightPath\") -or $rightPath.StartsWith("$leftPath\")) {
                    $conflicts += "$($left.slug) <-> $($right.slug): overlapping worktree paths"
                }
            }
        }

        if ($conflicts.Count -eq 0) {
            Write-Host "Runnable tasks have distinct worktree paths."
        }
        else {
            $conflicts | ForEach-Object { Write-Host $_ -ForegroundColor Yellow }
        }
    }

    Write-Section "Review queue"
    if ($review.Count -eq 0) {
        Write-Host "No tasks ready for review."
    }
    else {
        $index = 1
        $review | Sort-Object slug | ForEach-Object {
            Write-Host "$index. $($_.slug) branch=$($_.branch)"
            $index++
        }
    }

    Write-Section "Blocked tasks"
    if ($blocked.Count -eq 0) {
        Write-Host "None"
    }
    else {
        $blocked | Sort-Object readiness, slug | ForEach-Object {
            Write-Host "$($_.slug) [$($_.readiness)] status=$($_.status) worktree=$($_.worktreeStatus) provider=$($_.provider)"
        }
    }
}

function Get-SessionSummaryData {
    $repoRoot = Get-RepositoryRoot
    $providerHealth = Read-ProviderHealth
    $providerSummary = Get-ProviderHealthSummary -Health $providerHealth.health
    $tasks = @(Read-BrevityTasks)
    $runtimeTasks = @($tasks | ForEach-Object { Get-TaskRuntimeInfo -Task $_ })
    $runnableTasks = @($runtimeTasks | Where-Object { -not $_.runtime.stale -and -not $_.provider.gated -and $_.metadataStatus -eq "ready-for-worker" })
    $staleTasks = @($runtimeTasks | Where-Object { $_.runtime.stale })
    $providerGatedTasks = @($runtimeTasks | Where-Object { $_.provider.gated })
    $blockedTasks = @($runtimeTasks | Where-Object { $_.runtime.stale -or $_.provider.gated })
    $worktreeRecords = @(Get-GitWorktreeRecords)
    $tasksPath = Join-Path $repoRoot ".brevity\tasks.json"
    $lockInfo = Get-TaskMetadataLockInfo -TasksPath $tasksPath
    $config = Read-BrevityConfig
    $runtimeLogPath = Join-Path $config.vaultPath "runtime-log.md"
    $recentRuntimeMemory = @()
    if (Test-Path -LiteralPath $runtimeLogPath) {
        $recentRuntimeMemory = @(Get-Content -LiteralPath $runtimeLogPath -Tail 5)
    }

    $suggestedNextActions = @("No immediate runtime action suggested.")
    if ($lockInfo.exists -and $null -ne $lockInfo.ageMinutes -and $lockInfo.ageMinutes -ge 10) {
        $suggestedNextActions = @("Run .\brevity.ps1 doctor --repair to remove a stale task metadata lock after confirming no Brevity process is active.")
    }
    elseif ($staleTasks.Count -gt 0) {
        $suggestedNextActions = @("Run .\brevity.ps1 doctor to inspect stale task metadata.")
    }
    elseif ($providerGatedTasks.Count -gt 0 -or $providerSummary.degraded -gt 0 -or $providerSummary.unavailable -gt 0) {
        $suggestedNextActions = @("Run .\brevity.ps1 provider status and choose a healthy worker profile before launching more work.")
    }
    elseif ($runnableTasks.Count -gt 0) {
        $suggestedNextActions = @("Run .\brevity.ps1 plan workers to review execution grouping before starting workers.")
    }

    $taskCounts = [pscustomobject]@{ tracked = $runtimeTasks.Count; runnable = $runnableTasks.Count; stale = $staleTasks.Count; providerGated = $providerGatedTasks.Count; blocked = $blockedTasks.Count }

    return [pscustomobject]@{
        repoRoot = $repoRoot
        providers = $providerSummary
        taskCounts = $taskCounts
        activeWorktrees = $worktreeRecords
        lock = $lockInfo
        recentRuntimeMemory = $recentRuntimeMemory
        runtimeLogPath = $runtimeLogPath
        suggestedNextActions = $suggestedNextActions
        runtimeEventMessage = "tracked=$($taskCounts.tracked) runnable=$($taskCounts.runnable) stale=$($taskCounts.stale) providerGated=$($taskCounts.providerGated)"
    }
}

function Show-SessionSummary {
    param([switch]$Json)

    $summary = Get-SessionSummaryData

    if ($Json) {
        $summary | Select-Object providers, taskCounts, activeWorktrees, lock, recentRuntimeMemory, suggestedNextActions | ConvertTo-Json -Depth 10
    }
    else {
        Write-Host "Brevity session summary"
        Write-Host "Repo: $($summary.repoRoot)"
        Write-Host "Providers: $($summary.providers.total) total, $($summary.providers.degraded) degraded, $($summary.providers.unavailable) unavailable"
        Write-Host "Tasks: $($summary.taskCounts.tracked) tracked, $($summary.taskCounts.runnable) runnable, $($summary.taskCounts.stale) stale, $($summary.taskCounts.providerGated) provider-gated, $($summary.taskCounts.blocked) blocked"
        Write-Host "Active worktrees: $($summary.activeWorktrees.Count) registered"
        if ($summary.lock.exists) {
            if ($null -ne $summary.lock.ageMinutes) {
                Write-Host ("Task metadata lock: present ({0:N1} minutes)" -f $summary.lock.ageMinutes)
            }
            else {
                Write-Host "Task metadata lock: present"
            }
        }
        else {
            Write-Host "Task metadata lock: none"
        }

        Write-Section "Recent runtime memory"
        if ($summary.recentRuntimeMemory.Count -gt 0) {
            $summary.recentRuntimeMemory
        }
        else {
            Write-Host "No runtime log yet: $($summary.runtimeLogPath)"
        }

        Write-Section "Suggested next actions"
        $summary.suggestedNextActions | ForEach-Object { Write-Host $_ }
    }

    Write-VaultRuntimeEvent -Type "session-summary" -Message $summary.runtimeEventMessage
}

function Get-GitTaskBranches {
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $branches = @(& git for-each-ref --format="%(refname:short)" refs/heads/task 2>$null)
    $gitExitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference

    if ($gitExitCode -ne 0) {
        return @()
    }

    return @($branches | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
}

function Get-MergedTaskBranches {
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $branches = @(& git for-each-ref --merged HEAD --format="%(refname:short)" refs/heads/task 2>$null)
    $gitExitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference

    if ($gitExitCode -ne 0) {
        return @()
    }

    return @($branches | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
}

function Get-GitWorktreeRecords {
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $worktreeList = @(& git worktree list --porcelain 2>$null)
    $gitExitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference

    if ($gitExitCode -ne 0) {
        return @()
    }

    $records = @()
    $currentPath = ""
    $currentBranch = ""

    foreach ($line in $worktreeList) {
        if ($line.StartsWith("worktree ")) {
            if (-not [string]::IsNullOrWhiteSpace($currentPath)) {
                $records += [pscustomobject]@{
                    path = $currentPath
                    branch = $currentBranch
                }
            }

            $currentPath = $line.Substring("worktree ".Length)
            $currentBranch = ""
        }
        elseif ($line.StartsWith("branch ")) {
            $currentBranch = $line.Substring("branch ".Length) -replace "^refs/heads/", ""
        }
    }

    if (-not [string]::IsNullOrWhiteSpace($currentPath)) {
        $records += [pscustomobject]@{
            path = $currentPath
            branch = $currentBranch
        }
    }

    return @($records)
}

function ConvertTo-DoctorComparablePath {
    param([string]$Path)

    if ([string]::IsNullOrWhiteSpace($Path)) {
        return ""
    }

    $resolvedPath = $Path
    if (Test-Path -LiteralPath $Path) {
        $resolvedPath = (Resolve-Path -LiteralPath $Path).Path
    }

    return $resolvedPath.Replace("/", "\").TrimEnd("\").ToLowerInvariant()
}

function Show-ExecutionPolicyGuidance {
    Write-Host "PowerShell execution policy guidance"
    Write-Host ""
    Write-Host "Brevity does not change execution policy or unblock files automatically."
    Write-Host "For local development, use one of these explicit options:"
    Write-Host ""
    Write-Host "  Set-ExecutionPolicy -Scope Process Bypass"
    Write-Host "  .\brevity.ps1 doctor"
    Write-Host ""
    Write-Host "  Unblock-File .\brevity.ps1"
    Write-Host "  .\brevity.ps1 doctor"
    Write-Host ""
    Write-Host "  powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\brevity.ps1 doctor"
    Write-Host ""
    Write-Host "Use process-scoped bypass for a temporary shell session."
    Write-Host "Use Unblock-File only when you trust the local script contents."
}

function Get-TaskMetadataLockInfo {
    param([string]$TasksPath)

    $lockPath = Join-Path (Split-Path -Parent $TasksPath) "tasks.lock"
    $exists = Test-Path -LiteralPath $lockPath
    $ageMinutes = $null

    if ($exists) {
        $lockItem = Get-Item -LiteralPath $lockPath -ErrorAction SilentlyContinue
        if ($null -ne $lockItem) {
            $ageMinutes = ((Get-Date) - $lockItem.LastWriteTime).TotalMinutes
        }
    }

    return [pscustomobject]@{
        exists = $exists
        path = $lockPath
        ageMinutes = $ageMinutes
    }
}

function Show-DoctorReport {
    param([bool]$Repair = $false)

    $repoRoot = Get-RepositoryRoot
    $tasksPath = Join-Path $repoRoot ".brevity\tasks.json"
    $tasks = @(Read-BrevityTasks)
    $runtimeTasks = @($tasks | ForEach-Object { Get-TaskRuntimeInfo -Task $_ })
    $metadataBranches = @($runtimeTasks | ForEach-Object { $_.branch } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    $metadataWorktrees = @($runtimeTasks | ForEach-Object { ConvertTo-DoctorComparablePath -Path $_.worktree.path } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    $taskBranches = @(Get-GitTaskBranches)
    $mergedBranches = @(Get-MergedTaskBranches)
    $worktreeRecords = @(Get-GitWorktreeRecords)

    $orphanedBranches = @($taskBranches | Where-Object { $metadataBranches -notcontains $_ })
    $orphanedWorktrees = @($worktreeRecords | Where-Object {
        $record = $_
        $recordPath = ConvertTo-DoctorComparablePath -Path $record.path
        (-not [string]::IsNullOrWhiteSpace($record.branch)) -and $record.branch.StartsWith("task/") -and ($metadataWorktrees -notcontains $recordPath)
    })
    $mergedTaskBranches = @($mergedBranches | Where-Object { $metadataBranches -contains $_ })
    $staleTasks = @($runtimeTasks | Where-Object { $_.runtime.stale })
    $missingWorktreeTasks = @($runtimeTasks | Where-Object { $_.runtime.missingWorktree })
    $missingPromptTasks = @($runtimeTasks | Where-Object { $_.runtime.missingPrompt })
    $lockInfo = Get-TaskMetadataLockInfo -TasksPath $tasksPath

    Write-Host "Brevity doctor"
    Write-Host "Repo: $repoRoot"
    Write-Host "Tasks: $($runtimeTasks.Count) tracked, $($staleTasks.Count) stale"
    Write-Host "Branches: $($taskBranches.Count) task branches, $($orphanedBranches.Count) orphaned, $($mergedTaskBranches.Count) merged tracked"
    Write-Host "Worktrees: $($worktreeRecords.Count) registered, $($orphanedWorktrees.Count) orphaned task worktrees"

    Write-Section "Stale task metadata"
    if ($staleTasks.Count -eq 0) {
        Write-Host "None"
    }
    else {
        foreach ($task in $staleTasks) {
            Write-Host "$($task.slug): $($task.status)"
            if ($task.runtime.issues.Count -gt 0) {
                Write-Host "  Issues: $($task.runtime.issues -join ', ')"
            }
            Write-Host "  Worktree: $($task.worktree.path)"
            Write-Host "  Prompt: $($task.prompt.path)"
            Write-Host "  Branch: $($task.branch)"
        }
    }

    Write-Section "Missing worktrees"
    if ($missingWorktreeTasks.Count -eq 0) {
        Write-Host "None"
    }
    else {
        $missingWorktreeTasks | ForEach-Object { Write-Host "$($_.slug): $($_.worktree.path)" }
    }

    Write-Section "Missing prompt files"
    if ($missingPromptTasks.Count -eq 0) {
        Write-Host "None"
    }
    else {
        $missingPromptTasks | ForEach-Object { Write-Host "$($_.slug): $($_.prompt.path)" }
    }

    Write-Section "Orphaned branches"
    if ($orphanedBranches.Count -eq 0) {
        Write-Host "None"
    }
    else {
        $orphanedBranches | ForEach-Object { Write-Host $_ }
    }

    Write-Section "Orphaned worktrees"
    if ($orphanedWorktrees.Count -eq 0) {
        Write-Host "None"
    }
    else {
        $orphanedWorktrees | ForEach-Object { Write-Host "$($_.branch): $($_.path)" }
    }

    Write-Section "Merged task branches"
    if ($mergedTaskBranches.Count -eq 0) {
        Write-Host "None"
    }
    else {
        $mergedTaskBranches | ForEach-Object { Write-Host $_ }
    }

    Write-Section "Task metadata lock"
    if (-not $lockInfo.exists) {
        Write-Host "None"
    }
    else {
        Write-Host "Path: $($lockInfo.path)"
        if ($null -ne $lockInfo.ageMinutes) {
            Write-Host ("Age: {0:N1} minutes" -f $lockInfo.ageMinutes)
        }
        Write-Host "If no Brevity process is active, an old lock may be stale."
    }

    if ($Repair) {
        Write-Section "Repair"
        $repaired = $false
        if ($lockInfo.exists -and $null -ne $lockInfo.ageMinutes) {
            if ($lockInfo.ageMinutes -ge 10) {
                Remove-Item -LiteralPath $lockInfo.path -Force
                Write-Host "Removed stale task metadata lock: $($lockInfo.path)"
                Write-VaultRuntimeEvent -Type "doctor-repair" -Subject "tasks.lock" -Message "Removed stale task metadata lock."
                $repaired = $true
            }
            else {
                Write-Host ("Task metadata lock is fresh ({0:N1} minutes); not removing." -f $lockInfo.ageMinutes)
            }
        }

        $repairable = @($runtimeTasks | Where-Object { $_.runtime.missingWorktree -and $_.runtime.missingBranch })
        if ($repairable.Count -eq 0) {
            if (-not $repaired) {
                Write-Host "No conservative repairs available."
            }
            return
        }

        foreach ($task in $repairable) {
            Remove-TaskMetadataRecord -TasksPath $tasksPath -Tasks $tasks -Slug $task.slug
            $tasks = @($tasks | Where-Object { $_.slug -ne $task.slug })
            Write-VaultRuntimeEvent -Type "doctor-repair" -Subject $task.slug -Message "Removed stale task metadata record."
            $repaired = $true
        }
    }
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

function Get-ManagedContextFileNames {
    return @(
        "project.md",
        "architecture.md",
        "decisions.md",
        "current-state.md",
        "roadmap.md"
    )
}

function Copy-TaskWorkspaceContext {
    param(
        [string]$WorktreePath
    )

    if ([string]::IsNullOrWhiteSpace($WorktreePath)) {
        return @()
    }

    $config = Read-BrevityConfig
    if ($null -eq $config -or [string]::IsNullOrWhiteSpace($config.vaultPath)) {
        return @()
    }

    if (-not (Test-Path -LiteralPath $WorktreePath)) {
        return @()
    }

    $contextRoot = Join-Path $WorktreePath ".brevity\context"
    if (-not (Test-Path -LiteralPath $contextRoot)) {
        New-Item -ItemType Directory -Path $contextRoot -Force | Out-Null
    }

    $copied = @()
    foreach ($fileName in @(Get-ManagedContextFileNames)) {
        $sourcePath = Join-Path $config.vaultPath $fileName
        $targetPath = Join-Path $contextRoot $fileName
        if (-not (Test-Path -LiteralPath $sourcePath -PathType Leaf)) {
            if (Test-Path -LiteralPath $targetPath -PathType Leaf) {
                Remove-Item -LiteralPath $targetPath -Force
            }
            continue
        }

        Copy-Item -LiteralPath $sourcePath -Destination $targetPath -Force
        $copied += $targetPath
    }

    return @($copied)
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

    $contextFiles = @(Copy-TaskWorkspaceContext -WorktreePath $task.worktreePath)
    $materializedSpecPath = Update-TaskPromptFromSpec -PromptPath $task.promptPath -Slug $Slug -SpecPath $specPath
    $workerCommand = "codex -C $($task.worktreePath) -a never -s workspace-write"

    Write-Host "Task: $($task.slug)"
    Write-Host "Worktree: $($task.worktreePath)"
    Write-Host "Prompt: $($task.promptPath)"
    if (-not [string]::IsNullOrWhiteSpace($materializedSpecPath)) {
        Write-Host "Spec: $materializedSpecPath"
    }
    Write-Host "Context: $(Join-Path $task.worktreePath ".brevity\context") ($($contextFiles.Count) file(s))"
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

function Get-BrevityProfileAliases {
    return [ordered]@{
        "gemini-fast" = "gemini-flash"
        "gemini-balanced" = "gemini-pro"
        "gemini-default" = "gemini-flash"
        "codex-default" = "codex-balanced"
        "codex-standard" = "codex-balanced"
        "codex-pro" = "codex-deep"
    }
}

function Resolve-BrevityProfileName {
    param([string]$Name)

    $profileMatrix = Get-BrevityProfileMatrix
    $profileAliases = Get-BrevityProfileAliases
    $normalizedName = $Name.ToLowerInvariant()
    $canonicalName = $normalizedName
    $aliasName = ""

    if ($profileAliases.Contains($normalizedName)) {
        $canonicalName = $profileAliases[$normalizedName]
        $aliasName = $normalizedName
    }

    if (-not $profileMatrix.Contains($canonicalName)) {
        $validNames = @($profileMatrix.Keys) + @($profileAliases.Keys)
        throw "Unknown worker profile: $Name. Brevity v0 supports: $($validNames -join ', ')."
    }

    return [PSCustomObject]@{
        Name = $Name
        CanonicalName = $canonicalName
        AliasName = $aliasName
        IsAlias = (-not [string]::IsNullOrWhiteSpace($aliasName))
    }
}

function Get-BrevityProfileConfig {
    param([string]$Name)

    $profileMatrix = Get-BrevityProfileMatrix
    $resolvedProfile = Resolve-BrevityProfileName -Name $Name

    return $profileMatrix[$resolvedProfile.CanonicalName]
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

function Split-ProcessOutputLines {
    param([string]$Value)

    if ([string]::IsNullOrEmpty($Value)) {
        return @()
    }

    $lines = @($Value -split "\r?\n")
    if ($Value -match "(\r?\n)$" -and $lines.Count -gt 0) {
        return @($lines[0..($lines.Count - 2)])
    }

    return $lines
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

function Resolve-WorkerExecutablePath {
    param(
        [string]$Command
    )

    if ([string]::IsNullOrWhiteSpace($Command)) {
        return $Command
    }

    $resolved = Get-Command -Name $Command -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -ne $resolved -and -not [string]::IsNullOrWhiteSpace($resolved.Source)) {
        return [string]$resolved.Source
    }

    return $Command
}

function Invoke-WorkerCommand {
    param(
        [object]$WorkerCommand
    )

    $workerOutput = New-Object 'System.Collections.Generic.List[object]'
    $process = New-Object System.Diagnostics.Process
    try {
        $process.StartInfo.FileName = Resolve-WorkerExecutablePath -Command ([string]$WorkerCommand.command)
        $process.StartInfo.WorkingDirectory = [string]$WorkerCommand.workingDirectory
        $process.StartInfo.UseShellExecute = $false
        $process.StartInfo.RedirectStandardOutput = $true
        $process.StartInfo.RedirectStandardError = $true
        $process.StartInfo.RedirectStandardInput = $false
        $process.StartInfo.CreateNoWindow = $true

        $argumentsForWorker = [string[]]@($WorkerCommand.arguments)
        if (Get-Member -InputObject $process.StartInfo -Name "ArgumentList" -MemberType Property -ErrorAction SilentlyContinue) {
            foreach ($argument in $argumentsForWorker) {
                [void]$process.StartInfo.ArgumentList.Add([string]$argument)
            }
        }
        else {
            $process.StartInfo.Arguments = Format-CommandLine -Parts $argumentsForWorker
        }

        $workerEnvironment = [ordered]@{}
        if (Get-Member -InputObject $WorkerCommand -Name "environment" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
            if ($null -ne $WorkerCommand.environment) {
                $workerEnvironment = $WorkerCommand.environment
            }
        }

        foreach ($name in @($workerEnvironment.Keys)) {
            $process.StartInfo.EnvironmentVariables[[string]$name] = [string]$workerEnvironment[$name]
        }

        $useStdin = $false
        if (Get-Member -InputObject $WorkerCommand -Name "useStdin" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
            $useStdin = [bool]$WorkerCommand.useStdin
        }

        if ($useStdin) {
            $process.StartInfo.RedirectStandardInput = $true
        }

        if (-not $process.Start()) {
            throw "Worker process did not start: $($WorkerCommand.command)"
        }

        $stdoutTask = $process.StandardOutput.ReadToEndAsync()
        $stderrTask = $process.StandardError.ReadToEndAsync()

        if ($useStdin) {
            $promptContent = Get-Content -LiteralPath $WorkerCommand.promptPath -Raw
            $process.StandardInput.Write($promptContent)
            $process.StandardInput.Close()
        }

        $process.WaitForExit()
        $stdout = $stdoutTask.Result
        $stderr = $stderrTask.Result

        if (-not [string]::IsNullOrEmpty($stdout)) {
            foreach ($line in (Split-ProcessOutputLines -Value $stdout)) {
                Write-Host $line
                $workerOutput.Add($line)
            }
        }

        if (-not [string]::IsNullOrEmpty($stderr)) {
            foreach ($line in (Split-ProcessOutputLines -Value $stderr)) {
                [Console]::Error.WriteLine([string]$line)
                $workerOutput.Add([string]$line)
            }
        }

        return (New-Object PSObject -Property ([ordered]@{
            output = @($workerOutput.ToArray())
            exitCode = $process.ExitCode
        }))
    }
    finally {
        $process.Dispose()
    }
}

function Show-TaskRun {
    param(
        [string]$Slug,
        [bool]$Execute = $false,
        [string]$ProfileName,
        [bool]$Smoke = $false,
        [bool]$ForceProvider = $false
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

    if (-not (Test-Path -LiteralPath $task.worktreePath)) {
        Write-Host "Task worktree path does not exist for: $Slug" -ForegroundColor Red
        Write-Host "Expected path: $($task.worktreePath)"
        exit 1
    }

    if (-not (Test-Path -LiteralPath $task.promptPath)) {
        Write-Host "Task prompt file does not exist for: $Slug" -ForegroundColor Red
        Write-Host "Expected path: $($task.promptPath)"
        exit 1
    }

    $specPath = ""
    if (Get-Member -InputObject $task -Name "specPath" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $specPath = [string]$task.specPath
    }

    $resolvedProfile = $null
    $effectiveProfileName = $ProfileName
    if (-not [string]::IsNullOrWhiteSpace($ProfileName)) {
        try {
            $resolvedProfile = Resolve-BrevityProfileName -Name $ProfileName
            $effectiveProfileName = $resolvedProfile.CanonicalName
        }
        catch {
            Write-Host $_.Exception.Message -ForegroundColor Red
            exit 1
        }
    }

    Update-TaskPromptFromSpec -PromptPath $task.promptPath -Slug $Slug -SpecPath $specPath | Out-Null
    $contextFiles = @(Copy-TaskWorkspaceContext -WorktreePath $task.worktreePath)

    try {
        if ($Smoke) {
            $workerCommand = New-Object PSObject -Property ([ordered]@{
                provider = "smoke"
                command = "powershell"
                arguments = @(
                    "-NoProfile",
                    "-ExecutionPolicy",
                    "Bypass",
                    "-Command",
                    "Write-Output 'Brevity smoke OK'; Write-Output ((Get-Location).Path)"
                )
                executionPolicy = "Bypass"
                workingDirectory = $task.worktreePath
                environment = [ordered]@{}
                display = "powershell -NoProfile -ExecutionPolicy Bypass -Command <brevity-smoke>"
            })
        }
        else {
            $workerCommand = New-WorkerCommand -Config $config -WorktreePath $task.worktreePath -ProfileName $effectiveProfileName
        }
    }
    catch {
        Write-Host $_.Exception.Message -ForegroundColor Red
        exit 1
    }
    $providerHealth = Read-ProviderHealth
    $health = $providerHealth.health
    $providerName = ([string]$workerCommand.provider).ToLowerInvariant()
    $providerStatus = "unknown"
    $providerNote = ""

    if (Get-Member -InputObject $health -Name $providerName -MemberType NoteProperty -ErrorAction SilentlyContinue) {
        $providerState = $health.$providerName
        $providerStatus = [string]$providerState.status
        $providerNote = [string]$providerState.note

        if (-not [string]::IsNullOrWhiteSpace($providerStatus) -and
            $providerStatus -ne "healthy" -and
            $providerStatus -ne "unknown") {

            Write-Host "Warning: provider '$providerName' is currently $providerStatus." -ForegroundColor Yellow

            if (-not [string]::IsNullOrWhiteSpace($providerNote)) {
                Write-Host "Provider note: $providerNote" -ForegroundColor Gray
            }

            $suggestedProfile = Get-PreferredHealthyProfile -Health $health -CurrentProvider $providerName
            if (-not [string]::IsNullOrWhiteSpace($suggestedProfile)) {
                Write-Host "Suggested alternative profile: $suggestedProfile" -ForegroundColor Gray
            }
        }
    }
    if ($providerStatus -eq "unavailable") {
        Write-Host ""
        Write-Host "Provider '$providerName' is currently unavailable." -ForegroundColor Red

        if (-not $ForceProvider) {
            Write-Host "Execution blocked to avoid immediate worker failure." -ForegroundColor Red
            Write-Host "Use a different profile, reset provider state, or pass --force-provider." -ForegroundColor Gray
            exit 1
        }

        Write-Host "Provider gate overridden with --force-provider." -ForegroundColor Yellow
    }
    Write-Host "Task: $($task.slug)"
    Write-Host "Worktree: $($task.worktreePath)"
    Write-Host "Prompt: $($task.promptPath)"
    Write-Host "Context: $(Join-Path $task.worktreePath ".brevity\context") ($($contextFiles.Count) file(s))"
    if ($null -ne $resolvedProfile -and $resolvedProfile.IsAlias) {
        Write-Host "Resolved profile alias: $($resolvedProfile.AliasName) -> $($resolvedProfile.CanonicalName)"
    }
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
        $workerEnvironment = [ordered]@{}
        if (Get-Member -InputObject $workerCommand -Name "environment" -MemberType NoteProperty -ErrorAction SilentlyContinue) {
            if ($null -ne $workerCommand.environment) {
                $workerEnvironment = $workerCommand.environment
            }
        }

        foreach ($name in @($workerEnvironment.Keys)) {
            $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
            [Environment]::SetEnvironmentVariable($name, $workerEnvironment[$name], "Process")
        }

        Push-Location -LiteralPath $workerCommand.workingDirectory
        try {
            $workerResult = Invoke-WorkerCommand -WorkerCommand $workerCommand
            $workerOutput = @($workerResult.output)
            $exitCode = $workerResult.exitCode

            $logsRoot = Join-Path $repoRoot ".brevity\logs"
            $taskLogsRoot = Join-Path $logsRoot $Slug
            if (-not (Test-Path -LiteralPath $taskLogsRoot)) {
                New-Item -ItemType Directory -Path $taskLogsRoot -Force | Out-Null
            }

            $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
            $logPath = Join-Path $taskLogsRoot "$timestamp.log"

            $logLines = @(
                "Task: $Slug"
                "Provider: $($workerCommand.provider)"
                "Command: $($workerCommand.display)"
                "WorkingDirectory: $($workerCommand.workingDirectory)"
                "ExitCode: $exitCode"
                ""
                "Output:"
            ) + ($workerOutput | ForEach-Object { [string]$_ })

            $logLines | Set-Content -LiteralPath $logPath -Encoding UTF8
            Write-Host "Worker log: $logPath" -ForegroundColor Gray

            if ($exitCode -ne 0) {
                $renderedOutput = ($workerOutput | ForEach-Object { [string]$_ }) -join [Environment]::NewLine

                $failureKind = "worker-failed"
                $failureHint = "The worker process failed. Review the output above."

                if ($renderedOutput -match "QUOTA_EXHAUSTED|exhausted your capacity|usage limit|weekly limit|rate limit") {
                    $failureKind = "quota-constrained"
                    $failureHint = "Provider quota or rate limit was hit. Retry later or switch worker profile."
                }
                elseif ($renderedOutput -match "MODEL_CAPACITY_EXHAUSTED|model overloaded|capacity") {
                    $failureKind = "capacity-degraded"
                    $failureHint = "Provider/model capacity is degraded. Retry later or switch provider."
                }
                elseif ($renderedOutput -match "\[object Object\]") {
                    $failureKind = "provider-error-rendering"
                    $failureHint = "Provider CLI returned structured error details that were rendered poorly."
                }
                elseif ($renderedOutput -match "not recognized|command not found|not found|No such file or directory") {
                    $failureKind = "worker-command-unavailable"
                    $failureHint = "Worker executable or dependency was not found. Check provider config and PATH."
                }

                Write-Host ""
                Write-Host "Worker failed with exit code $exitCode." -ForegroundColor Yellow
                Write-Host "Failure kind: $failureKind" -ForegroundColor Yellow
                Write-Host $failureHint -ForegroundColor Gray
                Write-Host "Provider: $($workerCommand.provider)" -ForegroundColor Gray
                Write-Host "Command: $($workerCommand.command)" -ForegroundColor Gray

                if ($failureKind -eq "quota-constrained") {
                    Set-ProviderStatus `
                        -ProviderName $workerCommand.provider `
                        -Status "quota-constrained" `
                        -Note "Automatically detected from worker runtime failure."
                }
                elseif ($failureKind -eq "capacity-degraded") {
                    Set-ProviderStatus `
                        -ProviderName $workerCommand.provider `
                        -Status "capacity-degraded" `
                        -Note "Automatically detected from worker runtime failure."
                }
                elseif ($failureKind -eq "worker-command-unavailable") {
                    Set-ProviderStatus `
                        -ProviderName $workerCommand.provider `
                        -Status "unavailable" `
                        -Note "Worker executable was unavailable during runtime."
                }

                exit $exitCode
            }
            if ($exitCode -eq 0 -and $workerCommand.provider -ne "smoke") {
                Set-ProviderStatus `
                    -ProviderName $workerCommand.provider `
                    -Status "healthy" `
                    -Note "Automatically marked healthy after successful worker execution."
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

    Invoke-TaskMetadataLock -TasksPath $TasksPath -ScriptBlock {
        $currentTasks = @(Read-TaskMetadataFile -TasksPath $TasksPath)
        $remainingTasks = @($currentTasks | Where-Object { $_.slug -ne $Slug })
        Write-TaskMetadataFile -TasksPath $TasksPath -Tasks $remainingTasks
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

    $runtimeInfo = Get-TaskRuntimeInfo -Task $task

    if ([string]::IsNullOrWhiteSpace($runtimeInfo.worktree.path) -and -not $Force) {
        Write-Host "Task metadata is missing worktreePath for: $Slug" -ForegroundColor Red
        Write-Host "Use --force only if you want to remove metadata/branch state despite incomplete metadata." -ForegroundColor Yellow
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($runtimeInfo.branch)) {
        Write-Host "Task metadata is missing branch for: $Slug" -ForegroundColor Red
        exit 1
    }

    Write-Host "Cleaning up Brevity task: $Slug"
    Write-Host "Worktree: $($runtimeInfo.worktree.path)"
    Write-Host "Branch: $($runtimeInfo.branch)"
    if ($runtimeInfo.runtime.stale) {
        Write-Host "Runtime: stale" -ForegroundColor Yellow
        if ($runtimeInfo.runtime.issues.Count -gt 0) {
            Write-Host "Issues: $($runtimeInfo.runtime.issues -join ', ')" -ForegroundColor Yellow
        }
    }
    if ($Force) {
        Write-Host "Force: enabled"
    }

    $worktreeExists = $runtimeInfo.worktree.exists
    $worktreeRegistered = $runtimeInfo.worktree.registered

    if ((-not $worktreeExists) -or (-not $worktreeRegistered)) {
        Write-Host "Warning: recorded worktree is missing or not registered with Git: $($runtimeInfo.worktree.path)" -ForegroundColor Yellow
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

    Invoke-TaskMetadataLock -TasksPath $tasksPath -ScriptBlock {
        $currentTasks = @(Read-TaskMetadataFile -TasksPath $tasksPath)
        foreach ($taskRecord in $currentTasks) {
            if ($taskRecord.slug -eq $Slug) {
                $taskRecord.status = "merged"
            }
        }
        Write-TaskMetadataFile -TasksPath $tasksPath -Tasks $currentTasks
    }

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

    $contextFiles = @(Copy-TaskWorkspaceContext -WorktreePath $targetPath)
    $specPath = Update-TaskPromptFromSpec -PromptPath $promptPath -Slug $Slug
    Add-TaskMetadata -RepoRoot $repoRoot -Slug $Slug -Branch $branchName -WorktreePath $targetPath -PromptPath $promptPath -SpecPath $specPath

    Write-Host "Created task worktree"
    Write-Host "Path: $targetPath"
    Write-Host "Branch: $branchName"
    Write-Host "Prompt: $promptPath"
    Write-Host "Context: $(Join-Path $targetPath ".brevity\context") ($($contextFiles.Count) file(s))"
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

    $contextFiles = @(Copy-TaskWorkspaceContext -WorktreePath $worktreePath)
    Update-TaskPromptFromSpec -PromptPath $promptPath -Slug $Slug -SpecPath $specPath | Out-Null
    Add-TaskMetadata -RepoRoot $repoRoot -Slug $Slug -Branch $branchName -WorktreePath $worktreePath -PromptPath $promptPath -SpecPath $specPath

    Write-Host "Activated task worktree"
    Write-Host "Slug: $Slug"
    Write-Host "Path: $worktreePath"
    Write-Host "Branch: $branchName"
    Write-Host "Prompt: $promptPath"
    Write-Host "Context: $(Join-Path $worktreePath ".brevity\context") ($($contextFiles.Count) file(s))"
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
        elseif ($Subcommand.ToLowerInvariant() -eq "workers") {
            Show-WorkerPlan
        }
        elseif ($Subcommand.ToLowerInvariant() -eq "apply") {
            $plannerOutputPath = $null
            $createWorktrees = $false
            $startWorkers = $false

            if ($null -ne $RemainingArgs) {
                foreach ($planArg in $RemainingArgs) {
                    if ($planArg -eq "--create-worktrees") {
                        $createWorktrees = $true
                    }
                    elseif ($planArg -eq "--start") {
                        $createWorktrees = $true
                        $startWorkers = $true
                    }
                    elseif ([string]::IsNullOrWhiteSpace($plannerOutputPath)) {
                        $plannerOutputPath = [string]$planArg
                    }
                    else {
                        Write-Host "Unknown argument for brevity plan apply: $planArg" -ForegroundColor Red
                        Write-Host "Usage: .\brevity.ps1 plan apply <file> [--create-worktrees] [--start]"
                        exit 1
                    }
                }
            }

            Apply-PlannerOutput -Path $plannerOutputPath -CreateWorktrees $createWorktrees -StartWorkers $startWorkers
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
    "doctor" {
        $repair = $false
        $showExecutionPolicy = $false
        if (-not [string]::IsNullOrWhiteSpace($Subcommand)) {
            if ($Subcommand -eq "--repair") {
                $repair = $true
            }
            elseif ($Subcommand -eq "execution-policy") {
                $showExecutionPolicy = $true
            }
            else {
                Write-Host "Unknown brevity doctor argument: $Subcommand" -ForegroundColor Red
                Write-Host "Usage: .\brevity.ps1 doctor [--repair]"
                Write-Host "Usage: .\brevity.ps1 doctor execution-policy"
                exit 1
            }
        }

        if ($null -ne $RemainingArgs) {
            foreach ($doctorArg in $RemainingArgs) {
                if ($doctorArg -eq "--repair") {
                    $repair = $true
                }
                elseif ($doctorArg -eq "execution-policy") {
                    $showExecutionPolicy = $true
                }
                else {
                    Write-Host "Unknown brevity doctor argument: $doctorArg" -ForegroundColor Red
                    Write-Host "Usage: .\brevity.ps1 doctor [--repair]"
                    Write-Host "Usage: .\brevity.ps1 doctor execution-policy"
                    exit 1
                }
            }
        }

        if ($showExecutionPolicy) {
            Show-ExecutionPolicyGuidance
            exit 0
        }

        Show-DoctorReport -Repair $repair
    }
    "memory" {
        if ([string]::IsNullOrWhiteSpace($Subcommand)) {
            Write-Host "Missing brevity memory command." -ForegroundColor Red
            Write-Host "Usage: .\brevity.ps1 memory note <message>"
            exit 1
        }

        if ($Subcommand.ToLowerInvariant() -eq "note") {
            $message = ""
            if ($null -ne $RemainingArgs) {
                $message = ($RemainingArgs -join " ")
            }

            Add-MemoryNote -Message $message
        }
        else {
            Write-Host "Unknown brevity memory command: $Subcommand" -ForegroundColor Red
            Write-Host "Usage: .\brevity.ps1 memory note <message>"
            exit 1
        }
    }
    "logs" {
        if ([string]::IsNullOrWhiteSpace($Subcommand)) {
            Write-Host "Missing brevity logs command." -ForegroundColor Red
            Write-Host "Usage: .\brevity.ps1 logs recent [--count <n>]"
            Write-Host "Usage: .\brevity.ps1 logs task <slug> [--tail <n>]"
            exit 1
        }

        switch ($Subcommand.ToLowerInvariant()) {
            "recent" {
                $count = $null
                $index = 0
                while ($null -ne $RemainingArgs -and $index -lt $RemainingArgs.Length) {
                    $logsArg = [string]$RemainingArgs[$index]
                    if ($logsArg -eq "--count") {
                        if ($index + 1 -ge $RemainingArgs.Length) {
                            Write-Host "Missing value for --count." -ForegroundColor Red
                            Write-Host "Usage: .\brevity.ps1 logs recent [--count <n>]"
                            exit 1
                        }

                        $count = Resolve-PositiveIntegerOption -Name "--count" -Value ([string]$RemainingArgs[$index + 1]) -Usage "Usage: .\brevity.ps1 logs recent [--count <n>]"
                        $index += 2
                    }
                    else {
                        Write-Host "Unknown argument for brevity logs recent: $logsArg" -ForegroundColor Red
                        Write-Host "Usage: .\brevity.ps1 logs recent [--count <n>]"
                        exit 1
                    }
                }

                if ($null -eq $count) {
                    Show-RecentLogs
                }
                else {
                    Show-RecentLogs -RuntimeCount $count -WorkerCount $count
                }
            }
            "task" {
                $taskSlug = $null
                $tail = 20
                $index = 0
                while ($null -ne $RemainingArgs -and $index -lt $RemainingArgs.Length) {
                    $logsArg = [string]$RemainingArgs[$index]
                    if ($logsArg -eq "--tail") {
                        if ($index + 1 -ge $RemainingArgs.Length) {
                            Write-Host "Missing value for --tail." -ForegroundColor Red
                            Write-Host "Usage: .\brevity.ps1 logs task <slug> [--tail <n>]"
                            exit 1
                        }

                        $tail = Resolve-PositiveIntegerOption -Name "--tail" -Value ([string]$RemainingArgs[$index + 1]) -Usage "Usage: .\brevity.ps1 logs task <slug> [--tail <n>]"
                        $index += 2
                    }
                    elseif ([string]::IsNullOrWhiteSpace($taskSlug)) {
                        $taskSlug = $logsArg
                        $index++
                    }
                    else {
                        Write-Host "Unknown argument for brevity logs task: $logsArg" -ForegroundColor Red
                        Write-Host "Usage: .\brevity.ps1 logs task <slug> [--tail <n>]"
                        exit 1
                    }
                }

                Show-TaskLogs -Slug $taskSlug -Tail $tail
            }
            default {
                Write-Host "Unknown brevity logs command: $Subcommand" -ForegroundColor Red
                Write-Host "Usage: .\brevity.ps1 logs recent [--count <n>]"
                Write-Host "Usage: .\brevity.ps1 logs task <slug> [--tail <n>]"
                exit 1
            }
        }
    }
    "session" {
        if ([string]::IsNullOrWhiteSpace($Subcommand)) {
            Write-Host "Missing brevity session command." -ForegroundColor Red
            Write-Host "Usage: .\brevity.ps1 session summary [--json]"
            exit 1
        }

        if ($Subcommand.ToLowerInvariant() -eq "summary") {
            $json = $false
            if ($null -ne $RemainingArgs) {
                foreach ($sessionArg in $RemainingArgs) {
                    if ($sessionArg -eq "--json") {
                        $json = $true
                    }
                    else {
                        Write-Host "Unknown brevity session summary argument: $sessionArg" -ForegroundColor Red
                        Write-Host "Usage: .\brevity.ps1 session summary [--json]"
                        exit 1
                    }
                }
            }

            Show-SessionSummary -Json:$json
        }
        else {
            Write-Host "Unknown brevity session command: $Subcommand" -ForegroundColor Red
            Write-Host "Usage: .\brevity.ps1 session summary [--json]"
            exit 1
        }
    }
    "onboard" {
        Write-NotImplemented "onboard"
    }
    "provider" {
        if ([string]::IsNullOrWhiteSpace($Subcommand)) {
            Write-Host "Missing brevity provider command." -ForegroundColor Red
            Write-Host "Usage: .\brevity.ps1 provider status"
            Write-Host "Usage: .\brevity.ps1 provider docs"
            Write-Host "Usage: .\brevity.ps1 provider profiles [--profile <name>] [--json]"
            Write-Host "Usage: .\brevity.ps1 provider set <provider> <status> [-Note <note>]"
            exit 1
        }

        switch ($Subcommand.ToLowerInvariant()) {
            "status" { Show-ProviderStatus }
            "docs" { Show-ProviderDocs }
            "profiles" {
                $profileName = ""
                $jsonOutput = $false
                if ($null -ne $RemainingArgs) {
                    for ($i = 0; $i -lt $RemainingArgs.Length; $i++) {
                        $profileArg = $RemainingArgs[$i]
                        if ($profileArg -eq "--profile") {
                            if ($i + 1 -lt $RemainingArgs.Length) {
                                $profileName = [string]$RemainingArgs[$i + 1]
                                $i++
                            }
                            else {
                                Write-Host "Missing value for --profile." -ForegroundColor Red
                                exit 1
                            }
                        }
                        elseif ($profileArg -eq "--json") {
                            $jsonOutput = $true
                        }
                        else {
                            Write-Host "Unknown argument for brevity provider profiles: $profileArg" -ForegroundColor Red
                            Write-Host "Usage: .\brevity.ps1 provider profiles [--profile <name>] [--json]"
                            exit 1
                        }
                    }
                }

                Show-ProviderProfiles -ProfileName $profileName -Json $jsonOutput
            }
            "reset" {
                $providerName = $null

                if ($null -ne $RemainingArgs -and $RemainingArgs.Length -gt 0) {
                    $providerName = [string]$RemainingArgs[0]
                }

                if ([string]::IsNullOrWhiteSpace($providerName)) {
                    Write-Host "Missing provider name." -ForegroundColor Red
                    Write-Host "Usage: .\brevity.ps1 provider reset <provider>"
                    exit 1
                }

                Reset-ProviderStatus -ProviderName $providerName
            }
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
                $smokeTask = $false
                $forceProvider = $false
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
                        elseif ($arg -eq "--smoke") {
                            $smokeTask = $true
                        }
                        elseif ($arg -eq "--force-provider") {
                            $forceProvider = $true
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
                            Write-Host "Usage: .\brevity.ps1 task run <slug> [--execute] [--profile <name>] [--smoke]"
                            exit 1
                        }
                    }
                }

                Show-TaskRun -Slug $taskSlug -Execute $executeTask -ProfileName $profileName -Smoke $smokeTask -ForceProvider $forceProvider
            }
            "status" { Show-TaskStatus }
            "runtime-info" {
                $taskSlug = $null
                if ($null -ne $RemainingArgs) {
                    foreach ($taskArg in $RemainingArgs) {
                        $taskSlug = [string]$taskArg
                        break
                    }
                }

                Show-TaskRuntimeInfoCommand -Slug $taskSlug
            }
            "context" {
                $contextCommand = $null
                $taskSlug = $null
                if ($null -ne $RemainingArgs) {
                    if ($RemainingArgs.Length -ge 1) {
                        $contextCommand = [string]$RemainingArgs[0]
                    }
                    if ($RemainingArgs.Length -ge 2) {
                        $taskSlug = [string]$RemainingArgs[1]
                    }
                    if ($RemainingArgs.Length -gt 2) {
                        Write-Host "Unknown argument for brevity task context: $($RemainingArgs[2])" -ForegroundColor Red
                        Write-Host "Usage: .\brevity.ps1 task context refresh <slug>"
                        Write-Host "Usage: .\brevity.ps1 task context status <slug>"
                        exit 1
                    }
                }

                if ([string]::IsNullOrWhiteSpace($contextCommand)) {
                    Write-Host "Missing task context command." -ForegroundColor Red
                    Write-Host "Usage: .\brevity.ps1 task context refresh <slug>"
                    Write-Host "Usage: .\brevity.ps1 task context status <slug>"
                    exit 1
                }

                switch ($contextCommand.ToLowerInvariant()) {
                    "refresh" { Refresh-TaskContext -Slug $taskSlug }
                    "status" { Show-TaskContextStatus -Slug $taskSlug }
                    default {
                        Write-Host "Unknown brevity task context command: $contextCommand" -ForegroundColor Red
                        Write-Host "Usage: .\brevity.ps1 task context refresh <slug>"
                        Write-Host "Usage: .\brevity.ps1 task context status <slug>"
                        exit 1
                    }
                }
            }
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
