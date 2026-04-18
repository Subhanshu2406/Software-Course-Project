package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ledger-service/coordinator/shardmap"
	"ledger-service/shared/utils"
)

type seedJob struct {
	accountID string
	url       string
	body      string
}

type seedResult struct {
	accountID string
	err       error
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	v := envOrDefault(key, "")
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func main() {
	numAccounts := envIntOrDefault("NUM_ACCOUNTS", 1000)
	startingBalance := envIntOrDefault("STARTING_BALANCE", 10000)
	shardMapPath := envOrDefault("SHARD_MAP_PATH", "./config/shard_map.json")
	hostOverride := envOrDefault("SHARD_HOST", "localhost") // for running outside Docker
	workerCount := max(envIntOrDefault("SEED_WORKERS", 12), 1)
	maxRetries := max(envIntOrDefault("SEED_RETRIES", 6), 1)
	requestsPerSecond := envIntOrDefault("SEED_RPS", 60)

	sm, err := shardmap.LoadShardMap(shardMapPath)
	if err != nil {
		log.Fatalf("Failed to load shard map: %v", err)
	}

	totalPartitions := sm.PartitionCount()
	if totalPartitions == 0 {
		log.Fatalf("Shard map has no partitions")
	}

	mapper := utils.NewPartitionMapper(totalPartitions)
	client := newSeedHTTPClient(15 * time.Second)

	expectedTotal := numAccounts * startingBalance
	fmt.Println("=== Direct Seed (bypass coordinator) ===")
	fmt.Printf("Accounts:   %d\n", numAccounts)
	fmt.Printf("Balance:    %d per account\n", startingBalance)
	fmt.Printf("Expected:   %d total\n", expectedTotal)
	fmt.Printf("Partitions: %d\n", totalPartitions)
	fmt.Printf("Workers:    %d\n", workerCount)
	fmt.Printf("Retries:    %d\n", maxRetries)
	if requestsPerSecond > 0 {
		fmt.Printf("Rate limit: %d req/s\n", requestsPerSecond)
	} else {
		fmt.Println("Rate limit: disabled")
	}
	fmt.Println()

	jobs := make(chan seedJob, workerCount*2)
	results := make(chan seedResult, workerCount*2)
	var processed atomic.Int64

	var limiter <-chan time.Time
	if requestsPerSecond > 0 {
		ticker := time.NewTicker(time.Second / time.Duration(requestsPerSecond))
		defer ticker.Stop()
		limiter = ticker.C
	}

	var workers sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				err := seedAccount(client, job, limiter, maxRetries)
				processed.Add(1)
				results <- seedResult{accountID: job.accountID, err: err}
			}
		}()
	}

	go func() {
		for i := 0; i < numAccounts; i++ {
			accountID := fmt.Sprintf("user%d", i)

			shardInfo, err := sm.GetShardForAccount(accountID, mapper)
			if err != nil {
				results <- seedResult{accountID: accountID, err: fmt.Errorf("no shard for %s: %w", accountID, err)}
				continue
			}

			addr := rewriteShardAddr(shardInfo.Address, hostOverride)
			jobs <- seedJob{
				accountID: accountID,
				url:       fmt.Sprintf("http://%s/create-account", addr),
				body:      fmt.Sprintf(`{"account_id":"%s","balance":%d}`, accountID, startingBalance),
			}
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()

	var success, fail int64
	for result := range results {
		if result.err != nil {
			fail++
			log.Printf("seed: %s failed: %v", result.accountID, result.err)
		} else {
			success++
		}

		done := success + fail
		if done%50 == 0 || done == int64(numAccounts) {
			fmt.Printf("  Seeded %d/%d accounts...\n", done, numAccounts)
		}
	}

	fmt.Println()
	fmt.Println("=== Seed Complete ===")
	fmt.Printf("Success: %d\n", success)
	fmt.Printf("Failed:  %d\n", fail)
	fmt.Printf("Processed: %d\n", processed.Load())
	fmt.Printf("Total balance: $%d\n", success*int64(startingBalance))

	if fail > 0 {
		os.Exit(1)
	}
}

func seedAccount(client *http.Client, job seedJob, limiter <-chan time.Time, maxRetries int) error {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if limiter != nil {
			<-limiter
		}

		req, err := http.NewRequest(http.MethodPost, job.url, strings.NewReader(job.body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			switch {
			case resp.StatusCode < http.StatusMultipleChoices:
				return nil
			case !shouldRetry(resp.StatusCode):
				return fmt.Errorf("status %d", resp.StatusCode)
			default:
				lastErr = fmt.Errorf("retryable status %d", resp.StatusCode)
			}
		} else {
			lastErr = err
		}

		if attempt < maxRetries {
			time.Sleep(backoff(attempt))
		}
	}

	return lastErr
}

func shouldRetry(statusCode int) bool {
	return statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusTooManyRequests ||
		statusCode >= http.StatusInternalServerError
}

func backoff(attempt int) time.Duration {
	base := 150 * time.Millisecond
	delay := base * time.Duration(1<<(attempt-1))
	if delay > 2*time.Second {
		delay = 2 * time.Second
	}
	jitter := time.Duration(rand.Intn(125)) * time.Millisecond
	return delay + jitter
}

func rewriteShardAddr(addr, hostOverride string) string {
	if hostOverride == "" {
		return addr
	}
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) != 2 {
		return addr
	}
	return hostOverride + ":" + parts[1]
}

func newSeedHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        128,
		MaxIdleConnsPerHost: 64,
		MaxConnsPerHost:     64,
		IdleConnTimeout:     90 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
