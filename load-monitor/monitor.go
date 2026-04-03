// Package monitor tracks metrics and handles partition rebalancing.
package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"ledger-service/coordinator/shardmap"
	"ledger-service/shared/models"
)

// LoadMonitor periodically polls shards for metrics and rebalances.
type LoadMonitor struct {
	mu             sync.Mutex
	shardMap       *shardmap.ShardMap
	metrics        map[string]models.ShardMetrics
	thresholdDepth int
	pollInterval   time.Duration
}

// NewLoadMonitor creates a new load monitor.
func NewLoadMonitor(sm *shardmap.ShardMap, threshold int, interval time.Duration) *LoadMonitor {
	return &LoadMonitor{
		shardMap:       sm,
		metrics:        make(map[string]models.ShardMetrics),
		thresholdDepth: threshold,
		pollInterval:   interval,
	}
}

// Start begins a background loop to poll metrics.
func (m *LoadMonitor) Start() {
	go func() {
		for {
			time.Sleep(m.pollInterval)
			m.pollShards()
			m.checkHotspots()
		}
	}()
}

func (m *LoadMonitor) pollShards() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, shard := range m.shardMap.AllShards() {
		resp, err := http.Get(fmt.Sprintf("http://%s/metrics", shard.Address))
		if err != nil {
			log.Printf("monitor: failed to get metrics for shard %s: %v", shard.ShardID, err)
			continue
		}

		var metrics models.ShardMetrics
		if err := json.NewDecoder(resp.Body).Decode(&metrics); err == nil {
			m.metrics[shard.ShardID] = metrics
		}
		resp.Body.Close()
	}
}

func (m *LoadMonitor) checkHotspots() {
	m.mu.Lock()
	defer m.mu.Unlock()

	var hotShard *shardmap.ShardInfo
	var coolShard *shardmap.ShardInfo
	
	// Extremely naive "find the hottest and coolest shard"
	maxDepth := -1
	minDepth := 999999999

	for _, shard := range m.shardMap.AllShards() {
		metrics, ok := m.metrics[shard.ShardID]
		if !ok {
			continue
		}
		if metrics.QueueDepth > maxDepth {
			maxDepth = metrics.QueueDepth
			sh := shard
			hotShard = &sh
		}
		if metrics.QueueDepth < minDepth {
			minDepth = metrics.QueueDepth
			sh := shard
			coolShard = &sh
		}
	}

	if hotShard != nil && coolShard != nil && maxDepth > m.thresholdDepth && hotShard.ShardID != coolShard.ShardID {
		log.Printf("monitor: detected hotspot %s (depth %d), migrating to %s", hotShard.ShardID, maxDepth, coolShard.ShardID)
		go m.migratePartition(*hotShard, *coolShard)
	}
}

// migratePartition halts one partition on hotShard, moves it to coolShard, and updates routing.
func (m *LoadMonitor) migratePartition(hot shardmap.ShardInfo, cool shardmap.ShardInfo) {
	// Pick the first partition assigned to hotShard
	partitions := m.shardMap.GetPartitionsForShard(hot.ShardID)
	if len(partitions) == 0 {
		return
	}
	partID := partitions[0]

	// 1. Halt and Snapshot on hotShard
	reqBody, _ := json.Marshal(map[string]int{"partition_id": partID})
	resp, err := http.Post(fmt.Sprintf("http://%s/halt-partition", hot.Address), "application/json", bytes.NewBuffer(reqBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Printf("monitor: halt-partition failed on %s", hot.ShardID)
		return
	}

	var snapshotResp struct {
		Balances map[string]int64 `json:"balances"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&snapshotResp); err != nil {
		log.Printf("monitor: failed to decode snapshot")
		return
	}
	resp.Body.Close()

	// 2. Receive on coolShard
	receiveBody, _ := json.Marshal(map[string]interface{}{
		"partition_id": partID,
		"balances":     snapshotResp.Balances,
	})
	resp2, err := http.Post(fmt.Sprintf("http://%s/receive-partition", cool.Address), "application/json", bytes.NewBuffer(receiveBody))
	if err != nil || resp2.StatusCode != http.StatusOK {
		log.Printf("monitor: receive-partition failed on %s", cool.ShardID)
		return
	}
	resp2.Body.Close()

	// 3. Update ShardMap
	m.shardMap.UpdatePartition(partID, cool)

	// 4. Resume on coolShard
	resp3, err := http.Post(fmt.Sprintf("http://%s/resume-partition", cool.Address), "application/json", bytes.NewBuffer(reqBody))
	if err != nil || resp3.StatusCode != http.StatusOK {
		log.Printf("monitor: resume-partition failed on %s", cool.ShardID)
	} else {
		resp3.Body.Close()
		log.Printf("monitor: successfully migrated partition %d to %s", partID, cool.ShardID)
	}
}

// HandleHealth returns dummy health.
func (m *LoadMonitor) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GetMetrics returns the current snapshot of per-shard metrics.
func (m *LoadMonitor) GetMetrics() map[string]models.ShardMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(map[string]models.ShardMetrics, len(m.metrics))
	for k, v := range m.metrics {
		result[k] = v
	}
	return result
}
