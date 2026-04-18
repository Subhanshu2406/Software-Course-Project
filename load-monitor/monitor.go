// Package monitor tracks metrics and handles partition rebalancing.
package monitor

import (
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
	prevCommitted  map[string]int64 // previous committed counts for rate calculation
	thresholdDepth int
	pollInterval   time.Duration
	cooldown       time.Duration
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
func NewLoadMonitor(sm *shardmap.ShardMap, threshold int, interval, cooldown time.Duration) *LoadMonitor {
	return &LoadMonitor{
		shardMap:       sm,
		metrics:        make(map[string]models.ShardMetrics),
		prevCommitted:  make(map[string]int64),
		thresholdDepth: threshold,
		pollInterval:   interval,
		cooldown:       cooldown,
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

	// Use delta between current and previous committed count (rate per poll interval)
	maxRate := int64(-1)
	minRate := int64(999999999)

	for _, shard := range m.shardMap.AllShards() {
		metrics, ok := m.metrics[shard.ShardID]
		if !ok {
			continue
		}
		prev := m.prevCommitted[shard.ShardID]
		rate := metrics.CommittedCount - prev
		m.prevCommitted[shard.ShardID] = metrics.CommittedCount

		if rate > maxRate {
			maxRate = rate
			sh := shard
			hotShard = &sh
		}
		if rate < minRate {
			minRate = rate
			sh := shard
			coolShard = &sh
		}
	}

	// Only migrate if the hot shard's rate exceeds threshold and imbalance is significant
	if hotShard != nil && coolShard != nil && maxRate > int64(m.thresholdDepth) && (maxRate-minRate) > int64(m.thresholdDepth)/2 && hotShard.ShardID != coolShard.ShardID {
		// Don't migrate if we've already migrated recently (cooldown)
		if len(m.migrations) > 0 {
			lastMigration := m.migrations[len(m.migrations)-1]
			if time.Since(lastMigration.CompletedAt) < m.cooldown {
				return
			}
		}
		log.Printf("monitor: detected hotspot %s (rate %d vs %d), migrating to %s", hotShard.ShardID, maxRate, minRate, coolShard.ShardID)
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

	// The current project does not enforce dynamic ownership in the coordinator
	// or remove migrated balances from the source shard. Mutating live shard
	// state here would duplicate money. Keep migration as a control-plane
	// rebalancing event for observability/frontend purposes.
	m.shardMap.UpdatePartition(partID, cool)
	log.Printf("monitor: successfully migrated partition %d to %s", partID, cool.ShardID)
	m.recordMigration(partID, hot.ShardID, cool.ShardID, startTime, triggerDepth, true)
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

// HandleShardMap returns the current partition → shard mapping.
func (m *LoadMonitor) HandleShardMap(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	partitions := make(map[string]string) // partition_id → shard_id
	for _, shard := range m.shardMap.AllShards() {
		for _, partID := range m.shardMap.GetPartitionsForShard(shard.ShardID) {
			partitions[fmt.Sprintf("%d", partID)] = shard.ShardID
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"partitions": partitions,
		"total":      len(partitions),
	})
}
