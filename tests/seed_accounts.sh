#!/usr/bin/env bash
# seed_accounts.sh — Seeds NUM_ACCOUNTS accounts directly on the correct shard.
# Each account is created ONLY on the shard it hashes to (via SHA-256 partition mapping),
# with its starting balance pre-set. No coordinator round-trips needed.
set -e

echo "=== Direct Seed ==="
echo "Running Go seeder (bypasses coordinator, creates accounts on correct shards)..."
echo ""

# The Go seeder reads these env vars:
#   NUM_ACCOUNTS (default 1000)
#   STARTING_BALANCE (default 10000)
#   SHARD_MAP_PATH (default ./config/shard_map.json)
#   SHARD_HOST (default localhost — set to empty string inside Docker)
go run ./cmd/seed
