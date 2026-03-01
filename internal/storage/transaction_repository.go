package storage

import (
	"Software-Course-Project/internal/domain"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxTransactionRepository implements TransactionRepository using pgx
type PgxTransactionRepository struct {
	pool *pgxpool.Pool
}

// NewTransactionRepository creates a new transaction repository
func NewTransactionRepository(pool *pgxpool.Pool) *PgxTransactionRepository {
	return &PgxTransactionRepository{pool: pool}
}

// Create inserts a new transaction record.
// Should always be called inside a transfer transaction alongside balance updates,
// so the transaction log and account balances stay in sync.
func (r *PgxTransactionRepository) Create(ctx context.Context, transaction *domain.Transaction) error {
	query := `
		INSERT INTO transactions (id, from_account_id, to_account_id, amount, type, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.pool.Exec(ctx, query,
		transaction.ID,
		transaction.FromAccountID,
		transaction.ToAccountID,
		transaction.Amount,
		transaction.Type,
		transaction.Status,
		transaction.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating transaction: %w", err)
	}
	return nil
}

// CreateWithTx inserts a new transaction record within an existing DB transaction.
// Use this instead of Create when you need the insert to be part of a larger
// atomic operation (e.g. debit + credit + log all in one transaction).
func (r *PgxTransactionRepository) CreateWithTx(ctx context.Context, tx pgx.Tx, transaction *domain.Transaction) error {
	query := `
		INSERT INTO transactions (id, from_account_id, to_account_id, amount, type, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := tx.Exec(ctx, query,
		transaction.ID,
		transaction.FromAccountID,
		transaction.ToAccountID,
		transaction.Amount,
		transaction.Type,
		transaction.Status,
		transaction.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating transaction within tx: %w", err)
	}
	return nil
}

// GetByID retrieves a single transaction by its ID.
// Returns (nil, nil) if not found.
func (r *PgxTransactionRepository) GetByID(ctx context.Context, transactionID string) (*domain.Transaction, error) {
	query := `
		SELECT id, from_account_id, to_account_id, amount, type, status, created_at
		FROM transactions
		WHERE id = $1`

	transaction := &domain.Transaction{}
	err := r.pool.QueryRow(ctx, query, transactionID).Scan(
		&transaction.ID,
		&transaction.FromAccountID,
		&transaction.ToAccountID,
		&transaction.Amount,
		&transaction.Type,
		&transaction.Status,
		&transaction.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetching transaction %s: %w", transactionID, err)
	}
	return transaction, nil
}

// GetByAccountID retrieves all transactions where the given account is either
// the sender or the receiver, ordered by most recent first.
func (r *PgxTransactionRepository) GetByAccountID(ctx context.Context, accountID string, limit int, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT id, from_account_id, to_account_id, amount, type, status, created_at
		FROM transactions
		WHERE from_account_id = $1 OR to_account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, accountID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("fetching transactions for account %s: %w", accountID, err)
	}
	defer rows.Close()

	return scanTransactions(rows)
}

// Update updates the status of a transaction (e.g. pending -> success/failed).
// Only status is mutable — all other fields are immutable once written.
func (r *PgxTransactionRepository) Update(ctx context.Context, transaction *domain.Transaction) error {
	query := `
		UPDATE transactions
		SET status = $1
		WHERE id = $2`

	result, err := r.pool.Exec(ctx, query, transaction.Status, transaction.ID)
	if err != nil {
		return fmt.Errorf("updating transaction %s: %w", transaction.ID, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("transaction %s not found", transaction.ID)
	}
	return nil
}

// List retrieves a paginated list of all transactions, ordered by most recent first.
// Intended for admin use.
func (r *PgxTransactionRepository) List(ctx context.Context, limit int, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT id, from_account_id, to_account_id, amount, type, status, created_at
		FROM transactions
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`

	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing transactions: %w", err)
	}
	defer rows.Close()

	return scanTransactions(rows)
}

// scanTransactions is a shared helper to scan multiple transaction rows.
func scanTransactions(rows pgx.Rows) ([]*domain.Transaction, error) {
	var transactions []*domain.Transaction
	for rows.Next() {
		t := &domain.Transaction{}
		if err := rows.Scan(
			&t.ID,
			&t.FromAccountID,
			&t.ToAccountID,
			&t.Amount,
			&t.Type,
			&t.Status,
			&t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning transaction row: %w", err)
		}
		transactions = append(transactions, t)
	}
	return transactions, rows.Err()
}
