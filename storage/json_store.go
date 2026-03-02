// Package storage — JSONStore implementation.
//
// JSONStore implements the Engine interface using a JSON file for persistence.
// It uses atomic write-to-temp-then-rename for crash safety.
//
// This is intentionally simple and dependency-free, suitable for a course project.
// For production workloads, this would be replaced with BoltDB or RocksDB.
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// storeData is the on-disk representation of the storage state.
type storeData struct {
	Balances        map[string]int64 `json:"balances"`
	CheckpointLogID uint64           `json:"checkpoint_log_id"`
}

// JSONStore implements Engine using a single JSON file for persistence.
// All reads are served from memory; writes are flushed to disk atomically.
type JSONStore struct {
	mu       sync.RWMutex
	filePath string
	data     storeData
}

// NewJSONStore creates or loads a JSON-based storage engine.
// The parent directory is created if it does not exist.
func NewJSONStore(filePath string) (*JSONStore, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("storage: failed to create directory %s: %w", dir, err)
	}

	s := &JSONStore{
		filePath: filePath,
		data: storeData{
			Balances: make(map[string]int64),
		},
	}

	// Load existing data if the file exists
	if _, err := os.Stat(filePath); err == nil {
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("storage: failed to read %s: %w", filePath, err)
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &s.data); err != nil {
				return nil, fmt.Errorf("storage: failed to parse %s: %w", filePath, err)
			}
		}
		if s.data.Balances == nil {
			s.data.Balances = make(map[string]int64)
		}
	}

	return s, nil
}

func (s *JSONStore) GetBalance(accountID string) (int64, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bal, ok := s.data.Balances[accountID]
	return bal, ok, nil
}

func (s *JSONStore) SetBalance(accountID string, balance int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Balances[accountID] = balance
	return s.persist()
}

func (s *JSONStore) GetAllBalances() (map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]int64, len(s.data.Balances))
	for id, bal := range s.data.Balances {
		result[id] = bal
	}
	return result, nil
}

func (s *JSONStore) BatchSetBalances(balances map[string]int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Balances = make(map[string]int64, len(balances))
	for id, bal := range balances {
		s.data.Balances[id] = bal
	}
	return s.persist()
}

func (s *JSONStore) GetCheckpointLogID() (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.CheckpointLogID, nil
}

func (s *JSONStore) SetCheckpointLogID(logID uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.CheckpointLogID = logID
	return s.persist()
}

func (s *JSONStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persist()
}

// --- internal helpers ---

// persist writes the current state to disk atomically.
// Uses write-to-temp-then-rename to prevent corruption on crash.
func (s *JSONStore) persist() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("storage: marshal failed: %w", err)
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("storage: write temp file failed: %w", err)
	}

	if err := os.Rename(tmpPath, s.filePath); err != nil {
		return fmt.Errorf("storage: rename failed: %w", err)
	}

	return nil
}
