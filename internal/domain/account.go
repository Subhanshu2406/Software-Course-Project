package domain

import (
	"time"
)

// Account represents a user's account with balance and currency
type Account struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Balance   float64   `json:"balance"` // Using DECIMAL(19,4) in DB, float64 for precision in domain
	Currency  string    `json:"currency"`
	Version   int       `json:"version"` // For optimistic locking and auditing
	CreatedAt time.Time `json:"created_at"`
}
