package storage

import (
	"Software-Course-Project/internal/domain"
	"context"

	"github.com/jackc/pgx/v5"
)

// UserRepository defines methods for user data operations
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error                      //done
	GetByID(ctx context.Context, userID string) (*domain.User, error)         //done
	GetByEmail(ctx context.Context, email string) (*domain.User, error)       //done
	GetByMobileNo(ctx context.Context, mobileNo string) (*domain.User, error) //done
	Update(ctx context.Context, user *domain.User) error                      //done
	Delete(ctx context.Context, userID string) error                          //done
	List(ctx context.Context, limit int, offset int) ([]*domain.User, error)  //done
}

// CredentialRepository defines methods for credential data operations
type CredentialRepository interface {
	Create(ctx context.Context, credential *domain.Credential) error                  //done
	GetByUserID(ctx context.Context, userID string) (*domain.Credential, error)       //done
	UpdatePasswordHash(ctx context.Context, userID string, passwordHash string) error //done
	RecordLogin(ctx context.Context, userID string) error                             //done
}

// AccountRepository defines methods for account data operations
type AccountRepository interface {
	Create(ctx context.Context, account *domain.Account) error                                  //done
	GetByID(ctx context.Context, accountID string) (*domain.Account, error)                     //done
	GetByUserID(ctx context.Context, userID string) ([]*domain.Account, error)                  //done
	GetByIDForUpdate(ctx context.Context, tx pgx.Tx, accountID string) (*domain.Account, error) //done
	Update(ctx context.Context, account *domain.Account) error                                  //done
	Delete(ctx context.Context, accountID string) error                                         //done
}

// TransactionRepository defines methods for transaction data operations
type TransactionRepository interface {
	Create(ctx context.Context, transaction *domain.Transaction) error
	GetByID(ctx context.Context, transactionID string) (*domain.Transaction, error)
	GetByAccountID(ctx context.Context, accountID string, limit int, offset int) ([]*domain.Transaction, error)
	Update(ctx context.Context, transaction *domain.Transaction) error
	List(ctx context.Context, limit int, offset int) ([]*domain.Transaction, error)
}
