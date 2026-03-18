param(
    [string]$BaseUrl = "http://localhost:8000",
    [string]$AuthToken = "dummy-token"
)

$ErrorActionPreference = "Stop"

Set-Location $PSScriptRoot

if (-not (Get-Command k6 -ErrorAction SilentlyContinue)) {
    Write-Host "k6 is not installed or not in PATH." -ForegroundColor Red
    Write-Host "Install on Windows (choose one):" -ForegroundColor Yellow
    Write-Host "  winget install k6.k6"
    Write-Host "  choco install k6"
    exit 1
}

Write-Host "Running k6 load test on API Gateway..." -ForegroundColor Cyan
$env:BASE_URL = $BaseUrl
$env:AUTH_TOKEN = $AuthToken

$uri = [System.Uri]$BaseUrl
$port = if ($uri.IsDefaultPort) { 80 } else { $uri.Port }
$probe = Test-NetConnection -ComputerName $uri.Host -Port $port -WarningAction SilentlyContinue
if (-not $probe.TcpTestSucceeded) {
    Write-Host "API endpoint is not reachable at $BaseUrl (TCP $($uri.Host):$port)." -ForegroundColor Red
    Write-Host "Start services first, for example in separate terminals:" -ForegroundColor Yellow
    Write-Host "  go run .\cmd\shard"
    Write-Host "  go run .\cmd\coordinator"
    Write-Host "  go run .\cmd\api"
    Write-Host "Also ensure Kafka is running on localhost:9092 before load tests." -ForegroundColor Yellow
    exit 1
}

k6 run .\load.js
