package storage

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxCredRepository implements CredentialRepository using pgx
type PgxCredRepository struct {
	pool *pgxpool.Pool
}

// NewCredRepository creates a new credential repository
func NewCredRepository(pool *pgxpool.Pool) *PgxCredRepository {
	return &PgxCredRepository{pool: pool}
}
