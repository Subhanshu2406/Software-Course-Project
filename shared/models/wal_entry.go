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
	ShardID             string  `json:"shard_id"`
	Role                string  `json:"role"`
	CPUUsage            float64 `json:"cpu_usage"`
	TotalQPS            float64 `json:"total_qps"`
	QueueDepth          int     `json:"queue_depth"`
	ReplicationLag      int     `json:"replication_lag"`
	WALIndex            uint64  `json:"wal_index"`
	LastCheckpointLogID uint64  `json:"last_checkpoint_log_id"`
	FollowerCount       int     `json:"follower_count"`
	AccountCount        int     `json:"account_count"`
	TotalBalance        int64   `json:"total_balance"`
	UptimeSeconds       int64   `json:"uptime_seconds"`
	CommittedCount      int64   `json:"committed_count"`
	AbortedCount        int64   `json:"aborted_count"`
	PreparedCount       int64   `json:"prepared_count"`
}

// TxnSummary is a lightweight transaction record for recent-transaction feeds.
type TxnSummary struct {
	TxnID       string                     `json:"txn_id"`
	Source      string                     `json:"source"`
	Destination string                     `json:"destination"`
	Amount      int64                      `json:"amount"`
	Type        string                     `json:"type"`
	State       constants.TransactionState `json:"state"`
	LatencyMs   int64                      `json:"latency_ms"`
	Timestamp   time.Time                  `json:"timestamp"`
	ShardID     string                     `json:"shard_id"`
	Shards      []string                   `json:"shards,omitempty"`
}
