// Package ledger manages account balances for a single shard.
//
// The ledger enforces three invariants at all times (per the SRS):
//   1. No negative account balances: balance(account) >= 0
//   2. Conservation of total monetary value: sum(balances) is constant
//   3. No partial commits: debit+credit are atomic within a shard
//
// The ledger does NOT write to the WAL — that is the caller's responsibility.
// The caller (shard server) must always write to WAL BEFORE calling Apply*.
package ledger

import (
	"fmt"
	"sync"
)

// Ledger stores account balances for a subset of accounts on this shard.
// It is safe for concurrent reads and writes via its internal RWMutex.
type Ledger struct {
	mu       sync.RWMutex
	balances map[string]int64 // accountID → balance (in smallest currency unit)
}

// NewLedger creates an empty ledger.
func NewLedger() *Ledger {
	return &Ledger{
		balances: make(map[string]int64),
	}
}

// NewLedgerWithAccounts creates a ledger pre-populated with accounts and balances.
// Used during shard initialization and crash recovery.
func NewLedgerWithAccounts(initial map[string]int64) (*Ledger, error) {
	for id, bal := range initial {
		if bal < 0 {
			return nil, fmt.Errorf("ledger: initial balance for %s is negative (%d)", id, bal)
		}
	}
	l := &Ledger{
		balances: make(map[string]int64, len(initial)),
	}
	for id, bal := range initial {
		l.balances[id] = bal
	}
	return l, nil
}

// GetBalance returns the balance for an account.
// Returns 0 and false if the account does not exist.
func (l *Ledger) GetBalance(accountID string) (int64, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	bal, ok := l.balances[accountID]
	return bal, ok
}

// CreateAccount adds a new account with the given opening balance.
// Returns an error if the account already exists or if balance is negative.
func (l *Ledger) CreateAccount(accountID string, openingBalance int64) error {
	if openingBalance < 0 {
		return fmt.Errorf("ledger: opening balance cannot be negative")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.balances[accountID]; exists {
		return fmt.Errorf("ledger: account %s already exists", accountID)
	}

	l.balances[accountID] = openingBalance
	return nil
}

// ApplyTransfer atomically debits source and credits destination on this shard.
// This is used ONLY for single-shard transactions where both accounts live here.
//
// IMPORTANT: The caller MUST have already written and fsynced the WAL entry
// before calling this method. This enforces the log-before-apply rule.
//
// Returns error if:
//   - source account does not exist
//   - destination account does not exist
//   - source balance would go negative
//   - amount is not positive
func (l *Ledger) ApplyTransfer(sourceID, destID string, amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("ledger: transfer amount must be positive, got %d", amount)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	srcBal, ok := l.balances[sourceID]
	if !ok {
		return fmt.Errorf("ledger: source account %s not found", sourceID)
	}

	_, ok = l.balances[destID]
	if !ok {
		return fmt.Errorf("ledger: destination account %s not found", destID)
	}

	// Invariant check: no negative balances
	if srcBal < amount {
		return fmt.Errorf("ledger: insufficient balance in %s: have %d, need %d", sourceID, srcBal, amount)
	}

	// Atomic update — both happen under the same lock
	l.balances[sourceID] -= amount
	l.balances[destID] += amount

	return nil
}

// ApplyDebit subtracts amount from an account.
// Used in the PREPARE phase of cross-shard transactions (reserves the funds).
// The caller must have written a WAL entry first.
func (l *Ledger) ApplyDebit(accountID string, amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("ledger: debit amount must be positive, got %d", amount)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	bal, ok := l.balances[accountID]
	if !ok {
		return fmt.Errorf("ledger: account %s not found", accountID)
	}
	if bal < amount {
		return fmt.Errorf("ledger: insufficient balance in %s: have %d, need %d", accountID, bal, amount)
	}

	l.balances[accountID] -= amount
	return nil
}

// ApplyCredit adds amount to an account.
// Used in the COMMIT phase of cross-shard transactions (delivers the funds).
// The caller must have written a WAL entry first.
func (l *Ledger) ApplyCredit(accountID string, amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("ledger: credit amount must be positive, got %d", amount)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	_, ok := l.balances[accountID]
	if !ok {
		return fmt.Errorf("ledger: account %s not found", accountID)
	}

	l.balances[accountID] += amount
	return nil
}

// RollbackDebit reverses a previously applied debit (used on ABORT).
// The caller must have written a WAL ABORTED entry first.
func (l *Ledger) RollbackDebit(accountID string, amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("ledger: rollback amount must be positive")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	_, ok := l.balances[accountID]
	if !ok {
		return fmt.Errorf("ledger: account %s not found for rollback", accountID)
	}

	l.balances[accountID] += amount
	return nil
}

// ValidateDebit checks if a debit would succeed WITHOUT applying it.
// Used during the PREPARE phase to answer "can you do this?" before committing.
func (l *Ledger) ValidateDebit(accountID string, amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("ledger: debit amount must be positive")
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	bal, ok := l.balances[accountID]
	if !ok {
		return fmt.Errorf("ledger: account %s not found", accountID)
	}
	if bal < amount {
		return fmt.Errorf("ledger: insufficient balance in %s: have %d, need %d", accountID, bal, amount)
	}

	return nil
}

// TotalBalance returns the sum of all balances on this shard.
// Used for invariant verification in tests (conservation of money).
func (l *Ledger) TotalBalance() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var total int64
	for _, bal := range l.balances {
		total += bal
	}
	return total
}

// Snapshot returns a copy of all balances (used for partition migration and tests).
func (l *Ledger) Snapshot() map[string]int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	snap := make(map[string]int64, len(l.balances))
	for id, bal := range l.balances {
		snap[id] = bal
	}
	return snap
}

// AccountExists returns true if the account is known to this shard.
func (l *Ledger) AccountExists(accountID string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.balances[accountID]
	return ok
}

// SetBalance directly sets the balance for an account, creating it if necessary.
// Used ONLY during crash recovery where the WAL is the source of truth.
// This bypasses normal validation since recovery replays known-good committed ops.
func (l *Ledger) SetBalance(accountID string, balance int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.balances[accountID] = balance
}

// LoadBalances replaces all account balances from a snapshot.
// Used when loading state from the storage engine on startup.
func (l *Ledger) LoadBalances(balances map[string]int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.balances = make(map[string]int64, len(balances))
	for id, bal := range balances {
		l.balances[id] = bal
	}
}

// AccountCount returns the number of accounts in the ledger.
func (l *Ledger) AccountCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.balances)
}
