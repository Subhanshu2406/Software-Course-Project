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
	"strconv"
	"sync"
	"time"

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

	recentMu  sync.RWMutex
	recentTxns []models.TxnSummary // ring buffer, cap 1000

	startTime time.Time
}

// NewHTTPConsumer creates a consumer that accepts transactions over HTTP.
func NewHTTPConsumer(listenAddr string, txnRouter *router.Router) *HTTPConsumer {
	return &HTTPConsumer{
		listenAddr: listenAddr,
		router:     txnRouter,
		results:    make(map[string]models.TransactionResult),
		recentTxns: make([]models.TxnSummary, 0, 1000),
		startTime:  time.Now(),
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

	txnStart := time.Now()

	// Route the transaction
	result, err := c.router.Route(txn)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	latency := time.Since(txnStart).Milliseconds()

	// Store result for status polling
	c.mu.Lock()
	c.results[txn.TxnID] = result
	c.mu.Unlock()

	// Record in recent transactions
	c.addRecentTxn(models.TxnSummary{
		TxnID: txn.TxnID, Source: txn.Source, Destination: txn.Destination,
		Amount: txn.Amount, Type: "coordinator", State: result.State,
		LatencyMs: latency, Timestamp: time.Now().UTC(),
	})

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

// HandleTransferDirect handles synchronous POST /transfer requests.
func (c *HTTPConsumer) HandleTransferDirect(w http.ResponseWriter, r *http.Request) {
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

	txn.State = constants.StatePending
	txnStart := time.Now()

	result, err := c.router.Route(txn)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	latency := time.Since(txnStart).Milliseconds()

	c.mu.Lock()
	c.results[txn.TxnID] = result
	c.mu.Unlock()

	c.addRecentTxn(models.TxnSummary{
		TxnID: txn.TxnID, Source: txn.Source, Destination: txn.Destination,
		Amount: txn.Amount, Type: "transfer", State: result.State,
		LatencyMs: latency, Timestamp: time.Now().UTC(),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"txn_id":     txn.TxnID,
		"state":      result.State,
		"message":    result.Message,
		"latency_ms": latency,
	})
}

// HandleTransactionsDirect handles GET /transactions for coordinator-level history.
func (c *HTTPConsumer) HandleTransactionsDirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	txns := c.getRecentTxns(limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"transactions": txns,
		"total":        len(txns),
	})
}

// handleMetrics returns coordinator metrics in JSON format (compatible with frontend expectations).
func (c *HTTPConsumer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	c.mu.RLock()
	totalResults := len(c.results)
	c.mu.RUnlock()

	c.recentMu.RLock()
	committed, aborted := 0, 0
	for _, t := range c.recentTxns {
		if t.State == constants.StateCommitted {
			committed++
		} else if t.State == constants.StateAborted {
			aborted++
		}
	}
	c.recentMu.RUnlock()

	uptime := int64(time.Since(c.startTime).Seconds())
	tps := float64(totalResults) / max(float64(uptime), 1)

	metrics := map[string]interface{}{
		"shard_id":         "coordinator",
		"role":             "coordinator",
		"cpu_usage":        0.0,
		"total_qps":        tps,
		"queue_depth":      0,
		"replication_lag":  0,
		"wal_index":        0,
		"last_checkpoint_log_id": 0,
		"follower_count":   0,
		"account_count":    0,
		"total_balance":    0,
		"uptime_seconds":   uptime,
		"committed_count":  committed,
		"aborted_count":    aborted,
		"prepared_count":   0,
		"tps":              tps,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(metrics)
}

// HandleMetricsDirect exposes handleMetrics for use by an external mux.
func (c *HTTPConsumer) HandleMetricsDirect(w http.ResponseWriter, r *http.Request) {
	c.handleMetrics(w, r)
}

// HandlePrometheusMetrics returns coordinator metrics in Prometheus text format.
func (c *HTTPConsumer) HandlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	c.recentMu.RLock()
	total := len(c.recentTxns)
	var committed, aborted int
	for _, t := range c.recentTxns {
		if t.State == constants.StateCommitted {
			committed++
		} else if t.State == constants.StateAborted {
			aborted++
		}
	}
	c.recentMu.RUnlock()

	uptime := int64(time.Since(c.startTime).Seconds())
	tps := float64(total) / max(float64(uptime), 1)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP coordinator_tps Transactions per second\n")
	fmt.Fprintf(w, "# TYPE coordinator_tps gauge\n")
	fmt.Fprintf(w, "coordinator_tps %.2f\n", tps)
	fmt.Fprintf(w, "# HELP coordinator_committed_total Total committed\n")
	fmt.Fprintf(w, "# TYPE coordinator_committed_total counter\n")
	fmt.Fprintf(w, "coordinator_committed_total %d\n", committed)
	fmt.Fprintf(w, "# HELP coordinator_aborted_total Total aborted\n")
	fmt.Fprintf(w, "# TYPE coordinator_aborted_total counter\n")
	fmt.Fprintf(w, "coordinator_aborted_total %d\n", aborted)
	fmt.Fprintf(w, "# HELP coordinator_uptime_seconds Uptime\n")
	fmt.Fprintf(w, "# TYPE coordinator_uptime_seconds gauge\n")
	fmt.Fprintf(w, "coordinator_uptime_seconds %d\n", uptime)
}

func (c *HTTPConsumer) addRecentTxn(t models.TxnSummary) {
	c.recentMu.Lock()
	defer c.recentMu.Unlock()
	if len(c.recentTxns) >= 1000 {
		c.recentTxns = c.recentTxns[1:]
	}
	c.recentTxns = append(c.recentTxns, t)
}

func (c *HTTPConsumer) getRecentTxns(limit int) []models.TxnSummary {
	c.recentMu.RLock()
	defer c.recentMu.RUnlock()
	n := len(c.recentTxns)
	if limit > n {
		limit = n
	}
	out := make([]models.TxnSummary, limit)
	copy(out, c.recentTxns[n-limit:])
	return out
}
