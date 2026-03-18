package main

import (
	"log"
	"net/http"
	"golang.org/x/time/rate"

	"ledger-service/api/handlers"
	"ledger-service/api/kafka"
	"ledger-service/api/middleware"
)

func main() {
	// Initialize Kafka Producer
	producer := kafka.NewProducer([]string{"localhost:9092"}, "transactions")
	defer producer.Close()

	// Initialize API Handler
	txHandler := handlers.NewTransactionHandler(producer, "http://localhost:8080")

	// Setup Middlewares
	rateLimiter := middleware.NewIPTracker(rate.Limit(50), 100)

	mux := http.NewServeMux()

	// Endpoints
	mux.HandleFunc("/submit", txHandler.HandleSubmit)
	mux.HandleFunc("/status/", txHandler.HandleStatus)

	// Admin only endpoint example
	mux.HandleFunc("/admin", middleware.RequireRole("admin", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("admin area"))
	}))

	// Apply Middlewares to all requests
	protected := middleware.RequireAuth(mux)
	rateLimited := rateLimiter.RateLimit(protected)

	log.Println("API Gateway listening on :8000")
	if err := http.ListenAndServe(":8000", rateLimited); err != nil {
		log.Fatalf("API failed: %v", err)
	}
}
