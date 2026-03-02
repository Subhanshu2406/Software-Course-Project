package storage

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"Software-Course-Project/internal/domain"
)

type TransactionRepository struct {
	db *pgxpool.Pool
}

func NewTransactionRepository(db *pgxpool.Pool) *TransactionRepository {
	return &TransactionRepository{db: db}
}

func (r *TransactionRepository) Create(ctx context.Context, transaction *domain.Transaction) error {
	query := `
        INSERT INTO transactions (id, amount, description, timestamp, to_account_id, from_account_id)
        VALUES ($1, $2, $3, $4, $5, $6)
    `
	_, err := r.db.Exec(ctx, query,
		transaction.ID,
		transaction.Amount,
		transaction.Description,
		transaction.Timestamp,
		transaction.ToAccountID,
		transaction.FromAccountID,
	)
	return err
}

func (r *TransactionRepository) GetByID(ctx context.Context, id string) (*domain.Transaction, error) {
	var tx domain.Transaction
	query := `
        SELECT id, amount, description, timestamp, to_account_id, from_account_id
        FROM transactions WHERE id = $1
    `
	err := r.db.QueryRow(ctx, query, id).Scan(
		&tx.ID, &tx.Amount, &tx.Description, &tx.Timestamp,
		&tx.ToAccountID, &tx.FromAccountID,
	)
	return &tx, err
}

func (r *TransactionRepository) ListByAccountID(ctx context.Context, accountID string) ([]*domain.Transaction, error) {
	query := `
        SELECT id, amount, description, timestamp, to_account_id, from_account_id
        FROM transactions WHERE to_account_id = $1 OR from_account_id = $1
        ORDER BY timestamp DESC
    `
	rows, err := r.db.Query(ctx, query, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []*domain.Transaction
	for rows.Next() {
		var tx domain.Transaction
		err := rows.Scan(&tx.ID, &tx.Amount, &tx.Description, &tx.Timestamp, &tx.ToAccountID, &tx.FromAccountID)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, &tx)
	}
	return transactions, rows.Err()
}
