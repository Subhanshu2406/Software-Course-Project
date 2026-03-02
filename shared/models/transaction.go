package models

import (
	"time"

	"ledger-service/shared/constants"
)

// Transaction is the core unit of work in the ledger system.
// T = (txnID, sourceAccount, destinationAccount, amount) per the report spec.
type Transaction struct {
	TxnID       string                      `json:"txn_id"`
	Source      string                      `json:"source"`
	Destination string                      `json:"destination"`
	Amount      int64                       `json:"amount"` // stored in smallest unit (e.g. paise/cents) to avoid float issues
	State       constants.TransactionState  `json:"state"`
	CreatedAt   time.Time                   `json:"created_at"`
}

// TransactionResult is returned after attempting to execute a transaction.
type TransactionResult struct {
	TxnID   string                     `json:"txn_id"`
	State   constants.TransactionState `json:"state"`
	Message string                     `json:"message,omitempty"`
}
