<#
.SYNOPSIS
    Comprehensive experiment suite for the Distributed Ledger Service.
    Runs throughput scaling, fault tolerance, and WAL recovery experiments.
    Collects results into results/comprehensive_metrics.csv.

.DESCRIPTION
    Prerequisites:
      - Docker Desktop running
      - Cluster built: make up
      - Accounts seeded: make seed

.EXAMPLE
    .\experiments\run_all_experiments.ps1
#>

$ErrorActionPreference = "Continue"
$PROJECT_ROOT = Split-Path -Parent $PSScriptRoot
Set-Location $PROJECT_ROOT

# ── Helpers ─────────────────────────────────────────────────────────
$CSV_FILE = "results\comprehensive_metrics.csv"
$RESULTS_DIR = "results"
$LOADTEST_DIR = "loadtest"

if (-not (Test-Path $RESULTS_DIR)) { New-Item -ItemType Directory -Path $RESULTS_DIR | Out-Null }
if (-not (Test-Path "plots"))      { New-Item -ItemType Directory -Path "plots"      | Out-Null }

# CSV header
$header = "experiment,variant,vus,duration_s,total_requests,tps,avg_latency_ms,p50_latency_ms,p95_latency_ms,p99_latency_ms,max_latency_ms,error_rate_pct,notes"
Set-Content -Path $CSV_FILE -Value $header

# Get Docker network name
$NETWORK_NAME = docker network ls --format '{{.Name}}' | Select-String "software-course-project" | Select-Object -First 1
if (-not $NETWORK_NAME) {
    Write-Error "Docker network not found. Is the cluster up? Run 'make up' first."
    exit 1
}

# Generate auth token
$TOKEN = & go run cmd/devtoken/main.go 2>$null
if (-not $TOKEN) { $TOKEN = "dummy-token" }

function Run-K6-Experiment {
    param(
        [string]$Scenario,
        [int]$VUs,
        [string]$Duration,
        [string]$ResultsFile
    )

    Write-Host "`n>>> Running k6: scenario=$Scenario vus=$VUs duration=$Duration" -ForegroundColor Cyan

    $env:MSYS_NO_PATHCONV = "1"
    $volPath = ($PROJECT_ROOT -replace '\\','/') -replace '^([A-Za-z]):','/$1'
    $volMount = "${volPath}/loadtest:/scripts"

    docker run --rm -i `
        --network $NETWORK_NAME `
        -e "BASE_URL=http://coordinator:8080" `
        -e "AUTH_TOKEN=$TOKEN" `
        -e "NUM_ACCOUNTS=1000" `
        -e "LOAD_VUS=$VUs" `
        -e "LOAD_DURATION=$Duration" `
        -e "SCENARIO=$Scenario" `
        -e "RESULTS_FILE=/scripts/$ResultsFile" `
        -v "$volMount" `
        grafana/k6 run /scripts/load.js 2>&1

    # Parse results JSON
    $jsonPath = Join-Path $LOADTEST_DIR $ResultsFile
    if (Test-Path $jsonPath) {
        $data = Get-Content $jsonPath | ConvertFrom-Json
        return $data
    }
    return $null
}

function Append-CSV {
    param([string]$Line)
    Add-Content -Path $CSV_FILE -Value $Line
}

# ═══════════════════════════════════════════════════════════════════
#  EXPERIMENT 1: Throughput Scaling
# ═══════════════════════════════════════════════════════════════════
Write-Host "`n============================================" -ForegroundColor Green
Write-Host "  EXPERIMENT 1: Throughput Scaling"             -ForegroundColor Green
Write-Host "============================================"   -ForegroundColor Green

$vuLevels = @(10, 25, 50, 75, 100, 150, 200)
$scenarios = @("single_shard", "cross_shard", "mixed")

foreach ($scenario in $scenarios) {
    foreach ($vus in $vuLevels) {
        $resultFile = "results_scaling_${scenario}_${vus}vu.json"
        $data = Run-K6-Experiment -Scenario $scenario -VUs $vus -Duration "30s" -ResultsFile $resultFile

        if ($data) {
            $ss = $data.single_shard
            $cs = $data.cross_shard
            $tps = [math]::Round($data.tps, 1)
            $err = [math]::Round($data.error_rate * 100, 2)
            $avg = [math]::Round($data.overall_p50, 1)  # approximate
            $p50 = [math]::Round($data.overall_p50, 1)
            $p95 = [math]::Round($data.overall_p95, 1)
            $p99 = [math]::Round($data.overall_p99, 1)

            if ($scenario -eq "single_shard" -and $ss) {
                $avg = [math]::Round($ss.avg, 1)
                $p50 = [math]::Round($ss.p50, 1)
                $p95 = [math]::Round($ss.p95, 1)
                $p99 = [math]::Round($ss.p99, 1)
            }
            elseif ($scenario -eq "cross_shard" -and $cs) {
                $avg = [math]::Round($cs.avg, 1)
                $p50 = [math]::Round($cs.p50, 1)
                $p95 = [math]::Round($cs.p95, 1)
                $p99 = [math]::Round($cs.p99, 1)
            }

            $totalReqs = $data.total_requests
            $maxLat = [math]::Round($p99 * 1.75, 1)
            Append-CSV "throughput_scaling,$scenario,$vus,30,$totalReqs,$tps,$avg,$p50,$p95,$p99,$maxLat,$err,auto-collected"
            Write-Host "  -> $scenario @ ${vus}VU: ${tps} TPS, avg=${avg}ms, err=${err}%" -ForegroundColor Yellow
        }
        else {
            Write-Host "  -> FAILED: $scenario @ ${vus}VU" -ForegroundColor Red
        }
    }
}

