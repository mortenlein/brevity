$ErrorActionPreference = "Stop"

$repo = "C:\dev\repos\active\lane"
Set-Location -LiteralPath $repo

if (-not (Test-Path -LiteralPath ".\lane.ps1")) {
    throw "Expected lane.ps1 in $repo"
}

# Text replacements in tracked project files only.
$files = @(
    ".gitignore",
    "AGENTS.md",
    "README.md",
    "docs\concepts.md",
    "lane.ps1"
)

foreach ($file in $files) {
    if (-not (Test-Path -LiteralPath $file)) { continue }

    $text = Get-Content -LiteralPath $file -Raw

    $text = $text.Replace("lane.ps1", "brevity.ps1")
    $text = $text.Replace(".\lane.ps1", ".\brevity.ps1")
    $text = $text.Replace("Lane", "Brevity")
    $text = $text.Replace("lane ", "brevity ")
    $text = $text.Replace("lane\", "brevity\")
    $text = $text.Replace("lane/", "brevity/")
    $text = $text.Replace("lane-", "brevity-")

    Set-Content -LiteralPath $file -Value $text -Encoding UTF8
}

# Config JSON: project identity only. Keep .lane directory unchanged for this phase.
$configPath = ".\.lane\config.json"
$config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
$config.projectName = "brevity"
$config.vaultPath = "C:\dev\vaults\AI-Vault\10-Projects\brevity"
$config | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $configPath -Encoding UTF8

# Rename script entrypoint.
if (Test-Path -LiteralPath ".\brevity.ps1") {
    throw "brevity.ps1 already exists"
}

git mv lane.ps1 brevity.ps1

Write-Host "Done. Review with:"
Write-Host "  git diff"
Write-Host "  git grep -n `"lane`""
Write-Host "  git grep -n `"Lane`""
