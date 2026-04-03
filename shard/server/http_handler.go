// HTTP handlers for the shard server.
//
// This file exposes the ShardServer operations over HTTP for coordinator→shard
// and shard→shard communication. All endpoints accept/return JSON.
//
// Endpoints:
//   POST /prepare  — 2PC prepare phase
//   POST /commit   — 2PC commit phase
//   POST /abort    — 2PC abort phase
//   POST /execute  — single-shard transaction execution
//   GET  /balance  — query account balance
//   GET  /health   — health check
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"ledger-service/shared/constants"
	"ledger-service/shared/models"
)

// --- Request/Response types for the HTTP API ---

// PrepareRequest is the JSON body for POST /prepare.
type PrepareRequest struct {
	TxnID     string                  `json:"txn_id"`
	OpType    constants.OperationType `json:"op_type"`
	AccountID string                  `json:"account_id"`
	Amount    int64                   `json:"amount"`
}

// CommitRequest is the JSON body for POST /commit.
type CommitRequest struct {
	TxnID     string                  `json:"txn_id"`
	OpType    constants.OperationType `json:"op_type"`
	AccountID string                  `json:"account_id"`
	Amount    int64                   `json:"amount"`
}

// AbortRequest is the JSON body for POST /abort.
type AbortRequest struct {
	TxnID     string                  `json:"txn_id"`
	OpType    constants.OperationType `json:"op_type"`
	AccountID string                  `json:"account_id"`
	Amount    int64                   `json:"amount"`
}

// ExecuteRequest is the JSON body for POST /execute.
type ExecuteRequest struct {
	Transaction models.Transaction `json:"transaction"`
}

// BalanceResponse is the JSON body returned by GET /balance.
type BalanceResponse struct {
	AccountID string `json:"account_id"`
	Balance   int64  `json:"balance"`
	Exists    bool   `json:"exists"`
}

// ErrorResponse is returned on errors.
type ErrorResponse struct {
	Error string `json:"error"`
}

// HealthResponse is returned by GET /health.
type HealthResponse struct {
	ShardID   string `json:"shard_id"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// HTTPHandler wraps a ShardServer and exposes its operations via HTTP.
type HTTPHandler struct {
	server *ShardServer
}

// NewHTTPHandler creates a new HTTP handler for the given shard server.
func NewHTTPHandler(server *ShardServer) *HTTPHandler {
	return &HTTPHandler{server: server}
}

// RegisterRoutes registers the shard HTTP endpoints on the given mux.
func (h *HTTPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/prepare", h.handlePrepare)
	mux.HandleFunc("/commit", h.handleCommit)
	mux.HandleFunc("/abort", h.handleAbort)
	mux.HandleFunc("/execute", h.handleExecute)
	mux.HandleFunc("/balance", h.handleBalance)
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/metrics", h.handleMetrics)
	mux.HandleFunc("/metrics/prometheus", h.handlePrometheusMetrics)
	mux.HandleFunc("/wal", h.handleWAL)
	mux.HandleFunc("/transactions", h.handleTransactions)
	mux.HandleFunc("/halt-partition", h.handleHaltPartition)
	mux.HandleFunc("/receive-partition", h.handleReceivePartition)
	mux.HandleFunc("/resume-partition", h.handleResumePartition)
	mux.HandleFunc("/promote", h.handlePromote)
	mux.HandleFunc("/log-index", h.handleLogIndex)
	mux.HandleFunc("/create-account", h.handleCreateAccount)
}

// --- Handler implementations ---

func (h *HTTPHandler) handlePrepare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req PrepareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, fmt.Sprintf("invalid body: %s", err))
		return
	}

	if err := h.server.PrepareTransaction(req.TxnID, req.OpType, req.AccountID, req.Amount); err != nil {
		writeErrorJSON(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "PREPARED"})
}

func (h *HTTPHandler) handleCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req CommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, fmt.Sprintf("invalid body: %s", err))
		return
	}

	if err := h.server.CommitTransaction(req.TxnID, req.OpType, req.AccountID, req.Amount); err != nil {
		writeErrorJSON(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "COMMITTED"})
}

func (h *HTTPHandler) handleAbort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req AbortRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, fmt.Sprintf("invalid body: %s", err))
		return
	}

	if err := h.server.AbortTransaction(req.TxnID, req.OpType, req.AccountID, req.Amount); err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ABORTED"})
}

func (h *HTTPHandler) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, fmt.Sprintf("invalid body: %s", err))
		return
	}

	result, err := h.server.ExecuteSingleShard(req.Transaction)
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *HTTPHandler) handleBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	accountID := r.URL.Query().Get("account")
	if accountID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "missing 'account' query parameter")
		return
	}

	bal, exists := h.server.GetBalance(accountID)
	writeJSON(w, http.StatusOK, BalanceResponse{
		AccountID: accountID,
		Balance:   bal,
		Exists:    exists,
	})
}

func (h *HTTPHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{
		ShardID:   h.server.ShardID(),
		Status:    "healthy",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *HTTPHandler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	metrics := h.server.GetMetrics()
	writeJSON(w, http.StatusOK, metrics)
}

func (h *HTTPHandler) handleHaltPartition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PartitionID int `json:"partition_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "invalid body")
		return
	}
	snapshot, err := h.server.HaltAndSnapshotPartition(req.PartitionID)
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"balances": snapshot,
	})
}

