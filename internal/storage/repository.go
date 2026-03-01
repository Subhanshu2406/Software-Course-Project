package storage

import (
	"context"
	"Software-Course-Project/internal/domain"
)

// UserRepository defines methods for user data operations
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, userID string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByMobileNo(ctx context.Context, mobileNo string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, userID string) error
	List(ctx context.Context, limit int, offset int) ([]*domain.User, error)
}

// CredentialRepository defines methods for credential data operations
type CredentialRepository interface {
	Create(ctx context.Context, credential *domain.Credential) error
	GetByUserID(ctx context.Context, userID string) (*domain.Credential, error)
	UpdatePasswordHash(ctx context.Context, userID string, passwordHash string) error
	RecordLogin(ctx context.Context, userID string) error
}

// AccountRepository defines methods for account data operations
type AccountRepository interface {
	Create(ctx context.Context, account *domain.Account) error
	GetByID(ctx context.Context, accountID string) (*domain.Account, error)
	GetByUserID(ctx context.Context, userID string) ([]*domain.Account, error)
	Update(ctx context.Context, account *domain.Account) error
	Delete(ctx context.Context, accountID string) error
}

// TransactionRepository defines methods for transaction data operations
type TransactionRepository interface {
	Create(ctx context.Context, transaction *domain.Transaction) error
	GetByID(ctx context.Context, transactionID string) (*domain.Transaction, error)
	GetByAccountID(ctx context.Context, accountID string, limit int, offset int) ([]*domain.Transaction, error)
	Update(ctx context.Context, transaction *domain.Transaction) error
	List(ctx context.Context, limit int, offset int) ([]*domain.Transaction, error)
}
