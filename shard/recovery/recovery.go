// Package recovery implements WAL-based crash recovery for a shard.
//
// This implements Algorithm 5 from the project report:
//   1. Read WAL sequentially from stable storage
//   2. For each entry:
//      - If COMMITTED → apply operation to ledger state
//      - If PREPARED  → retain for coordinator reconciliation
//      - Otherwise    → skip (uncommitted, abort)
//   3. Reconstruct in-memory ledger state
//
// Three crash scenarios handled (per Section VII.E of report):
//   - Crash before flush:   entry absent from WAL → no state change needed
//   - Crash after flush but before commit: entry present, not committed → skip
//   - Crash after commit: COMMITTED record exists → replay safely
package recovery

import (
	"fmt"
	"log"

	"ledger-service/shared/constants"
	"ledger-service/shared/models"
	"ledger-service/shard/ledger"
	"ledger-service/shard/wal"
)

// RecoveryResult summarizes what happened during WAL replay.
type RecoveryResult struct {
	AppliedCount  int      // number of committed transactions replayed
	SkippedCount  int      // uncommitted entries skipped
	PendingTxns   []string // txnIDs in PREPARED state awaiting coordinator decision
}

// Recover replays the WAL to reconstruct ledger state after a crash.
// It reads all entries in order and rebuilds the ledger from committed operations.
// Returns a RecoveryResult describing what was found and applied.
func Recover(w *wal.WAL, l *ledger.Ledger) (*RecoveryResult, error) {
	entries, err := w.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("recovery: failed to read WAL: %w", err)
	}

	log.Printf("recovery: found %d WAL entries to process", len(entries))

	result := &RecoveryResult{}

	// First pass: build a map of txnID → final state
	// We need to know which transactions ultimately committed before applying ops.
	finalState := buildFinalStateMap(entries)

	// Track which accounts we've seen to rebuild the ledger
	// We process ops in log order, applying only those whose txn committed.
	for _, entry := range entries {
		switch entry.OpType {
		case constants.OpDebit:
			state, known := finalState[entry.TxnID]
			if !known || state != constants.StateCommitted {
				// This debit belongs to an uncommitted or aborted transaction
				result.SkippedCount++
				continue
			}
			// Apply debit to ledger (creates account at 0 if it doesn't exist,
			// then debits — this handles the case where the account was created
			// in this same recovery session)
			if err := ensureAccount(l, entry.AccountID); err != nil {
				return nil, fmt.Errorf("recovery: ensure account %s: %w", entry.AccountID, err)
			}
			if err := l.ApplyDebit(entry.AccountID, entry.Amount); err != nil {
				return nil, fmt.Errorf("recovery: apply debit for txn %s: %w", entry.TxnID, err)
			}
			result.AppliedCount++

		case constants.OpCredit:
			state, known := finalState[entry.TxnID]
			if !known || state != constants.StateCommitted {
				result.SkippedCount++
				continue
			}
			if err := ensureAccount(l, entry.AccountID); err != nil {
				return nil, fmt.Errorf("recovery: ensure account %s: %w", entry.AccountID, err)
			}
			if err := l.ApplyCredit(entry.AccountID, entry.Amount); err != nil {
				return nil, fmt.Errorf("recovery: apply credit for txn %s: %w", entry.TxnID, err)
			}
			result.AppliedCount++

		case constants.OpPrepared:
			// PREPARED means the coordinator sent PREPARE and this shard agreed,
			// but we haven't seen the final COMMIT or ABORT for it yet.
			// Per Algorithm 5: "Retain for coordinator reconciliation"
			state, known := finalState[entry.TxnID]
			if !known || (state != constants.StateCommitted && state != constants.StateAborted) {
				result.PendingTxns = append(result.PendingTxns, entry.TxnID)
				log.Printf("recovery: txn %s is in PREPARED state — awaiting coordinator decision", entry.TxnID)
			}

		case constants.OpCommitted, constants.OpAborted:
			// These are terminal markers — already handled in buildFinalStateMap.
			// Nothing to do here.

		default:
			log.Printf("recovery: unknown op type %q in entry %d — skipping", entry.OpType, entry.LogID)
			result.SkippedCount++
		}
	}

	log.Printf("recovery: complete — applied=%d skipped=%d pending=%d",
		result.AppliedCount, result.SkippedCount, len(result.PendingTxns))

	return result, nil
}

// buildFinalStateMap scans the WAL and determines the final state of each transaction.
// A txn is COMMITTED only if a COMMITTED record exists for it.
// A txn is ABORTED only if an ABORTED record exists for it.
// Otherwise it is considered in-flight / PREPARED.
func buildFinalStateMap(entries []models.WALEntry) map[string]constants.TransactionState {
	states := make(map[string]constants.TransactionState)

	for _, entry := range entries {
		switch entry.OpType {
		case constants.OpCommitted:
			states[entry.TxnID] = constants.StateCommitted
		case constants.OpAborted:
			// Only set ABORTED if not already COMMITTED (committed always wins)
			if states[entry.TxnID] != constants.StateCommitted {
				states[entry.TxnID] = constants.StateAborted
			}
		case constants.OpPrepared:
			if _, exists := states[entry.TxnID]; !exists {
				states[entry.TxnID] = constants.StatePrepared
			}
		case constants.OpDebit, constants.OpCredit:
			if _, exists := states[entry.TxnID]; !exists {
				states[entry.TxnID] = constants.StatePending
			}
		}
	}

	return states
}

// ensureAccount creates an account with zero balance if it doesn't already exist.
// During recovery, accounts may not exist yet if the ledger is freshly created.
func ensureAccount(l *ledger.Ledger, accountID string) error {
	if accountID == "" {
		return nil
	}
	if !l.AccountExists(accountID) {
		// Create with zero balance; the WAL replay will set the correct balance
		return l.CreateAccount(accountID, 0)
	}
	return nil
}
