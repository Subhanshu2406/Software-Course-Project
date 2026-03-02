package utils

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// PartitionMapper handles deterministic account-to-partition assignment.
// Per REQ-DATA-001: all entries belonging to the same account MUST always
// map to the same logical shard partition.
type PartitionMapper struct {
	numPartitions int // total logical partitions across the cluster
}

// NewPartitionMapper creates a mapper for a given number of logical partitions.
// A typical setup: 3 shards × 10 partitions each = 30 total partitions.
func NewPartitionMapper(numPartitions int) *PartitionMapper {
	if numPartitions <= 0 {
		panic("numPartitions must be positive")
	}
	return &PartitionMapper{numPartitions: numPartitions}
}

// GetPartition deterministically maps an accountID to a logical partition index [0, numPartitions).
// Uses SHA-256 so the distribution is uniform and stable across restarts.
func (p *PartitionMapper) GetPartition(accountID string) int {
	h := sha256.Sum256([]byte(accountID))
	// Take first 8 bytes as a uint64 and mod by partition count
	val := binary.BigEndian.Uint64(h[:8])
	return int(val % uint64(p.numPartitions))
}

// IsSameShard returns true if both accounts map to the same logical partition.
// The caller (Coordinator) uses this to decide single-shard vs cross-shard path.
func (p *PartitionMapper) IsSamePartition(accountA, accountB string) bool {
	return p.GetPartition(accountA) == p.GetPartition(accountB)
}

// PartitionKey returns a human-readable partition label for an account.
func (p *PartitionMapper) PartitionKey(accountID string) string {
	return fmt.Sprintf("partition-%d", p.GetPartition(accountID))
}

// NumPartitions returns the total number of logical partitions.
func (p *PartitionMapper) NumPartitions() int {
	return p.numPartitions
}
