# Windows PowerShell Makefile alternative
# Use as dot-sourcing: . make.ps1; make-up
# Or directly: powershell -File make.ps1 up
# Or via batch: make.bat up

param([string]$Command)

function make-build {
    Write-Host "Building Docker images..." -ForegroundColor Green
    docker compose build
}

function make-up {
    Write-Host "Generating shard map..." -ForegroundColor Green
    & "$PSScriptRoot/scripts/gen_shard_map.ps1"
    
    Write-Host "Cleaning up previous cluster..." -ForegroundColor Green
    docker compose down -v --remove-orphans 2>$null
    
    Write-Host "Building images..." -ForegroundColor Green
    docker compose build
    
    Write-Host "Starting cluster..." -ForegroundColor Green
    docker compose up -d
    
    Write-Host "Waiting for services to become healthy..." -ForegroundColor Green
    $maxAttempts = 60
    $attempt = 0
    while ($attempt -lt $maxAttempts) {
        $healthy = docker compose ps --format json | ConvertFrom-Json | Where-Object { $_.Health -eq "healthy" } | Measure-Object | Select-Object -ExpandProperty Count
        $total = docker compose ps --format json | ConvertFrom-Json | Measure-Object | Select-Object -ExpandProperty Count
        
        if ($healthy -eq $total -and $total -gt 0) {
            Write-Host "All $total services healthy." -ForegroundColor Green
            docker compose ps
            return
        }
        
        Write-Host "  Healthy: $healthy / $total..."
        Start-Sleep -Seconds 2
        $attempt++
    }
    Write-Host "Timeout waiting for services" -ForegroundColor Red
}

function make-down {
    Write-Host "Stopping cluster..." -ForegroundColor Green
    docker compose down -v --remove-orphans
}

function make-clean {
    Write-Host "Full cleanup..." -ForegroundColor Green
    docker compose down -v --remove-orphans --rmi local 2>$null
    Remove-Item -Path "loadtest\results*.json" -Force -ErrorAction SilentlyContinue
    Remove-Item -Path "data\", "logs\" -Recurse -Force -ErrorAction SilentlyContinue
}

function make-seed {
    Write-Host "Seeding accounts..." -ForegroundColor Green
    & bash tests/seed_accounts.sh
}

function make-token {
    Write-Host "Generating JWT token..." -ForegroundColor Green
    go run cmd/devtoken/main.go
}

function make-token-set {
    Write-Host "Generating token and instructions..." -ForegroundColor Green
    $token = go run cmd/devtoken/main.go 2>$null
    Write-Host "Token: $token" -ForegroundColor Cyan
    Write-Host "Set in browser console:" -ForegroundColor Yellow
    Write-Host "  localStorage.setItem('ledger_token', '$token')" -ForegroundColor Gray
}

function make-open {
    Write-Host "Opening frontend at http://localhost:3000" -ForegroundColor Green
    Start-Process "http://localhost:3000"
}

function make-open-grafana {
    Write-Host "Opening Grafana at http://localhost:3001" -ForegroundColor Green
    Start-Process "http://localhost:3001"
}

function make-open-prometheus {
    Write-Host "Opening Prometheus at http://localhost:9090" -ForegroundColor Green
    Start-Process "http://localhost:9090"
}

function make-frontend-dev {
    Write-Host "Starting frontend dev server..." -ForegroundColor Green
    Push-Location frontend
    npm install
    npm run dev
    Pop-Location
}

function make-frontend-build {
    Write-Host "Building frontend..." -ForegroundColor Green
    Push-Location frontend
    npm install
    npm run build
    Pop-Location
}

function make-unit-test {
    Write-Host "Running unit tests..." -ForegroundColor Green
    go test ./tests/unit/... -v -count=1
}

function make-test-health {
    Write-Host "=== T1: Health Check ===" -ForegroundColor Green
    $endpoints = @(
        "http://localhost:8000/health",
        "http://localhost:8080/health",
        "http://localhost:8081/health",
        "http://localhost:8082/health",
        "http://localhost:8083/health",
        "http://localhost:9081/health",
        "http://localhost:9082/health",
        "http://localhost:9083/health",
        "http://localhost:9084/health",
        "http://localhost:9085/health",
        "http://localhost:9086/health",
        "http://localhost:8090/health"
    )
    
    $pass = 0
    $fail = 0
    
    foreach ($url in $endpoints) {
        try {
            $response = Invoke-WebRequest -Uri $url -TimeoutSec 3 -ErrorAction Stop
            if ($response.StatusCode -eq 200) {
                $pass++
            } else {
                Write-Host "FAIL: $url ($($response.StatusCode))" -ForegroundColor Red
                $fail++
            }
        } catch {
            Write-Host "FAIL: $url (timeout/error)" -ForegroundColor Red
            $fail++
        }
    }
    
    Write-Host "PASS=$pass  FAIL=$fail" -ForegroundColor Cyan
    if ($fail -eq 0) {
        Write-Host "[PASS] T1 Health" -ForegroundColor Green
    } else {
        Write-Host "[FAIL] T1 Health" -ForegroundColor Red
        exit 1
    }
}

# Help
function make-help {
    Write-Host "Available targets:" -ForegroundColor Green
    Write-Host "  make-build              Build Docker images"
    Write-Host "  make-up                 Start the cluster"
    Write-Host "  make-down               Stop the cluster"
    Write-Host "  make-clean              Full cleanup"
    Write-Host "  make-seed               Seed accounts"
    Write-Host "  make-token              Generate JWT token"
    Write-Host "  make-token-set          Generate token + instructions"
    Write-Host "  make-open               Open frontend"
    Write-Host "  make-open-grafana       Open Grafana"
    Write-Host "  make-open-prometheus    Open Prometheus"
    Write-Host "  make-frontend-dev       Dev server with hot reload"
    Write-Host "  make-frontend-build     Build frontend"
    Write-Host "  make-unit-test          Run unit tests"
    Write-Host "  make-test-health        Test all health endpoints"
    Write-Host ""
    Write-Host "Usage:"
    Write-Host "  Dot-source: . make.ps1; make-up"
    Write-Host "  Direct:     powershell -File make.ps1 up"
    Write-Host "  Via batch:  make.bat up"
}

# Command dispatch
if ($Command) {
    $functionName = "make-$Command"
    if (Get-Command $functionName -ErrorAction SilentlyContinue) {
        & $functionName
    } else {
        Write-Host "Unknown command: $Command" -ForegroundColor Red
        Write-Host "Run 'powershell -File make.ps1' for help" -ForegroundColor Yellow
        exit 1
    }
} elseif ($MyInvocation.PSCommandPath -eq $MyInvocation.MyCommand.Definition) {
    # Running as script without params - show help
    make-help
}
