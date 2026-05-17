param(
    [Parameter(Mandatory=$true)]
    [string]$PromptPath,

    [string]$Model = "deepseek-ai/deepseek-v4-flash",

    [string]$BaseUrl = "https://integrate.api.nvidia.com/v1"
)

$apiKey = $env:NVIDIA_API_KEY
if ([string]::IsNullOrWhiteSpace($apiKey)) {
    throw "NVIDIA_API_KEY is not set."
}

if (-not (Test-Path -LiteralPath $PromptPath)) {
    throw "Prompt not found: $PromptPath"
}

$prompt = Get-Content -LiteralPath $PromptPath -Raw

$headers = @{
    Authorization = "Bearer $apiKey"
    "Content-Type" = "application/json"
}

$body = @{
    model = $Model
    messages = @(
        @{
            role = "user"
            content = $prompt
        }
    )
    temperature = 0
    max_tokens = 4096
} | ConvertTo-Json -Depth 20

$response = Invoke-RestMethod `
    -Uri ($BaseUrl.TrimEnd("/") + "/chat/completions") `
    -Method Post `
    -Headers $headers `
    -Body $body

$response.choices[0].message.content
