# Implementation Plan — High-Concurrency Ledger Service

## Current State Assessment

### What's Implemented (Sprint 1 — Persistence & Local Transactions)

| Package | File | Status | Notes |
|---------|------|--------|-------|
| `shared/constants` | `states.go` | ✅ Complete | TransactionState, OperationType, Role enums |
| `shared/models` | `transaction.go` | ✅ Complete | Transaction, TransactionResult structs |
| `shared/models` | `wal_entry.go` | ✅ Complete | WALEntry, ShardMetrics structs |
| `shared/utils` | `hash.go` | ✅ Complete | SHA-256 PartitionMapper (REQ-DATA-001) |
| `shard/ledger` | `ledger.go` | ✅ Complete | In-memory balance management with all invariants |
| `shard/wal` | `wal.go` | ✅ Complete | Append-only WAL with fsync (REQ-DATA-002) |
| `shard/recovery` | `recovery.go` | ⚠️ Bug | Algorithm 5 — has a recovery bug (see below) |
| `shard/server` | `shard_server.go` | ✅ Complete | Single-shard execution + 2PC stubs |
| `tests/` | 4 test files | ✅ Complete | Unit + integration tests for Sprint 1 |

### What's NOT Implemented

| Directory | Planned Content | Status |
|-----------|----------------|--------|
| `config/` | `config.yaml` + loader | Empty |
| `api/` | API Gateway (handlers, kafka producer, middleware) | Empty stubs |
| `coordinator/consumer/` | Kafka consumer | Empty |
| `coordinator/router/` | Transaction router | Empty |
| `coordinator/shardmap/` | Partition → shard mapping | Empty |
| `coordinator/twopc/` | Two-phase commit orchestrator | Empty |
| `shard/replication/` | Primary-follower WAL replication | Not created |
| `shard/failover/` | Heartbeat + leader election | Not created |
| `storage/` | Persistent storage engine | Empty |
| `load-monitor/` | Shard load monitoring | Empty |
| `messaging/` | gRPC/message definitions | Empty |

---

## Bug in Existing Code

### Recovery Module — Cannot Debit From Zero Balance

**File:** `shard/recovery/recovery.go`

**Problem:** During crash recovery with a fresh (empty) ledger, the recovery module calls `ensureAccount()` which creates accounts with balance 0, then calls `l.ApplyDebit(accountID, amount)`. Since the balance is 0, the debit fails with "insufficient balance."

**Root Cause:** Recovery replays individual DEBIT/CREDIT operations against validation-checked methods, but on a fresh ledger there's no initial balance to debit from.

**Fix:** During recovery, bypass balance validation — use a new `ForceSetBalance` or accumulate net changes per account, then set final balances. The WAL is the source of truth; if it says COMMITTED, the operations are valid by definition.

---

## Implementation Plan

The plan is organized into 6 phases, aligned with the SRS requirements and sprint schedule. Each phase builds on the previous one. Coding conventions are defined first to ensure consistency.

---

### Phase 0: Coding Standards & Conventions

Apply these rules **to all new and existing code** for consistency:

1. **Package comments**: Every package starts with `// Package <name> ...` doc comment
2. **Error wrapping**: Always use `fmt.Errorf("context: %w", err)` — never lose the chain
3. **Naming**: `CamelCase` for exported, `camelCase` for unexported. Interfaces end in `-er` when single-method
4. **Struct constructors**: Always `New<Type>(...)` factory functions, never direct struct literals outside the package
5. **Concurrency**: Mutexes guard specific fields; document which fields a mutex protects
6. **Logging**: Use `log.Printf` with prefix format `"component: message"` (e.g., `"wal: fsync failed"`)
7. **Configuration**: All magic numbers come from `config/` — no hardcoded values in business logic
8. **Testing**: Every new package gets `_test.go` in the same directory or `tests/unit/`
9. **File organization**: One primary type per file; helpers in the same file below `// --- internal helpers ---`
10. **JSON tags**: All model structs use `json:"snake_case"` tags

---

### Phase 1: Fix Recovery Bug + Config System + Storage Engine

**Goal:** Fix the existing bug, add proper configuration, and create the persistent storage layer.

#### 1.1 Fix Recovery Module Bug

**File:** `shard/recovery/recovery.go`

**Change:** Replace the current per-operation replay approach with a **net-balance accumulation strategy**:

