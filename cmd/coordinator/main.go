package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"ledger-service/coordinator/consumer"
	"ledger-service/coordinator/router"
	"ledger-service/coordinator/shardmap"
	"ledger-service/messaging"
	"ledger-service/shared/utils"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	addr := envOrDefault("COORDINATOR_ADDR", ":8080")
	shardMapPath := envOrDefault("SHARD_MAP_PATH", "./config/shard_map.json")
	consumerType := envOrDefault("CONSUMER_TYPE", "http")
	kafkaBrokers := envOrDefault("KAFKA_BROKERS", "kafka:9092")
	kafkaTopic := envOrDefault("KAFKA_TOPIC", "transactions")

	// Load shard map from JSON file
	shardMap, err := shardmap.LoadShardMap(shardMapPath)
	if err != nil {
		log.Fatalf("failed to load shard map from %s: %v", shardMapPath, err)
	}

	totalPartitions := shardMap.PartitionCount()
	if totalPartitions == 0 {
		log.Fatalf("shard map at %s has no partitions", shardMapPath)
	}

	mapper := utils.NewPartitionMapper(totalPartitions)
	client := messaging.NewShardClient(5 * time.Second)
	txnRouter := router.NewRouter(shardMap, mapper, client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create HTTP mux for health and submit endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	var httpConsumer *consumer.HTTPConsumer

	if consumerType == "kafka" {
		// Kafka consumer mode
		brokers := strings.Split(kafkaBrokers, ",")
		kafkaCons := consumer.NewKafkaConsumer(brokers, kafkaTopic, "coordinator-group", txnRouter)
		kafkaCons.Start(ctx)
		defer kafkaCons.Stop()

		// Also wire up HTTP /submit for direct submissions
		httpConsumer = consumer.NewHTTPConsumer(addr, txnRouter)
		mux.HandleFunc("/submit", httpConsumer.HandleSubmitDirect)
		mux.HandleFunc("/status", httpConsumer.HandleStatusDirect)

		log.Printf("Coordinator: Kafka consumer (%s, topic=%s) + HTTP on %s", kafkaBrokers, kafkaTopic, addr)
	} else {
		// HTTP consumer mode (default)
		httpConsumer = consumer.NewHTTPConsumer(addr, txnRouter)
		mux.HandleFunc("/submit", httpConsumer.HandleSubmitDirect)
		mux.HandleFunc("/status", httpConsumer.HandleStatusDirect)

		log.Printf("Coordinator: HTTP consumer on %s", addr)
	}

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("Coordinator listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	log.Println("Shutting down coordinator...")
	server.Shutdown(context.Background())
}
