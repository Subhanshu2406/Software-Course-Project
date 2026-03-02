package integration_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"ledger-service/coordinator/shardmap"
	"ledger-service/coordinator/twopc"
	"ledger-service/messaging"
	"ledger-service/shared/constants"
	"ledger-service/shared/models"
	"ledger-service/shard/server"
)

// getFreePort finds an available TCP port.
func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("getFreePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// startShardHTTP starts a shard server with an HTTP listener and returns
// the address and a cleanup function.
func startShardHTTP(t *testing.T, shardID string, accounts map[string]int64) (string, *server.ShardServer) {
	t.Helper()
	dir := t.TempDir()
	walPath := filepath.Join(dir, "shard.wal")

	s, err := server.NewShardServer(shardID, walPath, accounts)
	if err != nil {
		t.Fatalf("NewShardServer %s: %v", shardID, err)
	}

	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	handler := server.NewHTTPHandler(s)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	srv := &http.Server{Addr: addr, Handler: mux}
	go srv.ListenAndServe()
	t.Cleanup(func() {
		srv.Close()
		s.Close()
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

	return addr, s
}

// TestCrossShardTransferVia2PC verifies a cross-shard transaction using 2PC.
func TestCrossShardTransferVia2PC(t *testing.T) {
	// Start two shard servers
	addr1, shard1 := startShardHTTP(t, "shard-1", map[string]int64{
		"alice": 1000,
	})
	addr2, shard2 := startShardHTTP(t, "shard-2", map[string]int64{
		"bob": 500,
	})

	// Create a 2PC coordinator
	client := messaging.NewShardClient(5 * time.Second)
	coordinator := twopc.NewCoordinator(client)

	txn := models.Transaction{
		TxnID:       "cross-txn-1",
		Source:      "alice",
		Destination: "bob",
		Amount:      300,
	}

	srcShard := shardmap.ShardInfo{ShardID: "shard-1", Address: addr1}
	dstShard := shardmap.ShardInfo{ShardID: "shard-2", Address: addr2}

	result, err := coordinator.Execute(txn, srcShard, dstShard)
	if err != nil {
		t.Fatalf("2PC Execute: %v", err)
	}

	if result.State != constants.StateCommitted {
		t.Errorf("expected COMMITTED, got %s: %s", result.State, result.Message)
	}

	// Verify balances
	aliceBal, _ := shard1.GetBalance("alice")
	bobBal, _ := shard2.GetBalance("bob")

	if aliceBal != 700 {
		t.Errorf("alice = %d, want 700", aliceBal)
	}
	if bobBal != 800 {
		t.Errorf("bob = %d, want 800", bobBal)
	}
}

// TestCrossShardTransferInsufficientFundsAborts verifies 2PC abort on validation failure.
func TestCrossShardTransferInsufficientFundsAborts(t *testing.T) {
	addr1, shard1 := startShardHTTP(t, "shard-1", map[string]int64{
		"alice": 100,
	})
	addr2, shard2 := startShardHTTP(t, "shard-2", map[string]int64{
		"bob": 500,
	})

	client := messaging.NewShardClient(5 * time.Second)
	coordinator := twopc.NewCoordinator(client)

	txn := models.Transaction{
		TxnID:       "cross-txn-fail",
		Source:      "alice",
		Destination: "bob",
		Amount:      500, // more than alice has
	}

	result, err := coordinator.Execute(txn,
		shardmap.ShardInfo{ShardID: "shard-1", Address: addr1},
		shardmap.ShardInfo{ShardID: "shard-2", Address: addr2},
	)
	if err != nil {
		t.Fatalf("2PC Execute: %v", err)
	}

	if result.State != constants.StateAborted {
		t.Errorf("expected ABORTED, got %s: %s", result.State, result.Message)
	}

	// Balances should be unchanged
	aliceBal, _ := shard1.GetBalance("alice")
	bobBal, _ := shard2.GetBalance("bob")

	if aliceBal != 100 {
		t.Errorf("alice should be unchanged: got %d, want 100", aliceBal)
	}
	if bobBal != 500 {
		t.Errorf("bob should be unchanged: got %d, want 500", bobBal)
	}
}

// TestHTTPHandlerBalance verifies the /balance endpoint works via HTTP.
func TestHTTPHandlerBalance(t *testing.T) {
	addr, _ := startShardHTTP(t, "shard-bal", map[string]int64{
		"alice": 1234,
	})

	client := messaging.NewShardClient(5 * time.Second)
	bal, exists, err := client.GetBalance(addr, "alice")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if !exists {
		t.Error("alice should exist")
	}
	if bal != 1234 {
		t.Errorf("balance = %d, want 1234", bal)
	}
}

// TestHTTPHandlerHealth verifies the /health endpoint.
func TestHTTPHandlerHealth(t *testing.T) {
	addr, _ := startShardHTTP(t, "shard-health", nil)

	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d, want 200", resp.StatusCode)
	}

	var health map[string]string
	json.NewDecoder(resp.Body).Decode(&health)
	if health["shard_id"] != "shard-health" {
		t.Errorf("shard_id = %s, want shard-health", health["shard_id"])
	}
}

// TestHTTPHandlerExecuteSingleShard verifies the /execute endpoint.
func TestHTTPHandlerExecuteSingleShard(t *testing.T) {
	addr, _ := startShardHTTP(t, "shard-exec", map[string]int64{
		"alice": 1000,
		"bob":   500,
	})

	client := messaging.NewShardClient(5 * time.Second)
	txn := models.Transaction{
		TxnID:       "http-exec-1",
		Source:      "alice",
		Destination: "bob",
		Amount:      200,
		State:       constants.StatePending,
	}

	result, err := client.Execute(addr, txn)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result.State != constants.StateCommitted {
		t.Errorf("expected COMMITTED, got %s", result.State)
	}

	// Verify via balance endpoint
	aliceBal, _, _ := client.GetBalance(addr, "alice")
	bobBal, _, _ := client.GetBalance(addr, "bob")

	if aliceBal != 800 {
		t.Errorf("alice = %d, want 800", aliceBal)
	}
	if bobBal != 700 {
		t.Errorf("bob = %d, want 700", bobBal)
	}
}

// TestConservationAcrossShards verifies total money is conserved in cross-shard txns.
func TestConservationAcrossShards(t *testing.T) {
	addr1, shard1 := startShardHTTP(t, "shard-1", map[string]int64{
		"alice":   1000,
		"charlie": 500,
	})
	addr2, shard2 := startShardHTTP(t, "shard-2", map[string]int64{
		"bob":  800,
		"dave": 200,
	})

	totalBefore := shard1.TotalBalance() + shard2.TotalBalance()

	client := messaging.NewShardClient(5 * time.Second)
	coordinator := twopc.NewCoordinator(client)

	// Execute multiple cross-shard transactions
	txns := []struct {
		id     string
		src    string
		dst    string
		amount int64
		srcSh  string
		dstSh  string
	}{
		{"cs-1", "alice", "bob", 200, addr1, addr2},
		{"cs-2", "bob", "charlie", 100, addr2, addr1},
		{"cs-3", "alice", "dave", 50, addr1, addr2},
	}

	for _, tx := range txns {
		result, err := coordinator.Execute(
			models.Transaction{TxnID: tx.id, Source: tx.src, Destination: tx.dst, Amount: tx.amount},
			shardmap.ShardInfo{ShardID: "shard-1", Address: tx.srcSh},
			shardmap.ShardInfo{ShardID: "shard-2", Address: tx.dstSh},
		)
		if err != nil {
			t.Fatalf("txn %s: %v", tx.id, err)
		}
		if result.State != constants.StateCommitted {
			t.Errorf("txn %s: expected COMMITTED, got %s", tx.id, result.State)
		}
	}

	totalAfter := shard1.TotalBalance() + shard2.TotalBalance()
	if totalAfter != totalBefore {
		t.Errorf("conservation violated: before=%d after=%d", totalBefore, totalAfter)
	}
}
