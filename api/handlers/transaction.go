// Package handlers provides the HTTP handlers for the API Gateway.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"ledger-service/api/kafka"
	"ledger-service/shared/models"
)

// TransactionHandler handles incoming HTTP requests for transactions.
type TransactionHandler struct {
	producer       *kafka.Producer
	coordinatorURL string // URL of the coordinator to check status
}

// NewTransactionHandler creates a new handler.
func NewTransactionHandler(p *kafka.Producer, coordinatorURL string) *TransactionHandler {
	return &TransactionHandler{
		producer:       p,
		coordinatorURL: coordinatorURL,
	}
}

// HandleSubmit processes POST /submit.
func (h *TransactionHandler) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var txn models.Transaction
	if err := json.NewDecoder(r.Body).Decode(&txn); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if txn.TxnID == "" || txn.Source == "" || txn.Destination == "" || txn.Amount <= 0 {
		http.Error(w, "Invalid transaction fields", http.StatusBadRequest)
		return
	}

	if err := h.producer.Publish(r.Context(), txn); err != nil {
		http.Error(w, "Failed to publish transation", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"txn_id":  txn.TxnID,
		"message": "transaction accepted for processing",
	})
}

// HandleStatus processes GET /status.
func (h *TransactionHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	txnID := strings.TrimPrefix(r.URL.Path, "/status/")
	if txnID == "" || txnID == "/status" {
		http.Error(w, "Missing txn_id", http.StatusBadRequest)
		return
	}

	// For status, just forward it to the coordinator /status endpoint
	resp, err := http.Get(fmt.Sprintf("%s/status?txn_id=%s", h.coordinatorURL, txnID))
	if err != nil {
		http.Error(w, "Failed to reach coordinator", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	json.NewEncoder(w).Encode(result)
}
