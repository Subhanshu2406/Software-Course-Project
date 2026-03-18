$ErrorActionPreference = "Stop"

$repoRoot = Split-Path $PSScriptRoot -Parent

Write-Host "Starting local ledger stack in separate PowerShell windows..." -ForegroundColor Cyan

$kafkaProbe = Test-NetConnection -ComputerName localhost -Port 9092 -WarningAction SilentlyContinue
if (-not $kafkaProbe.TcpTestSucceeded) {
    Write-Host "Kafka is not reachable on localhost:9092. /submit may return errors until Kafka is up." -ForegroundColor Yellow
}

Start-Process powershell -ArgumentList @(
    "-NoExit",
    "-Command",
    "Set-Location '$repoRoot'; go run .\cmd\shard"
)

Start-Process powershell -ArgumentList @(
    "-NoExit",
    "-Command",
    "Set-Location '$repoRoot'; go run .\cmd\coordinator"
)

Start-Process powershell -ArgumentList @(
    "-NoExit",
    "-Command",
    "Set-Location '$repoRoot'; go run .\cmd\api"
)

Write-Host "Launched shard (:8081), coordinator (:8080), and API gateway (:8000)." -ForegroundColor Green
Write-Host "Now run: .\run_k6.ps1 -BaseUrl http://localhost:8000 -AuthToken <valid-jwt>" -ForegroundColor Cyan