```
1. Scan WAL → build finalStateMap (existing, works correctly)
2. For each COMMITTED transaction:
   - Accumulate net balance changes per account
     (DEBIT subtracts, CREDIT adds)
3. After processing all entries:
   - Set each account's balance to its accumulated net value
4. Return pending PREPARED txnIDs (unchanged)
```

**New method needed on Ledger:** `SetBalance(accountID string, balance int64)` — sets balance directly, used only during recovery.

**Files to change:**
- `shard/recovery/recovery.go` — rewrite `Recover()` to accumulate balances
- `shard/ledger/ledger.go` — add `SetBalance()` method

#### 1.2 Configuration System

**New files:**
- `config/config.yaml` — default configuration
- `internal/config/config.go` — config loader

**Config structure:**
```yaml
cluster:
  num_shards: 3
  partitions_per_shard: 10
  total_partitions: 30

shard:
  wal_dir: "./data/wal"
  data_dir: "./data/storage"
  replica_count: 2           # 1 leader + 2 followers
  quorum_size: 2             # ceil(3/2)

replication:
  heartbeat_interval_ms: 5000
  heartbeat_miss_limit: 3

coordinator:
  kafka_brokers: ["localhost:9092"]
  kafka_topic: "transactions"
  consumer_group: "coordinators"

performance:
  min_tps_per_shard: 100
  single_shard_latency_p99_ms: 50
  cross_shard_latency_p99_ms: 200
```

**Config loader (`internal/config/config.go`):**
- `Load(path string) (*Config, error)` — reads YAML file
- `LoadDefault() *Config` — returns hardcoded defaults
- Struct fields match the YAML structure

#### 1.3 Persistent Storage Engine

**Purpose:** Provide durable key-value storage for account balances, separate from the WAL. The WAL provides write durability; the storage engine provides point-in-time state that survives restarts without full WAL replay.

**New files:**
- `storage/engine.go` — storage interface
- `storage/boltdb.go` — BoltDB-based implementation (embedded, no external deps)

**Why BoltDB over PostgreSQL for the hot path:**
The SRS mentions PostgreSQL, but the ledger's hot path needs low-latency key-value access. BoltDB (pure Go, embedded) fits perfectly. PostgreSQL can be used for audit trails / admin queries later.

**Interface:**
```go
// Storage is the persistent key-value store for shard data.
type Storage interface {
    GetBalance(accountID string) (int64, bool, error)
    SetBalance(accountID string, balance int64) error
    GetAllBalances() (map[string]int64, error)
    BatchSetBalances(balances map[string]int64) error
    Close() error
}
```

**Integration with Shard:**
- On startup: load balances from storage → populate in-memory ledger
- After WAL commit: asynchronously (or periodically) checkpoint ledger state to storage
- On recovery: load from storage + replay WAL entries after last checkpoint

---

### Phase 2: WAL Enhancements + Database Integration

**Goal:** Add checkpointing, compaction, and integrate WAL with the storage engine.

#### 2.1 WAL Checkpointing

**Purpose:** Avoid replaying the entire WAL on every restart. Periodically snapshot the ledger to storage and record the checkpoint position in the WAL.

**New operation type:** `OpCheckpoint` — added to `shared/constants/states.go`

**New WAL methods:**
```go
// WriteCheckpoint records a checkpoint marker in the WAL.
// All entries before this point have been persisted to storage.
func (w *WAL) WriteCheckpoint(lastLogID uint64) error

// ReadFrom reads WAL entries starting from a given log ID.
// Used during recovery to skip already-checkpointed entries.
func (w *WAL) ReadFrom(startLogID uint64) ([]models.WALEntry, error)
```

**WALEntry addition:**
- Add `CheckpointLogID uint64` field to WALEntry (used only for checkpoint entries)

#### 2.2 Enhanced Recovery with Checkpoints

**Updated recovery flow:**
```
1. Load last checkpoint from storage (gives base balances + checkpoint log ID)
2. Read WAL entries FROM checkpoint log ID onward
3. Apply only post-checkpoint committed operations
4. Result: full state restored with minimal replay
```

**Files to change:**
- `shard/recovery/recovery.go` — accept storage engine, read from checkpoint
- `shard/server/shard_server.go` — integrate storage, periodic checkpointing

#### 2.3 WAL Compaction

**Purpose:** Prevent unbounded WAL growth.

**Strategy:** After a successful checkpoint, WAL entries before the checkpoint can be truncated.

