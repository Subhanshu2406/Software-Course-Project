param(
    [string]$BaseUrl = "http://localhost:8000",
    [string]$AuthToken = "dummy-token"
)

$ErrorActionPreference = "Stop"

Set-Location $PSScriptRoot

if (-not (Get-Command vegeta -ErrorAction SilentlyContinue)) {
    Write-Host "vegeta is not installed or not in PATH." -ForegroundColor Red
    Write-Host "Install on Windows (choose one):" -ForegroundColor Yellow
    Write-Host "  winget install tsenart.vegeta"
    Write-Host "  choco install vegeta"
    exit 1
}

$targetFile = Join-Path $PSScriptRoot "targets.json"
$resultFile = Join-Path $PSScriptRoot "results.bin"

if (Test-Path $targetFile) { Remove-Item $targetFile -Force }
if (Test-Path $resultFile) { Remove-Item $resultFile -Force }

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

Write-Host "Generating $targetFile ..." -ForegroundColor Cyan

for ($i = 1; $i -le 10000; $i++) {
    $src = "user$([int](Get-Random -Minimum 0 -Maximum 100))"
    $dst = "user$([int](Get-Random -Minimum 0 -Maximum 100))"
    $amt = [int](Get-Random -Minimum 1 -Maximum 101)
    $txnid = "txn-v-$i-$([int](Get-Random -Minimum 10000 -Maximum 99999))"

    $payloadObj = @{
        TxnID       = $txnid
        Source      = $src
        Destination = $dst
        Amount      = $amt
    }

    $jsonBody = $payloadObj | ConvertTo-Json -Compress
    $bodyB64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($jsonBody))

    $targetObj = @{
        method = "POST"
        url    = "$BaseUrl/submit"
        header = @{
            "Content-Type" = @("application/json")
            Authorization   = @("Bearer $AuthToken")
        }
        body = $bodyB64
    }

    $line = $targetObj | ConvertTo-Json -Compress
    Add-Content -Path $targetFile -Value $line
}

Write-Host "Running Vegeta attack at 500 req/s for 30s..." -ForegroundColor Cyan
vegeta attack -format=json -rate=500 -duration=30s -targets=$targetFile | Tee-Object -FilePath $resultFile | vegeta report

Write-Host "" 
Write-Host "Latency histogram:" -ForegroundColor Cyan
vegeta report -type=hist[0,50ms,100ms,200ms,500ms,1s] $resultFile
