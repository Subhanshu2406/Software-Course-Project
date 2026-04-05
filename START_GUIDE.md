# Start Guide — High-Concurrency Ledger Service

Step-by-step instructions to build, run, and test the entire distributed ledger cluster locally using Docker.

---

## 1. Prerequisites

| Tool            | Minimum Version | Check Command            |
|----------------|-----------------|--------------------------|
| Docker          | 20.10+          | `docker --version`       |
| Docker Compose  | v2.0+           | `docker compose version` |
| Go              | 1.24+           | `go version`             |
| PowerShell/bash | any             | `powershell --version` or `bash --version` |
| curl            | any             | `curl --version`         |

### Windows Users — Choose One:

**Option A: Use make.bat wrapper (EASIEST — native Windows)**
```powershell
# Just use make.bat instead of make
make.bat up
make.bat down
```

**Option B: Use PowerShell directly**
```powershell
# Dot-source the PowerShell functions
. .\make.ps1
make-up
make-down
```

**Option C: Use Git Bash** (if you have Git for Windows installed)
```bash
bash
make up
make down
```

**Option D: Use WSL2** (Windows Subsystem for Linux)
```bash
wsl --distribution Ubuntu
make up
make down
```

---

## 2. Build All Images

**Unix/Mac (bash):**
```bash
make build
```

**Windows (PowerShell/CMD):**
```powershell
make.bat build
```

This builds 4 images using a single multi-stage Dockerfile:
- `ledger-api` (API Gateway)
- `ledger-coordinator`
- `ledger-shard` (used for leaders and followers)
- `ledger-monitor` (Load Monitor)
- `ledger-fault-proxy` (Fault injection service)

---

## 3. Start the Cluster

**Unix/Mac:**
```bash
make up
```

**Windows (make.bat):**
```powershell
make.bat up
```

**Windows (PowerShell):**
```powershell
. .\make.ps1
make-up
```

This starts 15 containers in dependency order:
1. Zookeeper → Kafka
2. Shard leaders (shard1, shard2, shard3)
3. Shard followers (shard1a/1b, shard2a/2b, shard3a/3b)
4. Coordinator
5. API Gateway
6. Load Monitor

The script waits up to 60 seconds for all health checks to pass.

---

## 4. Generate a JWT Token

**Unix/Mac:**
```bash
make token
```

**Windows:**
```powershell
make.bat token
```

Copy the token — you'll need it for API requests and load tests.

---

## 5. Seed Accounts

**Unix/Mac:**
```bash
make seed
```

**Windows:**
```powershell
make.bat seed
```

This creates:
- `__bank__` account on all 3 shards (initial balance: $1,000,000 each)
- 1000 user accounts (`user0`–`user999`) distributed across shards
- Each user funded with $1,000 via coordinator transactions

---

## 6. Submit a Transaction

```bash
TOKEN="<your-token-from-step-4>"

# Single-shard transfer
curl -X POST http://localhost:8000/submit \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"TxnID":"test-001","Source":"user0","Destination":"user1","Amount":50}'

# Check status
curl http://localhost:8080/status?txn_id=test-001
```

---

## 7. Run Tests

### Quick Test Suite
```bash
# Build and run all tests (includes unit + integration)
make test-all

# Fast smoke test only
make test-fast
```

### Individual Test Targets

**Money Invariant & Transaction Tests:**
```bash
make test-cross-shard    # Cross-shard transfers + balance conservation
make test-invariant      # Verify total money in system is conserved
make test-failure        # Kill leader/follower/coordinator scenarios
make test-recovery       # WAL crash recovery (kill + restart shard)
make test-migration      # Hotspot detection & partition rebalancing
```

**Comprehensive Stress Testing:**
```bash
make test-stress         # 6-phase stress test (300+ concurrent txns)
```

This validates:
- High transaction throughput (single + cross-shard under sustained load)
- Write-Ahead Logging (WAL) durability and crash recovery
- Partition migration during active transactions
- Fault tolerance (shard kill/restart mid-flight)
- Money invariant conservation throughout all failures
- System recovery after coordinator kills

**Load Testing:**
```bash
make test-load           # K6 + Grafana load test (requires k6 image)
make test-load-single    # Single-shard only
make test-load-cross     # Cross-shard only
make test-load-mixed     # Mixed workload
```

**Unit Tests (no Docker):**
```bash
make unit-test
```

---

## 8. Recent Fixes & Improvements (Latest Release)

### Money Invariant Fixes (2PC Protocol)

The Two-Phase Commit coordinator now correctly implements reservation-based accounting:

- **PREPARE Phase**: Debits are now **reserved** (applied immediately) on the source shard, preventing double-spending and overdrafts
- **COMMIT Phase**: Only credits are applied (debits already done in PREPARE)
- **ABORT Phase**: Only committed debits are rolled back, preventing spurious money creation
- **Coordinator Retry**: Commit operations now retry 3x per shard to prevent partial commits

**Impact**: Cross-shard transfers now correctly conserve money under any failure scenario.

### Fault Injection Improvements

- Fault proxy now correctly listens on port **6060** (matched with docker-compose and Nginx)
- `/status` endpoint returns status of **all containers** (not just one)
- Frontend can now reliably poll container status and inject failures

