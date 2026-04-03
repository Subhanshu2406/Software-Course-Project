package failover

import (
	"context"
	"testing"
	"time"

	"ledger-service/messaging"
)

func TestHeartbeatMonitor_MissCount(t *testing.T) {
	// Use a non-routable address to ensure health checks fail
	peers := []string{"192.0.2.1:9999"}
	client := messaging.NewShardClient(100 * time.Millisecond)

	hm := NewHeartbeatMonitor("test-shard", peers, client, 100*time.Millisecond, 3)

	failedPeer := ""
	hm.OnFailure(func(peer string) {
		failedPeer = peer
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	hm.Start(ctx)

	// Wait enough for several heartbeats
	time.Sleep(600 * time.Millisecond)

	if failedPeer == "" {
		t.Error("expected onFailure to be called for unreachable peer")
	}
	if failedPeer != "192.0.2.1:9999" {
		t.Errorf("expected failed peer 192.0.2.1:9999, got %s", failedPeer)
	}
}

func TestHeartbeatMonitor_NoFailureWhenHealthy(t *testing.T) {
	// No peers means no failures
	client := messaging.NewShardClient(100 * time.Millisecond)
	hm := NewHeartbeatMonitor("test-shard", []string{}, client, 100*time.Millisecond, 3)

	failed := false
	hm.OnFailure(func(peer string) {
		failed = true
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	hm.Start(ctx)

	time.Sleep(400 * time.Millisecond)

	if failed {
		t.Error("onFailure should not be called when there are no peers")
	}
}
