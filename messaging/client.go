// Package messaging provides HTTP clients for inter-component communication.
//
// ShardClient is used by the coordinator to communicate with shard servers.
// It implements the PREPARE, COMMIT, ABORT, and EXECUTE protocols
// required for both single-shard and cross-shard (2PC) transactions.
package messaging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"ledger-service/shared/constants"
	"ledger-service/shared/models"
)

// ShardClient communicates with shard servers over HTTP.
// Used by the coordinator to send PREPARE, COMMIT, ABORT, and EXECUTE messages.
type ShardClient struct {
	httpClient *http.Client
}

// NewShardClient creates a new shard client with the given timeout.
func NewShardClient(timeout time.Duration) *ShardClient {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 128,
		MaxConnsPerHost:     128,
		IdleConnTimeout:     90 * time.Second,
	}

	return &ShardClient{
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// --- Request/Response types ---

// PrepareRequest is sent to a shard during the PREPARE phase of 2PC.
type PrepareRequest struct {
	TxnID     string                  `json:"txn_id"`
	OpType    constants.OperationType `json:"op_type"`
	AccountID string                  `json:"account_id"`
	Amount    int64                   `json:"amount"`
}

// CommitRequest is sent to a shard during the COMMIT phase of 2PC.
type CommitRequest struct {
	TxnID     string                  `json:"txn_id"`
	OpType    constants.OperationType `json:"op_type"`
	AccountID string                  `json:"account_id"`
	Amount    int64                   `json:"amount"`
}

// AbortRequest is sent to a shard during the ABORT phase of 2PC.
type AbortRequest struct {
	TxnID     string                  `json:"txn_id"`
	OpType    constants.OperationType `json:"op_type"`
	AccountID string                  `json:"account_id"`
	Amount    int64                   `json:"amount"`
}

// ExecuteRequest is sent for single-shard transaction execution.
type ExecuteRequest struct {
	Transaction models.Transaction `json:"transaction"`
}

// BalanceResponse is returned by the balance endpoint.
type BalanceResponse struct {
	AccountID string `json:"account_id"`
	Balance   int64  `json:"balance"`
	Exists    bool   `json:"exists"`
}

// --- Public API ---

// Prepare sends a PREPARE request to a shard.
func (c *ShardClient) Prepare(shardAddr, txnID string, opType constants.OperationType, accountID string, amount int64) error {
	req := PrepareRequest{
		TxnID:     txnID,
		OpType:    opType,
		AccountID: accountID,
		Amount:    amount,
	}
	return c.postJSON(shardAddr, "/prepare", req)
}

// Commit sends a COMMIT request to a shard.
func (c *ShardClient) Commit(shardAddr, txnID string, opType constants.OperationType, accountID string, amount int64) error {
	req := CommitRequest{
		TxnID:     txnID,
		OpType:    opType,
		AccountID: accountID,
		Amount:    amount,
	}
	return c.postJSON(shardAddr, "/commit", req)
}

// Abort sends an ABORT request to a shard.
func (c *ShardClient) Abort(shardAddr, txnID string, opType constants.OperationType, accountID string, amount int64) error {
	req := AbortRequest{
		TxnID:     txnID,
		OpType:    opType,
		AccountID: accountID,
		Amount:    amount,
	}
	return c.postJSON(shardAddr, "/abort", req)
}

// Execute sends a single-shard transaction to a shard for execution.
func (c *ShardClient) Execute(shardAddr string, txn models.Transaction) (models.TransactionResult, error) {
	req := ExecuteRequest{Transaction: txn}

	body, err := c.doPost(shardAddr, "/execute", req)
	if err != nil {
		return models.TransactionResult{}, err
	}

	var result models.TransactionResult
	if err := json.Unmarshal(body, &result); err != nil {
		return models.TransactionResult{}, fmt.Errorf("client: failed to decode execute response: %w", err)
	}
	return result, nil
}

// GetBalance queries the balance of an account on a shard.
func (c *ShardClient) GetBalance(shardAddr, accountID string) (int64, bool, error) {
	url := fmt.Sprintf("http://%s/balance?account=%s", shardAddr, accountID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return 0, false, fmt.Errorf("client: balance request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, false, fmt.Errorf("client: read balance response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("client: balance request returned %d: %s", resp.StatusCode, string(body))
	}

	var balResp BalanceResponse
	if err := json.Unmarshal(body, &balResp); err != nil {
		return 0, false, fmt.Errorf("client: decode balance response failed: %w", err)
	}
	return balResp.Balance, balResp.Exists, nil
}

// HealthCheck pings the shard to check if it's alive.
func (c *ShardClient) HealthCheck(shardAddr string) error {
	url := fmt.Sprintf("http://%s/health", shardAddr)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("client: health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("client: shard %s unhealthy (status %d)", shardAddr, resp.StatusCode)
	}
	return nil
}

// --- internal helpers ---

func (c *ShardClient) postJSON(shardAddr, path string, payload interface{}) error {
	_, err := c.doPost(shardAddr, path, payload)
	return err
}

func (c *ShardClient) doPost(shardAddr, path string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("client: marshal request failed: %w", err)
	}

	url := fmt.Sprintf("http://%s%s", shardAddr, path)
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("client: request to %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("client: read response failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("client: %s returned %d: %s", path, resp.StatusCode, string(body))
	}

	return body, nil
}
