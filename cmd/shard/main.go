package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"Software-Course-Project/internal/config"
	"Software-Course-Project/internal/storage"
)

func main() {
	ctx := context.Background()

	// Initialize database
	dbConfig := config.NewDatabaseConfig()
	db, err := config.ConnectDatabase(ctx, dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	log.Println("Database connected successfully")

	// Initialize repositories
	accountRepo := storage.NewAccountRepository(db)
	txRepo := storage.NewTransactionRepository(db)

	// TODO: Start shard server with repositories
	_ = accountRepo
	_ = txRepo

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	db.Close()
}
