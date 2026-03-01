package main

import (
	"Software-Course-Project/internal/config"
	"Software-Course-Project/internal/service"
	"log"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	// 1. Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// 2. Initialize database connection pool
	pool := config.NewDB(cfg.PostgresDSN)
	defer pool.Close()

	// 3. Initialize all repositories
	store := service.NewStore(pool)

	// 4. Now you have access to everything:
	// store.User, store.Account, etc.
	// Pass 'store' to your HTTP handlers or business logic
}
