package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/time/rate"

	"ledger-service/api/handlers"
	"ledger-service/api/kafka"
	"ledger-service/api/middleware"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	addr := envOrDefault("API_ADDR", ":8000")
	kafkaBrokers := strings.Split(envOrDefault("KAFKA_BROKERS", "kafka:9092"), ",")
	kafkaTopic := envOrDefault("KAFKA_TOPIC", "transactions")
	coordinatorURL := envOrDefault("COORDINATOR_URL", "http://coordinator:8080")

	// Initialize Kafka Producer
	producer := kafka.NewProducer(kafkaBrokers, kafkaTopic)
	defer producer.Close()

	// Initialize API Handler
	txHandler := handlers.NewTransactionHandler(producer, coordinatorURL)

	// Setup Middlewares
	rateLimiter := middleware.NewIPTracker(rate.Limit(50), 100)

	// Protected mux (requires auth)
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("/submit", txHandler.HandleSubmit)
	protectedMux.HandleFunc("/status/", txHandler.HandleStatus)
	protectedMux.HandleFunc("/admin", middleware.RequireRole("admin", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("admin area"))
	}))

	// Wrap protected routes with auth + rate limiting
	protected := middleware.RequireAuth(protectedMux)
	rateLimited := rateLimiter.RateLimit(protected)

	// Outer mux: health bypasses auth
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/", rateLimited)

	log.Printf("API Gateway listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("API failed: %v", err)
	}
}
