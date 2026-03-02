// Package shardmap maintains the mapping between logical partitions and physical shards.
//
// Per the SRS: "The Shard Map maintains the mapping between logical partitions
// and physical shard servers. Account identifiers are deterministically mapped
// to partitions using a hash-based scheme."
//
// The shard map is persisted to a JSON file and supports runtime updates
// for partition migration / load rebalancing.
package shardmap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"ledger-service/shared/utils"
)

// ShardInfo describes a physical shard server.
type ShardInfo struct {
	ShardID string `json:"shard_id"`
	Address string `json:"address"` // host:port for HTTP communication
	Role    string `json:"role"`    // PRIMARY or FOLLOWER
}

// ShardMap maintains the partition → shard assignment table.
// It is safe for concurrent use.
type ShardMap struct {
	mu         sync.RWMutex
	partitions map[int]ShardInfo // partition index → shard info
	filePath   string
}

// shardMapData is the on-disk JSON representation.
// Uses string keys because JSON does not support integer keys.
type shardMapData struct {
	Partitions map[string]ShardInfo `json:"partitions"`
}

// LoadShardMap loads the shard map from a JSON file.
// If the file does not exist, an empty shard map is returned.
func LoadShardMap(filePath string) (*ShardMap, error) {
	sm := &ShardMap{
		partitions: make(map[int]ShardInfo),
		filePath:   filePath,
	}

	if _, err := os.Stat(filePath); err == nil {
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("shardmap: failed to read %s: %w", filePath, err)
		}
		if len(raw) > 0 {
			var data shardMapData
			if err := json.Unmarshal(raw, &data); err != nil {
				return nil, fmt.Errorf("shardmap: failed to parse %s: %w", filePath, err)
			}
			for key, info := range data.Partitions {
				var partID int
				if _, err := fmt.Sscanf(key, "%d", &partID); err != nil {
					return nil, fmt.Errorf("shardmap: invalid partition key %q: %w", key, err)
				}
				sm.partitions[partID] = info
			}
		}
	}

	return sm, nil
}

// NewShardMap creates a shard map with an initial even partition distribution.
// Partitions are assigned to shards in round-robin order.
func NewShardMap(filePath string, shards []ShardInfo, totalPartitions int) (*ShardMap, error) {
	if len(shards) == 0 {
		return nil, fmt.Errorf("shardmap: at least one shard is required")
	}

	// Ensure parent directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("shardmap: failed to create directory %s: %w", dir, err)
	}

	sm := &ShardMap{
		partitions: make(map[int]ShardInfo, totalPartitions),
		filePath:   filePath,
	}

	// Round-robin assignment
	for i := 0; i < totalPartitions; i++ {
		sm.partitions[i] = shards[i%len(shards)]
	}

	if err := sm.Save(); err != nil {
		return nil, fmt.Errorf("shardmap: failed to save initial map: %w", err)
	}

	return sm, nil
}

// GetShard returns the shard info for a given partition ID.
func (sm *ShardMap) GetShard(partitionID int) (ShardInfo, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	info, ok := sm.partitions[partitionID]
	return info, ok
}

// GetShardForAccount resolves the shard for an account using the partition mapper.
func (sm *ShardMap) GetShardForAccount(accountID string, mapper *utils.PartitionMapper) (ShardInfo, error) {
	partitionID := mapper.GetPartition(accountID)

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	info, ok := sm.partitions[partitionID]
	if !ok {
		return ShardInfo{}, fmt.Errorf("shardmap: no shard assigned for partition %d (account %s)", partitionID, accountID)
	}
	return info, nil
}

// UpdatePartition reassigns a partition to a different shard.
// Used during partition migration / load rebalancing.
func (sm *ShardMap) UpdatePartition(partitionID int, shard ShardInfo) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.partitions[partitionID] = shard
	return sm.saveLocked()
}

// AllShards returns a deduplicated list of all shard servers in the map.
func (sm *ShardMap) AllShards() []ShardInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	seen := make(map[string]bool)
	var shards []ShardInfo
	for _, info := range sm.partitions {
		if !seen[info.ShardID] {
			seen[info.ShardID] = true
			shards = append(shards, info)
		}
	}
	return shards
}

// GetPartitionsForShard returns all partition IDs assigned to a given shard.
func (sm *ShardMap) GetPartitionsForShard(shardID string) []int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var partitions []int
	for partID, info := range sm.partitions {
		if info.ShardID == shardID {
			partitions = append(partitions, partID)
		}
	}
	return partitions
}

// PartitionCount returns the total number of partitions in the map.
func (sm *ShardMap) PartitionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.partitions)
}

// Save persists the shard map to disk atomically.
func (sm *ShardMap) Save() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.saveLocked()
}

// saveLocked persists the shard map (caller must hold at least a read lock).
func (sm *ShardMap) saveLocked() error {
	data := shardMapData{
		Partitions: make(map[string]ShardInfo, len(sm.partitions)),
	}
	for partID, info := range sm.partitions {
		key := fmt.Sprintf("%d", partID)
		data.Partitions[key] = info
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("shardmap: marshal failed: %w", err)
	}

	tmpPath := sm.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0644); err != nil {
		return fmt.Errorf("shardmap: write failed: %w", err)
	}

	if err := os.Rename(tmpPath, sm.filePath); err != nil {
		return fmt.Errorf("shardmap: rename failed: %w", err)
	}

	return nil
}
