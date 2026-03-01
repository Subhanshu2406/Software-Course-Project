package storage

import (
	"Software-Course-Project/internal/domain"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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

// Create inserts a new credential record for a user.
// Should be called immediately after creating a user.
func (r *PgxCredRepository) Create(ctx context.Context, credential *domain.Credential) error {
	query := `
		INSERT INTO credentials (user_id, password_hash, last_login, password_changed_at)
		VALUES ($1, $2, $3, $4)`
	_, err := r.pool.Exec(ctx, query,
		credential.UserID,
		credential.PasswordHash,
		credential.LastLogin,
		credential.PasswordChangedAt,
	)
	if err != nil {
		return fmt.Errorf("creating credential: %w", err)
	}
	return nil
}

// GetByUserID retrieves credentials for a given user.
// Returns (nil, nil) if no credential record exists for that user.
func (r *PgxCredRepository) GetByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	query := `
		SELECT user_id, password_hash, last_login, password_changed_at
		FROM credentials
		WHERE user_id = $1`

	cred := &domain.Credential{}
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&cred.UserID,
		&cred.PasswordHash,
		&cred.LastLogin,
		&cred.PasswordChangedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetching credential for user %s: %w", userID, err)
	}
	return cred, nil
}

// UpdatePasswordHash sets a new password hash and records the time of the change.
// Used during password reset or change flows.
func (r *PgxCredRepository) UpdatePasswordHash(ctx context.Context, userID string, passwordHash string) error {
	query := `
		UPDATE credentials
		SET password_hash = $1, password_changed_at = $2
		WHERE user_id = $3`

	result, err := r.pool.Exec(ctx, query, passwordHash, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("updating password hash for user %s: %w", userID, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no credential found for user %s", userID)
	}
	return nil
}

// RecordLogin updates the last_login timestamp to now.
// Should be called on every successful authentication.
func (r *PgxCredRepository) RecordLogin(ctx context.Context, userID string) error {
	query := `
		UPDATE credentials
		SET last_login = $1
		WHERE user_id = $2`

	result, err := r.pool.Exec(ctx, query, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("recording login for user %s: %w", userID, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no credential found for user %s", userID)
	}
	return nil
}
