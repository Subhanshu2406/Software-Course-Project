package storage

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"Software-Course-Project/internal/domain"
)

type AccountRepository struct {
	db *pgxpool.Pool
}

func NewAccountRepository(db *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) Create(ctx context.Context, account *domain.Account) error {
	query := `
        INSERT INTO accounts (id, passkey, name_of_account_holder, balance, date_created, status)
        VALUES ($1, $2, $3, $4, $5, $6)
    `
	_, err := r.db.Exec(ctx, query,
		account.ID,
		account.Passkey,
		account.NameOfAccountHolder,
		account.Balance,
		account.DateCreated,
		account.Status,
	)
	return err
}

func (r *AccountRepository) GetByID(ctx context.Context, id string) (*domain.Account, error) {
	var account domain.Account
	query := `
        SELECT id, passkey, name_of_account_holder, balance, date_created, status
        FROM accounts WHERE id = $1
    `
	err := r.db.QueryRow(ctx, query, id).Scan(
		&account.ID,
		&account.Passkey,
		&account.NameOfAccountHolder,
		&account.Balance,
		&account.DateCreated,
		&account.Status,
	)
	if err == pgx.ErrNoRows {
		return nil, errors.New("account not found")
	}
	return &account, err
}

func (r *AccountRepository) Update(ctx context.Context, account *domain.Account) error {
	query := `
        UPDATE accounts
        SET balance = $1, status = $2
        WHERE id = $3
    `
	_, err := r.db.Exec(ctx, query, account.Balance, account.Status, account.ID)
	return err
}

func (r *AccountRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, "DELETE FROM accounts WHERE id = $1", id)
	return err
}
