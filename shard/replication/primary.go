// PrimaryReplicator handles WAL entry replication from leader to followers.
//
// Per REQ-REP-002: "PRIMARY SHALL replicate log entries to FOLLOWERS"
// Per REQ-REP-003: "PRIMARY SHALL wait for acknowledgment from the majority
// of replicas (quorum) before considering a write committed."
//
// The replicator sends WAL entries to all followers in parallel and waits
// until a quorum of acknowledgments is received before returning success.
package replication

import (
	"fmt"
	"log"
	"sync"
	"time"

	"ledger-service/shared/models"
)

// FollowerConn represents a connection to a follower shard.
type FollowerConn struct {
	FollowerID string
	Address    string
	client     *ReplicationClient
}

// PrimaryReplicator sends WAL entries to followers and waits for quorum ACK.
type PrimaryReplicator struct {
	mu         sync.RWMutex
	shardID    string
	followers  []FollowerConn
	quorumSize int // number of ACKs needed (including self)
	timeout    time.Duration
}

// NewPrimaryReplicator creates a replicator for a shard leader.
// quorumSize is the minimum number of replicas (including the leader itself)
// that must persist an entry before it is considered committed.
func NewPrimaryReplicator(shardID string, quorumSize int, timeout time.Duration) *PrimaryReplicator {
	return &PrimaryReplicator{
		shardID:    shardID,
		quorumSize: quorumSize,
		timeout:    timeout,
	}
}

// AddFollower registers a follower for replication.
func (p *PrimaryReplicator) AddFollower(followerID, address string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.followers = append(p.followers, FollowerConn{
		FollowerID: followerID,
		Address:    address,
		client:     NewReplicationClient(p.timeout),
	})
	log.Printf("replication: added follower %s at %s", followerID, address)
}

// Replicate sends a WAL entry to all followers and waits for quorum acknowledgment.
// The leader itself counts as one of the quorum, so we need (quorumSize - 1)
// follower ACKs. Returns nil if quorum is achieved.
func (p *PrimaryReplicator) Replicate(entry models.WALEntry) error {
	p.mu.RLock()
	followers := make([]FollowerConn, len(p.followers))
	copy(followers, p.followers)
	p.mu.RUnlock()

	if len(followers) == 0 {
		// No followers — leader-only mode
		if p.quorumSize <= 1 {
			return nil
		}
		return fmt.Errorf("replication: no followers available, cannot meet quorum of %d", p.quorumSize)
	}

	// We need (quorumSize - 1) follower ACKs since the leader already has the entry
	neededACKs := p.quorumSize - 1
	if neededACKs <= 0 {
		return nil
	}

	// Send to all followers in parallel
	type ackResult struct {
		followerID string
		err        error
	}
	ackChan := make(chan ackResult, len(followers))

	for _, f := range followers {
		go func(fc FollowerConn) {
			err := fc.client.SendEntry(fc.Address, entry)
			ackChan <- ackResult{followerID: fc.FollowerID, err: err}
		}(f)
	}

	// Wait for quorum
	ackCount := 0
	totalResponses := 0
	for totalResponses < len(followers) {
		r := <-ackChan
		totalResponses++

		if r.err != nil {
			log.Printf("replication: follower %s failed to ACK entry %d: %v",
				r.followerID, entry.LogID, r.err)
		} else {
			ackCount++
			log.Printf("replication: follower %s ACKed entry %d", r.followerID, entry.LogID)
		}

		// Check if we've reached quorum
		if ackCount >= neededACKs {
			return nil
		}

		// Check if quorum is still possible
		remaining := len(followers) - totalResponses
		if ackCount+remaining < neededACKs {
			return fmt.Errorf("replication: quorum impossible — need %d ACKs, have %d with %d remaining",
				neededACKs, ackCount, remaining)
		}
	}

	if ackCount >= neededACKs {
		return nil
	}
	return fmt.Errorf("replication: quorum not met — needed %d ACKs, got %d", neededACKs, ackCount)
}

// FollowerCount returns the number of registered followers.
func (p *PrimaryReplicator) FollowerCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.followers)
}
