package domain

import (
	"time"
)

type AccountStatus int

const (
	AccountActive AccountStatus = iota
	AccountClosed
	// AccountSuspended when a user is making too many transactions in a short period of time, or if there are suspicious activities on the account, we will suspend the accoutn for the time being. This logic shall come in the second sprint.
)

// need to check how the attributes  are being pulled
type Account struct {
	ID          string        `json:"id"`
	UserID      string        `json:"user_id"`
	Balance     int64         `json:"balance"` //stores balance in paise not rupees to avoid floating point precision issues
	DateCreated time.Time     `json:"timestamp"`
	Status      AccountStatus `json:"status"`
}

// Basic functions for account management: create account, deposit, withdraw, transfer
func NewAccount(id string, userID string, balance int64) (*Account, error) {
	return &Account{
		ID:          id,
		UserID:      userID,
		Balance:     balance,
		DateCreated: time.Now(),
		Status:      AccountActive,
	}, nil
}
