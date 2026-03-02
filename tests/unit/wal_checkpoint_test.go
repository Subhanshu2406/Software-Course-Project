package unit_test

import (
	"path/filepath"
	"testing"

	"ledger-service/shared/constants"
	"ledger-service/shard/wal"
)

// TestWALWriteCheckpoint verifies checkpoint marker writing.
func TestWALWriteCheckpoint(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "wal_ckpt.log")
	w, err := wal.Open(walPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	// Append some entries
	w.Append("txn-1", constants.OpDebit, "alice", 100)
	w.Append("txn-1", constants.OpCredit, "bob", 100)
	w.MarkCommitted("txn-1")

	// Write checkpoint at current position
	checkpointAt := w.NextLogID()
	if err := w.WriteCheckpoint(checkpointAt); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	// Verify checkpoint entry exists in WAL
	entries, err := w.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	lastEntry := entries[len(entries)-1]
	if lastEntry.OpType != constants.OpCheckpoint {
		t.Errorf("last entry op = %s, want CHECKPOINT", lastEntry.OpType)
	}
	if lastEntry.CheckpointLogID != checkpointAt {
		t.Errorf("checkpoint log ID = %d, want %d", lastEntry.CheckpointLogID, checkpointAt)
	}
}

// TestWALReadFrom verifies partial WAL reads.
func TestWALReadFrom(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "wal_readfrom.log")
	w, err := wal.Open(walPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	// Write 5 entries
	for i := 0; i < 5; i++ {
		w.Append("txn", constants.OpDebit, "alice", 10)
	}

	// ReadFrom entry 3
	entries, err := w.ReadFrom(3)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("ReadFrom(3) returned %d entries, want 2", len(entries))
	}

	for _, e := range entries {
		if e.LogID < 3 {
			t.Errorf("ReadFrom(3) returned entry with LogID %d", e.LogID)
		}
	}
}

// TestWALTruncate verifies WAL compaction.
func TestWALTruncate(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "wal_truncate.log")
	w, err := wal.Open(walPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	// Write 6 entries (IDs 0-5)
	for i := 0; i < 6; i++ {
		w.Append("txn", constants.OpDebit, "alice", 10)
	}

	// Truncate entries before ID 4 (keep 4 and 5)
	if err := w.Truncate(4); err != nil {
		t.Fatalf("Truncate: %v", err)
	}

	// Read remaining entries
	entries, err := w.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll after truncate: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("after truncate: %d entries, want 2", len(entries))
	}

	if entries[0].LogID != 4 {
		t.Errorf("first remaining entry LogID = %d, want 4", entries[0].LogID)
	}

	// Verify we can still append after truncate
	_, err = w.Append("txn-new", constants.OpCredit, "bob", 50)
	if err != nil {
		t.Fatalf("Append after truncate: %v", err)
	}

	entries, _ = w.ReadAll()
	if len(entries) != 3 {
		t.Errorf("after truncate+append: %d entries, want 3", len(entries))
	}
}

// TestWALTruncateAll removes everything if beforeLogID is very high.
func TestWALTruncateAll(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "wal_truncall.log")
	w, err := wal.Open(walPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	w.Append("txn", constants.OpDebit, "alice", 100)
	w.Append("txn", constants.OpCredit, "bob", 100)

	if err := w.Truncate(999); err != nil {
		t.Fatalf("Truncate: %v", err)
	}

	entries, _ := w.ReadAll()
	if len(entries) != 0 {
		t.Errorf("after truncate all: %d entries, want 0", len(entries))
	}
}

// TestWALCheckpointAndTruncateWorkflow simulates a full checkpoint cycle.
func TestWALCheckpointAndTruncateWorkflow(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "wal_flow.log")
	w, err := wal.Open(walPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	// Simulate some committed transactions
	w.Append("txn-1", constants.OpDebit, "alice", 100)
	w.Append("txn-1", constants.OpCredit, "bob", 100)
	w.MarkCommitted("txn-1")

	ckptAt := w.NextLogID()

	// More entries after checkpoint position
	w.Append("txn-2", constants.OpDebit, "bob", 50)
	w.Append("txn-2", constants.OpCredit, "alice", 50)
	w.MarkCommitted("txn-2")

	// Write checkpoint marker
	w.WriteCheckpoint(ckptAt)

	// Truncate entries before checkpoint
	if err := w.Truncate(ckptAt); err != nil {
		t.Fatalf("Truncate: %v", err)
	}

	// Only entries from txn-2 and the checkpoint should remain
	entries, _ := w.ReadAll()
	for _, e := range entries {
		if e.LogID < ckptAt {
			t.Errorf("found entry %d which should have been truncated (before %d)", e.LogID, ckptAt)
		}
	}
}
