// Package twopc implements the Two-Phase Commit protocol for cross-shard transactions.
//
// Per Algorithm 2 from the project report:
//   1. Send PREPARE to all participating shards concurrently
//   2. If ALL respond PREPARED → send COMMIT to all
//   3. If ANY responds ABORT → send ABORT to all PREPARED shards
//
// The coordinator is stateless — all durability is in the shard WALs.
// If the coordinator crashes during 2PC, shards with PREPARED entries
// will query the coordinator on recovery to resolve the outcome.
package twopc

import (
	"log"

	"ledger-service/coordinator/shardmap"
	"ledger-service/messaging"
	"ledger-service/shared/constants"
	"ledger-service/shared/models"
)

// Coordinator orchestrates the Two-Phase Commit protocol.
type Coordinator struct {
	client *messaging.ShardClient
}

// NewCoordinator creates a new 2PC coordinator.
func NewCoordinator(client *messaging.ShardClient) *Coordinator {
	return &Coordinator{client: client}
}

// Execute runs the full 2PC protocol for a cross-shard transaction.
// sourceShard handles the DEBIT; destShard handles the CREDIT.
func (c *Coordinator) Execute(txn models.Transaction, sourceShard, destShard shardmap.ShardInfo) (models.TransactionResult, error) {
	log.Printf("2pc: starting txn %s (source shard %s → dest shard %s)",
		txn.TxnID, sourceShard.ShardID, destShard.ShardID)

	// ---- PHASE 1: PREPARE ----
	sourcePrepared, destPrepared := c.sendPrepares(txn, sourceShard, destShard)

	// ---- PHASE 2: COMMIT or ABORT ----
	if sourcePrepared && destPrepared {
		return c.commitAll(txn, sourceShard, destShard)
	}
	return c.abortAll(txn, sourceShard, destShard, sourcePrepared, destPrepared)
}

// sendPrepares sends PREPARE to both shards concurrently and returns success flags.
func (c *Coordinator) sendPrepares(txn models.Transaction, sourceShard, destShard shardmap.ShardInfo) (bool, bool) {
	type prepareResult struct {
		shardID string
		err     error
	}
	results := make(chan prepareResult, 2)

	// Prepare source shard (DEBIT)
	go func() {
		err := c.client.Prepare(sourceShard.Address, txn.TxnID, constants.OpDebit, txn.Source, txn.Amount)
		results <- prepareResult{shardID: sourceShard.ShardID, err: err}
	}()

	// Prepare dest shard (CREDIT)
	go func() {
		err := c.client.Prepare(destShard.Address, txn.TxnID, constants.OpCredit, txn.Destination, txn.Amount)
		results <- prepareResult{shardID: destShard.ShardID, err: err}
	}()

	// Collect responses
	var sourcePrepared, destPrepared bool
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			log.Printf("2pc: txn %s — shard %s PREPARE failed: %v", txn.TxnID, r.shardID, r.err)
		} else {
			if r.shardID == sourceShard.ShardID {
				sourcePrepared = true
			} else {
				destPrepared = true
			}
			log.Printf("2pc: txn %s — shard %s PREPARED", txn.TxnID, r.shardID)
		}
	}

	return sourcePrepared, destPrepared
}

// commitAll sends COMMIT to both shards with retries to prevent partial commit.
// Once PREPARE has succeeded on both shards, the transaction MUST eventually commit.
// If a COMMIT message fails, it is retried to preserve the money invariant.
func (c *Coordinator) commitAll(txn models.Transaction, sourceShard, destShard shardmap.ShardInfo) (models.TransactionResult, error) {
	log.Printf("2pc: txn %s — all shards PREPARED, sending COMMIT", txn.TxnID)

	// Commit source shard (apply DEBIT was already reserved, this just marks committed)
	if err := c.commitWithRetry(sourceShard.Address, txn.TxnID, constants.OpDebit, txn.Source, txn.Amount, sourceShard.ShardID); err != nil {
		// Even on failure after retries, we must still try to commit the other side
		log.Printf("2pc: txn %s — WARNING: commit debit failed after retries on shard %s: %v", txn.TxnID, sourceShard.ShardID, err)
	}

	// Commit dest shard (apply CREDIT)
	if err := c.commitWithRetry(destShard.Address, txn.TxnID, constants.OpCredit, txn.Destination, txn.Amount, destShard.ShardID); err != nil {
		log.Printf("2pc: txn %s — WARNING: commit credit failed after retries on shard %s: %v", txn.TxnID, destShard.ShardID, err)
	}

	log.Printf("2pc: txn %s COMMITTED", txn.TxnID)
	return models.TransactionResult{
		TxnID:   txn.TxnID,
		State:   constants.StateCommitted,
		Message: "cross-shard: committed on all shards",
	}, nil
}

// commitWithRetry attempts to commit on a shard with up to 3 retries.
func (c *Coordinator) commitWithRetry(addr, txnID string, opType constants.OperationType, accountID string, amount int64, shardID string) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := c.client.Commit(addr, txnID, opType, accountID, amount); err != nil {
			lastErr = err
			log.Printf("2pc: txn %s — commit %s on %s attempt %d failed: %v", txnID, opType, shardID, attempt+1, err)
			continue
		}
		return nil
	}
	return lastErr
}

// abortAll sends ABORT to all shards that were prepared.
func (c *Coordinator) abortAll(txn models.Transaction, sourceShard, destShard shardmap.ShardInfo, sourcePrepared, destPrepared bool) (models.TransactionResult, error) {
	log.Printf("2pc: txn %s — not all shards prepared, sending ABORT", txn.TxnID)

	if sourcePrepared {
		if err := c.client.Abort(sourceShard.Address, txn.TxnID, constants.OpDebit, txn.Source, txn.Amount); err != nil {
			log.Printf("2pc: txn %s — abort debit on shard %s failed: %v", txn.TxnID, sourceShard.ShardID, err)
		}
	}

	if destPrepared {
		if err := c.client.Abort(destShard.Address, txn.TxnID, constants.OpCredit, txn.Destination, txn.Amount); err != nil {
			log.Printf("2pc: txn %s — abort credit on shard %s failed: %v", txn.TxnID, destShard.ShardID, err)
		}
	}

	return models.TransactionResult{
		TxnID:   txn.TxnID,
		State:   constants.StateAborted,
		Message: "cross-shard: one or more shards rejected",
	}, nil
}
