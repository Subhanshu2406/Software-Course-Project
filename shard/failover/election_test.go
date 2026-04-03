package failover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ledger-service/coordinator/shardmap"
	"ledger-service/messaging"
)

func TestElectionManager_TriggerElection(t *testing.T) {
	// Create mock shard servers
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/log-index":
			json.NewEncoder(w).Encode(map[string]uint64{"last_log_id": 10})
		case "/promote":
			json.NewEncoder(w).Encode(map[string]string{"status": "PROMOTED"})
		case "/health":
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/log-index":
			json.NewEncoder(w).Encode(map[string]uint64{"last_log_id": 15})
		case "/promote":
			json.NewEncoder(w).Encode(map[string]string{"status": "PROMOTED"})
		case "/health":
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server2.Close()

	// Strip http:// prefix for addresses
	addr1 := server1.Listener.Addr().String()
	addr2 := server2.Listener.Addr().String()

	// Create a shard map with some partitions
	sm, err := shardmap.NewShardMap(t.TempDir()+"/test_shard_map.json", []shardmap.ShardInfo{
		{ShardID: "test-shard", Address: addr1, Role: "PRIMARY"},
	}, 5)
	if err != nil {
		t.Fatalf("failed to create shard map: %v", err)
	}

	client := messaging.NewShardClient(2 * time.Second)
	em := NewElectionManager("test-shard", []string{addr1, addr2}, client, sm)

	err = em.TriggerElection()
	if err != nil {
		t.Fatalf("TriggerElection failed: %v", err)
	}

	// Verify that all partitions now point to server2 (higher log index)
	for i := 0; i < 5; i++ {
		info, ok := sm.GetShard(i)
		if !ok {
			t.Fatalf("partition %d not found", i)
		}
		if info.Address != addr2 {
			t.Errorf("partition %d: expected address %s, got %s", i, addr2, info.Address)
		}
	}
}

func TestElectionManager_NoReachableReplicas(t *testing.T) {
	sm, _ := shardmap.NewShardMap(t.TempDir()+"/test_shard_map.json", []shardmap.ShardInfo{
		{ShardID: "test-shard", Address: "192.0.2.1:9999", Role: "PRIMARY"},
	}, 5)

	client := messaging.NewShardClient(100 * time.Millisecond)
	em := NewElectionManager("test-shard", []string{"192.0.2.1:9999"}, client, sm)

	err := em.TriggerElection()
	if err == nil {
		t.Error("expected error when no replicas are reachable")
	}
}
