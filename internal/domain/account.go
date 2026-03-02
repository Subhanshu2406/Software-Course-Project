package domain

import (
	"fmt"
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
	ID                  string        `json:"id"`
	Passkey             string        `json:"passkey"`
	NameOfAccountHolder string        `json:"name_of_account_holder"`
	Balance             float64       `json:"balance"`
	DateCreated         time.Time     `json:"timestamp"`
	Status              AccountStatus `json:"status"`
}

// Basic functions for account management: create account, deposit, withdraw, transfer
func NewAccount(id string, nameOfAccountHolder string, balance float64, passkey string) (*Account, error) {
	return &Account{
		ID:                  id,
		NameOfAccountHolder: nameOfAccountHolder,
		Balance:             balance,
		Passkey:             passkey,
		DateCreated:         time.Now(),
		Status:              AccountActive,
	}, nil
}

func (a *Account) Deposit(amount float64) {
	a.Balance += amount
}

func (a *Account) Withdraw(amount float64) {
	a.Balance -= amount
}

func (a *Account) Transfer(amount float64, toAccount *Account) error {
	if a.Balance == 0 {
		return fmt.Errorf("Account balance is zero. Transfer aborted")
	}
	if a.Status != AccountActive {
		return fmt.Errorf("Sender account is %v. Transfer aborted", a.Status)
	}
	if toAccount.Status != AccountActive {
		return fmt.Errorf("Recipient account is %v. Transfer aborted", toAccount.Status)
	}

	if a.Balance >= amount {
		NewTransaction("", amount, "Transfer", toAccount.ID, a.ID)
		a.Withdraw(amount)
		toAccount.Deposit(amount)

		return nil
	} else {
		return fmt.Errorf("Insufficient balance")
	}
}

func (a *Account) CloseAccount() {
	a.Status = AccountClosed
}

func (a *Account) ReopenAccount() {
	a.Status = AccountActive
}