**New method:**
```go
// Truncate removes all entries before the given log ID.
// Called after a successful checkpoint.
func (w *WAL) Truncate(beforeLogID uint64) error
```

---

### Phase 3: Shard Map + Partition Management

**Goal:** Implement the mapping layer between logical partitions and physical shards.

#### 3.1 Shard Map

**New file:** `coordinator/shardmap/shard_map.go`

**Design:**
- JSON-file-backed mapping of partition index → shard address
- Thread-safe reads (most operations) and infrequent writes (rebalancing)
- Supports dynamic updates for partition migration

**Data structure:**
```go
type ShardInfo struct {
    ShardID  string `json:"shard_id"`
    Address  string `json:"address"`   // host:port for gRPC
    Role     string `json:"role"`      // PRIMARY or FOLLOWER
}

type ShardMap struct {
    mu         sync.RWMutex
    partitions map[int]ShardInfo  // partition index → shard info
    filePath   string             // JSON persistence path
}
```

**Methods:**
```go
func LoadShardMap(filePath string) (*ShardMap, error)
func (sm *ShardMap) GetShard(partitionID int) (ShardInfo, bool)
func (sm *ShardMap) GetShardForAccount(accountID string, mapper *utils.PartitionMapper) (ShardInfo, error)
func (sm *ShardMap) UpdatePartition(partitionID int, shard ShardInfo) error
func (sm *ShardMap) Save() error
func (sm *ShardMap) AllShards() []ShardInfo
```

#### 3.2 Partition Manager

**New file:** `shard/partition/partition.go`

**Purpose:** Each shard manages multiple logical partitions. The partition manager tracks which partitions this shard owns and routes operations accordingly.

**Design:**
```go
type PartitionManager struct {
    shardID      string
    ownedPartitions map[int]bool    // set of partition IDs this shard owns
    mu              sync.RWMutex
}
```

**Methods:**
```go
func NewPartitionManager(shardID string, partitions []int) *PartitionManager
func (pm *PartitionManager) OwnsPartition(partitionID int) bool
func (pm *PartitionManager) AddPartition(partitionID int)
func (pm *PartitionManager) RemovePartition(partitionID int)
func (pm *PartitionManager) OwnedPartitions() []int
func (pm *PartitionManager) HaltPartition(partitionID int)   // for migration
func (pm *PartitionManager) ResumePartition(partitionID int) // after migration
```

---

### Phase 4: Inter-Service Communication + Coordinator

**Goal:** Implement gRPC communication between coordinator and shards, and the full coordinator logic.

#### 4.1 gRPC Service Definitions

**New file:** `messaging/proto/shard.proto`

Define the RPC interface between coordinators and shards:

```protobuf
service ShardService {
    rpc Prepare(PrepareRequest) returns (PrepareResponse);
    rpc Commit(CommitRequest) returns (CommitResponse);
    rpc Abort(AbortRequest) returns (AbortResponse);
    rpc GetBalance(BalanceRequest) returns (BalanceResponse);
    rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
}
```

**Alternative (simpler, no proto dependency):** Use plain HTTP/JSON-RPC via Go's `net/http`. This is simpler for a course project and avoids proto tooling.

**Recommended approach:** HTTP/JSON-RPC for simplicity. Each shard runs an HTTP server exposing:
- `POST /prepare` — handle 2PC prepare
- `POST /commit` — handle 2PC commit  
- `POST /abort` — handle 2PC abort
- `GET /balance/:account` — query balance
- `GET /health` — heartbeat

**New files:**
- `shard/server/http_handler.go` — HTTP handlers wrapping ShardServer methods
- `messaging/client.go` — HTTP client for coordinator → shard communication

#### 4.2 Transaction Coordinator

**New file:** `coordinator/router/router.go`

**Purpose:** Given a transaction, determine if it's single-shard or cross-shard, route to correct shard(s).

```go
type Router struct {
    shardMap  *shardmap.ShardMap
    mapper   *utils.PartitionMapper
    client   *messaging.ShardClient
}

func (r *Router) Route(txn models.Transaction) (models.TransactionResult, error)
```

**Logic:**
1. `mapper.GetPartition(txn.Source)` → source partition
2. `mapper.GetPartition(txn.Destination)` → dest partition
3. `shardMap.GetShard(sourcePartition)` → source shard address
4. `shardMap.GetShard(destPartition)` → dest shard address
5. If same shard → delegate to single-shard path
6. If different shards → delegate to 2PC coordinator

