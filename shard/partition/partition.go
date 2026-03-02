// Package partition tracks which logical partitions a shard owns
// and their operational state.
//
// Partitions can be in two states:
//   - ACTIVE:  accepting transactions normally
//   - HALTED:  temporarily stopped for migration (Algorithm 6 step 1)
//
// This supports REQ-DATA-003 (partition migration) and enables controlled
// halt/resume during live rebalancing.
package partition

import (
	"fmt"
	"sync"
)

// State represents the current operational state of a partition on this shard.
type State string

const (
	StateActive State = "ACTIVE" // partition is accepting transactions
	StateHalted State = "HALTED" // partition is halted for migration
)

// Manager tracks which logical partitions this shard owns
// and their operational state.
type Manager struct {
	mu         sync.RWMutex
	shardID    string
	partitions map[int]State // partition ID → state
}

// NewManager creates a partition manager with the given owned partitions.
// All partitions start in ACTIVE state.
func NewManager(shardID string, partitionIDs []int) *Manager {
	pm := &Manager{
		shardID:    shardID,
		partitions: make(map[int]State, len(partitionIDs)),
	}
	for _, id := range partitionIDs {
		pm.partitions[id] = StateActive
	}
	return pm
}

// OwnsPartition returns true if this shard owns the given partition.
func (pm *Manager) OwnsPartition(partitionID int) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	_, ok := pm.partitions[partitionID]
	return ok
}

// IsActive returns true if the partition is owned and in ACTIVE state.
// Returns false if the partition is halted or not owned.
func (pm *Manager) IsActive(partitionID int) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	state, ok := pm.partitions[partitionID]
	return ok && state == StateActive
}

// AddPartition adds a new partition to this shard (e.g., after migration).
// The partition starts in ACTIVE state.
func (pm *Manager) AddPartition(partitionID int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.partitions[partitionID] = StateActive
}

// RemovePartition removes a partition from this shard (e.g., after migration away).
func (pm *Manager) RemovePartition(partitionID int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.partitions, partitionID)
}

// HaltPartition temporarily stops accepting transactions for a partition.
// Used during partition migration (Algorithm 6 step 1).
// This operation is idempotent.
func (pm *Manager) HaltPartition(partitionID int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	_, ok := pm.partitions[partitionID]
	if !ok {
		return fmt.Errorf("partition: %d not owned by shard %s", partitionID, pm.shardID)
	}
	pm.partitions[partitionID] = StateHalted
	return nil
}

// ResumePartition resumes transaction processing for a previously halted partition.
// Used after migration completes (Algorithm 6 step 6).
// This operation is idempotent.
func (pm *Manager) ResumePartition(partitionID int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	_, ok := pm.partitions[partitionID]
	if !ok {
		return fmt.Errorf("partition: %d not owned by shard %s", partitionID, pm.shardID)
	}
	pm.partitions[partitionID] = StateActive
	return nil
}

// OwnedPartitions returns a copy of all partition IDs owned by this shard.
func (pm *Manager) OwnedPartitions() []int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	ids := make([]int, 0, len(pm.partitions))
	for id := range pm.partitions {
		ids = append(ids, id)
	}
	return ids
}

// ActivePartitions returns partition IDs that are in ACTIVE state.
func (pm *Manager) ActivePartitions() []int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var ids []int
	for id, state := range pm.partitions {
		if state == StateActive {
			ids = append(ids, id)
		}
	}
	return ids
}

// ShardID returns the shard ID this manager belongs to.
func (pm *Manager) ShardID() string {
	return pm.shardID
}
