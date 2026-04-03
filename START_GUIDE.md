# Start Guide — High-Concurrency Ledger Service

Step-by-step instructions to build, run, and test the entire distributed ledger cluster locally using Docker.

---

## 1. Prerequisites

| Tool            | Minimum Version | Check Command            |
|----------------|-----------------|--------------------------|
| Docker          | 20.10+          | `docker --version`       |
| Docker Compose  | v2.0+           | `docker compose version` |
| Go              | 1.24+           | `go version`             |
| bash            | 4.0+            | `bash --version`         |
| curl            | any             | `curl --version`         |

> **Windows users**: Use Git Bash, WSL2, or PowerShell with bash available.

---

## 2. Build All Images

```bash
make build
# or directly:
docker compose build
```

This builds 4 images using a single multi-stage Dockerfile:
- `ledger-api` (API Gateway)
- `ledger-coordinator`
- `ledger-shard` (used for leaders and followers)
- `ledger-monitor` (Load Monitor)

---

## 3. Start the Cluster

```bash
make up
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

```bash
make token
# or:
go run cmd/devtoken/main.go
```

Copy the token — you'll need it for API requests and load tests.

---

## 5. Seed Accounts

```bash
make seed
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

---

## 9. Troubleshooting

| Symptom | Fix |
|---------|-----|
| Services not starting | `docker compose logs <service>` to check errors |
| Kafka not ready | Wait longer — Kafka can take 30+ seconds to initialize |
| Token rejected | Ensure the token was generated with `cmd/devtoken/main.go` |
| Seed fails | Ensure all shards are healthy: `curl localhost:8081/health` |
| Port conflict | Change port mappings in `docker-compose.yml` |

---

## 10. Stop & Clean Up

```bash
# Stop everything, remove volumes
make down

# Full cleanup (removes images too)
make clean
```
