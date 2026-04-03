# High-Concurrency Ledger Service

A fault-tolerant, high-throughput, strongly consistent ledger service designed for financial-grade transaction processing under extreme concurrency.

## Architecture

```
┌─────────────┐       ┌───────┐       ┌─────────────┐
│ API Gateway │──────▶│ Kafka │──────▶│ Coordinator │
│   :8000     │       │ :9092 │       │   :8080     │
└─────────────┘       └───────┘       └──────┬──────┘
                                             │ 2PC
                        ┌────────────────────┼────────────────────┐
                        ▼                    ▼                    ▼
                 ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
                 │   Shard 1   │     │   Shard 2   │     │   Shard 3   │
                 │   :8081     │     │   :8082     │     │   :8083     │
                 │ P0-P9       │     │ P10-P19     │     │ P20-P29    │
                 └──┬─────┬────┘     └──┬─────┬────┘     └──┬─────┬────┘
                    │     │             │     │             │     │
                    ▼     ▼             ▼     ▼             ▼     ▼
                  F1a   F1b           F2a   F2b           F3a   F3b
                 :9081 :9082         :9083 :9084         :9085 :9086
                                              
                        ┌─────────────┐
                        │Load Monitor │
                        │   :8090     │
                        └─────────────┘
```

- **API Gateway** — JWT auth, rate limiting, RBAC, pushes transactions to Kafka
- **Coordinator** — consumes from Kafka, routes transactions, runs 2PC for cross-shard
- **Shards** — 3 leaders (each with 10 partitions) + 6 followers for quorum replication
- **Load Monitor** — polls shard metrics, detects hotspots, triggers partition migration
- **WAL** — JSON-line write-ahead log with fsync, crash recovery via balance accumulation

## Prerequisites

- **Docker** & **Docker Compose** v2+
- **Go 1.24+** (for local development / token generation)
- **k6** (optional, for load testing — or use Docker)
- **bash** (for test scripts — Git Bash on Windows)

## Quick Start

```bash
# 1. Build all images
make build

# 2. Start the cluster (waits for all services to be healthy)
make up

# 3. Generate a JWT token for testing
make token

# 4. Seed 1000 accounts with $1000 each
make seed

# 5. Run all integration tests
make test-all
```

## Port Mapping

| Service       | Port  | Description            |
|--------------|-------|------------------------|
| API Gateway  | 8000  | Client-facing REST API |
| Coordinator  | 8080  | Transaction routing    |
| Shard 1      | 8081  | Leader (P0–P9)         |
| Shard 2      | 8082  | Leader (P10–P19)       |
| Shard 3      | 8083  | Leader (P20–P29)       |
| Shard 1a     | 9081  | Follower               |
| Shard 1b     | 9082  | Follower               |
| Shard 2a     | 9083  | Follower               |
| Shard 2b     | 9084  | Follower               |
| Shard 3a     | 9085  | Follower               |
| Shard 3b     | 9086  | Follower               |
| Load Monitor | 8090  | Metrics & hotspot      |
| Kafka        | 9092  | Message broker         |

## Makefile Targets

| Target              | Description                                 |
|--------------------|---------------------------------------------|
| `make build`       | Build all Docker images                     |
| `make up`          | Start cluster and wait for healthy status   |
| `make down`        | Stop cluster, remove volumes                |
| `make seed`        | Seed 1000 accounts across all shards        |
| `make token`       | Generate a dev JWT token                    |
| `make test-all`    | Run all integration tests                   |
| `make test-failure`| Kill-leader, kill-follower, kill-coordinator |
| `make test-recovery`| WAL crash recovery test                    |
| `make test-cross-shard`| Cross-shard money conservation          |
| `make test-invariant`| Balance invariant check                   |
| `make test-migration`| Hotspot detection & partition migration   |
| `make test-load`   | k6 load test (single + cross shard)         |
| `make unit-test`   | Go unit tests                               |
| `make clean`       | Remove images, volumes, and artifacts       |

## Testing

### Failure Tests (`make test-failure`)
- **Kill leader** — stops a shard leader, verifies cluster degrades gracefully
- **Kill follower** — stops a follower, verifies writes continue with remaining quorum
- **Kill coordinator mid-2PC** — restarts coordinator during a cross-shard transaction
- **Network partition** — pauses a shard, verifies it recovers after unpause

### Recovery Test (`make test-recovery`)
Sends transactions, force-kills a shard (SIGKILL), restarts it, and verifies balances are recovered from the WAL.

### Money Conservation (`make test-invariant`)
Queries all account balances across all shards and verifies the total money supply is unchanged.

### Load Test (`make test-load`)
Runs k6 with 100 VUs (50 single-shard + 50 cross-shard) for 30 seconds. Thresholds: single-shard p99 < 200ms, cross-shard p99 < 500ms.

## Project Structure

```
cmd/
  api/            API Gateway binary
  coordinator/    Coordinator binary
  shard/          Shard server binary
  load-monitor/   Load Monitor binary
  devtoken/       Dev token generator
  loadgen/        Load generator & account seeder
api/              HTTP handlers, Kafka producer, middleware
coordinator/      Consumer, router, shard map, 2PC
shard/            Ledger, WAL, replication, failover, recovery
shared/           Models, constants, hash utilities
storage/          Storage engine interface
config/           Configuration files
tests/            Unit and integration tests
loadtest/         k6 load test scripts
```