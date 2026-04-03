// FollowerReceiver handles incoming WAL entries from the leader.
//
// Per REQ-REP-002: "FOLLOWERS replicate PRIMARY's WAL entries"
// Each follower "SHALL persist the received WAL entry to stable storage
// before acknowledging the Leader."
//
// The follower exposes two HTTP endpoints:
//   POST /replicate  — receive and persist a WAL entry
//   GET  /log-index  — report the latest persisted log ID
package replication

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"ledger-service/shared/models"
	"ledger-service/shard/wal"
)

// FollowerReceiver handles incoming WAL entries from the leader.
type FollowerReceiver struct {
	shardID   string
	walLog    *wal.WAL
	lastLogID uint64
}

// NewFollowerReceiver creates a receiver for WAL replication.
func NewFollowerReceiver(shardID string, walLog *wal.WAL) *FollowerReceiver {
	return &FollowerReceiver{
		shardID: shardID,
		walLog:  walLog,
	}
}

// ReceiveEntry persists a WAL entry from the leader and returns an ACK.
// The entry is fsynced to disk before returning — this is the durability
// guarantee that makes quorum-based replication safe.
func (f *FollowerReceiver) ReceiveEntry(entry models.WALEntry) error {
	_, err := f.walLog.Append(entry.TxnID, entry.OpType, entry.AccountID, entry.Amount)
	if err != nil {
		return fmt.Errorf("follower %s: failed to persist entry %d: %w", f.shardID, entry.LogID, err)
	}

	f.lastLogID = entry.LogID
	log.Printf("follower %s: persisted entry %d (txn %s, op %s)",
		f.shardID, entry.LogID, entry.TxnID, entry.OpType)
	return nil
}

// LastLogID returns the highest log ID this follower has received.
// Used during leader election to select the most up-to-date follower
// (per Algorithm 4: "Select follower with highest WAL log index").
func (f *FollowerReceiver) LastLogID() uint64 {
	return f.lastLogID
}

// RegisterHTTPHandlers adds replication endpoints to a ServeMux.
func (f *FollowerReceiver) RegisterHTTPHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/replicate", f.handleReplicate)
}

// --- HTTP handlers ---

func (f *FollowerReceiver) handleReplicate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var entry models.WALEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, fmt.Sprintf("invalid body: %s", err), http.StatusBadRequest)
		return
	}

	if err := f.ReceiveEntry(entry); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ACK"})
}

func (f *FollowerReceiver) handleLogIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]uint64{"last_log_id": f.lastLogID})
}
