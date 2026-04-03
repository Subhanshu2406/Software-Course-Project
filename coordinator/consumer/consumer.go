// Package consumer receives transactions and feeds them to the router.
//
// HTTPConsumer is the initial implementation — transactions arrive via
// HTTP POST /submit. This can be extended to KafkaConsumer for Sprint 3+
// where Kafka is the input source.
//
// Transaction results are stored in memory for status polling via
// GET /status?txn_id=XXX.
package consumer

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"ledger-service/coordinator/router"
	"ledger-service/shared/constants"
	"ledger-service/shared/models"
)

// HTTPConsumer receives transactions via HTTP POST and routes them.
type HTTPConsumer struct {
	listenAddr string
	router     *router.Router
	server     *http.Server

	mu      sync.RWMutex
	results map[string]models.TransactionResult // txnID → result for status polling
}

// NewHTTPConsumer creates a consumer that accepts transactions over HTTP.
func NewHTTPConsumer(listenAddr string, txnRouter *router.Router) *HTTPConsumer {
	return &HTTPConsumer{
		listenAddr: listenAddr,
		router:     txnRouter,
		results:    make(map[string]models.TransactionResult),
	}
}

// Start begins listening for transaction submissions.
// This is non-blocking — the HTTP server runs in a background goroutine.
func (c *HTTPConsumer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/submit", c.handleSubmit)
	mux.HandleFunc("/status", c.handleStatus)

	c.server = &http.Server{
		Addr:    c.listenAddr,
		Handler: mux,
	}

	log.Printf("consumer: listening on %s", c.listenAddr)
	go func() {
		if err := c.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("consumer: server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the HTTP consumer.
func (c *HTTPConsumer) Stop() error {
	if c.server != nil {
		return c.server.Close()
	}
	return nil
}

// --- HTTP handlers ---

// handleSubmit receives a transaction, validates it, routes it, and returns the result.
func (c *HTTPConsumer) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var txn models.Transaction
	if err := json.NewDecoder(r.Body).Decode(&txn); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid body: %s", err))
		return
	}

	if err := validateTransaction(txn); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Set initial state
	txn.State = constants.StatePending

	// Route the transaction
	result, err := c.router.Route(txn)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Store result for status polling
	c.mu.Lock()
	c.results[txn.TxnID] = result
	c.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// handleStatus returns the result of a previously submitted transaction.
func (c *HTTPConsumer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	txnID := r.URL.Query().Get("txn_id")
	if txnID == "" {
		writeError(w, http.StatusBadRequest, "missing 'txn_id' query parameter")
		return
	}

	c.mu.RLock()
	result, ok := c.results[txnID]
	c.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("transaction %s not found", txnID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// --- helpers ---

// validateTransaction checks basic transaction validity.
func validateTransaction(txn models.Transaction) error {
	if txn.TxnID == "" {
		return fmt.Errorf("txn_id is required")
	}
	if txn.Source == "" {
		return fmt.Errorf("source account is required")
	}
	if txn.Destination == "" {
		return fmt.Errorf("destination account is required")
	}
	if txn.Source == txn.Destination {
		return fmt.Errorf("source and destination must be different")
	}
	if txn.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// HandleSubmitDirect exposes handleSubmit for use by an external mux.
func (c *HTTPConsumer) HandleSubmitDirect(w http.ResponseWriter, r *http.Request) {
	c.handleSubmit(w, r)
}

// HandleStatusDirect exposes handleStatus for use by an external mux.
func (c *HTTPConsumer) HandleStatusDirect(w http.ResponseWriter, r *http.Request) {
	c.handleStatus(w, r)
}
