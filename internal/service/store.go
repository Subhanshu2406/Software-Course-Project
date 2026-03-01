package service

import (
	"Software-Course-Project/internal/storage"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store holds all repositories (dependency injection container)
type Store struct {
	User        storage.UserRepository
	Account     storage.AccountRepository
	Credential  storage.CredentialRepository
	Transaction storage.TransactionRepository
}

// NewStore initializes all repositories with a single pool
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		User:        storage.NewUserRepository(pool), // the current error is due to the missing methods in each repository
		Account:     storage.NewAccountRepository(pool),
		Credential:  storage.NewCredRepository(pool),
		Transaction: storage.NewTransactionRepository(pool),
	}
}

// import "github.com/google/uuid"

// // in your service, before calling repo.Create:    this needs to be done in the service layer, not the repository layer
// user.ID = uuid.New().String()
// err := r.userRepo.Create(ctx, user)
