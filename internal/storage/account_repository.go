package storage

import (
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
