package main

import (
	"fmt"
	"log"
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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	numAccounts, _ := strconv.Atoi(envOrDefault("NUM_ACCOUNTS", "1000"))
	startingBalance, _ := strconv.Atoi(envOrDefault("STARTING_BALANCE", "10000"))
	shardMapPath := envOrDefault("SHARD_MAP_PATH", "./config/shard_map.json")
	hostOverride := envOrDefault("SHARD_HOST", "localhost") // for running outside Docker
	batchSize := 50

	sm, err := shardmap.LoadShardMap(shardMapPath)
	if err != nil {
		log.Fatalf("Failed to load shard map: %v", err)
	}

	totalPartitions := sm.PartitionCount()
	if totalPartitions == 0 {
		log.Fatalf("Shard map has no partitions")
	}

	mapper := utils.NewPartitionMapper(totalPartitions)
	client := &http.Client{Timeout: 10 * time.Second}

	expectedTotal := numAccounts * startingBalance
	fmt.Println("=== Direct Seed (bypass coordinator) ===")
	fmt.Printf("Accounts:  %d\n", numAccounts)
	fmt.Printf("Balance:   %d per account\n", startingBalance)
	fmt.Printf("Expected:  %d total\n", expectedTotal)
	fmt.Printf("Partitions: %d\n", totalPartitions)
	fmt.Println()

	var success, fail int64

	for batch := 0; batch < numAccounts; batch += batchSize {
		end := batch + batchSize
		if end > numAccounts {
			end = numAccounts
		}

		var wg sync.WaitGroup
		for i := batch; i < end; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				accountID := fmt.Sprintf("user%d", idx)

				shardInfo, err := sm.GetShardForAccount(accountID, mapper)
				if err != nil {
					log.Printf("ERROR: no shard for %s: %v", accountID, err)
					atomic.AddInt64(&fail, 1)
					return
				}

				// Replace Docker-internal hostname with host override
				addr := shardInfo.Address
				if hostOverride != "" {
					parts := strings.SplitN(addr, ":", 2)
					if len(parts) == 2 {
						addr = hostOverride + ":" + parts[1]
					}
				}

				url := fmt.Sprintf("http://%s/create-account", addr)
				body := fmt.Sprintf(`{"account_id":"%s","balance":%d}`, accountID, startingBalance)

				resp, err := client.Post(url, "application/json", strings.NewReader(body))
				if err != nil {
					atomic.AddInt64(&fail, 1)
					return
				}
				resp.Body.Close()

				if resp.StatusCode < 300 {
					atomic.AddInt64(&success, 1)
				} else {
					atomic.AddInt64(&fail, 1)
				}
			}(i)
		}
		wg.Wait()
		fmt.Printf("  Seeded %d/%d accounts...\n", end, numAccounts)
	}

	fmt.Println()
	fmt.Println("=== Seed Complete ===")
	fmt.Printf("Success: %d\n", success)
	fmt.Printf("Failed:  %d\n", fail)
	fmt.Printf("Total balance: $%d\n", success*int64(startingBalance))

	if fail > 0 {
		os.Exit(1)
	}
}
