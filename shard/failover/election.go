// Package failover implements heartbeat monitoring and leader election.
//
// ElectionManager follows Algorithm 4 from the IMPLEMENTATION_PLAN:
//   1. Query GET /log-index on all live replicas
//   2. Pick the one with the highest last_log_id
//   3. Call POST /promote on that replica
//   4. Update ShardMap to mark that replica as PRIMARY
package failover

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"ledger-service/coordinator/shardmap"
	"ledger-service/messaging"
)

// ElectionManager handles leader election when a PRIMARY fails.
type ElectionManager struct {
	shardID    string
	replicaSet []string // addresses of all replicas in this shard group
	client     *messaging.ShardClient
	shardMap   *shardmap.ShardMap
	role       string // "PRIMARY" or "FOLLOWER"
	mu         sync.Mutex
}

// NewElectionManager creates a new election manager for a shard group.
func NewElectionManager(shardID string, replicas []string, client *messaging.ShardClient, sm *shardmap.ShardMap) *ElectionManager {
	return &ElectionManager{
		shardID:    shardID,
		replicaSet: replicas,
		client:     client,
		shardMap:   sm,
		role:       "FOLLOWER",
	}
}

// TriggerElection runs the leader election protocol (Algorithm 4).
// It queries all replicas for their log index, picks the most up-to-date one,
// promotes it, and updates the shard map.
func (e *ElectionManager) TriggerElection() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	log.Printf("election: starting election for shard group %s", e.shardID)

	type replicaInfo struct {
		addr     string
		logIndex uint64
	}

	var candidates []replicaInfo

	httpClient := &http.Client{Timeout: 3 * time.Second}

	for _, addr := range e.replicaSet {
		url := fmt.Sprintf("http://%s/log-index", addr)
		resp, err := httpClient.Get(url)
		if err != nil {
			log.Printf("election: replica %s unreachable: %v", addr, err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil || resp.StatusCode != http.StatusOK {
			log.Printf("election: replica %s returned error", addr)
			continue
		}

		var result map[string]uint64
		if err := json.Unmarshal(body, &result); err != nil {
			log.Printf("election: replica %s bad response: %v", addr, err)
			continue
		}

		candidates = append(candidates, replicaInfo{addr: addr, logIndex: result["last_log_id"]})
		log.Printf("election: replica %s has log index %d", addr, result["last_log_id"])
	}

	if len(candidates) == 0 {
		return fmt.Errorf("election: no reachable replicas for shard %s", e.shardID)
	}

	// Pick the candidate with the highest log index
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.logIndex > best.logIndex {
			best = c
		}
	}

	log.Printf("election: promoting %s (log index %d) as new PRIMARY for %s",
		best.addr, best.logIndex, e.shardID)

	// Call POST /promote on the best candidate
	promoteURL := fmt.Sprintf("http://%s/promote", best.addr)
	promoteBody, _ := json.Marshal(map[string]string{"shard_id": e.shardID})
	resp, err := httpClient.Post(promoteURL, "application/json", bytes.NewReader(promoteBody))
	if err != nil {
		return fmt.Errorf("election: failed to promote %s: %w", best.addr, err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("election: promote returned %d for %s", resp.StatusCode, best.addr)
	}

	// Update shard map — reassign all partitions of this shard to the new primary
	newShardInfo := shardmap.ShardInfo{
		ShardID: e.shardID,
		Address: best.addr,
		Role:    "PRIMARY",
	}

	partitions := e.shardMap.GetPartitionsForShard(e.shardID)
	for _, partID := range partitions {
		if err := e.shardMap.UpdatePartition(partID, newShardInfo); err != nil {
			log.Printf("election: failed to update partition %d: %v", partID, err)
		}
	}

	log.Printf("election: %s is now PRIMARY for shard %s (%d partitions updated)",
		best.addr, e.shardID, len(partitions))

	return nil
}
