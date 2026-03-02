// Package replication implements WAL-based leader→follower replication.
//
// This satisfies REQ-REP-001 through REQ-REP-004:
//   - REQ-REP-001: Each shard SHALL have one PRIMARY and configurable FOLLOWERS
//   - REQ-REP-002: PRIMARY SHALL replicate log entries to FOLLOWERS
//   - REQ-REP-003: Quorum-based acknowledgment before commit
//   - REQ-REP-004: Leader election on PRIMARY failure
//
// ReplicationClient handles the HTTP transport layer for sending WAL entries
// from the leader to followers.
package replication

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ledger-service/shared/models"
)

// ReplicationClient sends WAL entries from leader to followers over HTTP.
type ReplicationClient struct {
	httpClient *http.Client
}

// NewReplicationClient creates a client with the given timeout.
func NewReplicationClient(timeout time.Duration) *ReplicationClient {
	return &ReplicationClient{
		httpClient: &http.Client{Timeout: timeout},
	}
}

// SendEntry sends a WAL entry to a follower and waits for ACK.
// The follower must fsync the entry before responding (per REQ-REP-002).
func (c *ReplicationClient) SendEntry(followerAddr string, entry models.WALEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("replication-client: marshal failed: %w", err)
	}

	url := fmt.Sprintf("http://%s/replicate", followerAddr)
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("replication-client: request to %s failed: %w", followerAddr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("replication-client: follower %s returned %d: %s", followerAddr, resp.StatusCode, string(body))
	}

	return nil
}

// GetLogIndex queries a follower's last replicated log index.
// Used during leader election to select the most up-to-date follower.
func (c *ReplicationClient) GetLogIndex(followerAddr string) (uint64, error) {
	url := fmt.Sprintf("http://%s/log-index", followerAddr)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return 0, fmt.Errorf("replication-client: log-index request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("replication-client: read response failed: %w", err)
	}

	var result map[string]uint64
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("replication-client: decode failed: %w", err)
	}

	return result["last_log_id"], nil
}
