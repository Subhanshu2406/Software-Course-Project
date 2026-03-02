package domain

import "time"

type Transaction struct {
	ID            string    `json:"id"`
	Amount        float64   `json:"amount"`
	Description   string    `json:"description"`
	Timestamp     time.Time `json:"date"`
	ToAccountID   string    `json:"to_account_id"`
	FromAccountID string    `json:"from_account_id"`
}

//need to check for the existence of the to and from accounts before creating a transaction

func NewTransaction(id string, amount float64, description string, toAccountID string, fromAccountID string) *Transaction {
	return &Transaction{
		ID:            id,
		Amount:        amount,
		Description:   description,
		Timestamp:     time.Now(),
		ToAccountID:   toAccountID,
		FromAccountID: fromAccountID,
	}
}
