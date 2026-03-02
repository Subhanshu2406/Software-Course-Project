// Package wal implements the Write-Ahead Log for durable shard persistence.
//
// The fundamental rule enforced here is: LOG BEFORE APPLY.
// No state change ever happens without a prior fsync of the WAL entry.
//
// This satisfies REQ-DATA-002:
//   - Each write appends to WAL before applying to ledger state
//   - All WAL writes are synchronously flushed to disk (fsync)
//   - WAL supports crash recovery through log replay
package wal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"ledger-service/shared/constants"
	"ledger-service/shared/models"
)

// WAL manages the write-ahead log for a single shard.
// It is safe for concurrent use via its internal mutex.
type WAL struct {
	mu       sync.Mutex
	filePath string
	file     *os.File
	writer   *bufio.Writer
	nextLogID uint64
}

// Open opens (or creates) a WAL file at the given path.
// On restart this file is replayed by the recovery module.
func Open(filePath string) (*WAL, error) {
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("wal: failed to open file %s: %w", filePath, err)
	}

	w := &WAL{
		filePath:  filePath,
		file:      f,
		writer:    bufio.NewWriter(f),
		nextLogID: 0,
	}

	// Count existing entries to set nextLogID correctly after a restart
	count, err := w.countEntries()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("wal: failed to count existing entries: %w", err)
	}
	w.nextLogID = uint64(count)

	return w, nil
}

// Append writes a new WAL entry and fsyncs it to stable storage.
// This is the core of the log-before-apply guarantee.
// Returns the assigned LogID for the entry.
func (w *WAL) Append(txnID string, opType constants.OperationType, accountID string, amount int64) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry := models.WALEntry{
		LogID:     w.nextLogID,
		TxnID:     txnID,
		OpType:    opType,
		AccountID: accountID,
		Amount:    amount,
		Timestamp: time.Now(),
		Committed: false, // will be marked committed after quorum ACK
	}

	if err := w.writeEntry(entry); err != nil {
		return 0, err
	}

	id := w.nextLogID
	w.nextLogID++
	return id, nil
}

// MarkCommitted writes a COMMITTED record for a given transaction.
// Called after majority-quorum acknowledgment is received.
// This is what crash recovery uses to know which entries to replay.
func (w *WAL) MarkCommitted(txnID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry := models.WALEntry{
		LogID:     w.nextLogID,
		TxnID:     txnID,
		OpType:    constants.OpCommitted,
		Timestamp: time.Now(),
		Committed: true,
	}

	if err := w.writeEntry(entry); err != nil {
		return err
	}

	w.nextLogID++
	return nil
}

// MarkAborted writes an ABORTED record for a given transaction.
// Called when a transaction is rejected or the coordinator sends ABORT.
func (w *WAL) MarkAborted(txnID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry := models.WALEntry{
		LogID:     w.nextLogID,
		TxnID:     txnID,
		OpType:    constants.OpAborted,
		Timestamp: time.Now(),
		Committed: false,
	}

	if err := w.writeEntry(entry); err != nil {
		return err
	}

	w.nextLogID++
	return nil
}

// ReadAll reads all WAL entries from the beginning of the file.
// Used by the recovery module during shard restart.
func (w *WAL) ReadAll() ([]models.WALEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Flush any buffered writes before reading
	if err := w.writer.Flush(); err != nil {
		return nil, fmt.Errorf("wal: flush before read failed: %w", err)
	}

	// Seek to beginning
	if _, err := w.file.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("wal: seek failed: %w", err)
	}

	var entries []models.WALEntry
	scanner := bufio.NewScanner(w.file)

	// Each line is one JSON-encoded WAL entry
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry models.WALEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("wal: failed to decode entry: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("wal: scan error: %w", err)
	}

	return entries, nil
}

// Close flushes and closes the WAL file.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("wal: flush on close failed: %w", err)
	}
	return w.file.Close()
}

// FilePath returns the path of the WAL file (useful for tests).
func (w *WAL) FilePath() string {
	return w.filePath
}

// NextLogID returns the next log ID that will be assigned (useful for tests).
func (w *WAL) NextLogID() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.nextLogID
}

