package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"ledger-service/shared/models"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	baseURL := envOrDefault("BASE_URL", "http://localhost:8080")
	token := envOrDefault("AUTH_TOKEN", "")
	numAccountsStr := envOrDefault("NUM_ACCOUNTS", "1000")
	startingBalance := int64(1000)
	numWorkersStr := envOrDefault("NUM_WORKERS", "100")
	durationStr := envOrDefault("DURATION", "10s")

	numAccounts, _ := strconv.Atoi(numAccountsStr)
	numWorkers, _ := strconv.Atoi(numWorkersStr)
	duration, _ := time.ParseDuration(durationStr)

	// Shard addresses for direct seeding
	shardAddrs := map[string]string{
		"shard1": envOrDefault("SHARD1_ADDR", "shard1:8081"),
		"shard2": envOrDefault("SHARD2_ADDR", "shard2:8082"),
		"shard3": envOrDefault("SHARD3_ADDR", "shard3:8083"),
	}

	totalSystemBalance := int64(numAccounts) * startingBalance

	fmt.Println("=== Starting Seed Phase ===")

	// First, seed __bank__ account on each shard with enough balance
	bankBalance := totalSystemBalance
	for shardName, shardAddr := range shardAddrs {
		err := seedAccountDirect(shardAddr, "__bank__", bankBalance)
		if err != nil {
			log.Printf("Warning: failed to seed __bank__ on %s (%s): %v", shardName, shardAddr, err)
		} else {
			log.Printf("Seeded __bank__ on %s with balance %d", shardName, bankBalance)
		}
	}

	// Now create individual accounts via coordinator /submit
	// Each account gets startingBalance transferred from __bank__
	client := &http.Client{Timeout: 10 * time.Second}
	successCount := 0
	failCount := 0

	for i := 0; i < numAccounts; i++ {
		accountID := fmt.Sprintf("user%d", i)

		// First create the account directly on the appropriate shard
		// We'll create on all shards; only the right one will use it
		for _, shardAddr := range shardAddrs {
			_ = seedAccountDirect(shardAddr, accountID, 0)
		}

		// Transfer startingBalance from __bank__ to the user via coordinator
		txn := models.Transaction{
			TxnID:       fmt.Sprintf("seed-%s", accountID),
			Source:      "__bank__",
			Destination: accountID,
			Amount:      startingBalance,
		}

		body, _ := json.Marshal(txn)
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/submit", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			failCount++
			if i < 5 {
				log.Printf("Seed %s failed: %v", accountID, err)
			}
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			successCount++
		} else {
			failCount++
			if i < 5 {
				log.Printf("Seed %s returned status %d", accountID, resp.StatusCode)
			}
		}

		if (i+1)%100 == 0 {
			log.Printf("Seeded %d/%d accounts...", i+1, numAccounts)
		}
	}

	log.Printf("Seed phase complete: %d success, %d failed, Total System Balance: $%d",
		successCount, failCount, totalSystemBalance)

	fmt.Println("=== Starting Attack Phase ===")
	var wg sync.WaitGroup
	startTime := time.Now()
	var txnSuccess, txnErr int64
	var mu sync.Mutex

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			c := &http.Client{Timeout: 2 * time.Second}
			for time.Since(startTime) < duration {
				src := fmt.Sprintf("user%d", rand.Intn(numAccounts))
				dst := fmt.Sprintf("user%d", rand.Intn(numAccounts))
				if src == dst {
					continue
				}

				txn := models.Transaction{
					TxnID:       fmt.Sprintf("w%d-%d", workerID, time.Now().UnixNano()),
					Source:      src,
					Destination: dst,
					Amount:      int64(rand.Intn(10) + 1),
				}

				body, _ := json.Marshal(txn)
				req, _ := http.NewRequest(http.MethodPost, baseURL+"/submit", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}

				resp, err := c.Do(req)

				mu.Lock()
				if err != nil || (resp != nil && resp.StatusCode >= 400) {
					txnErr++
				} else {
					txnSuccess++
				}
				mu.Unlock()

				if resp != nil && resp.Body != nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}
		}(i)
	}

	wg.Wait()
	fmt.Printf("=== Attack Complete ===\n")
	fmt.Printf("Total duration: %s\n", duration)
	fmt.Printf("Successes: %d\n", txnSuccess)
	fmt.Printf("Errors:    %d\n", txnErr)
	fmt.Printf("Total Requests: %d\n", txnSuccess+txnErr)
	fmt.Printf("TPS:       %.2f\n", float64(txnSuccess)/duration.Seconds())
	fmt.Printf("Expected Total System Balance: $%d\n", totalSystemBalance)
}

// seedAccountDirect creates an account on a shard via POST /execute.
func seedAccountDirect(shardAddr string, accountID string, balance int64) error {
	txn := models.Transaction{
		TxnID:       fmt.Sprintf("init-%s", accountID),
		Source:      accountID,
		Destination: accountID,
		Amount:      balance,
	}

	type executeReq struct {
		Transaction models.Transaction `json:"transaction"`
	}

	body, _ := json.Marshal(executeReq{Transaction: txn})

	// For account creation, we use a direct call to the shard's create-account logic.
	// We'll use POST /execute with a special create-account pattern, or just set balance directly.
	// Actually, we use a simpler approach: POST to a custom endpoint.
	// The shard server exposes /execute for transactions; for account creation we'll
	// call the shard's /balance or use the ledger directly via a special endpoint.

	// Use a direct balance-set approach: POST /create-account
	createBody, _ := json.Marshal(map[string]interface{}{
		"account_id": accountID,
		"balance":    balance,
	})

	resp, err := http.Post(fmt.Sprintf("http://%s/create-account", shardAddr), "application/json", bytes.NewBuffer(createBody))
	if err != nil {
		return fmt.Errorf("create account request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	_ = txn
	_ = body

	if resp.StatusCode >= 400 {
		return fmt.Errorf("create account returned %d", resp.StatusCode)
	}
	return nil
}
