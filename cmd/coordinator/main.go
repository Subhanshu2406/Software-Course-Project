package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"ledger-service/coordinator/consumer"
	"ledger-service/coordinator/router"
	"ledger-service/coordinator/shardmap"
	"ledger-service/messaging"
	"ledger-service/shared/utils"
)

func main() {
	// Initialize Dependencies
	shardMap, err := shardmap.NewShardMap("shard_map.json", []shardmap.ShardInfo{
		{ShardID: "shard1", Address: "localhost:8081", Role: "PRIMARY"},
		{ShardID: "shard2", Address: "localhost:8082", Role: "PRIMARY"},
	}, 10)
	if err != nil {
		log.Fatalf("failed to init shard map: %v", err)
	}

	mapper := utils.NewPartitionMapper(10)
	client := messaging.NewShardClient(5 * time.Second)

	txnRouter := router.NewRouter(shardMap, mapper, client)

	// Since prompt asks for KafkaConsumer replacing HTTPConsumer:
	kafkaCons := consumer.NewKafkaConsumer([]string{"localhost:9092"}, "transactions", "coordinator-group", txnRouter)
	
	// Start Kafka consumer in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	kafkaCons.Start(ctx)

	// Keep an HTTP server alive for status/health checks that API gateway forwards to
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		log.Println("Coordinator listening on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	log.Println("Shutting down coordinator...")
	server.Shutdown(context.Background())
	kafkaCons.Stop()
}
