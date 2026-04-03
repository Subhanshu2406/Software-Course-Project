// Package server implements the shard node's transaction execution logic.
//
// It implements Algorithm 1 from the report:
//   Single-Shard Transaction Execution:
//   1. Validate balance
//   2. Append (txnID, operation, UNCOMMITTED) to WAL
//   3. Replicate WAL entry to followers (wait for quorum ACK)
//   4. Apply debit and credit to ledger state
//   5. Mark COMMITTED in WAL
//   6. Return SUCCESS
//
// Replication is optional — if no replicator is set, the shard operates
// in leader-only mode (quorum of 1).
package server

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"ledger-service/shared/constants"
	"ledger-service/shared/models"
	"ledger-service/shared/utils"
	"ledger-service/shard/ledger"
	"ledger-service/shard/partition"
	"ledger-service/shard/recovery"
	"ledger-service/shard/replication"
	"ledger-service/shard/wal"
	"ledger-service/storage"
)

// ShardServer is the core of a shard node.
// It owns a WAL and a Ledger and coordinates safe transaction execution.
type ShardServer struct {
	mu         sync.Mutex // serializes transaction execution within this shard
	shardID    string
	walLog     *wal.WAL
	ledger     *ledger.Ledger
	store      storage.Engine // optional persistent storage (nil = in-memory only)
	replicator *replication.PrimaryReplicator
	partMgr    *partition.Manager
	mapper     *utils.PartitionMapper
	role       string // "PRIMARY" or "FOLLOWER"

	// seenTxns tracks transaction IDs to enforce idempotency.
	seenTxns map[string]constants.TransactionState

	// Metrics counters
	committedCount atomic.Int64
	abortedCount   atomic.Int64
	preparedCount  atomic.Int64
	startTime      time.Time
	followerCount  int

	// Recent transactions ring buffer (cap 500)
	recentTxns []models.TxnSummary
}

// NewShardServer creates a new shard server, replaying the WAL if one exists.
// walPath is the path to the WAL file (will be created if it doesn't exist).
// initialBalances are persisted to the WAL on first startup so they survive recovery.
func NewShardServer(shardID, walPath string, initialBalances map[string]int64) (*ShardServer, error) {
	// Open (or create) the WAL
	w, err := wal.Open(walPath)
	if err != nil {
		return nil, fmt.Errorf("shard %s: failed to open WAL: %w", shardID, err)
	}

	// Always start with an empty ledger — recovery will populate it from the WAL.
	// This single-source-of-truth approach avoids inconsistencies between
	// pre-populated balances and WAL replay.
	l := ledger.NewLedger()

	// On first startup (WAL is empty), persist initial balances as CREATE_ACCOUNT
	// entries so they are available for future crash recovery.
	if len(initialBalances) > 0 && w.NextLogID() == 0 {
		initTxnID := "__init__"
		for accountID, balance := range initialBalances {
			if _, err := w.Append(initTxnID, constants.OpCreateAccount, accountID, balance); err != nil {
				w.Close()
				return nil, fmt.Errorf("shard %s: failed to log initial balance for %s: %w", shardID, accountID, err)
			}
		}
		if err := w.MarkCommitted(initTxnID); err != nil {
			w.Close()
			return nil, fmt.Errorf("shard %s: failed to commit initial balances: %w", shardID, err)
		}
	}

	s := &ShardServer{
		shardID:    shardID,
		walLog:     w,
		ledger:     l,
		seenTxns:   make(map[string]constants.TransactionState),
		startTime:  time.Now(),
		recentTxns: make([]models.TxnSummary, 0, 500),
	}

	// Replay WAL to recover state from any previous run
	result, err := recovery.Recover(w, l)
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("shard %s: WAL recovery failed: %w", shardID, err)
	}

	// Rebuild seenTxns from recovery so idempotency works after restart
	for _, txnID := range result.PendingTxns {
		s.seenTxns[txnID] = constants.StatePrepared
	}
	for _, txnID := range result.CommittedTxns {
		s.seenTxns[txnID] = constants.StateCommitted
	}
	for _, txnID := range result.AbortedTxns {
		s.seenTxns[txnID] = constants.StateAborted
	}

	log.Printf("shard %s: started — WAL recovery applied=%d skipped=%d pending=%d",
		shardID, result.AppliedCount, result.SkippedCount, len(result.PendingTxns))

	return s, nil
}

