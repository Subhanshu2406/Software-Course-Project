#!/usr/bin/env bash
# wal_replay_test.sh — Tests WAL crash recovery by force-killing a shard.
set -e

SHARD1_URL=${SHARD1_URL:-"http://localhost:8081"}
COORDINATOR_URL=${COORDINATOR_URL:-"http://localhost:8080"}

echo "=== WAL Replay / Recovery Test ==="

# Step 1: Ensure accounts exist on shard1
echo "Step 1: Creating test accounts on shard1..."
for i in $(seq 100 110); do
    curl -s -X POST "$SHARD1_URL/create-account" \
        -H "Content-Type: application/json" \
        -d "{\"account_id\":\"wal_user$i\",\"balance\":1000}" \
        -o /dev/null || true
done
echo "  Accounts created."

# Step 2: Submit transfers and record expected balances
echo "Step 2: Submitting 50 single-shard transfers..."
for i in $(seq 1 50); do
    SRC=$((100 + (i % 5)))
    DST=$((105 + (i % 5)))
    curl -s -X POST "$COORDINATOR_URL/submit" \
        -H "Content-Type: application/json" \
        -d "{\"txn_id\":\"wal-test-$i\",\"source\":\"wal_user$SRC\",\"destination\":\"wal_user$DST\",\"amount\":1}" \
        -o /dev/null || true
done
sleep 2

# Record balances after transfers
echo "Step 3: Recording post-transfer balances..."
declare -A EXPECTED_BALANCES
BALANCE_RECORD=""
for i in $(seq 100 110); do
    BAL=$(curl -s "$SHARD1_URL/balance?account=wal_user$i" | grep -o '"balance":[0-9]*' | cut -d: -f2 || echo "0")
    BALANCE_RECORD="$BALANCE_RECORD wal_user$i=$BAL"
done
echo "  Post-transfer balances: $BALANCE_RECORD"

# Step 4: Force-kill shard1 (SIGKILL — simulates crash)
echo "Step 4: Force-killing shard1..."
docker compose kill -s KILL shard1 2>/dev/null

sleep 3

# Step 5: Restart shard1
echo "Step 5: Restarting shard1..."
docker compose start shard1 2>/dev/null

# Wait for health
echo "  Waiting for shard1 to recover..."
ELAPSED=0
RECOVERED=false
while [ $ELAPSED -lt 30 ]; do
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$SHARD1_URL/health" 2>/dev/null || echo "000")
    if [ "$STATUS" = "200" ]; then
        RECOVERED=true
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

if [ "$RECOVERED" = "false" ]; then
    echo "FAIL: Shard1 did not recover within 30s"
    exit 1
fi

echo "  Shard1 recovered."

# Step 6: Compare balances
echo "Step 6: Verifying recovered balances..."
MISMATCH=0
RECOVERED_RECORD=""
for i in $(seq 100 110); do
    RECOVERED_BAL=$(curl -s "$SHARD1_URL/balance?account=wal_user$i" | grep -o '"balance":[0-9]*' | cut -d: -f2 || echo "-1")
    RECOVERED_RECORD="$RECOVERED_RECORD wal_user$i=$RECOVERED_BAL"
    
    # Find expected balance from our record
    EXPECTED=$(echo "$BALANCE_RECORD" | grep -o "wal_user$i=[0-9]*" | cut -d= -f2 || echo "0")
    
    if [ "$RECOVERED_BAL" != "$EXPECTED" ]; then
        echo "  MISMATCH: wal_user$i expected=$EXPECTED, got=$RECOVERED_BAL"
        MISMATCH=$((MISMATCH + 1))
    fi
done

echo "  Recovered balances: $RECOVERED_RECORD"

if [ $MISMATCH -eq 0 ]; then
    echo "PASS: All balances match after crash recovery"
else
    echo "FAIL: $MISMATCH balance mismatches after crash recovery"
    exit 1
fi
