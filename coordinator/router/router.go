// Package router determines whether a transaction is single-shard or cross-shard
// and dispatches it to the appropriate execution path.
//
// Per the SRS: "The Transaction Coordinator is responsible for consuming
// transaction events, determining target shard(s) for involved accounts,
// routing requests to appropriate shard leaders, and orchestrating 2PC
// when multiple shards are involved."
package router

import (
	"fmt"
	"log"

	"ledger-service/coordinator/shardmap"
	"ledger-service/coordinator/twopc"
	"ledger-service/messaging"
	"ledger-service/shared/models"
	"ledger-service/shared/utils"
)

// Router determines the execution path for each transaction.
type Router struct {
	shardMap *shardmap.ShardMap
	mapper   *utils.PartitionMapper
	client   *messaging.ShardClient
	twoPC    *twopc.Coordinator
}

// NewRouter creates a new transaction router.
func NewRouter(shardMap *shardmap.ShardMap, mapper *utils.PartitionMapper, client *messaging.ShardClient) *Router {
	return &Router{
		shardMap: shardMap,
		mapper:   mapper,
		client:   client,
		twoPC:    twopc.NewCoordinator(client),
	}
}

// Route processes a transaction by routing it to the correct shard(s).
// Returns the result of the transaction execution.
func (r *Router) Route(txn models.Transaction) (models.TransactionResult, error) {
	// Determine which partition each account belongs to
	srcPartition := r.mapper.GetPartition(txn.Source)
	dstPartition := r.mapper.GetPartition(txn.Destination)

	// Resolve shards for each partition
	srcShard, err := r.shardMap.GetShardForAccount(txn.Source, r.mapper)
	if err != nil {
		return models.TransactionResult{}, fmt.Errorf("router: failed to resolve source shard: %w", err)
	}

	dstShard, err := r.shardMap.GetShardForAccount(txn.Destination, r.mapper)
	if err != nil {
		return models.TransactionResult{}, fmt.Errorf("router: failed to resolve destination shard: %w", err)
	}

	// Route based on whether both accounts are on the same shard
	if srcShard.ShardID == dstShard.ShardID {
		log.Printf("router: txn %s → single-shard (shard %s, partitions %d→%d)",
			txn.TxnID, srcShard.ShardID, srcPartition, dstPartition)
		return r.executeSingleShard(txn, srcShard)
	}

	log.Printf("router: txn %s → cross-shard (src shard %s p%d, dst shard %s p%d)",
		txn.TxnID, srcShard.ShardID, srcPartition, dstShard.ShardID, dstPartition)
	return r.executeCrossShard(txn, srcShard, dstShard)
}

// executeSingleShard delegates a single-shard transaction to the owning shard.
func (r *Router) executeSingleShard(txn models.Transaction, shard shardmap.ShardInfo) (models.TransactionResult, error) {
	result, err := r.client.Execute(shard.Address, txn)
	if err != nil {
		return models.TransactionResult{}, fmt.Errorf("router: single-shard execution failed: %w", err)
	}
	return result, nil
}

// executeCrossShard uses the 2PC coordinator for cross-shard transactions.
func (r *Router) executeCrossShard(txn models.Transaction, srcShard, dstShard shardmap.ShardInfo) (models.TransactionResult, error) {
	return r.twoPC.Execute(txn, srcShard, dstShard)
}
