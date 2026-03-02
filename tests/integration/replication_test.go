package integration_test

import (
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"ledger-service/shared/constants"
	"ledger-service/shared/models"
	"ledger-service/shard/replication"
	"ledger-service/shard/wal"
)

// startFollowerHTTP starts a follower with HTTP endpoints and returns its address.
func startFollowerHTTP(t *testing.T, followerID string) (string, *wal.WAL) {
	t.Helper()
	dir := t.TempDir()
	walPath := filepath.Join(dir, "follower.wal")

	w, err := wal.Open(walPath)
	if err != nil {
		t.Fatalf("Open WAL for follower %s: %v", followerID, err)
	}

	receiver := replication.NewFollowerReceiver(followerID, w)

	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	mux := http.NewServeMux()
	receiver.RegisterHTTPHandlers(mux)

	srv := &http.Server{Addr: addr, Handler: mux}
	go srv.ListenAndServe()
	t.Cleanup(func() {
		srv.Close()
		w.Close()
	})

	// Wait for server to be ready
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	return addr, w
}

// TestReplicationToFollower verifies WAL entries are replicated and persisted.
func TestReplicationToFollower(t *testing.T) {
	addr, followerWAL := startFollowerHTTP(t, "follower-1")

	// Create a replication client and send an entry
	client := replication.NewReplicationClient(5 * time.Second)

	entry := models.WALEntry{
		LogID:     0,
		TxnID:     "txn-repl-1",
		OpType:    constants.OpDebit,
		AccountID: "alice",
		Amount:    100,
	}

	if err := client.SendEntry(addr, entry); err != nil {
		t.Fatalf("SendEntry: %v", err)
	}

	// Verify entry was persisted to follower's WAL
	entries, err := followerWAL.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("follower WAL has %d entries, want 1", len(entries))
	}
	if entries[0].TxnID != "txn-repl-1" {
		t.Errorf("replicated txnID = %s, want txn-repl-1", entries[0].TxnID)
	}
}

// TestReplicationLogIndex verifies log index reporting.
func TestReplicationLogIndex(t *testing.T) {
	addr, _ := startFollowerHTTP(t, "follower-idx")

	client := replication.NewReplicationClient(5 * time.Second)

	// Send a few entries
	for i := uint64(0); i < 3; i++ {
		entry := models.WALEntry{
			LogID:     i,
			TxnID:     fmt.Sprintf("txn-%d", i),
			OpType:    constants.OpDebit,
			AccountID: "alice",
			Amount:    10,
		}
		if err := client.SendEntry(addr, entry); err != nil {
			t.Fatalf("SendEntry %d: %v", i, err)
		}
	}

	// Query log index
	logID, err := client.GetLogIndex(addr)
	if err != nil {
		t.Fatalf("GetLogIndex: %v", err)
	}
	if logID != 2 {
		t.Errorf("last log ID = %d, want 2", logID)
	}
}

// TestPrimaryReplicatorQuorum verifies quorum-based replication.
func TestPrimaryReplicatorQuorum(t *testing.T) {
	// Start 2 followers
	addr1, _ := startFollowerHTTP(t, "follower-1")
	addr2, _ := startFollowerHTTP(t, "follower-2")

	// Create primary replicator with quorum of 2 (leader + 1 follower)
	primary := replication.NewPrimaryReplicator("leader", 2, 5*time.Second)
	primary.AddFollower("follower-1", addr1)
	primary.AddFollower("follower-2", addr2)

	entry := models.WALEntry{
		LogID:     0,
		TxnID:     "txn-quorum-1",
		OpType:    constants.OpDebit,
		AccountID: "alice",
		Amount:    500,
	}

	// Replicate — should succeed because we have 2 followers and need 1 ACK
	if err := primary.Replicate(entry); err != nil {
		t.Fatalf("Replicate: %v", err)
	}
}

// TestPrimaryReplicatorNoFollowers works in leader-only mode.
func TestPrimaryReplicatorNoFollowers(t *testing.T) {
	primary := replication.NewPrimaryReplicator("leader", 1, 5*time.Second)

	entry := models.WALEntry{
		LogID:  0,
		TxnID:  "solo-txn",
		OpType: constants.OpDebit,
	}

	// With quorum=1, no followers needed
	if err := primary.Replicate(entry); err != nil {
		t.Fatalf("Replicate in leader-only mode: %v", err)
	}
}

// TestPrimaryReplicatorQuorumImpossible fails when quorum can't be met.
func TestPrimaryReplicatorQuorumImpossible(t *testing.T) {
	// Quorum of 3 but only 1 follower and it's unreachable
	primary := replication.NewPrimaryReplicator("leader", 3, 1*time.Second)
	primary.AddFollower("follower-dead", "127.0.0.1:1") // unreachable

	entry := models.WALEntry{
		LogID:  0,
		TxnID:  "fail-txn",
		OpType: constants.OpDebit,
	}

	if err := primary.Replicate(entry); err == nil {
		t.Error("expected error when quorum is impossible")
	}
}

// TestReplicationMultipleEntries verifies sequential replication of multiple entries.
func TestReplicationMultipleEntries(t *testing.T) {
	addr, followerWAL := startFollowerHTTP(t, "follower-multi")

	client := replication.NewReplicationClient(5 * time.Second)

	// Send 5 entries
	for i := uint64(0); i < 5; i++ {
		entry := models.WALEntry{
			LogID:     i,
			TxnID:     fmt.Sprintf("txn-%d", i),
			OpType:    constants.OpCredit,
			AccountID: "bob",
			Amount:    int64(i * 100),
		}
		if err := client.SendEntry(addr, entry); err != nil {
			t.Fatalf("SendEntry %d: %v", i, err)
		}
	}

	// Verify all entries persisted
	entries, err := followerWAL.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(entries) != 5 {
		t.Errorf("follower has %d entries, want 5", len(entries))
	}
}
