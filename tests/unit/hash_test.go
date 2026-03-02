package unit_test

import (
	"fmt"
	"testing"

	"ledger-service/shared/utils"
)

// TestGetPartitionIsDeterministic verifies the same accountID always maps to the same partition.
func TestGetPartitionIsDeterministic(t *testing.T) {
	pm := utils.NewPartitionMapper(30)

	for i := 0; i < 100; i++ {
		p1 := pm.GetPartition("alice")
		p2 := pm.GetPartition("alice")
		if p1 != p2 {
			t.Errorf("non-deterministic: run %d got %d then %d", i, p1, p2)
		}
	}
}

// TestGetPartitionInRange verifies partition IDs are always in [0, numPartitions).
func TestGetPartitionInRange(t *testing.T) {
	numPartitions := 30
	pm := utils.NewPartitionMapper(numPartitions)

	for i := 0; i < 1000; i++ {
		accountID := fmt.Sprintf("account-%d", i)
		p := pm.GetPartition(accountID)
		if p < 0 || p >= numPartitions {
			t.Errorf("partition %d out of range [0, %d) for account %s", p, numPartitions, accountID)
		}
	}
}

// TestIsSamePartitionConsistency verifies two accounts with the same partition
// are correctly identified as same-partition.
func TestIsSamePartitionConsistency(t *testing.T) {
	pm := utils.NewPartitionMapper(30)

	// We can't predict which accounts share a partition, but we can verify
	// that if GetPartition says they're the same, IsSamePartition agrees.
	accounts := []string{"alice", "bob", "charlie", "dave", "eve"}
	for _, a := range accounts {
		for _, b := range accounts {
			same := pm.IsSamePartition(a, b)
			manualSame := pm.GetPartition(a) == pm.GetPartition(b)
			if same != manualSame {
				t.Errorf("IsSamePartition(%s,%s)=%v but GetPartition comparison=%v",
					a, b, same, manualSame)
			}
		}
	}
}

// TestDistributionIsReasonablyUniform verifies accounts spread across partitions.
// With 30 partitions and 3000 accounts, each partition should get roughly 100 accounts.
// We allow ±60% tolerance to account for hash variance.
func TestDistributionIsReasonablyUniform(t *testing.T) {
	numPartitions := 30
	numAccounts := 3000
	pm := utils.NewPartitionMapper(numPartitions)

	counts := make(map[int]int, numPartitions)
	for i := 0; i < numAccounts; i++ {
		p := pm.GetPartition(fmt.Sprintf("account-%d", i))
		counts[p]++
	}

	expected := numAccounts / numPartitions
	tolerance := float64(expected) * 0.60

	for p := 0; p < numPartitions; p++ {
		count := counts[p]
		if float64(count) < float64(expected)-tolerance || float64(count) > float64(expected)+tolerance {
			t.Errorf("partition %d has %d accounts, expected ~%d (±%.0f)", p, count, expected, tolerance)
		}
	}
}

// TestPartitionKeyFormat verifies the human-readable key format.
func TestPartitionKeyFormat(t *testing.T) {
	pm := utils.NewPartitionMapper(10)
	key := pm.PartitionKey("alice")
	if len(key) == 0 {
		t.Error("PartitionKey returned empty string")
	}
	// Should start with "partition-"
	expected := fmt.Sprintf("partition-%d", pm.GetPartition("alice"))
	if key != expected {
		t.Errorf("PartitionKey = %q, want %q", key, expected)
	}
}

// TestNewPartitionMapperPanicsOnZero verifies invalid config panics early.
func TestNewPartitionMapperPanicsOnZero(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for numPartitions=0, got none")
		}
	}()
	utils.NewPartitionMapper(0)
}