// --- internal helpers ---

// writeEntry serializes entry as JSON, writes it as a newline-delimited record,
// then calls fsync to guarantee it reaches stable storage.
// This is the critical durability path — if fsync fails, we return an error
// rather than silently proceeding.
func (w *WAL) writeEntry(entry models.WALEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("wal: marshal failed: %w", err)
	}

	// Write JSON line
	if _, err := w.writer.Write(data); err != nil {
		return fmt.Errorf("wal: write failed: %w", err)
	}
	if err := w.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("wal: write newline failed: %w", err)
	}

	// Flush bufio buffer to OS
	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("wal: flush failed: %w", err)
	}

	// fsync to stable storage — this is the durability guarantee
	// Per REQ-DATA-002: "All WAL writes SHALL be synchronously flushed to disk (fsync)"
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("wal: fsync failed: %w", err)
	}

	return nil
}

// WriteCheckpoint records a checkpoint marker in the WAL.
// The marker indicates that all entries up to lastLogID have been persisted
// to the storage engine and don't need full replay on recovery.
func (w *WAL) WriteCheckpoint(lastLogID uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry := models.WALEntry{
		LogID:           w.nextLogID,
		OpType:          constants.OpCheckpoint,
		Timestamp:       time.Now(),
		CheckpointLogID: lastLogID,
	}

	if err := w.writeEntry(entry); err != nil {
		return err
	}

	w.nextLogID++
	return nil
}

// ReadFrom reads WAL entries starting at or after the given log ID.
// Used during recovery to skip already-checkpointed entries.
func (w *WAL) ReadFrom(startLogID uint64) ([]models.WALEntry, error) {
	all, err := w.ReadAll()
	if err != nil {
		return nil, err
	}

	var result []models.WALEntry
	for _, entry := range all {
		if entry.LogID >= startLogID {
			result = append(result, entry)
		}
	}
	return result, nil
}

// Truncate removes all WAL entries before the given log ID by rewriting the file.
// Called after a successful checkpoint to prevent unbounded WAL growth.
func (w *WAL) Truncate(beforeLogID uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Flush buffered writes
	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("wal: flush before truncate failed: %w", err)
	}

	// Read all entries
	if _, err := w.file.Seek(0, 0); err != nil {
		return fmt.Errorf("wal: seek failed: %w", err)
	}

	var keep []models.WALEntry
	scanner := bufio.NewScanner(w.file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry models.WALEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return fmt.Errorf("wal: decode failed during truncate: %w", err)
		}
		if entry.LogID >= beforeLogID {
			keep = append(keep, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("wal: scan error during truncate: %w", err)
	}

	// Close old file
	w.file.Close()

	// Write kept entries to a temp file, then rename for atomicity
	tmpPath := w.filePath + ".truncate.tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("wal: create temp file failed: %w", err)
	}
	tmpWriter := bufio.NewWriter(tmpFile)
	for _, entry := range keep {
		data, err := json.Marshal(entry)
		if err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("wal: marshal during truncate failed: %w", err)
		}
		if _, err := tmpWriter.Write(data); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("wal: write during truncate failed: %w", err)
		}
		if err := tmpWriter.WriteByte('\n'); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("wal: write newline during truncate failed: %w", err)
		}
	}
	if err := tmpWriter.Flush(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("wal: flush during truncate failed: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("wal: fsync during truncate failed: %w", err)
	}
	tmpFile.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, w.filePath); err != nil {
		return fmt.Errorf("wal: rename during truncate failed: %w", err)
	}

	// Reopen the WAL file
	f, err := os.OpenFile(w.filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("wal: reopen after truncate failed: %w", err)
	}
	w.file = f
	w.writer = bufio.NewWriter(f)

	return nil
}

// --- internal helpers ---

// countEntries reads the file to count existing entries (used on startup).
func (w *WAL) countEntries() (int, error) {
	if _, err := w.file.Seek(0, 0); err != nil {
		return 0, err
	}
	count := 0
	scanner := bufio.NewScanner(w.file)
	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			count++
		}
	}
	return count, scanner.Err()
}
