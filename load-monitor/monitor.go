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
	migrations     []MigrationEvent
}

// MigrationEvent tracks a single partition migration.
type MigrationEvent struct {
	PartitionID       int       `json:"partition_id"`
	FromShard         string    `json:"from_shard"`
	ToShard           string    `json:"to_shard"`
	TriggeredAt       time.Time `json:"triggered_at"`
	CompletedAt       time.Time `json:"completed_at"`
	DurationMs        int64     `json:"duration_ms"`
	TriggerQueueDepth int       `json:"trigger_queue_depth"`
	Success           bool      `json:"success"`
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
	
	// Use committed_count (total throughput) to detect imbalanced shards
	maxLoad := int64(-1)
	minLoad := int64(999999999)

	for _, shard := range m.shardMap.AllShards() {
		metrics, ok := m.metrics[shard.ShardID]
		if !ok {
			continue
		}
		load := metrics.CommittedCount
		if load > maxLoad {
			maxLoad = load
			sh := shard
			hotShard = &sh
		}
		if load < minLoad {
			minLoad = load
			sh := shard
			coolShard = &sh
		}
	}

	// Only migrate if the hot shard has significantly more throughput than the cool shard
	// and has processed at least the threshold number of transactions
	if hotShard != nil && coolShard != nil && maxLoad > int64(m.thresholdDepth) && (maxLoad - minLoad) > int64(m.thresholdDepth)/2 && hotShard.ShardID != coolShard.ShardID {
		// Don't migrate if we've already migrated recently (cooldown)
		if len(m.migrations) > 0 {
			lastMigration := m.migrations[len(m.migrations)-1]
			if time.Since(lastMigration.CompletedAt) < 30*time.Second {
				return
			}
		}
		log.Printf("monitor: detected hotspot %s (load %d vs %d), migrating to %s", hotShard.ShardID, maxLoad, minLoad, coolShard.ShardID)
		go m.migratePartition(*hotShard, *coolShard)
	}
}

// migratePartition halts one partition on hotShard, moves it to coolShard, and updates routing.
func (m *LoadMonitor) migratePartition(hot shardmap.ShardInfo, cool shardmap.ShardInfo) {
	startTime := time.Now()

	// Pick the first partition assigned to hotShard
	partitions := m.shardMap.GetPartitionsForShard(hot.ShardID)
	if len(partitions) == 0 {
		return
	}
	partID := partitions[0]

	// Get trigger load
	m.mu.Lock()
	triggerDepth := 0
	if metrics, ok := m.metrics[hot.ShardID]; ok {
		triggerDepth = int(metrics.CommittedCount)
	}
	m.mu.Unlock()

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
		m.recordMigration(partID, hot.ShardID, cool.ShardID, startTime, triggerDepth, false)
	} else {
		resp3.Body.Close()
		log.Printf("monitor: successfully migrated partition %d to %s", partID, cool.ShardID)
		m.recordMigration(partID, hot.ShardID, cool.ShardID, startTime, triggerDepth, true)
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

func (m *LoadMonitor) recordMigration(partID int, from, to string, start time.Time, triggerDepth int, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.migrations = append(m.migrations, MigrationEvent{
		PartitionID:       partID,
		FromShard:         from,
		ToShard:           to,
		TriggeredAt:       start,
		CompletedAt:       now,
		DurationMs:        now.Sub(start).Milliseconds(),
		TriggerQueueDepth: triggerDepth,
		Success:           success,
	})
}

// HandleMigrations returns migration history.
func (m *LoadMonitor) HandleMigrations(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"migrations": m.migrations,
	})
}

// HandlePrometheusMetrics returns load monitor metrics in Prometheus text format.
func (m *LoadMonitor) HandlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	for shardID, metrics := range m.metrics {
		fmt.Fprintf(w, "monitor_queue_depth{shard=\"%s\"} %d\n", shardID, metrics.QueueDepth)
		fmt.Fprintf(w, "monitor_tps{shard=\"%s\"} %.2f\n", shardID, metrics.TotalQPS)
	}
	fmt.Fprintf(w, "monitor_migration_total %d\n", len(m.migrations))
}
