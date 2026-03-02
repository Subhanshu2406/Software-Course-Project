package constants

// TransactionState represents the lifecycle state of a transaction.
// Transitions: PENDING → PREPARED → COMMITTED
//              PENDING/PREPARED → ABORTED
type TransactionState string

const (
	StatePending   TransactionState = "PENDING"
	StatePrepared  TransactionState = "PREPARED"
	StateCommitted TransactionState = "COMMITTED"
	StateAborted   TransactionState = "ABORTED"
)

// OperationType represents the type of WAL log entry operation.
type OperationType string

const (
	OpDebit         OperationType = "DEBIT"
	OpCredit        OperationType = "CREDIT"
	OpPrepared      OperationType = "PREPARED"
	OpCommitted     OperationType = "COMMITTED"
	OpAborted       OperationType = "ABORTED"
	OpCreateAccount OperationType = "CREATE_ACCOUNT" // initial account setup
	OpCheckpoint    OperationType = "CHECKPOINT"      // WAL checkpoint marker
)

// Role represents the role of a shard node in the primary-follower model.
type Role string

const (
	RolePrimary  Role = "PRIMARY"
	RoleFollower Role = "FOLLOWER"
)

// Default configuration values (from SRS REQ-REP-003)
const (
	DefaultHeartbeatIntervalSec = 5
	DefaultHeartbeatMissLimit   = 3
	DefaultReplicaCount         = 2 // 1 leader + 2 followers per shard
)
