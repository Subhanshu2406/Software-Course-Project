package unit_test

import (
	"os"
	"path/filepath"
	"testing"

	"ledger-service/storage"
)

// TestNewJSONStoreCreatesFile verifies that a new store creates the file.
func TestNewJSONStoreCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test_store.json")

	store, err := storage.NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}
	defer store.Close()

	// File should exist after first write
	if err := store.SetBalance("alice", 1000); err != nil {
		t.Fatalf("SetBalance: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected store file to exist after write")
	}
}

// TestJSONStoreSetAndGetBalance verifies basic read/write.
func TestJSONStoreSetAndGetBalance(t *testing.T) {
	store := newTestStore(t)

	if err := store.SetBalance("alice", 1000); err != nil {
		t.Fatalf("SetBalance: %v", err)
	}

	bal, exists, err := store.GetBalance("alice")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if !exists {
		t.Error("expected alice to exist")
	}
	if bal != 1000 {
		t.Errorf("balance = %d, want 1000", bal)
	}
}

// TestJSONStoreGetNonExistentAccount returns false for missing accounts.
func TestJSONStoreGetNonExistentAccount(t *testing.T) {
	store := newTestStore(t)

	_, exists, err := store.GetBalance("nobody")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if exists {
		t.Error("expected non-existent account to return exists=false")
	}
}

// TestJSONStoreBatchSetBalances verifies atomic batch write.
func TestJSONStoreBatchSetBalances(t *testing.T) {
	store := newTestStore(t)

	balances := map[string]int64{
		"alice":   1000,
		"bob":     500,
		"charlie": 250,
	}

	if err := store.BatchSetBalances(balances); err != nil {
		t.Fatalf("BatchSetBalances: %v", err)
	}

	all, err := store.GetAllBalances()
	if err != nil {
		t.Fatalf("GetAllBalances: %v", err)
	}

	if len(all) != 3 {
		t.Errorf("expected 3 accounts, got %d", len(all))
	}

	for id, expectedBal := range balances {
		if all[id] != expectedBal {
			t.Errorf("%s balance = %d, want %d", id, all[id], expectedBal)
		}
	}
}

// TestJSONStoreCheckpointLogID verifies checkpoint tracking.
func TestJSONStoreCheckpointLogID(t *testing.T) {
	store := newTestStore(t)

	logID, err := store.GetCheckpointLogID()
	if err != nil {
		t.Fatalf("GetCheckpointLogID: %v", err)
	}
	if logID != 0 {
		t.Errorf("initial checkpoint log ID = %d, want 0", logID)
	}

	if err := store.SetCheckpointLogID(42); err != nil {
		t.Fatalf("SetCheckpointLogID: %v", err)
	}

	logID, err = store.GetCheckpointLogID()
	if err != nil {
		t.Fatalf("GetCheckpointLogID: %v", err)
	}
	if logID != 42 {
		t.Errorf("checkpoint log ID = %d, want 42", logID)
	}
}

// TestJSONStorePersistence verifies data survives close and reopen.
func TestJSONStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist_store.json")

	// Write data
	store1, err := storage.NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}
	if err := store1.SetBalance("alice", 999); err != nil {
		t.Fatalf("SetBalance: %v", err)
	}
	if err := store1.SetCheckpointLogID(10); err != nil {
		t.Fatalf("SetCheckpointLogID: %v", err)
	}
	store1.Close()

	// Reopen and verify
	store2, err := storage.NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore reopen: %v", err)
	}
	defer store2.Close()

	bal, exists, err := store2.GetBalance("alice")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if !exists || bal != 999 {
		t.Errorf("after reopen: alice exists=%v balance=%d, want exists=true balance=999", exists, bal)
	}

	logID, err := store2.GetCheckpointLogID()
	if err != nil {
		t.Fatalf("GetCheckpointLogID: %v", err)
	}
	if logID != 10 {
		t.Errorf("after reopen: checkpoint log ID = %d, want 10", logID)
	}
}

// TestJSONStoreBatchOverwrites verifies that batch replaces all balances.
func TestJSONStoreBatchOverwrites(t *testing.T) {
	store := newTestStore(t)

	// Set initial
	_ = store.SetBalance("alice", 500)
	_ = store.SetBalance("bob", 300)

	// Batch overwrite (alice stays, bob disappears, charlie appears)
	err := store.BatchSetBalances(map[string]int64{
		"alice":   1000,
		"charlie": 750,
	})
	if err != nil {
		t.Fatalf("BatchSetBalances: %v", err)
	}

	// bob should be gone
	_, exists, _ := store.GetBalance("bob")
	if exists {
		t.Error("bob should not exist after batch overwrite")
	}

	// alice should be updated
	bal, _, _ := store.GetBalance("alice")
	if bal != 1000 {
		t.Errorf("alice = %d, want 1000", bal)
	}

	// charlie should exist
	bal, exists, _ = store.GetBalance("charlie")
	if !exists || bal != 750 {
		t.Errorf("charlie exists=%v balance=%d, want exists=true balance=750", exists, bal)
	}
}

// --- helper ---

func newTestStore(t *testing.T) *storage.JSONStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test_store.json")
	store, err := storage.NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}