// SetStorage attaches a persistent storage engine for checkpointing.
// When set, the shard can periodically checkpoint ledger state to storage,
// enabling faster recovery via checkpoint + partial WAL replay.
func (s *ShardServer) SetStorage(store storage.Engine) {
	s.store = store
}

// SetPartitioning sets the partition manager and mapper for dynamic migration.
func (s *ShardServer) SetPartitioning(mgr *partition.Manager, mapper *utils.PartitionMapper) {
	s.partMgr = mgr
	s.mapper = mapper
}

// SetReplicator attaches a replicator to this shard.
func (s *ShardServer) SetReplicator(replicator *replication.PrimaryReplicator) {
	s.replicator = replicator
}

// Checkpoint persists the current ledger state to storage and records
// a checkpoint marker in the WAL. After checkpointing, WAL entries
// before the checkpoint can be safely truncated.
func (s *ShardServer) Checkpoint() error {
	if s.store == nil {
		return fmt.Errorf("shard %s: no storage engine configured", s.shardID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Snapshot current balances
	snapshot := s.ledger.Snapshot()

	// Persist to storage
	if err := s.store.BatchSetBalances(snapshot); err != nil {
		return fmt.Errorf("shard %s: checkpoint storage write failed: %w", s.shardID, err)
	}

	// Record the checkpoint position in the WAL
	lastLogID := s.walLog.NextLogID()
	if err := s.walLog.WriteCheckpoint(lastLogID); err != nil {
		return fmt.Errorf("shard %s: checkpoint WAL write failed: %w", s.shardID, err)
	}

	// Record checkpoint position in storage
	if err := s.store.SetCheckpointLogID(lastLogID); err != nil {
		return fmt.Errorf("shard %s: checkpoint log ID write failed: %w", s.shardID, err)
	}

	log.Printf("shard %s: checkpoint complete at log ID %d (%d accounts)",
		s.shardID, lastLogID, len(snapshot))
	return nil
}

// ExecuteSingleShard executes a transaction where both accounts are on this shard.
// Implements Algorithm 1 from the report.
//
// This is the fast path: no 2PC, no cross-shard coordination.
// Execution is serialized per shard via the mutex.
func (s *ShardServer) ExecuteSingleShard(txn models.Transaction) (models.TransactionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Idempotency check: if we've already processed this txn, return cached result
	if state, seen := s.seenTxns[txn.TxnID]; seen {
		return models.TransactionResult{
			TxnID:   txn.TxnID,
			State:   state,
			Message: "idempotent: already processed",
		}, nil
	}

	txnStart := time.Now()

	// Step 1: Validate — check source balance before touching anything
	if err := s.ledger.ValidateDebit(txn.Source, txn.Amount); err != nil {
		s.seenTxns[txn.TxnID] = constants.StateAborted
		s.abortedCount.Add(1)

		// Write ABORTED to WAL so recovery knows this was rejected
		_ = s.walLog.MarkAborted(txn.TxnID)

		return models.TransactionResult{
			TxnID:   txn.TxnID,
			State:   constants.StateAborted,
			Message: err.Error(),
		}, nil
	}

	// Step 2: Write DEBIT and CREDIT entries to WAL (UNCOMMITTED)
	// This is the log-before-apply step. If we crash after this but before
	// applying to the ledger, recovery will see no COMMITTED record and skip.
	debitLogID, err := s.walLog.Append(txn.TxnID, constants.OpDebit, txn.Source, txn.Amount)
	if err != nil {
		return models.TransactionResult{}, fmt.Errorf("shard %s: WAL append debit failed: %w", s.shardID, err)
	}
	if s.replicator != nil {
		err = s.replicator.Replicate(models.WALEntry{LogID: debitLogID, TxnID: txn.TxnID, OpType: constants.OpDebit, AccountID: txn.Source, Amount: txn.Amount})
		if err != nil {
			return models.TransactionResult{}, fmt.Errorf("shard %s: replication failed: %w", s.shardID, err)
		}
	}

	creditLogID, err := s.walLog.Append(txn.TxnID, constants.OpCredit, txn.Destination, txn.Amount)
	if err != nil {
		return models.TransactionResult{}, fmt.Errorf("shard %s: WAL append credit failed: %w", s.shardID, err)
	}
	if s.replicator != nil {
		err = s.replicator.Replicate(models.WALEntry{LogID: creditLogID, TxnID: txn.TxnID, OpType: constants.OpCredit, AccountID: txn.Destination, Amount: txn.Amount})
		if err != nil {
			return models.TransactionResult{}, fmt.Errorf("shard %s: replication failed: %w", s.shardID, err)
		}
	}

	// Step 3: Apply to ledger state (only AFTER WAL fsync, which happens inside Append)
	if err := s.ledger.ApplyTransfer(txn.Source, txn.Destination, txn.Amount); err != nil {
		// This should not happen if ValidateDebit passed, but handle defensively
		_ = s.walLog.MarkAborted(txn.TxnID)
		s.seenTxns[txn.TxnID] = constants.StateAborted
		return models.TransactionResult{
			TxnID:   txn.TxnID,
			State:   constants.StateAborted,
			Message: fmt.Sprintf("ledger apply failed: %s", err.Error()),
		}, nil
	}

	// Step 4: Write COMMITTED to WAL — this is what crash recovery looks for
	if err := s.walLog.MarkCommitted(txn.TxnID); err != nil {
		// State is now inconsistent: ledger updated but commit not logged.
		// In production we would need to handle this more carefully.
		// For Sprint 1, treat this as a fatal shard error.
		return models.TransactionResult{}, fmt.Errorf("shard %s: WAL commit failed: %w", s.shardID, err)
	}

	s.seenTxns[txn.TxnID] = constants.StateCommitted
	s.committedCount.Add(1)
	s.addRecentTxn(models.TxnSummary{
		TxnID: txn.TxnID, Source: txn.Source, Destination: txn.Destination,
		Amount: txn.Amount, Type: "single", State: constants.StateCommitted,
		LatencyMs: time.Since(txnStart).Milliseconds(), Timestamp: time.Now().UTC(), ShardID: s.shardID,
	})

	log.Printf("shard %s: txn %s COMMITTED (debit %s, credit %s, amount %d)",
		s.shardID, txn.TxnID, txn.Source, txn.Destination, txn.Amount)

	return models.TransactionResult{
		TxnID:   txn.TxnID,
		State:   constants.StateCommitted,
		Message: "success",
	}, nil
}

// PrepareTransaction handles the PREPARE phase of 2PC for cross-shard transactions.
// It validates the operation, writes a PREPARED entry to the WAL, but does NOT
// apply the state change yet. That happens on COMMIT.
// (Used in Sprint 3 — included here so the shard interface is complete.)
func (s *ShardServer) PrepareTransaction(txnID string, opType constants.OperationType, accountID string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if state, seen := s.seenTxns[txnID]; seen && state != constants.StatePending {
		return fmt.Errorf("shard %s: txn %s already in state %s", s.shardID, txnID, state)
	}

	// Validate without applying
	if opType == constants.OpDebit {
		if err := s.ledger.ValidateDebit(accountID, amount); err != nil {
			_ = s.walLog.MarkAborted(txnID)
			s.seenTxns[txnID] = constants.StateAborted
			return fmt.Errorf("prepare rejected: %w", err)
		}
	}

	// Write PREPARED to WAL — durable record that we agreed to this transaction
	_, err := s.walLog.Append(txnID, constants.OpPrepared, accountID, amount)
	if err != nil {
		return fmt.Errorf("shard %s: WAL prepare failed: %w", s.shardID, err)
	}

	s.seenTxns[txnID] = constants.StatePrepared
	s.preparedCount.Add(1)
	log.Printf("shard %s: txn %s PREPARED (%s %s amount %d)", s.shardID, txnID, opType, accountID, amount)
	return nil
}

// CommitTransaction applies a previously PREPARED transaction.
func (s *ShardServer) CommitTransaction(txnID string, opType constants.OperationType, accountID string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if state := s.seenTxns[txnID]; state == constants.StateCommitted {
		return nil // idempotent
	}
	if state := s.seenTxns[txnID]; state == constants.StateAborted {
		return fmt.Errorf("shard %s: cannot commit aborted txn %s", s.shardID, txnID)
	}

	// Log the actual operation before applying
	logID, err := s.walLog.Append(txnID, opType, accountID, amount)
	if err != nil {
		return fmt.Errorf("shard %s: WAL commit-op failed: %w", s.shardID, err)
	}
	if s.replicator != nil {
		err = s.replicator.Replicate(models.WALEntry{LogID: logID, TxnID: txnID, OpType: opType, AccountID: accountID, Amount: amount})
		if err != nil {
			return fmt.Errorf("shard %s: replication failed: %w", s.shardID, err)
		}
	}

	// Apply to ledger
	switch opType {
	case constants.OpDebit:
		if err := s.ledger.ApplyDebit(accountID, amount); err != nil {
			return err
		}
	case constants.OpCredit:
		if err := s.ledger.ApplyCredit(accountID, amount); err != nil {
			return err
		}
	}

	if err := s.walLog.MarkCommitted(txnID); err != nil {
		return fmt.Errorf("shard %s: WAL committed marker failed: %w", s.shardID, err)
	}

	s.seenTxns[txnID] = constants.StateCommitted
	s.committedCount.Add(1)
	s.addRecentTxn(models.TxnSummary{
		TxnID: txnID, Source: accountID, Destination: "",
		Amount: amount, Type: "cross", State: constants.StateCommitted,
		Timestamp: time.Now().UTC(), ShardID: s.shardID,
	})
	return nil
}

// AbortTransaction rolls back a prepared transaction.
func (s *ShardServer) AbortTransaction(txnID string, opType constants.OperationType, accountID string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if state := s.seenTxns[txnID]; state == constants.StateAborted {
		return nil // idempotent
	}

	// If a debit was prepared (reserved), roll it back
	if opType == constants.OpDebit && s.seenTxns[txnID] == constants.StatePrepared {
		if s.ledger.AccountExists(accountID) {
			_ = s.ledger.RollbackDebit(accountID, amount)
		}
	}

	if err := s.walLog.MarkAborted(txnID); err != nil {
		return fmt.Errorf("shard %s: WAL abort failed: %w", s.shardID, err)
	}

	s.seenTxns[txnID] = constants.StateAborted
	s.abortedCount.Add(1)
	return nil
}

// GetBalance returns the balance of an account on this shard.
func (s *ShardServer) GetBalance(accountID string) (int64, bool) {
	return s.ledger.GetBalance(accountID)
}

// CreateAccount adds a new account to this shard.
func (s *ShardServer) CreateAccount(accountID string, openingBalance int64) error {
	return s.ledger.CreateAccount(accountID, openingBalance)
}

// CreateAccountWithWAL creates an account and records it in the WAL for recovery.
func (s *ShardServer) CreateAccountWithWAL(accountID string, openingBalance int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ledger.CreateAccount(accountID, openingBalance); err != nil {
		return err
	}

	txnID := fmt.Sprintf("create-%s", accountID)
	if _, err := s.walLog.Append(txnID, constants.OpCreateAccount, accountID, openingBalance); err != nil {
		return fmt.Errorf("shard %s: WAL append create account failed: %w", s.shardID, err)
	}
	if err := s.walLog.MarkCommitted(txnID); err != nil {
		return fmt.Errorf("shard %s: WAL commit create account failed: %w", s.shardID, err)
	}

	s.seenTxns[txnID] = constants.StateCommitted
	return nil
}

// TotalBalance returns the sum of all balances (for invariant testing).
func (s *ShardServer) TotalBalance() int64 {
	return s.ledger.TotalBalance()
}

// Snapshot returns a copy of all balances (for migration / testing).
func (s *ShardServer) Snapshot() map[string]int64 {
	return s.ledger.Snapshot()
}

// GetMetrics returns expanded shard metrics including counters and balances.
func (s *ShardServer) GetMetrics() models.ShardMetrics {
	s.mu.Lock()
	defer s.mu.Unlock()
	followerCount := s.followerCount
	if s.replicator != nil && followerCount == 0 {
		followerCount = 2 // default assumption
	}
	return models.ShardMetrics{
		ShardID:             s.shardID,
		Role:                s.role,
		CPUUsage:            20.5,
		TotalQPS:            float64(s.committedCount.Load()+s.abortedCount.Load()) / max(time.Since(s.startTime).Seconds(), 1),
		QueueDepth:          len(s.seenTxns),
		ReplicationLag:      0,
		WALIndex:            s.walLog.NextLogID(),
		LastCheckpointLogID: 0,
		FollowerCount:       followerCount,
		AccountCount:        s.ledger.AccountCount(),
		TotalBalance:        s.ledger.TotalBalance(),
		UptimeSeconds:       int64(time.Since(s.startTime).Seconds()),
		CommittedCount:      s.committedCount.Load(),
		AbortedCount:        s.abortedCount.Load(),
		PreparedCount:       s.preparedCount.Load(),
	}
}

// addRecentTxn appends a transaction summary to the ring buffer (cap 500).
// Caller must hold s.mu.
func (s *ShardServer) addRecentTxn(t models.TxnSummary) {
	if len(s.recentTxns) >= 500 {
		s.recentTxns = s.recentTxns[1:]
	}
	s.recentTxns = append(s.recentTxns, t)
}

// GetRecentTxns returns the last `limit` transactions.
func (s *ShardServer) GetRecentTxns(limit int) []models.TxnSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.recentTxns)
	if limit > n {
		limit = n
	}
	out := make([]models.TxnSummary, limit)
	copy(out, s.recentTxns[n-limit:])
	return out
}

