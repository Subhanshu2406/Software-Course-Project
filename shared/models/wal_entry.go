package models

import (
	"time"

	"ledger-service/shared/constants"
)

// WALEntry is a single record in the Write-Ahead Log.
// Per REQ-DATA-002: must contain log ID, transaction ID, operation type, timestamp.
// The log-before-apply rule means this MUST be fsynced before any state change.
type WALEntry struct {
	LogID     uint64                 `json:"log_id"`    // monotonically increasing
	TxnID     string                 `json:"txn_id"`
	OpType    constants.OperationType `json:"op_type"`
	AccountID string                 `json:"account_id,omitempty"` // which account is affected
	Amount    int64                  `json:"amount,omitempty"`     // positive for both debit and credit; op_type disambiguates
	Timestamp time.Time              `json:"timestamp"`

	// Committed marks whether this entry has been committed (used during recovery).
	// An entry is uncommitted if written but not yet quorum-acked.
	Committed bool `json:"committed"`

	// CheckpointLogID is set only for CHECKPOINT entries.
	// It records the highest LogID that has been persisted to stable storage.
	CheckpointLogID uint64 `json:"checkpoint_log_id,omitempty"`
}

// ShardMetrics is reported by each shard to the Load Monitor.
// Used by REQ-DATA-003 (load balancing) and REQ-REP-003 (heartbeat).
type ShardMetrics struct {
	ShardID        string  `json:"shard_id"`
	CPUUsage       float64 `json:"cpu_usage"`        // 0.0 - 1.0
	TotalQPS       float64 `json:"total_qps"`        // transactions per second
	QueueDepth     int     `json:"queue_depth"`      // pending requests
	ReplicationLag int     `json:"replication_lag"`  // entries not yet replicated
}
