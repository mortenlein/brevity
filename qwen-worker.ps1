param([Parameter(Mandatory=$true)][string]$PromptPath,[string]$Model="qwen/qwen3-next-80b-a3b-thinking",[int]$TimeoutSec=60,[int]$MaxTokens=1200)
if([string]::IsNullOrWhiteSpace($env:NVIDIA_API_KEY)){throw "NVIDIA_API_KEY is not set"}
if(-not(Test-Path -LiteralPath $PromptPath)){throw "Prompt not found: $PromptPath"}
$prompt=Get-Content -LiteralPath $PromptPath -Raw
$response=$null
try{
$response=Invoke-RestMethod -Uri "https://integrate.api.nvidia.com/v1/chat/completions" -Method Post -Headers @{Authorization="Bearer $env:NVIDIA_API_KEY";"Content-Type"="application/json"} -Body (@{model=$Model;messages=@(@{role="user";content=$prompt});temperature=0;max_tokens=$MaxTokens}|ConvertTo-Json -Depth 10) -TimeoutSec $TimeoutSec
$response.choices[0].message.content
exit 0
}catch{
Write-Host "Qwen worker failed: $($_.Exception.Message)" -ForegroundColor Yellow
exit 1
}
