package storage

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxUserRepository implements UserRepository using pgx
type PgxUserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new user repository
func NewUserRepository(pool *pgxpool.Pool) *PgxUserRepository {
	return &PgxUserRepository{pool: pool}
}
