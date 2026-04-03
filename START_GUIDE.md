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

```bash
# All integration tests
make test-all

# Individual tests
make test-cross-shard   # Money conservation across shards
make test-invariant     # Balance invariant check
make test-failure       # Kill leader/follower/coordinator tests
make test-recovery      # WAL crash recovery
make test-migration     # Hotspot detection & partition migration

# Unit tests (no Docker required)
make unit-test

# Load test (requires k6 Docker image)
make test-load
```

---

## 8. Monitor the System

```bash
# Load Monitor metrics
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

## 9. Troubleshooting

| Symptom | Fix |
|---------|-----|
| Services not starting | `docker compose logs <service>` to check errors |
| Kafka not ready | Wait longer — Kafka can take 30+ seconds to initialize |
| Token rejected | Ensure the token was generated with `cmd/devtoken/main.go` |
| Seed fails | Ensure all shards are healthy: `curl localhost:8081/health` |
| Port conflict | Change port mappings in `docker-compose.yml` |
| Windows: grep not found | Use `make.bat` (Windows wrapper) instead of `make` |
| Windows: bash not found | Make sure Docker Compose CLI is installed and works with `docker compose ps` |

---

## 10. Stop & Clean Up

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
