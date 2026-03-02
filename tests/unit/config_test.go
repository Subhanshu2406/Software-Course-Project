package unit_test

import (
	"os"
	"path/filepath"
	"testing"

	"ledger-service/internal/config"
)

// TestLoadConfigFromFile verifies config file loading.
func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	// Write a valid config
	content := `{
		"cluster": {"num_shards": 5, "partitions_per_shard": 6, "total_partitions": 30},
		"shard": {"wal_dir": "./data/wal", "data_dir": "./data/storage", "replica_count": 2, "quorum_size": 2},
		"replication": {"heartbeat_interval_ms": 3000, "heartbeat_miss_limit": 5},
		"coordinator": {"listen_addr": ":9090", "shard_map_path": "./sm.json"},
		"performance": {"min_tps_per_shard": 200, "single_shard_latency_p99_ms": 30, "cross_shard_latency_p99_ms": 100}
	}`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Cluster.NumShards != 5 {
		t.Errorf("NumShards = %d, want 5", cfg.Cluster.NumShards)
	}
	if cfg.Coordinator.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %s, want :9090", cfg.Coordinator.ListenAddr)
	}
	if cfg.Replication.HeartbeatMissLimit != 5 {
		t.Errorf("HeartbeatMissLimit = %d, want 5", cfg.Replication.HeartbeatMissLimit)
	}
}

// TestDefaultConfig verifies default values are reasonable.
func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.Cluster.NumShards <= 0 {
		t.Error("default NumShards should be positive")
	}
	if cfg.Shard.QuorumSize <= 0 {
		t.Error("default QuorumSize should be positive")
	}
	if cfg.Coordinator.ListenAddr == "" {
		t.Error("default ListenAddr should not be empty")
	}
}

// TestValidateRejectsInvalid verifies validation catches bad configs.
func TestValidateRejectsInvalid(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cluster.NumShards = 0 // invalid

	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for NumShards=0")
	}
}

// TestValidateRejectsZeroQuorum verifies quorum validation.
func TestValidateRejectsZeroQuorum(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Shard.QuorumSize = 0 // invalid

	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for QuorumSize=0")
	}
}

// TestLoadNonExistentFile returns error.
func TestLoadNonExistentFile(t *testing.T) {
	_, err := config.Load("/nonexistent/config.json")
	if err == nil {
		t.Error("expected error loading non-existent file")
	}
}

// TestLoadInvalidJSON returns error.
func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.json")

	if err := os.WriteFile(cfgPath, []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Error("expected error loading invalid JSON")
	}
}