#### 4.3 Two-Phase Commit Coordinator

**New file:** `coordinator/twopc/coordinator.go`

**Purpose:** Implement Algorithm 2 (Cross-Shard Coordination).

```go
type TwoPCCoordinator struct {
    client *messaging.ShardClient
}

func (c *TwoPCCoordinator) Execute(txn models.Transaction, 
    sourceShard, destShard shardmap.ShardInfo) (models.TransactionResult, error)
```

**Flow (per Algorithm 2):**
```
1. Send PREPARE(txnID, DEBIT, source, amount) to sourceShard
2. Send PREPARE(txnID, CREDIT, dest, amount) to destShard
3. If BOTH respond PREPARED:
   a. Send COMMIT to both shards
   b. Return COMMITTED
4. If ANY responds ABORT:
   a. Send ABORT to all PREPARED shards
   b. Return ABORTED
```

**Timeout handling:** If a shard doesn't respond within the configured timeout, treat as ABORT.

#### 4.4 Coordinator Consumer (Kafka Placeholder)

**New file:** `coordinator/consumer/consumer.go`

For Sprint 2-3, implement a **direct HTTP submission** mode (no Kafka dependency). The consumer interface is abstracted so Kafka can be swapped in later.

```go
// TransactionSource provides transactions to the coordinator.
type TransactionSource interface {
    Next() (models.Transaction, error)
    Ack(txnID string) error
}

// HTTPSource receives transactions via HTTP POST.
type HTTPSource struct { ... }

// KafkaSource consumes from Kafka (Sprint 3+).
// type KafkaSource struct { ... }
```

---

### Phase 5: Database Replication (Primary-Follower)

**Goal:** Implement WAL-based replication with quorum acknowledgment (REQ-REP-001, REQ-REP-002).

#### 5.1 Replication Protocol

**New files:**
- `shard/replication/primary.go` — leader-side replication logic
- `shard/replication/follower.go` — follower-side WAL receiver

**Primary (Leader) logic:**
```go
type PrimaryReplicator struct {
    followers []FollowerConn   // connections to follower shards
    quorumSize int             // ceil(N/2)
}

// Replicate sends a WAL entry to all followers and waits for quorum ACK.
func (p *PrimaryReplicator) Replicate(entry models.WALEntry) error {
    // Send to all followers in parallel
    // Wait for quorumSize ACKs
    // Return success if quorum met, error if timeout
}
```

**Follower logic:**
```go
type FollowerReceiver struct {
    walLog  *wal.WAL
    ledger  *ledger.Ledger
}

// ReceiveEntry persists a WAL entry from the leader and sends ACK.
func (f *FollowerReceiver) ReceiveEntry(entry models.WALEntry) error {
    // Append to local WAL
    // fsync
    // Return ACK
}
```

**Updated commit flow (replaces current Sprint 1 flow):**
```
1. Leader appends to WAL + fsync
2. Leader sends entry to all followers
3. Each follower appends to own WAL + fsync → sends ACK
4. Leader waits for quorum (ceil(N/2)) ACKs
5. Leader advances commit index
6. Leader applies to ledger
```

#### 5.2 Follower Endpoints

Add replication endpoints to the shard HTTP server:
- `POST /replicate` — receive WAL entry from leader
- `GET /log-index` — return highest log index (for election)

#### 5.3 Shard Server Integration

**File:** `shard/server/shard_server.go`

Update `ExecuteSingleShard` to include replication step between WAL append and commit:

```
Current:  WAL append → apply ledger → mark committed
Updated:  WAL append → replicate to followers → wait quorum → apply ledger → mark committed
```

---

### Phase 6: Heartbeat, Failover, and Leader Election

**Goal:** Implement failure detection and automatic leader promotion (REQ-REP-003, REQ-REP-004).

#### 6.1 Heartbeat Monitor

**New file:** `shard/failover/heartbeat.go`

```go
type HeartbeatMonitor struct {
    interval     time.Duration
    missLimit    int
    peers        map[string]*PeerState  // shardID → state
}

// Start begins sending/receiving heartbeats.
func (h *HeartbeatMonitor) Start(ctx context.Context)

// OnFailure registers a callback for when a peer is detected as failed.
func (h *HeartbeatMonitor) OnFailure(callback func(shardID string))
```

#### 6.2 Leader Election

**New file:** `shard/failover/election.go`

