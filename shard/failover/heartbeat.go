// Package failover implements heartbeat monitoring and leader election
// for shard replica groups.
//
// Per REQ-REP-004: "The system SHALL support automatic leader election
// when a PRIMARY node fails."
package failover

import (
	"context"
	"log"
	"sync"
	"time"

	"ledger-service/messaging"
)

// HeartbeatMonitor periodically pings peer replicas and detects failures.
// When a peer misses `missLimit` consecutive heartbeats, the onFailure
// callback is invoked.
type HeartbeatMonitor struct {
	shardID   string
	peerAddrs []string // other replicas in this shard's group
	client    *messaging.ShardClient
	interval  time.Duration
	missLimit int
	missCount map[string]int // peer → consecutive misses
	mu        sync.Mutex     // guards missCount
	onFailure func(peerAddr string)
}

// NewHeartbeatMonitor creates a new heartbeat monitor for a shard group.
func NewHeartbeatMonitor(shardID string, peers []string, client *messaging.ShardClient, interval time.Duration, missLimit int) *HeartbeatMonitor {
	mc := make(map[string]int, len(peers))
	for _, p := range peers {
		mc[p] = 0
	}
	return &HeartbeatMonitor{
		shardID:   shardID,
		peerAddrs: peers,
		client:    client,
		interval:  interval,
		missLimit: missLimit,
		missCount: mc,
	}
}

// OnFailure registers a callback that fires when a peer is deemed unreachable.
func (h *HeartbeatMonitor) OnFailure(fn func(peerAddr string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onFailure = fn
}

// Start begins the heartbeat loop. It runs until the context is cancelled.
func (h *HeartbeatMonitor) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("heartbeat: monitor for shard %s stopped", h.shardID)
				return
			case <-ticker.C:
				h.pingAll()
			}
		}
	}()
}

// pingAll pings every peer and updates miss counts.
func (h *HeartbeatMonitor) pingAll() {
	for _, peer := range h.peerAddrs {
		err := h.client.HealthCheck(peer)

		h.mu.Lock()
		if err != nil {
			h.missCount[peer]++
			log.Printf("heartbeat: shard %s — peer %s missed heartbeat (%d/%d)",
				h.shardID, peer, h.missCount[peer], h.missLimit)

			if h.missCount[peer] >= h.missLimit && h.onFailure != nil {
				log.Printf("heartbeat: shard %s — peer %s declared FAILED after %d misses",
					h.shardID, peer, h.missCount[peer])
				fn := h.onFailure
				h.mu.Unlock()
				fn(peer)
				continue
			}
		} else {
			if h.missCount[peer] > 0 {
				log.Printf("heartbeat: shard %s — peer %s recovered", h.shardID, peer)
			}
			h.missCount[peer] = 0
		}
		h.mu.Unlock()
	}
}

// GetMissCount returns the current miss count for a peer (used in tests).
func (h *HeartbeatMonitor) GetMissCount(peer string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.missCount[peer]
}
