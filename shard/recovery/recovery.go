// Package recovery implements WAL-based crash recovery for a shard.
//
// This implements Algorithm 5 from the project report:
//   1. Read WAL sequentially from stable storage
//   2. For each entry:
//      - If CREATE_ACCOUNT → set initial balance directly
//      - If COMMITTED debit/credit → accumulate net balance change
//      - If PREPARED  → retain for coordinator reconciliation
//      - Otherwise    → skip (uncommitted, abort)
//   3. Apply accumulated balance deltas to reconstruct in-memory ledger state
//
// Uses a balance-accumulation strategy rather than replaying individual
// ApplyDebit/ApplyCredit calls. This avoids the bug where a debit fails on
// a zero-balance account during recovery from an empty ledger.
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
	AppliedCount  int      // number of committed operations replayed
	SkippedCount  int      // uncommitted entries skipped
	PendingTxns   []string // txnIDs in PREPARED state awaiting coordinator decision
	CommittedTxns []string // txnIDs that were committed (for idempotency rebuild)
	AbortedTxns   []string // txnIDs that were aborted (for idempotency rebuild)
}

// Recover replays the WAL to reconstruct ledger state after a crash.
// It reads all entries in order, accumulates net balance changes from committed
// transactions, and applies them to the ledger.
// Returns a RecoveryResult describing what was found and applied.
func Recover(w *wal.WAL, l *ledger.Ledger) (*RecoveryResult, error) {
	entries, err := w.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("recovery: failed to read WAL: %w", err)
	}

	log.Printf("recovery: found %d WAL entries to process", len(entries))

	if len(entries) == 0 {
		return &RecoveryResult{}, nil
	}

	result := &RecoveryResult{}

	// First pass: build a map of txnID → final state.
	// We need to know which transactions ultimately committed before applying ops.
	finalState := buildFinalStateMap(entries)

	// Populate committed/aborted txn lists for idempotency map rebuild.
	for txnID, state := range finalState {
		switch state {
		case constants.StateCommitted:
			result.CommittedTxns = append(result.CommittedTxns, txnID)
		case constants.StateAborted:
			result.AbortedTxns = append(result.AbortedTxns, txnID)
		}
	}

	// Second pass: process entries.
	// - CREATE_ACCOUNT entries set initial balances directly.
	// - DEBIT/CREDIT entries are accumulated as deltas (avoids the
	//   bug where ApplyDebit fails on a zero-balance recovery ledger).
	// - PREPARED entries with no final decision are marked as pending.
	balanceDeltas := make(map[string]int64)

	for _, entry := range entries {
		switch entry.OpType {
		case constants.OpCreateAccount:
			state, known := finalState[entry.TxnID]
			if !known || state != constants.StateCommitted {
				result.SkippedCount++
				continue
			}
			// Set initial balance directly — this creates the account
			l.SetBalance(entry.AccountID, entry.Amount)
			result.AppliedCount++

		case constants.OpDebit:
			state, known := finalState[entry.TxnID]
			if !known || state != constants.StateCommitted {
				result.SkippedCount++
				continue
			}
			// Accumulate debit as negative delta
			balanceDeltas[entry.AccountID] -= entry.Amount
			result.AppliedCount++

		case constants.OpCredit:
			state, known := finalState[entry.TxnID]
			if !known || state != constants.StateCommitted {
				result.SkippedCount++
				continue
			}
			// Accumulate credit as positive delta
			balanceDeltas[entry.AccountID] += entry.Amount
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

		case constants.OpCommitted, constants.OpAborted, constants.OpCheckpoint:
			// Terminal markers and checkpoints — handled in buildFinalStateMap.
			// Nothing to do here.

		default:
			log.Printf("recovery: unknown op type %q in entry %d — skipping", entry.OpType, entry.LogID)
			result.SkippedCount++
		}
	}

	// Apply accumulated balance deltas to the ledger.
	// The ledger already has initial balances from CREATE_ACCOUNT entries;
	// we now apply the net effect of all committed DEBIT/CREDIT operations.
	for accountID, delta := range balanceDeltas {
		currentBal, exists := l.GetBalance(accountID)
		if !exists {
			currentBal = 0
		}
		l.SetBalance(accountID, currentBal+delta)
	}

	log.Printf("recovery: complete — applied=%d skipped=%d pending=%d committed_txns=%d aborted_txns=%d",
		result.AppliedCount, result.SkippedCount, len(result.PendingTxns),
		len(result.CommittedTxns), len(result.AbortedTxns))

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
		case constants.OpDebit, constants.OpCredit, constants.OpCreateAccount:
			if _, exists := states[entry.TxnID]; !exists {
				states[entry.TxnID] = constants.StatePending
			}
		}
	}

	return states
}
