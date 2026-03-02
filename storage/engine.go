// Package storage defines the interface for persistent shard data storage.
//
// The storage engine provides durable key-value storage for account balances,
// separate from the WAL. The WAL provides write-path durability (log-before-apply);
// the storage engine provides point-in-time state snapshots that enable faster
// recovery via checkpoint + partial WAL replay (instead of full WAL replay).
package storage

// Engine is the persistent key-value store for shard data.
// Implementations must be safe for concurrent use.
type Engine interface {
	// GetBalance returns the balance for an account.
	// Returns the balance, whether the account exists, and any error.
	GetBalance(accountID string) (int64, bool, error)

	// SetBalance sets the balance for an account, creating it if needed.
	SetBalance(accountID string, balance int64) error

	// GetAllBalances returns a copy of all account balances.
	GetAllBalances() (map[string]int64, error)

	// BatchSetBalances atomically replaces all balances.
	// Used during checkpointing to persist the entire ledger snapshot.
	BatchSetBalances(balances map[string]int64) error

	// GetCheckpointLogID returns the WAL log ID of the last successful checkpoint.
	// Returns 0 if no checkpoint has been taken.
	GetCheckpointLogID() (uint64, error)

	// SetCheckpointLogID records the WAL log ID of the latest checkpoint.
	SetCheckpointLogID(logID uint64) error

	// Close flushes pending writes and closes the storage engine.
	Close() error
}
