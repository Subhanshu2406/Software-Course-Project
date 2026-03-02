// Package config provides configuration loading and validation for the ledger service.
//
// All configuration values are loaded from a JSON file, with sensible defaults
// provided by DefaultConfig(). This eliminates hardcoded magic numbers from
// business logic throughout the codebase.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// ClusterConfig defines the overall cluster topology.
type ClusterConfig struct {
	NumShards          int `json:"num_shards"`
	PartitionsPerShard int `json:"partitions_per_shard"`
	TotalPartitions    int `json:"total_partitions"`
}

// ShardConfig defines shard-level settings.
type ShardConfig struct {
	WALDir       string `json:"wal_dir"`
	DataDir      string `json:"data_dir"`
	ReplicaCount int    `json:"replica_count"` // followers per shard (e.g., 2 → 1 leader + 2 followers)
	QuorumSize   int    `json:"quorum_size"`   // minimum ACKs required (including leader)
}

// ReplicationConfig defines replication and heartbeat parameters.
// Per REQ-REP-003: heartbeat interval and miss limit.
type ReplicationConfig struct {
	HeartbeatIntervalMs int `json:"heartbeat_interval_ms"`
	HeartbeatMissLimit  int `json:"heartbeat_miss_limit"`
}

// CoordinatorConfig defines the coordinator's network and routing settings.
type CoordinatorConfig struct {
	ListenAddr   string `json:"listen_addr"`
	ShardMapPath string `json:"shard_map_path"`
}

// PerformanceConfig defines target performance metrics.
// Per REQ-PERF-001 through REQ-PERF-004.
type PerformanceConfig struct {
	MinTPSPerShard          int `json:"min_tps_per_shard"`
	SingleShardLatencyP99Ms int `json:"single_shard_latency_p99_ms"`
	CrossShardLatencyP99Ms  int `json:"cross_shard_latency_p99_ms"`
}

// Config is the top-level configuration for the entire ledger service.
type Config struct {
	Cluster     ClusterConfig     `json:"cluster"`
	Shard       ShardConfig       `json:"shard"`
	Replication ReplicationConfig `json:"replication"`
	Coordinator CoordinatorConfig `json:"coordinator"`
	Performance PerformanceConfig `json:"performance"`
}

// Load reads configuration from a JSON file at the given path.
// Missing fields are filled with defaults, and the result is validated.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: failed to read %s: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: failed to parse %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: validation failed: %w", err)
	}

	return cfg, nil
}

// DefaultConfig returns a Config with reasonable defaults matching the SRS.
func DefaultConfig() *Config {
	return &Config{
		Cluster: ClusterConfig{
			NumShards:          3,
			PartitionsPerShard: 10,
			TotalPartitions:    30,
		},
		Shard: ShardConfig{
			WALDir:       "./data/wal",
			DataDir:      "./data/storage",
			ReplicaCount: 2,
			QuorumSize:   2,
		},
		Replication: ReplicationConfig{
			HeartbeatIntervalMs: 5000,
			HeartbeatMissLimit:  3,
		},
		Coordinator: CoordinatorConfig{
			ListenAddr:   ":8080",
			ShardMapPath: "./config/shard_map.json",
		},
		Performance: PerformanceConfig{
			MinTPSPerShard:          100,
			SingleShardLatencyP99Ms: 50,
			CrossShardLatencyP99Ms:  200,
		},
	}
}

// Validate checks for obvious configuration errors.
func (c *Config) Validate() error {
	if c.Cluster.NumShards <= 0 {
		return fmt.Errorf("cluster.num_shards must be positive, got %d", c.Cluster.NumShards)
	}
	if c.Cluster.TotalPartitions <= 0 {
		return fmt.Errorf("cluster.total_partitions must be positive, got %d", c.Cluster.TotalPartitions)
	}
	if c.Shard.ReplicaCount < 0 {
		return fmt.Errorf("shard.replica_count cannot be negative, got %d", c.Shard.ReplicaCount)
	}
	if c.Shard.QuorumSize <= 0 {
		return fmt.Errorf("shard.quorum_size must be positive, got %d", c.Shard.QuorumSize)
	}
	if c.Replication.HeartbeatIntervalMs <= 0 {
		return fmt.Errorf("replication.heartbeat_interval_ms must be positive, got %d", c.Replication.HeartbeatIntervalMs)
	}
	if c.Replication.HeartbeatMissLimit <= 0 {
		return fmt.Errorf("replication.heartbeat_miss_limit must be positive, got %d", c.Replication.HeartbeatMissLimit)
	}
	return nil
}
