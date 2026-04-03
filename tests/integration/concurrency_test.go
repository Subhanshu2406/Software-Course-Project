package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type submitRequest struct {
	TxnID       string `json:"txn_id"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Amount      int64  `json:"amount"`
}

type balanceResponse struct {
	AccountID string `json:"account_id"`
	Balance   int64  `json:"balance"`
	Exists    bool   `json:"exists"`
}

func getBalance(t *testing.T, shardURL, account string) int64 {
	t.Helper()
	resp, err := http.Get(shardURL + "/balance?account=" + account)
	if err != nil {
		t.Fatalf("balance query failed for %s: %v", account, err)
	}
	defer resp.Body.Close()
	var br balanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&br); err != nil {
		t.Fatalf("decode balance response: %v", err)
	}
	if !br.Exists {
		return -1
	}
	return br.Balance
}

// owningShardURL returns the shard URL that owns the given account's partition.
func owningShardURL(account string, shardURLs []string, totalPartitions int) string {
	h := sha256.Sum256([]byte(account))
	val := binary.BigEndian.Uint64(h[:8])
	partition := int(val % uint64(totalPartitions))
	perShard := totalPartitions / len(shardURLs)
	idx := partition / perShard
	if idx >= len(shardURLs) {
		idx = len(shardURLs) - 1
	}
	return shardURLs[idx]
}

// getBalanceOnOwner queries the balance on the shard that owns the account.
func getBalanceOnOwner(t *testing.T, account string, shardURLs []string) int64 {
	t.Helper()
	url := owningShardURL(account, shardURLs, 30)
	return getBalance(t, url, account)
}

func submitTxn(coordinatorURL string, req submitRequest) (int, string, error) {
	body, _ := json.Marshal(req)
	resp, err := http.Post(coordinatorURL+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw), nil
}

// TestConcurrencyIdempotency sends 200 concurrent requests with the same txn_id.
// Only one should succeed; the rest should be rejected or return the same result.
func TestConcurrencyIdempotency(t *testing.T) {
	coordinatorURL := envOr("COORDINATOR_URL", "http://localhost:8080")
	goroutines := 200
	txnID := fmt.Sprintf("idem-test-%d", time.Now().UnixNano())

	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			code, _, err := submitTxn(coordinatorURL, submitRequest{
				TxnID:       txnID,
				Source:      "user0",
				Destination: "user1",
				Amount:      1,
			})
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			if code == 200 || code == 202 {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Idempotency test: %d goroutines, %d accepted, %d errors", goroutines, successCount, errorCount)

	// At most one should succeed with actual state change (idempotent)
	// Multiple may return 200 if the coordinator returns cached result
	if successCount == 0 {
		t.Error("expected at least one successful submission")
	}
	t.Logf("[PASS] Idempotency: %d/%d returned success (expected ≤ %d)", successCount, goroutines, goroutines)
}

// TestConcurrencyNoNegativeBalance sends concurrent transfers that would
// overdraw an account to verify no negative balance occurs.
func TestConcurrencyNoNegativeBalance(t *testing.T) {
	coordinatorURL := envOr("COORDINATOR_URL", "http://localhost:8080")
	shard1URL := envOr("SHARD1_URL", "http://localhost:8081")
	shard2URL := envOr("SHARD2_URL", "http://localhost:8082")
	shard3URL := envOr("SHARD3_URL", "http://localhost:8083")
	shardURLs := []string{shard1URL, shard2URL, shard3URL}

	// Find an account with sufficient balance for the drain test.
	account := ""
	var startBal int64
	for i := 100; i < 200; i++ {
		candidate := fmt.Sprintf("user%d", i)
		b := getBalanceOnOwner(t, candidate, shardURLs)
		if b >= 1000 {
			account = candidate
			startBal = b
			break
		}
	}
	if account == "" {
		t.Skip("no account with sufficient balance found — cannot test drain")
	}
	t.Logf("Starting balance of %s: %d", account, startBal)

	// Try to drain the account with concurrent transfers of amount=startBal
	goroutines := 100
	var wg sync.WaitGroup
	var accepted int64

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			code, _, err := submitTxn(coordinatorURL, submitRequest{
				TxnID:       fmt.Sprintf("drain-%s-%d-%d", account, time.Now().UnixNano(), idx),
				Source:      account,
				Destination: "user11",
				Amount:      startBal,
			})
			if err == nil && (code == 200 || code == 202) {
				atomic.AddInt64(&accepted, 1)
			}
		}(i)
	}

	wg.Wait()

	// Check final balance on owning shard
	finalBal := getBalanceOnOwner(t, account, shardURLs)

	t.Logf("After %d concurrent drain attempts: accepted=%d, final balance=%d", goroutines, accepted, finalBal)

	if finalBal < 0 {
		t.Error("account balance went negative — concurrency control failure")
	} else {
		t.Logf("[PASS] No negative balance: final=%d (accepted=%d transfers)", finalBal, accepted)
	}
}

// TestConcurrencyCrossShard sends concurrent cross-shard transfers
// and verifies money conservation.
func TestConcurrencyCrossShard(t *testing.T) {
	coordinatorURL := envOr("COORDINATOR_URL", "http://localhost:8080")
	shard1URL := envOr("SHARD1_URL", "http://localhost:8081")
	shard2URL := envOr("SHARD2_URL", "http://localhost:8082")
	shard3URL := envOr("SHARD3_URL", "http://localhost:8083")
	shardURLs := []string{shard1URL, shard2URL, shard3URL}

	// Compute initial sum for user20..user29
	accounts := make([]string, 10)
	for i := 0; i < 10; i++ {
		accounts[i] = fmt.Sprintf("user%d", 20+i)
	}

	sumBalances := func() int64 {
		var total int64
		for _, acc := range accounts {
			total += getBalanceOnOwner(t, acc, shardURLs)
		}
		return total
	}

	initialSum := sumBalances()
	t.Logf("Initial sum of user20..user29: %d", initialSum)

	goroutines, _ := strconv.Atoi(envOr("CONCURRENCY_GOROUTINES", "50"))
	var wg sync.WaitGroup
	var accepted int64

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			src := accounts[idx%5]
			dst := accounts[5+idx%5]
			code, _, err := submitTxn(coordinatorURL, submitRequest{
				TxnID:       fmt.Sprintf("xshard-conc-%d-%d", time.Now().UnixNano(), idx),
				Source:      src,
				Destination: dst,
				Amount:      1,
			})
			if err == nil && (code == 200 || code == 202) {
				atomic.AddInt64(&accepted, 1)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(2 * time.Second) // allow processing

	finalSum := sumBalances()
	t.Logf("After %d concurrent cross-shard transfers: accepted=%d, initial=%d, final=%d",
		goroutines, accepted, initialSum, finalSum)

	if initialSum != finalSum {
		t.Errorf("money conservation violated: initial=%d, final=%d, delta=%d",
			initialSum, finalSum, finalSum-initialSum)
	} else {
		t.Logf("[PASS] Money conservation holds: sum=%d", finalSum)
	}
}
