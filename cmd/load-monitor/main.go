package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"ledger-service/coordinator/shardmap"
	monitor "ledger-service/load-monitor"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	addr := envOrDefault("MONITOR_ADDR", ":8090")
	shardMapPath := envOrDefault("SHARD_MAP_PATH", "./config/shard_map.json")
	pollIntervalStr := envOrDefault("POLL_INTERVAL", "5s")
	thresholdStr := envOrDefault("HOTSPOT_THRESHOLD", "500")

	pollInterval, err := time.ParseDuration(pollIntervalStr)
	if err != nil {
		log.Fatalf("invalid POLL_INTERVAL: %v", err)
	}

	threshold, err := strconv.Atoi(thresholdStr)
	if err != nil {
		log.Fatalf("invalid HOTSPOT_THRESHOLD: %v", err)
	}

	sm, err := shardmap.LoadShardMap(shardMapPath)
	if err != nil {
		log.Fatalf("failed to load shard map from %s: %v", shardMapPath, err)
	}

	lm := monitor.NewLoadMonitor(sm, threshold, pollInterval)
	lm.Start()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", lm.HandleHealth)
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		metrics := lm.GetMetrics()
		json.NewEncoder(w).Encode(metrics)
	})

	log.Printf("Load Monitor listening on %s (poll=%s, threshold=%d)", addr, pollInterval, threshold)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("load monitor failed: %v", err)
	}
}
