$ErrorActionPreference = "Stop"

$files = @(
    "README.md",
    "docs/concepts.md",
    "docs/architecture.md",
    "docs/roadmap.md",
    "goal.md",
    "architecture.md",
    "roadmap.md",
    "project.md"
)

foreach ($file in $files) {
    if (Test-Path $file) {
        $text = Get-Content $file -Raw

        $text = $text -replace '\bLane\b', 'Brevity'
        $text = $text -replace '\blane\b', 'brevity'

        # Preserve runtime compatibility names.
        $text = $text -replace '\.brevity/', '.lane/'
        $text = $text -replace '\.brevity\\', '.lane\'
        $text = $text -replace 'brevity\.ps1', 'lane.ps1'
        $text = $text -replace 'Brevity\.ps1', 'lane.ps1'

        Set-Content -Path $file -Value $text -NoNewline
        Write-Host "Updated $file"
    }
}

Write-Host ""
Write-Host "Rename pass complete."
Write-Host "Review git diff carefully before committing."