package unit_test

import (
	"os"
	"path/filepath"
	"testing"

	"ledger-service/shared/constants"
	"ledger-service/shard/wal"
)

func tempWALPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.wal")
}

// TestAppendAndRead verifies that entries written to the WAL can be read back correctly.
func TestAppendAndRead(t *testing.T) {
	path := tempWALPath(t)
	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	logID, err := w.Append("txn-001", constants.OpDebit, "alice", 100)
	if err != nil {
		t.Fatalf("Append debit: %v", err)
	}
	if logID != 0 {
		t.Errorf("expected first logID=0, got %d", logID)
	}

	logID, err = w.Append("txn-001", constants.OpCredit, "bob", 100)
	if err != nil {
		t.Fatalf("Append credit: %v", err)
	}
	if logID != 1 {
		t.Errorf("expected second logID=1, got %d", logID)
	}

	entries, err := w.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].TxnID != "txn-001" {
		t.Errorf("entries[0].TxnID = %q, want %q", entries[0].TxnID, "txn-001")
	}
	if entries[0].OpType != constants.OpDebit {
		t.Errorf("entries[0].OpType = %q, want %q", entries[0].OpType, constants.OpDebit)
	}
	if entries[0].AccountID != "alice" {
		t.Errorf("entries[0].AccountID = %q, want %q", entries[0].AccountID, "alice")
	}
	if entries[0].Amount != 100 {
		t.Errorf("entries[0].Amount = %d, want 100", entries[0].Amount)
	}
}

// TestMarkCommitted verifies that a COMMITTED marker is written correctly.
func TestMarkCommitted(t *testing.T) {
	path := tempWALPath(t)
	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	_, _ = w.Append("txn-002", constants.OpDebit, "alice", 50)
	_, _ = w.Append("txn-002", constants.OpCredit, "charlie", 50)

	if err := w.MarkCommitted("txn-002"); err != nil {
		t.Fatalf("MarkCommitted: %v", err)
	}

	entries, _ := w.ReadAll()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	last := entries[len(entries)-1]
	if last.OpType != constants.OpCommitted {
		t.Errorf("last entry OpType = %q, want COMMITTED", last.OpType)
	}
	if last.TxnID != "txn-002" {
		t.Errorf("last entry TxnID = %q, want txn-002", last.TxnID)
	}
}

// TestMarkAborted verifies that an ABORTED marker is written correctly.
func TestMarkAborted(t *testing.T) {
	path := tempWALPath(t)
	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	_, _ = w.Append("txn-003", constants.OpDebit, "alice", 9999)
	_ = w.MarkAborted("txn-003")

	entries, _ := w.ReadAll()
	last := entries[len(entries)-1]
	if last.OpType != constants.OpAborted {
		t.Errorf("expected last entry = ABORTED, got %q", last.OpType)
	}
}

// TestPersistenceAcrossReopen verifies that WAL entries survive a close/reopen cycle.
// This simulates what happens when a process restarts after a clean shutdown.
func TestPersistenceAcrossReopen(t *testing.T) {
	path := tempWALPath(t)

	// Write and close
	w1, _ := wal.Open(path)
	_, _ = w1.Append("txn-persist", constants.OpDebit, "alice", 200)
	_, _ = w1.Append("txn-persist", constants.OpCredit, "bob", 200)
	_ = w1.MarkCommitted("txn-persist")
	w1.Close()

	// Reopen — simulates restart
	w2, err := wal.Open(path)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer w2.Close()

	entries, err := w2.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll after reopen: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries after reopen, got %d", len(entries))
	}

	// Verify nextLogID is set correctly after reopen (no ID collision)
	expectedNext := uint64(3)
	if w2.NextLogID() != expectedNext {
		t.Errorf("nextLogID after reopen = %d, want %d", w2.NextLogID(), expectedNext)
	}
}

// TestNextLogIDMonotonicallyIncreases verifies log IDs never repeat.
func TestNextLogIDMonotonicallyIncreases(t *testing.T) {
	path := tempWALPath(t)
	w, _ := wal.Open(path)
	defer w.Close()

	var prevID uint64
	for i := 0; i < 10; i++ {
		id, err := w.Append("txn-mono", constants.OpDebit, "x", int64(i+1))
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		if i > 0 && id <= prevID {
			t.Errorf("logID not monotone: prev=%d current=%d", prevID, id)
		}
		prevID = id
	}
}

// TestWALFileExistsAfterWrite verifies the WAL file is actually created on disk.
func TestWALFileExistsAfterWrite(t *testing.T) {
	path := tempWALPath(t)
	w, _ := wal.Open(path)
	_, _ = w.Append("txn-file", constants.OpDebit, "alice", 10)
	w.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("WAL file %s does not exist after write", path)
	}
}

// TestEmptyWALReadAll verifies ReadAll on an empty WAL returns nil, not an error.
func TestEmptyWALReadAll(t *testing.T) {
	path := tempWALPath(t)
	w, _ := wal.Open(path)
	defer w.Close()

	entries, err := w.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll on empty WAL: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries on empty WAL, got %d", len(entries))
	}
}
