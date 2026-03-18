package main

import (
	"log"
	"net/http"
	"time"

	"ledger-service/shard/partition"
	"ledger-service/shard/replication"
	"ledger-service/shard/server"
	"ledger-service/shared/utils"
	"ledger-service/storage"
)

func main() {
	// Let's assume we read these from flags/env. Hardcoded for brevity.
	shardID := "shard1"
	addr := ":8081"
	walPath := "data/shard1.wal"
	storePath := "data/shard1_state.json"

	store, err := storage.NewJSONStore(storePath)
	if err != nil {
		log.Fatalf("failed to open storage: %v", err)
	}

	shardServer, err := server.NewShardServer(shardID, walPath, nil)
	if err != nil {
		log.Fatalf("failed to start shard server: %v", err)
	}
	shardServer.SetStorage(store)

	mgr := partition.NewManager(shardID, []int{0, 1, 2, 3, 4})
	mapper := utils.NewPartitionMapper(10)
	shardServer.SetPartitioning(mgr, mapper)

	// Replication setup
	replicator := replication.NewPrimaryReplicator(shardID, 2, 5*time.Second)
	// Add dummy follower just as an example
	// replicator.AddFollower("follower1A", "localhost:9081")
	shardServer.SetReplicator(replicator)

	handler := server.NewHTTPHandler(shardServer)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	log.Printf("Shard %s listening on %s", shardID, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("listen failed: %v", err)
	}
}
