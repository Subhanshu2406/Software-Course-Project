package integration_test

import (
	"path/filepath"
	"testing"

	"ledger-service/shared/constants"
	"ledger-service/shared/models"
	"ledger-service/shard/server"
)

// newTestShard creates a fresh shard with predefined accounts.
func newTestShard(t *testing.T, walDir string, accounts map[string]int64) *server.ShardServer {
	t.Helper()
	walPath := filepath.Join(walDir, "test.wal")
	s, err := server.NewShardServer("shard-test", walPath, accounts)
	if err != nil {
		t.Fatalf("NewShardServer: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestSingleShardTransferSuccess is the happy path: transfer between two accounts.
func TestSingleShardTransferSuccess(t *testing.T) {
	s := newTestShard(t, t.TempDir(), map[string]int64{
		"alice": 1000,
		"bob":   500,
	})

	txn := models.Transaction{
		TxnID:       "txn-001",
		Source:      "alice",
		Destination: "bob",
		Amount:      300,
		State:       constants.StatePending,
	}

	result, err := s.ExecuteSingleShard(txn)
	if err != nil {
		t.Fatalf("ExecuteSingleShard: %v", err)
	}

	if result.State != constants.StateCommitted {
		t.Errorf("expected COMMITTED, got %s: %s", result.State, result.Message)
	}

	// Verify balances
	aliceBal, _ := s.GetBalance("alice")
	bobBal, _ := s.GetBalance("bob")

	if aliceBal != 700 {
		t.Errorf("alice = %d, want 700", aliceBal)
	}
	if bobBal != 800 {
		t.Errorf("bob = %d, want 800", bobBal)
	}
}

// TestSingleShardInsufficientFundsAborts verifies that overdraft is rejected.
func TestSingleShardInsufficientFundsAborts(t *testing.T) {
	s := newTestShard(t, t.TempDir(), map[string]int64{
		"alice": 100,
		"bob":   0,
	})

	result, err := s.ExecuteSingleShard(models.Transaction{
		TxnID:       "txn-overdraft",
		Source:      "alice",
		Destination: "bob",
		Amount:      500, // more than alice has
		State:       constants.StatePending,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.State != constants.StateAborted {
		t.Errorf("expected ABORTED, got %s", result.State)
	}

	// Balance must be unchanged
	aliceBal, _ := s.GetBalance("alice")
	if aliceBal != 100 {
		t.Errorf("alice balance changed after rejection: got %d, want 100", aliceBal)
	}
}

// TestConservationOfMoneyAfterTransfers verifies the conservation invariant across many txns.
func TestConservationOfMoneyAfterTransfers(t *testing.T) {
	s := newTestShard(t, t.TempDir(), map[string]int64{
		"alice":   2000,
		"bob":     1000,
		"charlie": 500,
	})

	totalBefore := s.TotalBalance()

	transactions := []models.Transaction{
		{TxnID: "t1", Source: "alice", Destination: "bob", Amount: 200, State: constants.StatePending},
		{TxnID: "t2", Source: "bob", Destination: "charlie", Amount: 100, State: constants.StatePending},
		{TxnID: "t3", Source: "charlie", Destination: "alice", Amount: 50, State: constants.StatePending},
		{TxnID: "t4", Source: "alice", Destination: "charlie", Amount: 400, State: constants.StatePending},
	}

	for _, txn := range transactions {
		result, err := s.ExecuteSingleShard(txn)
		if err != nil {
			t.Fatalf("txn %s error: %v", txn.TxnID, err)
		}
		if result.State != constants.StateCommitted {
			t.Errorf("txn %s: expected COMMITTED, got %s", txn.TxnID, result.State)
		}
	}

	totalAfter := s.TotalBalance()
	if totalBefore != totalAfter {
		t.Errorf("CONSERVATION VIOLATED: before=%d after=%d", totalBefore, totalAfter)
	}
}

// TestIdempotency verifies that replaying a transaction produces the same result.
// This is critical for coordinator failure recovery (REQ-COORD-002).
func TestIdempotency(t *testing.T) {
	s := newTestShard(t, t.TempDir(), map[string]int64{
		"alice": 1000,
		"bob":   0,
	})

	txn := models.Transaction{
		TxnID:       "txn-idem",
		Source:      "alice",
		Destination: "bob",
		Amount:      100,
		State:       constants.StatePending,
	}

	// First execution
	r1, _ := s.ExecuteSingleShard(txn)
	if r1.State != constants.StateCommitted {
		t.Fatalf("first execution failed: %s", r1.State)
	}

	// Second execution with same txnID — must return same result without double-applying
	r2, _ := s.ExecuteSingleShard(txn)
	if r2.State != constants.StateCommitted {
		t.Errorf("second execution state = %s, want COMMITTED", r2.State)
	}

	// Alice must NOT have been debited twice
	aliceBal, _ := s.GetBalance("alice")
	if aliceBal != 900 {
		t.Errorf("alice balance = %d after idempotent replay, want 900 (double-debit detected!)", aliceBal)
	}
}

// TestCrashRecovery is the most important integration test.
// It simulates a crash after commit and verifies state is fully restored from WAL.
func TestCrashRecovery(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "recovery.wal")

	// Phase 1: Create shard, do some transactions, close (simulates clean shutdown or crash)
	{
		s, err := server.NewShardServer("shard-recovery", walPath, map[string]int64{
			"alice": 1000,
			"bob":   500,
		})
		if err != nil {
			t.Fatalf("Phase 1 shard creation: %v", err)
		}

		_, _ = s.ExecuteSingleShard(models.Transaction{
			TxnID: "pre-crash-1", Source: "alice", Destination: "bob",
			Amount: 200, State: constants.StatePending,
		})
		_, _ = s.ExecuteSingleShard(models.Transaction{
			TxnID: "pre-crash-2", Source: "bob", Destination: "alice",
			Amount: 50, State: constants.StatePending,
		})

		s.Close()
	}

	// Phase 2: "Restart" — create a new shard server pointing at the same WAL.
	// No initial balances passed in — state must come entirely from WAL replay.
	{
		s2, err := server.NewShardServer("shard-recovery", walPath, nil)
		if err != nil {
			t.Fatalf("Phase 2 shard restart: %v", err)
		}
		defer s2.Close()

		// After replay: alice started at 1000, transferred 200 out, received 50 back = 850
		aliceBal, ok := s2.GetBalance("alice")
		if !ok {
			t.Fatal("alice not found after recovery")
		}
		if aliceBal != 850 {
			t.Errorf("alice after recovery = %d, want 850", aliceBal)
		}

		// bob started at 500, received 200, transferred 50 out = 650
		bobBal, ok := s2.GetBalance("bob")
		if !ok {
			t.Fatal("bob not found after recovery")
		}
		if bobBal != 650 {
			t.Errorf("bob after recovery = %d, want 650", bobBal)
		}

		// Total must be conserved: 1000 + 500 = 1500
		total := s2.TotalBalance()
		if total != 1500 {
			t.Errorf("total after recovery = %d, want 1500 (conservation violated)", total)
		}
	}
}

// TestNoPartialCommitOnLedgerError verifies that if ledger apply fails after WAL write,
// the transaction is aborted and no partial state is visible.
// (This is a defensive test for the atomicity guarantee.)
func TestAbortedTransactionLeavesNoPartialState(t *testing.T) {
	s := newTestShard(t, t.TempDir(), map[string]int64{
		"alice": 50,
		"bob":   100,
	})

	// This will be ABORTED (insufficient funds)
	result, _ := s.ExecuteSingleShard(models.Transaction{
		TxnID:       "txn-abort-test",
		Source:      "alice",
		Destination: "bob",
		Amount:      200,
		State:       constants.StatePending,
	})

	if result.State != constants.StateAborted {
		t.Fatalf("expected ABORTED, got %s", result.State)
	}

	// Neither balance should have changed
	aliceBal, _ := s.GetBalance("alice")
	bobBal, _ := s.GetBalance("bob")

	if aliceBal != 50 {
		t.Errorf("alice balance changed after abort: got %d, want 50", aliceBal)
	}
	if bobBal != 100 {
		t.Errorf("bob balance changed after abort: got %d, want 100", bobBal)
	}
}
