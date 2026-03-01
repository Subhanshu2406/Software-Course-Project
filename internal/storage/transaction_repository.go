package storage

import (
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
