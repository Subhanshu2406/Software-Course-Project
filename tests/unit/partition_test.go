package unit_test

import (
	"testing"

	"ledger-service/shard/partition"
)

// TestNewManagerActiveState verifies all partitions start ACTIVE.
func TestNewManagerActiveState(t *testing.T) {
	pm := partition.NewManager("shard-1", []int{0, 1, 2})

	for _, id := range []int{0, 1, 2} {
		if !pm.OwnsPartition(id) {
			t.Errorf("partition %d should be owned", id)
		}
		if !pm.IsActive(id) {
			t.Errorf("partition %d should be ACTIVE", id)
		}
	}
}

// TestOwnsPartitionFalseForUnowned verifies non-owned partitions.
func TestOwnsPartitionFalseForUnowned(t *testing.T) {
	pm := partition.NewManager("shard-1", []int{0, 1})

	if pm.OwnsPartition(99) {
		t.Error("partition 99 should not be owned")
	}
	if pm.IsActive(99) {
		t.Error("partition 99 should not be active")
	}
}

// TestHaltAndResumePartition verifies lifecycle transitions.
func TestHaltAndResumePartition(t *testing.T) {
	pm := partition.NewManager("shard-1", []int{0, 1, 2})

	// Halt partition 1
	if err := pm.HaltPartition(1); err != nil {
		t.Fatalf("HaltPartition: %v", err)
	}

	// Partition 1 should be owned but not active
	if !pm.OwnsPartition(1) {
		t.Error("halted partition should still be owned")
	}
	if pm.IsActive(1) {
		t.Error("halted partition should not be active")
	}

	// Other partitions unaffected
	if !pm.IsActive(0) {
		t.Error("partition 0 should still be active")
	}

	// Resume partition 1
	if err := pm.ResumePartition(1); err != nil {
		t.Fatalf("ResumePartition: %v", err)
	}
	if !pm.IsActive(1) {
		t.Error("resumed partition should be active")
	}
}

// TestHaltNonOwnedPartitionErrors verifies error handling.
func TestHaltNonOwnedPartitionErrors(t *testing.T) {
	pm := partition.NewManager("shard-1", []int{0})

	if err := pm.HaltPartition(99); err == nil {
		t.Error("expected error halting non-owned partition")
	}
}

// TestAddAndRemovePartition verifies dynamic partition management.
func TestAddAndRemovePartition(t *testing.T) {
	pm := partition.NewManager("shard-1", []int{0, 1})

	// Add partition 5
	pm.AddPartition(5)
	if !pm.IsActive(5) {
		t.Error("newly added partition should be active")
	}

	// Remove partition 0
	pm.RemovePartition(0)
	if pm.OwnsPartition(0) {
		t.Error("removed partition should not be owned")
	}
}

// TestOwnedPartitions returns all owned IDs.
func TestOwnedPartitions(t *testing.T) {
	pm := partition.NewManager("shard-1", []int{3, 5, 7})

	owned := pm.OwnedPartitions()
	if len(owned) != 3 {
		t.Errorf("owned count = %d, want 3", len(owned))
	}
}

// TestActivePartitionsExcludesHalted verifies filtering.
func TestActivePartitionsExcludesHalted(t *testing.T) {
	pm := partition.NewManager("shard-1", []int{0, 1, 2})
	_ = pm.HaltPartition(1)

	active := pm.ActivePartitions()
	if len(active) != 2 {
		t.Errorf("active count = %d, want 2", len(active))
	}

	for _, id := range active {
		if id == 1 {
			t.Error("halted partition should not be in active list")
		}
	}
}

// TestHaltIdempotent verifies that halting twice is safe.
func TestHaltIdempotent(t *testing.T) {
	pm := partition.NewManager("shard-1", []int{0})

	_ = pm.HaltPartition(0)
	if err := pm.HaltPartition(0); err != nil {
		t.Errorf("second halt should be idempotent, got: %v", err)
	}
}

// TestShardID returns correct ID.
func TestShardID(t *testing.T) {
	pm := partition.NewManager("shard-42", []int{0})
	if pm.ShardID() != "shard-42" {
		t.Errorf("ShardID = %s, want shard-42", pm.ShardID())
	}
}
