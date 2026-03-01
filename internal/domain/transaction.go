package domain

import "time"

type Transaction struct {
	ID            string    `json:"id"`
	Amount        int64     `json:"amount"`
	Description   string    `json:"description"`
	Timestamp     time.Time `json:"date"`
	ToAccountID   string    `json:"to_account_id"`
	FromAccountID string    `json:"from_account_id"`
}

//need to check for the existence of the to and from accounts before creating a transaction