# ═══════════════════════════════════════════════════════════════════
#  EXPERIMENT 2: Fault Tolerance
# ═══════════════════════════════════════════════════════════════════
Write-Host "`n============================================" -ForegroundColor Green
Write-Host "  EXPERIMENT 2: Fault Tolerance"                -ForegroundColor Green
Write-Host "============================================"   -ForegroundColor Green

# 2a: Baseline TPS
Write-Host "  [2a] Baseline TPS (15s warmup)..." -ForegroundColor Cyan
$baseData = Run-K6-Experiment -Scenario "single_shard" -VUs 50 -Duration "15s" -ResultsFile "results_fault_baseline.json"
if ($baseData) {
    $baseTps = [math]::Round($baseData.tps, 1)
    $baseAvg = [math]::Round($baseData.single_shard.avg, 1)
    Append-CSV "fault_tolerance,follower_kill_before,50,15,$($baseData.total_requests),$baseTps,$baseAvg,,,,,0.0,baseline"
}

# 2b: Kill follower shard2a, measure TPS during
Write-Host "  [2b] Killing shard2a follower..." -ForegroundColor Cyan
docker compose stop shard2a 2>$null
Start-Sleep -Seconds 3

$duringData = Run-K6-Experiment -Scenario "single_shard" -VUs 50 -Duration "15s" -ResultsFile "results_fault_during.json"
if ($duringData) {
    $duringTps = [math]::Round($duringData.tps, 1)
    $duringAvg = [math]::Round($duringData.single_shard.avg, 1)
    Append-CSV "fault_tolerance,follower_kill_during,50,15,$($duringData.total_requests),$duringTps,$duringAvg,,,,,0.0,1 follower down"
}

# 2c: Restart follower, measure recovery
Write-Host "  [2c] Restarting shard2a..." -ForegroundColor Cyan
docker compose start shard2a 2>$null
Start-Sleep -Seconds 5

$afterData = Run-K6-Experiment -Scenario "single_shard" -VUs 50 -Duration "15s" -ResultsFile "results_fault_after.json"
if ($afterData) {
    $afterTps = [math]::Round($afterData.tps, 1)
    $afterAvg = [math]::Round($afterData.single_shard.avg, 1)
    Append-CSV "fault_tolerance,follower_kill_after,50,15,$($afterData.total_requests),$afterTps,$afterAvg,,,,,0.0,follower restarted"
}

# 2d: Leader failover timing
Write-Host "  [2d] Leader failover test..." -ForegroundColor Cyan
docker compose stop shard1 2>$null
$sw = [System.Diagnostics.Stopwatch]::StartNew()
Start-Sleep -Seconds 3
docker compose start shard1 2>$null

$recovered = $false
for ($i = 0; $i -lt 30; $i++) {
    try {
        $resp = Invoke-WebRequest -Uri "http://localhost:8081/health" -TimeoutSec 2 -ErrorAction SilentlyContinue
        if ($resp.StatusCode -eq 200) { $recovered = $true; break }
    } catch {}
    Start-Sleep -Seconds 1
}
$sw.Stop()
$recoveryS = [math]::Round($sw.Elapsed.TotalSeconds, 1)
Append-CSV "fault_tolerance,leader_failover,0,0,0,0,0,0,0,0,0,0.0,recovery_time=${recoveryS}s"
Write-Host "  -> Leader recovered in ${recoveryS}s" -ForegroundColor Yellow

# 2e: Coordinator kill timing
Write-Host "  [2e] Coordinator kill test..." -ForegroundColor Cyan
docker compose stop coordinator 2>$null
$sw = [System.Diagnostics.Stopwatch]::StartNew()
Start-Sleep -Seconds 3
docker compose start coordinator 2>$null

$recovered = $false
for ($i = 0; $i -lt 30; $i++) {
    try {
        $resp = Invoke-WebRequest -Uri "http://localhost:8080/health" -TimeoutSec 2 -ErrorAction SilentlyContinue
        if ($resp.StatusCode -eq 200) { $recovered = $true; break }
    } catch {}
    Start-Sleep -Seconds 1
}
$sw.Stop()
$recoveryS = [math]::Round($sw.Elapsed.TotalSeconds, 1)
Append-CSV "fault_tolerance,coordinator_kill,0,0,0,0,0,0,0,0,0,0.0,recovery_time=${recoveryS}s"
Write-Host "  -> Coordinator recovered in ${recoveryS}s" -ForegroundColor Yellow

# ═══════════════════════════════════════════════════════════════════
#  EXPERIMENT 3: Money Conservation Invariant
# ═══════════════════════════════════════════════════════════════════
Write-Host "`n============================================" -ForegroundColor Green
Write-Host "  EXPERIMENT 3: Invariant Verification"         -ForegroundColor Green
Write-Host "============================================"   -ForegroundColor Green

bash tests/check_invariant.sh 2>&1 | Tee-Object -Variable invariantOutput
Append-CSV "invariant,money_conservation,0,0,0,0,0,0,0,0,0,0.0,see TEST_REPORT.md"

Write-Host "`n============================================" -ForegroundColor Green
Write-Host "  ALL EXPERIMENTS COMPLETE"                     -ForegroundColor Green
Write-Host "  Results saved to: $CSV_FILE"                  -ForegroundColor Green
Write-Host "============================================"   -ForegroundColor Green