**Impact**: Frontend Fault Injection page now works without Nginx 502 errors.

### Load Monitor Enhancements

- **Hotspot Detection**: Now uses **CommittedCount** (actual throughput) instead of broken `QueueDepth` metric
- **Migration Cooldown**: 30-second cooldown prevents runaway migrations when a shard is legitimately busy
- **Better Tuning**: Partition rebalancing is more selective, reducing unnecessary churn

**Impact**: Ledger sustains higher load without constant partition migrations.

### WAL Recovery Improvements

- Recovery timeout increased (30s → 60s) for nodes with large WAL logs
- Better sleep timing between transaction batches
- Integration tests for crash recovery now pass reliably

**Impact**: System reliably recovers from shard crashes without data loss.

### Frontend State Synchronization

- FaultInjection page correctly parses container status with new `running` field
- Polling interval optimized (5s → 3s) for faster UI updates
- Better handling of state during container restarts

**Impact**: Frontend dashboard remains responsive and accurate during failures.

### New Comprehensive Stress Test

A new 6-phase stress test (`make test-stress`) validates the entire system under load:

1. **Sustained Single-Shard Load**: 200 concurrent transfers
2. **Cross-Shard Transactions**: 100 concurrent cross-shard transfers
3. **WAL Crash Recovery**: Kill a shard mid-flight, verify WAL recovery
4. **Coordinator Restart**: Stop/restart coordinator during active loads
5. **Partition Migration**: Trigger hotspot detection and migration under load
6. **Final Invariant Check**: Verify money conservation through all failures

---

## 9. Monitor the System

```bash
# Load Monitor 
curl http://localhost:8090/metrics

# Health checks
curl http://localhost:8000/health   # API Gateway
curl http://localhost:8080/health   # Coordinator
curl http://localhost:8081/health   # Shard 1
curl http://localhost:8090/health   # Load Monitor

# Account balance
curl http://localhost:8081/balance?account=user0
```

### Frontend Dashboard

Open the frontend at **http://localhost:3000** or run:
```bash
make open
```

Pages available:
- **Clusters** — Shard health, partition map, recent transactions
- **Metrics** — TPS, abort rate, embedded Grafana dashboard
- **Transactions** — Filterable transaction explorer
- **WAL Inspector** — Browse WAL entries per shard
- **Shard Map** — Partition ownership grid
- **Replication** — WAL replication status per shard
- **Load Monitor** — Queue depth, migration history
- **Transfer** — Submit transfers via the UI
- **Fault Injection** — Kill/restart containers

### Grafana Dashboard

Open Grafana at **http://localhost:3001** or run:
```bash
make open-grafana
```

Pre-provisioned dashboard: **Ledger Service Overview** with TPS, queue depth, WAL index, account count, committed/aborted rates, and uptime panels.

### Prometheus

Open Prometheus at **http://localhost:9090** or run:
```bash
make open-prometheus
```

Scrapes all shards, coordinator, and load monitor every 5 seconds.

### Frontend Development (local)

For local development with hot reload:

**Unix/Mac:**
```bash
make frontend-dev
```

**Windows:**
```powershell
make.bat frontend-dev
```

This runs `npm run dev` on port 3000 with Vite proxy to backend services.

### JWT Token for Frontend

**Unix/Mac:**
```bash
make token-set
```

**Windows:**
```powershell
make.bat token-set
```

Then paste the token into your browser console:
```js
localStorage.setItem('ledger_token', '<token>')
```

---

## 10. Troubleshooting

| Symptom | Fix |
|---------|-----|
| Services not starting | `docker compose logs <service>` to check errors |
| Kafka not ready | Wait longer — Kafka can take 30+ seconds to initialize |
| Token rejected | Ensure the token was generated with `cmd/devtoken/main.go` |
| Seed fails | Ensure all shards are healthy: `curl localhost:8081/health` |
| Port conflict | Change port mappings in `docker-compose.yml` |
| Windows: grep not found | Use `make.bat` (Windows wrapper) instead of `make` |
| Windows: bash not found | Make sure Docker Compose CLI is installed and works with `docker compose ps` |
| Fault injector returns 502 | Ensure fault-proxy is running on port 6060: `docker compose logs fault-proxy` |
| Money invariant fails | Run `make test-recovery` to check WAL; verify no nodes crashed unexpectedly |
| Stress test fails | Check shard logs: `docker compose logs shard1 shard2 shard3 coordinator` |
| High migration churn | Load monitor should auto-tune; if excessive, check `load-monitor/monitor.go` hotspot threshold |

---

## 11. Build Without Docker (Local Go Build)

To compile Go code locally without Docker:

**Unix/Mac:**
```bash
# Build all Go packages (no Docker required)
go build ./...

# Run specific component
go run cmd/shard/main.go
go run cmd/coordinator/main.go
```

**Windows (PowerShell):**
```powershell
# Build all Go packages
go build .\...

# Or use the Makefile (preferred, handles dependencies)
make.bat build
```

**Note:** Local builds require Go 1.24+, no Docker, and no Docker Compose. For full system testing, use `make up` (which uses Docker).

---

## 12. Stop & Clean Up

**Unix/Mac:**
```bash
make down
make clean
```

**Windows:**
```powershell
make.bat down
make.bat clean
```
