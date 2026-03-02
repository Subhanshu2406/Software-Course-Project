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

	"ledger-service/shared/constants"
	"ledger-service/shared/models"
	"ledger-service/shard/ledger"
	"ledger-service/shard/recovery"
	"ledger-service/shard/wal"
	"ledger-service/storage"
)

// ShardServer is the core of a shard node.
// It owns a WAL and a Ledger and coordinates safe transaction execution.
type ShardServer struct {
	mu      sync.Mutex // serializes transaction execution within this shard
	shardID string
	walLog  *wal.WAL
	ledger  *ledger.Ledger
	store   storage.Engine // optional persistent storage (nil = in-memory only)

	// seenTxns tracks transaction IDs to enforce idempotency.
	// If a txnID is re-submitted (e.g. due to coordinator retry), we return
	// the cached result rather than double-applying.
	seenTxns map[string]constants.TransactionState
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
		shardID:  shardID,
		walLog:   w,
		ledger:   l,
		seenTxns: make(map[string]constants.TransactionState),
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

	// Step 1: Validate — check source balance before touching anything
	if err := s.ledger.ValidateDebit(txn.Source, txn.Amount); err != nil {
		s.seenTxns[txn.TxnID] = constants.StateAborted

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
	_, err := s.walLog.Append(txn.TxnID, constants.OpDebit, txn.Source, txn.Amount)
	if err != nil {
		return models.TransactionResult{}, fmt.Errorf("shard %s: WAL append debit failed: %w", s.shardID, err)
	}

	_, err = s.walLog.Append(txn.TxnID, constants.OpCredit, txn.Destination, txn.Amount)
	if err != nil {
		return models.TransactionResult{}, fmt.Errorf("shard %s: WAL append credit failed: %w", s.shardID, err)
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
	_, err := s.walLog.Append(txnID, opType, accountID, amount)
	if err != nil {
		return fmt.Errorf("shard %s: WAL commit-op failed: %w", s.shardID, err)
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

// TotalBalance returns the sum of all balances (for invariant testing).
func (s *ShardServer) TotalBalance() int64 {
	return s.ledger.TotalBalance()
}

// Snapshot returns a copy of all balances (for migration / testing).
func (s *ShardServer) Snapshot() map[string]int64 {
	return s.ledger.Snapshot()
}

// ShardID returns this shard's identifier.
func (s *ShardServer) ShardID() string {
	return s.shardID
}

// Close shuts down the shard server cleanly.
func (s *ShardServer) Close() error {
	return s.walLog.Close()
}
