package unit_test

import (
	"os"
	"path/filepath"
	"testing"

	"ledger-service/coordinator/shardmap"
	"ledger-service/shared/utils"
)

// TestNewShardMapCreatesFile verifies initial shard map creation.
func TestNewShardMapCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shard_map.json")

	shards := []shardmap.ShardInfo{
		{ShardID: "shard-1", Address: "localhost:9001", Role: "PRIMARY"},
		{ShardID: "shard-2", Address: "localhost:9002", Role: "PRIMARY"},
	}

	sm, err := shardmap.NewShardMap(path, shards, 6)
	if err != nil {
		t.Fatalf("NewShardMap: %v", err)
	}

	if sm.PartitionCount() != 6 {
		t.Errorf("partition count = %d, want 6", sm.PartitionCount())
	}

	// File should exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected shard map file to exist")
	}
}

// TestShardMapRoundRobinDistribution verifies even partition distribution.
func TestShardMapRoundRobinDistribution(t *testing.T) {
	shards := []shardmap.ShardInfo{
		{ShardID: "shard-1", Address: "localhost:9001", Role: "PRIMARY"},
		{ShardID: "shard-2", Address: "localhost:9002", Role: "PRIMARY"},
		{ShardID: "shard-3", Address: "localhost:9003", Role: "PRIMARY"},
	}

	sm, err := shardmap.NewShardMap(
		filepath.Join(t.TempDir(), "sm.json"),
		shards, 9,
	)
	if err != nil {
		t.Fatalf("NewShardMap: %v", err)
	}

	// Each shard should own exactly 3 partitions
	for _, s := range shards {
		parts := sm.GetPartitionsForShard(s.ShardID)
		if len(parts) != 3 {
			t.Errorf("shard %s owns %d partitions, want 3", s.ShardID, len(parts))
		}
	}
}

// TestShardMapGetShardForAccount resolves accounts to shards via hashing.
func TestShardMapGetShardForAccount(t *testing.T) {
	shards := []shardmap.ShardInfo{
		{ShardID: "shard-1", Address: "localhost:9001", Role: "PRIMARY"},
		{ShardID: "shard-2", Address: "localhost:9002", Role: "PRIMARY"},
	}

	sm, err := shardmap.NewShardMap(
		filepath.Join(t.TempDir(), "sm.json"),
		shards, 10,
	)
	if err != nil {
		t.Fatalf("NewShardMap: %v", err)
	}

	mapper := utils.NewPartitionMapper(10)

	// Any account should resolve to a shard
	info, err := sm.GetShardForAccount("alice", mapper)
	if err != nil {
		t.Fatalf("GetShardForAccount: %v", err)
	}
	if info.ShardID != "shard-1" && info.ShardID != "shard-2" {
		t.Errorf("unexpected shard: %s", info.ShardID)
	}

	// Same account should always resolve to the same shard (deterministic)
	info2, _ := sm.GetShardForAccount("alice", mapper)
	if info.ShardID != info2.ShardID {
		t.Error("GetShardForAccount should be deterministic")
	}
}

// TestShardMapUpdatePartition verifies partition reassignment.
func TestShardMapUpdatePartition(t *testing.T) {
	shards := []shardmap.ShardInfo{
		{ShardID: "shard-1", Address: "localhost:9001", Role: "PRIMARY"},
		{ShardID: "shard-2", Address: "localhost:9002", Role: "PRIMARY"},
	}

	sm, err := shardmap.NewShardMap(
		filepath.Join(t.TempDir(), "sm.json"),
		shards, 4,
	)
	if err != nil {
		t.Fatalf("NewShardMap: %v", err)
	}

	// Get initial shard for partition 0
	original, _ := sm.GetShard(0)

	// Reassign partition 0 to a new shard
	newShard := shardmap.ShardInfo{ShardID: "shard-3", Address: "localhost:9003", Role: "PRIMARY"}
	if err := sm.UpdatePartition(0, newShard); err != nil {
		t.Fatalf("UpdatePartition: %v", err)
	}

	updated, ok := sm.GetShard(0)
	if !ok {
		t.Fatal("partition 0 should still be assigned")
	}
	if updated.ShardID == original.ShardID {
		t.Error("partition 0 should now be on a different shard")
	}
	if updated.ShardID != "shard-3" {
		t.Errorf("partition 0 shard = %s, want shard-3", updated.ShardID)
	}
}

// TestShardMapPersistence verifies the map survives close and reload.
func TestShardMapPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sm.json")

	shards := []shardmap.ShardInfo{
		{ShardID: "shard-1", Address: "localhost:9001", Role: "PRIMARY"},
	}

	sm1, err := shardmap.NewShardMap(path, shards, 5)
	if err != nil {
		t.Fatalf("NewShardMap: %v", err)
	}
	_ = sm1 // just need the file

	// Reload
	sm2, err := shardmap.LoadShardMap(path)
	if err != nil {
		t.Fatalf("LoadShardMap: %v", err)
	}

	if sm2.PartitionCount() != 5 {
		t.Errorf("after reload: partition count = %d, want 5", sm2.PartitionCount())
	}
}

// TestShardMapAllShards returns deduplicated shard list.
func TestShardMapAllShards(t *testing.T) {
	shards := []shardmap.ShardInfo{
		{ShardID: "shard-1", Address: "localhost:9001", Role: "PRIMARY"},
		{ShardID: "shard-2", Address: "localhost:9002", Role: "PRIMARY"},
	}

	sm, _ := shardmap.NewShardMap(
		filepath.Join(t.TempDir(), "sm.json"),
		shards, 6,
	)

	all := sm.AllShards()
	if len(all) != 2 {
		t.Errorf("AllShards returned %d shards, want 2", len(all))
	}
}

// TestShardMapNoShardsError verifies error on empty shard list.
func TestShardMapNoShardsError(t *testing.T) {
	_, err := shardmap.NewShardMap(
		filepath.Join(t.TempDir(), "sm.json"),
		[]shardmap.ShardInfo{}, 10,
	)
	if err == nil {
		t.Error("expected error when creating shard map with no shards")
	}
}
