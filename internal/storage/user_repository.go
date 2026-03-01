package storage

import (
	"Software-Course-Project/internal/domain"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
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

func (r *PgxUserRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (id, full_name, email, mobile_no, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.pool.Exec(ctx, query,
		user.ID, user.FullName, user.Email, user.MobileNo, user.IsActive, user.CreatedAt,
	)
	return err
}

func (r *PgxUserRepository) GetByID(ctx context.Context, userID string) (*domain.User, error) {
	query := `SELECT id, full_name, email, mobile_no, is_active, created_at FROM users WHERE id = $1`
	row := r.pool.QueryRow(ctx, query, userID)
	return scanUser(row)
}

func (r *PgxUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `SELECT id, full_name, email, mobile_no, is_active, created_at FROM users WHERE email = $1`
	row := r.pool.QueryRow(ctx, query, email)
	return scanUser(row)
}

func (r *PgxUserRepository) GetByMobileNo(ctx context.Context, mobileNo string) (*domain.User, error) {
	query := `SELECT id, full_name, email, mobile_no, is_active, created_at FROM users WHERE mobile_no = $1`
	row := r.pool.QueryRow(ctx, query, mobileNo)
	return scanUser(row)
}

func (r *PgxUserRepository) Update(ctx context.Context, user *domain.User) error {
	query := `
		UPDATE users SET full_name = $1, email = $2, mobile_no = $3, is_active = $4
		WHERE id = $5`
	result, err := r.pool.Exec(ctx, query, user.FullName, user.Email, user.MobileNo, user.IsActive, user.ID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("user not found")
	}
	return nil
}

func (r *PgxUserRepository) Delete(ctx context.Context, userID string) error {
	query := `DELETE FROM users WHERE id = $1`
	result, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("user not found")
	}
	return nil
}

func (r *PgxUserRepository) List(ctx context.Context, limit int, offset int) ([]*domain.User, error) {
	query := `SELECT id, full_name, email, mobile_no, is_active, created_at FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		user := &domain.User{}
		if err := rows.Scan(&user.ID, &user.FullName, &user.Email, &user.MobileNo, &user.IsActive, &user.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func scanUser(row pgx.Row) (*domain.User, error) {
	user := &domain.User{}
	err := row.Scan(&user.ID, &user.FullName, &user.Email, &user.MobileNo, &user.IsActive, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}