**Algorithm 4 (Leader Promotion):**
```
1. Detect primary failure (missed heartbeats)
2. Among available followers, select the one with highest log index
3. Promote to PRIMARY
4. Replay committed WAL entries to ensure consistent state
5. Update shard map
6. Resume write processing
```

```go
type ElectionManager struct {
    shardID     string
    replicaSet  []ReplicaInfo
    shardMap    *shardmap.ShardMap
}

func (e *ElectionManager) TriggerElection() (newLeaderID string, err error)
```

---

## File Creation Summary

### New Files to Create

```
config/
  config.yaml                        # Phase 1.2

internal/config/
  config.go                          # Phase 1.2

storage/
  engine.go                          # Phase 1.3  (interface)
  boltdb.go                          # Phase 1.3  (implementation)
  boltdb_test.go                     # Phase 1.3  (tests)

coordinator/shardmap/
  shard_map.go                       # Phase 3.1
  shard_map_test.go                  # Phase 3.1

shard/partition/
  partition.go                       # Phase 3.2
  partition_test.go                  # Phase 3.2

messaging/
  client.go                          # Phase 4.1  (HTTP client for shard comm)
  client_test.go                     # Phase 4.1

shard/server/
  http_handler.go                    # Phase 4.1  (HTTP server for shard)

coordinator/router/
  router.go                          # Phase 4.2
  router_test.go                     # Phase 4.2

coordinator/twopc/
  coordinator.go                     # Phase 4.3
  coordinator_test.go                # Phase 4.3

coordinator/consumer/
  consumer.go                        # Phase 4.4
  consumer_test.go                   # Phase 4.4

shard/replication/
  primary.go                         # Phase 5.1
  follower.go                        # Phase 5.1
  replication_test.go                # Phase 5.1

shard/failover/
  heartbeat.go                       # Phase 6.1
  election.go                        # Phase 6.2
  heartbeat_test.go                  # Phase 6.1
  election_test.go                   # Phase 6.2

tests/integration/
  cross_shard_test.go                # Phase 4.3
  recovery_test.go                   # Phase 2.2
  replication_test.go                # Phase 5

tests/fault_injection/
  primary_failure_test.go            # Phase 6
  coordinator_failure_test.go        # Phase 4
```

### Files to Modify

```
shard/recovery/recovery.go          # Phase 1.1 (fix bug)
shard/ledger/ledger.go              # Phase 1.1 (add SetBalance)
shard/server/shard_server.go        # Phase 2.2, 5.3 (storage + replication integration)
shard/wal/wal.go                    # Phase 2.1 (checkpointing, compaction)
shared/constants/states.go          # Phase 2.1 (add OpCheckpoint)
shared/models/wal_entry.go          # Phase 2.1 (add CheckpointLogID field)
go.mod                              # Add dependencies (BoltDB, YAML parser)
```

---

## Dependency Order

```
Phase 1 ──► Phase 2 ──► Phase 3 ──► Phase 4 ──► Phase 5 ──► Phase 6
  │            │            │            │            │            │
  Fix Bug      WAL          Shard Map    Coordinator  Replication  Failover
  Config       Checkpoint   Partitions   Router/2PC   Quorum ACK   Election
  Storage      Compaction                gRPC/HTTP    
```

Phases 3 (Shard Map) and 2 (WAL enhancements) can be done in parallel.
Phase 5 (Replication) and Phase 4 (Coordinator) can overlap partially.

---

## Test Strategy

| Phase | Test Type | What's Tested |
|-------|-----------|---------------|
| 1 | Unit | Fixed recovery, config loading, storage engine CRUD |
| 2 | Unit + Integration | Checkpoint/compaction, recovery with checkpoints |
| 3 | Unit | Shard map CRUD, partition ownership |
| 4 | Integration | Single-shard routing, cross-shard 2PC (happy + failure paths) |
| 5 | Integration | Quorum replication, follower sync |
| 6 | Fault injection | Primary failure → election → recovery, coordinator crash |

---

## Sprint Mapping (per SRS schedule)

| Sprint | Dates | Phases | Key Deliverables |
|--------|-------|--------|-------------------|
| Sprint 2 | Feb 15–28 | Phase 1, 2, 3 | Bug fix, config, storage, shard map, partitions |
| Sprint 3 | Mar 1–14 | Phase 4 | Coordinator, router, 2PC, HTTP comm |
| Sprint 4 | Mar 15–31 | Phase 5, 6 | Replication, heartbeat, failover, acceptance tests |
