Software Course Project


STRUCTURE:

repo/
│
├── api-gateway/                   # Edge layer — entry point for all client requests
│   ├── main.go
│   ├── handlers/
│   │   └── transaction.go         # submitTransaction, validateRequest
│   ├── middleware/
│   │   ├── auth.go                # JWT validation
│   │   ├── rate_limiter.go        # Rate limiting logic
│   │   └── rbac.go                # Role-based access control
│   └── kafka/
│       └── producer.go            # Publishes validated txns to Kafka topic
│
├── coordinator/                   # Stateless transaction coordinator
│   ├── main.go
│   ├── kafka/
│   │   └── consumer.go            # Consumes from Kafka consumer group
│   ├── shard_map/
│   │   └── shard_map.go           # Reads/updates partition-to-shard mapping (JSON)
│   ├── router/
│   │   └── router.go              # Routes txn to correct shard leader(s)
│   └── two_phase_commit/
│       └── coordinator.go         # PREPARE / COMMIT / ABORT orchestration (REQ-TX-003)
│
├── shard/                         # Core shard node (leader + follower logic)
│   ├── main.go
│   ├── ledger/
│   │   └── ledger.go              # Account balances, debit/credit, invariant checks
│   ├── wal/
│   │   └── wal.go                 # WAL append, fsync, replay (REQ-DATA-002)
│   ├── replication/
│   │   ├── primary.go             # Replicates WAL to followers, waits for quorum ACK
│   │   └── follower.go            # Receives WAL entries, persists, sends ACK
│   ├── failover/
│   │   ├── heartbeat.go           # Sends/receives heartbeats (REQ-REP-003)
│   │   └── election.go            # Leader election on primary failure (REQ-REP-004)
│   └── recovery/
│       └── recovery.go            # WAL replay on crash restart (Algorithm 5 in report)
│
├── load-monitor/                  # Shard Load Monitor (SLM)
│   ├── main.go
│   ├── monitor.go                 # Collects shard metrics (CPU, QPS, queue depth)
│   └── rebalancer/
│       └── migration.go           # Partition halt → transfer → shard map update → resume
│
├── shared/                        # Shared types and utilities across services
│   ├── models/
│   │   ├── transaction.go         # Transaction struct: txnID, source, dest, amount
│   │   ├── wal_entry.go           # WAL entry: logID, txnID, opType, timestamp
│   │   └── shard_metrics.go       # ShardMetrics, UserMetrics structs
│   ├── constants/
│   │   └── states.go              # TransactionState: PENDING/PREPARED/COMMITTED/ABORTED
│   └── utils/
│       └── hash.go                # Hash-based account → partition mapping (REQ-DATA-001)
│
├── config/
│   └── config.yaml                # Shard count, heartbeat interval, quorum size, Kafka config
│
├── tests/
│   ├── unit/
│   │   ├── wal_test.go
│   │   ├── ledger_test.go
│   │   └── hash_test.go
│   ├── integration/
│   │   ├── single_shard_test.go   # REQ-TX-002
│   │   ├── cross_shard_test.go    # REQ-TX-003
│   │   └── recovery_test.go       # REQ-SAFE-002
│   └── fault_injection/
│       ├── primary_failure_test.go
│       └── coordinator_failure_test.go
│
├── frontend/                      # React.js UI (dashboard, transaction submission)
│   ├── src/
│   │   ├── components/
│   │   └── App.jsx
│   └── package.json
│
├── deploy/
│   ├── docker-compose.yml         # Local multi-shard setup
│   └── k8s/                       # Kubernetes manifests
│       ├── api-gateway.yaml
│       ├── coordinator.yaml
│       ├── shard.yaml
│       └── load-monitor.yaml
│
└── docs/
    ├── SRS_version_1.pdf
    └── Software_Project_Report_1.pdf