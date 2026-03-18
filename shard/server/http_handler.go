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
	mux.HandleFunc("/halt-partition", h.handleHaltPartition)
	mux.HandleFunc("/receive-partition", h.handleReceivePartition)
	mux.HandleFunc("/resume-partition", h.handleResumePartition)
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