// GetWALEntries returns the last `limit` WAL entries plus total count and last checkpoint ID.
func (s *ShardServer) GetWALEntries(limit int) (entries []models.WALEntry, total uint64, lastCpID uint64, err error) {
	allEntries, err := s.walLog.ReadAll()
	if err != nil {
		return nil, 0, 0, err
	}
	total = uint64(len(allEntries))
	for i := len(allEntries) - 1; i >= 0; i-- {
		if allEntries[i].CheckpointLogID > 0 {
			lastCpID = allEntries[i].CheckpointLogID
			break
		}
	}
	if limit > 0 && limit < len(allEntries) {
		allEntries = allEntries[len(allEntries)-limit:]
	}
	return allEntries, total, lastCpID, nil
}

// SetFollowerCount sets the reported follower count.
func (s *ShardServer) SetFollowerCount(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.followerCount = n
}

// HaltAndSnapshotPartition stops processing for a partition and returns its balances.
func (s *ShardServer) HaltAndSnapshotPartition(partitionID int) (map[string]int64, error) {
	if s.partMgr == nil || s.mapper == nil {
		return nil, fmt.Errorf("partition manager not configured")
	}

	if err := s.partMgr.HaltPartition(partitionID); err != nil {
		return nil, err
	}

	balances := s.ledger.Snapshot()
	partBalances := make(map[string]int64)

	for acc, bal := range balances {
		if s.mapper.GetPartition(acc) == partitionID {
			partBalances[acc] = bal
		}
	}
	return partBalances, nil
}

