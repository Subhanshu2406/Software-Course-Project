package storage

import (
	"Software-Course-Project/internal/domain"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxAccountRepository implements AccountRepository using pgx
type PgxAccountRepository struct {
	pool *pgxpool.Pool
}

// NewAccountRepository creates a new account repository
func NewAccountRepository(pool *pgxpool.Pool) *PgxAccountRepository {
	return &PgxAccountRepository{pool: pool}
}

// Create inserts a new account for a user.
func (r *PgxAccountRepository) Create(ctx context.Context, account *domain.Account) error {
	query := `
		INSERT INTO accounts (id, user_id, balance, currency, version, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.pool.Exec(ctx, query,
		account.ID,
		account.UserID,
		account.Balance,
		account.Currency,
		account.Version,
		account.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating account: %w", err)
	}
	return nil
}

// GetByID retrieves a single account by its ID.
// Returns (nil, nil) if not found.
func (r *PgxAccountRepository) GetByID(ctx context.Context, accountID string) (*domain.Account, error) {
	query := `
		SELECT id, user_id, balance, currency, version, created_at
		FROM accounts
		WHERE id = $1`

	account := &domain.Account{}
	err := r.pool.QueryRow(ctx, query, accountID).Scan(
		&account.ID,
		&account.UserID,
		&account.Balance,
		&account.Currency,
		&account.Version,
		&account.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetching account %s: %w", accountID, err)
	}
	return account, nil
}

// GetByUserID retrieves all accounts belonging to a user.
func (r *PgxAccountRepository) GetByUserID(ctx context.Context, userID string) ([]*domain.Account, error) {
	query := `
		SELECT id, user_id, balance, currency, version, created_at
		FROM accounts
		WHERE user_id = $1
		ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("fetching accounts for user %s: %w", userID, err)
	}
	defer rows.Close()

	var accounts []*domain.Account
	for rows.Next() {
		account := &domain.Account{}
		if err := rows.Scan(
			&account.ID,
			&account.UserID,
			&account.Balance,
			&account.Currency,
			&account.Version,
			&account.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning account row: %w", err)
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (r *PgxAccountRepository) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, accountID string) (*domain.Account, error) {
	query := `
		SELECT id, user_id, balance, currency, version, created_at
		FROM accounts
		WHERE id = $1
		FOR UPDATE`

	account := &domain.Account{}
	err := tx.QueryRow(ctx, query, accountID).Scan(
		&account.ID,
		&account.UserID,
		&account.Balance,
		&account.Currency,
		&account.Version,
		&account.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetching account %s for update: %w", accountID, err)
	}
	return account, nil
}

// Update saves a new balance using optimistic locking.
// The WHERE clause matches on both id AND the current version, then increments
// version. If another transaction updated the account first, version will have
// changed and RowsAffected will be 0 — signalling a conflict to the caller.
func (r *PgxAccountRepository) Update(ctx context.Context, account *domain.Account) error {
	query := `
		UPDATE accounts
		SET balance = $1, version = version + 1
		WHERE id = $2 AND version = $3`

	result, err := r.pool.Exec(ctx, query, account.Balance, account.ID, account.Version)
	if err != nil {
		return fmt.Errorf("updating account %s: %w", account.ID, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("account %s: version conflict", account.ID)
	}
	return nil
}

// Delete removes an account by ID.
func (r *PgxAccountRepository) Delete(ctx context.Context, accountID string) error {
	query := `DELETE FROM accounts WHERE id = $1`
	result, err := r.pool.Exec(ctx, query, accountID)
	if err != nil {
		return fmt.Errorf("deleting account %s: %w", accountID, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("account %s not found", accountID)
	}
	return nil
}
