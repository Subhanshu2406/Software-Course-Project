package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"ledger-service/shard/partition"
	"ledger-service/shard/replication"
	"ledger-service/shard/server"
	"ledger-service/shared/utils"
	"ledger-service/storage"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	shardID := envOrDefault("SHARD_ID", "shard1")
	addr := envOrDefault("SHARD_ADDR", ":8081")
	walPath := envOrDefault("SHARD_WAL_PATH", "./data/shard.wal")
	storePath := envOrDefault("SHARD_STORE_PATH", "./data/shard_state.json")
	role := envOrDefault("SHARD_ROLE", "PRIMARY")
	followerAddrs := envOrDefault("SHARD_FOLLOWER_ADDRS", "")
	partitionsStr := envOrDefault("SHARD_PARTITIONS", "0,1,2,3,4,5,6,7,8,9")
	quorumStr := envOrDefault("SHARD_QUORUM_SIZE", "2")
	totalPartStr := envOrDefault("SHARD_TOTAL_PARTITIONS", "30")

	quorumSize, err := strconv.Atoi(quorumStr)
	if err != nil {
		log.Fatalf("invalid SHARD_QUORUM_SIZE: %v", err)
	}

	totalPartitions, err := strconv.Atoi(totalPartStr)
	if err != nil {
		log.Fatalf("invalid SHARD_TOTAL_PARTITIONS: %v", err)
	}

	// Parse partition IDs
	var partitionIDs []int
	for _, s := range strings.Split(partitionsStr, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := strconv.Atoi(s)
		if err != nil {
			log.Fatalf("invalid partition ID %q: %v", s, err)
		}
		partitionIDs = append(partitionIDs, id)
	}

	store, err := storage.NewJSONStore(storePath)
	if err != nil {
		log.Fatalf("failed to open storage: %v", err)
	}

	shardServer, err := server.NewShardServer(shardID, walPath, nil)
	if err != nil {
		log.Fatalf("failed to start shard server: %v", err)
	}
	shardServer.SetStorage(store)
	shardServer.SetRole(role)

	mgr := partition.NewManager(shardID, partitionIDs)
	mapper := utils.NewPartitionMapper(totalPartitions)
	shardServer.SetPartitioning(mgr, mapper)

	// Replication setup (PRIMARY only)
	replicator := replication.NewPrimaryReplicator(shardID, quorumSize, 5*time.Second)
	if role == "PRIMARY" && followerAddrs != "" {
		for _, fa := range strings.Split(followerAddrs, ",") {
			fa = strings.TrimSpace(fa)
			if fa == "" {
				continue
			}
			replicator.AddFollower(fa, fa)
		}
	}
	shardServer.SetReplicator(replicator)

	handler := server.NewHTTPHandler(shardServer)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// If FOLLOWER, register follower replication endpoints
	if role == "FOLLOWER" {
		follower := replication.NewFollowerReceiver(shardID, shardServer.WAL())
		follower.RegisterHTTPHandlers(mux)
	}

	log.Printf("Shard %s (%s) listening on %s (partitions: %v)", shardID, role, addr, partitionIDs)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("listen failed: %v", err)
	}
}