// ReceivePartition loads pre-existing balances into the ledger for a migrating partition.
func (s *ShardServer) ReceivePartition(partitionID int, balances map[string]int64) error {
	if s.partMgr == nil {
		return fmt.Errorf("partition manager not configured")
	}

	for acc, bal := range balances {
		s.ledger.SetBalance(acc, bal)
	}
	s.partMgr.AddPartition(partitionID)
	return nil
}

// ResumePartition resumes processing for a partition.
func (s *ShardServer) ResumePartition(partitionID int) error {
	if s.partMgr == nil {
		return fmt.Errorf("partition manager not configured")
	}
	return s.partMgr.ResumePartition(partitionID)
}

// ShardID returns this shard's identifier.
func (s *ShardServer) ShardID() string {
	return s.shardID
}

// Close shuts down the shard server cleanly.
func (s *ShardServer) Close() error {
	return s.walLog.Close()
}

// SetRole sets the role of this shard server (PRIMARY or FOLLOWER).
func (s *ShardServer) SetRole(role string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.role = role
}

// Role returns the current role of this shard server.
func (s *ShardServer) Role() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.role
}

// WAL returns the underlying WAL (used by follower receiver setup).
func (s *ShardServer) WAL() *wal.WAL {
	return s.walLog
}

// Promote switches this shard from FOLLOWER to PRIMARY.
func (s *ShardServer) Promote() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.role = "PRIMARY"
	log.Printf("shard %s: promoted to PRIMARY", s.shardID)
}