func (h *HTTPHandler) handleReceivePartition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PartitionID int             `json:"partition_id"`
		Balances    map[string]int64 `json:"balances"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.server.ReceivePartition(req.PartitionID, req.Balances); err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

func (h *HTTPHandler) handleResumePartition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PartitionID int `json:"partition_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.server.ResumePartition(req.PartitionID); err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("http: failed to write response: %v", err)
	}
}

func writeErrorJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

func (h *HTTPHandler) handlePromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h.server.Promote()
	writeJSON(w, http.StatusOK, map[string]string{"status": "PROMOTED", "shard_id": h.server.ShardID()})
}

func (h *HTTPHandler) handleLogIndex(w http.ResponseWriter, r *http.Request) {
	logID := h.server.WAL().NextLogID()
	writeJSON(w, http.StatusOK, map[string]uint64{"last_log_id": logID})
}

func (h *HTTPHandler) handleWAL(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	entries, total, cpID, err := h.server.GetWALEntries(limit)
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"shard_id":              h.server.ShardID(),
		"entries":               entries,
		"total_entries":         total,
		"last_checkpoint_log_id": cpID,
	})
}

func (h *HTTPHandler) handleTransactions(w http.ResponseWriter, r *http.Request) {
	limit := 25
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	txns := h.server.GetRecentTxns(limit)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"shard_id":     h.server.ShardID(),
		"transactions": txns,
		"total":        len(txns),
	})
}

func (h *HTTPHandler) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	m := h.server.GetMetrics()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP ledger_queue_depth Current transaction queue depth\n")
	fmt.Fprintf(w, "# TYPE ledger_queue_depth gauge\n")
	fmt.Fprintf(w, "ledger_queue_depth{shard=\"%s\",role=\"%s\"} %d\n", m.ShardID, m.Role, m.QueueDepth)
	fmt.Fprintf(w, "# HELP ledger_wal_index Current WAL log index\n")
	fmt.Fprintf(w, "# TYPE ledger_wal_index gauge\n")
	fmt.Fprintf(w, "ledger_wal_index{shard=\"%s\"} %d\n", m.ShardID, m.WALIndex)
	fmt.Fprintf(w, "# HELP ledger_tps Transactions per second\n")
	fmt.Fprintf(w, "# TYPE ledger_tps gauge\n")
	fmt.Fprintf(w, "ledger_tps{shard=\"%s\"} %.2f\n", m.ShardID, m.TotalQPS)
	fmt.Fprintf(w, "# HELP ledger_committed_total Total committed transactions\n")
	fmt.Fprintf(w, "# TYPE ledger_committed_total counter\n")
	fmt.Fprintf(w, "ledger_committed_total{shard=\"%s\"} %d\n", m.ShardID, m.CommittedCount)
	fmt.Fprintf(w, "# HELP ledger_aborted_total Total aborted transactions\n")
	fmt.Fprintf(w, "# TYPE ledger_aborted_total counter\n")
	fmt.Fprintf(w, "ledger_aborted_total{shard=\"%s\"} %d\n", m.ShardID, m.AbortedCount)
	fmt.Fprintf(w, "# HELP ledger_account_count Number of accounts\n")
	fmt.Fprintf(w, "# TYPE ledger_account_count gauge\n")
	fmt.Fprintf(w, "ledger_account_count{shard=\"%s\"} %d\n", m.ShardID, m.AccountCount)
	fmt.Fprintf(w, "# HELP ledger_total_balance Total balance across all accounts\n")
	fmt.Fprintf(w, "# TYPE ledger_total_balance gauge\n")
	fmt.Fprintf(w, "ledger_total_balance{shard=\"%s\"} %d\n", m.ShardID, m.TotalBalance)
	fmt.Fprintf(w, "# HELP ledger_replication_lag Replication lag in entries\n")
	fmt.Fprintf(w, "# TYPE ledger_replication_lag gauge\n")
	fmt.Fprintf(w, "ledger_replication_lag{shard=\"%s\"} %d\n", m.ShardID, m.ReplicationLag)
	fmt.Fprintf(w, "# HELP ledger_uptime_seconds Shard uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE ledger_uptime_seconds gauge\n")
	fmt.Fprintf(w, "ledger_uptime_seconds{shard=\"%s\"} %d\n", m.ShardID, m.UptimeSeconds)
}

func (h *HTTPHandler) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		AccountID string `json:"account_id"`
		Balance   int64  `json:"balance"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, fmt.Sprintf("invalid body: %s", err))
		return
	}

	if req.AccountID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "account_id is required")
		return
	}

	if err := h.server.CreateAccountWithWAL(req.AccountID, req.Balance); err != nil {
		// Account may already exist — treat as idempotent success
		writeJSON(w, http.StatusOK, map[string]string{"status": "exists", "account_id": req.AccountID})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "account_id": req.AccountID})
}
