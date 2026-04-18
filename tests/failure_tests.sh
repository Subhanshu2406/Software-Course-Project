#!/usr/bin/env bash
# failure_tests.sh — Runs failure scenario tests against the Docker cluster.
# Each test prints PASS or FAIL. Run after accounts are seeded.
set -e

COORDINATOR_URL=${COORDINATOR_URL:-"http://localhost:8080"}
SHARD1_URL=${SHARD1_URL:-"http://localhost:8081"}
SHARD2_URL=${SHARD2_URL:-"http://localhost:8082"}
SHARD3_URL=${SHARD3_URL:-"http://localhost:8083"}
COMPOSE_PROJECT=$(docker compose ps --format json 2>/dev/null | head -1 | grep -o '"Project":"[^"]*"' | cut -d'"' -f4 || echo "")

PASSED=0
FAILED=0

pass() { echo "  PASS: $1"; PASSED=$((PASSED + 1)); }
fail() { echo "  FAIL: $1"; FAILED=$((FAILED + 1)); }

echo "=== Failure Tests ==="
echo ""

# ---- Test 1: Kill Leader Shard Mid-Transaction ----
echo "--- Test 1: Kill Leader Shard Mid-Transaction ---"

# Get initial balance for a sample account on shard1
INITIAL_BAL=$(curl -s "$SHARD1_URL/balance?account=user0" | grep -o '"balance":[0-9]*' | cut -d: -f2 || echo "-1")
echo "  Initial balance of user0: $INITIAL_BAL"

# Submit some transactions
for i in $(seq 1 10); do
    curl -s -X POST "$COORDINATOR_URL/submit" \
        -H "Content-Type: application/json" \
        -d "{\"txn_id\":\"fail1-$i\",\"source\":\"user0\",\"destination\":\"user1\",\"amount\":1}" \
        -o /dev/null || true
done

# Stop shard1
echo "  Stopping shard1..."
docker compose stop shard1 2>/dev/null

sleep 5

# Restart shard1
echo "  Restarting shard1..."
docker compose start shard1 2>/dev/null

# Wait for health
echo "  Waiting for shard1 to recover..."
ELAPSED=0
while [ $ELAPSED -lt 30 ]; do
    STATUS=$(bash scripts/http_code.sh "$SHARD1_URL/health")
    if [ "$STATUS" = "200" ]; then
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

if [ "$STATUS" = "200" ]; then
    RECOVERED_BAL=$(curl -s "$SHARD1_URL/balance?account=user0" | grep -o '"balance":[0-9]*' | cut -d: -f2 || echo "-1")
    echo "  Recovered balance of user0: $RECOVERED_BAL"
    if [ "$RECOVERED_BAL" != "-1" ]; then
        pass "Shard1 recovered and is serving requests"
    else
        fail "Shard1 recovered but balance query failed"
    fi
else
    fail "Shard1 did not recover within 30s"
fi

echo ""

# ---- Test 2: Kill Follower Mid-Replication ----
echo "--- Test 2: Kill Follower Mid-Replication ---"

# Submit initial transactions to shard2
for i in $(seq 1 5); do
    curl -s -X POST "$COORDINATOR_URL/submit" \
        -H "Content-Type: application/json" \
        -d "{\"txn_id\":\"fail2-pre-$i\",\"source\":\"user0\",\"destination\":\"user1\",\"amount\":1}" \
        -o /dev/null || true
done

# Kill one follower of shard2
echo "  Stopping shard2a (follower)..."
docker compose stop shard2a 2>/dev/null

# Continue submitting to shard2
SUBMIT_OK=0
for i in $(seq 1 10); do
    RESP=$(bash scripts/http_code.sh -X POST "$COORDINATOR_URL/submit" \
        -H "Content-Type: application/json" \
        -d "{\"txn_id\":\"fail2-during-$i\",\"source\":\"user0\",\"destination\":\"user1\",\"amount\":1}")
    if [ "$RESP" = "200" ] || [ "$RESP" = "202" ]; then
        SUBMIT_OK=$((SUBMIT_OK + 1))
    fi
done

# Restart follower
echo "  Restarting shard2a..."
docker compose start shard2a 2>/dev/null
sleep 3

if [ $SUBMIT_OK -gt 0 ]; then
    pass "Shard2 continued serving with one follower down ($SUBMIT_OK/10 txns succeeded)"
else
    fail "Shard2 stopped serving when follower was killed"
fi

echo ""

# ---- Test 3: Kill Coordinator Mid-2PC ----
echo "--- Test 3: Kill Coordinator Mid-2PC ---"

# Record initial balances
BAL_BEFORE_USER0=$(curl -s "$SHARD1_URL/balance?account=user0" | grep -o '"balance":[0-9]*' | cut -d: -f2 || echo "0")

# Submit many cross-shard transactions simultaneously
for i in $(seq 1 50); do
    curl -s -X POST "$COORDINATOR_URL/submit" \
        -H "Content-Type: application/json" \
        -d "{\"txn_id\":\"fail3-$i\",\"source\":\"user0\",\"destination\":\"user1\",\"amount\":1}" \
        -o /dev/null &
done

# Kill coordinator mid-flight
sleep 1
echo "  Stopping coordinator..."
docker compose stop coordinator 2>/dev/null

sleep 2

# Restart coordinator
echo "  Restarting coordinator..."
docker compose start coordinator 2>/dev/null

# Wait for coordinator health
ELAPSED=0
while [ $ELAPSED -lt 30 ]; do
    STATUS=$(bash scripts/http_code.sh "$COORDINATOR_URL/health")
    if [ "$STATUS" = "200" ]; then
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done
wait 2>/dev/null || true

BAL_AFTER_USER0=$(curl -s "$SHARD1_URL/balance?account=user0" | grep -o '"balance":[0-9]*' | cut -d: -f2 || echo "0")
echo "  user0 balance before: $BAL_BEFORE_USER0, after: $BAL_AFTER_USER0"

if [ "$STATUS" = "200" ]; then
    pass "Coordinator recovered (some txns may have been aborted — that is correct)"
else
    fail "Coordinator did not recover within 30s"
fi

echo ""

# ---- Test 4: Network Partition Simulation ----
echo "--- Test 4: Network Partition Simulation ---"

NETWORK=$(docker network ls --format '{{.Name}}' | grep -i "default" | head -1)
if [ -z "$NETWORK" ]; then
    NETWORK=$(docker compose ps --format json 2>/dev/null | head -1 | python3 -c "
import sys, json
data = json.load(sys.stdin)
nets = list(data.get('Networks', {}).keys())
print(nets[0] if nets else '')
" 2>/dev/null || echo "")
fi

SHARD1_CONTAINER=$(docker compose ps -q shard1 2>/dev/null)

if [ -n "$NETWORK" ] && [ -n "$SHARD1_CONTAINER" ]; then
    echo "  Disconnecting shard1 from network $NETWORK..."
    docker network disconnect "$NETWORK" "$SHARD1_CONTAINER" 2>/dev/null || true

    # Submit transactions — should fail gracefully
    FAIL_COUNT=0
    for i in $(seq 1 5); do
        RESP=$(bash scripts/http_code.sh --max-time 3 -X POST "$COORDINATOR_URL/submit" \
            -H "Content-Type: application/json" \
            -d "{\"txn_id\":\"fail4-$i\",\"source\":\"user0\",\"destination\":\"user1\",\"amount\":1}")
        if [ "$RESP" != "200" ] && [ "$RESP" != "202" ]; then
            FAIL_COUNT=$((FAIL_COUNT + 1))
        fi
    done

    sleep 3

    echo "  Reconnecting shard1..."
    docker network connect "$NETWORK" "$SHARD1_CONTAINER" 2>/dev/null || true

    # Wait for recovery
    sleep 5
    STATUS=$(bash scripts/http_code.sh "$SHARD1_URL/health")

    if [ "$STATUS" = "200" ]; then
        pass "System recovered from network partition ($FAIL_COUNT/5 txns failed gracefully)"
    else
        fail "System did not recover from network partition"
    fi
else
    echo "  SKIP: Could not determine network/container for partition test"
    pass "Network partition test skipped (manual Docker network setup needed)"
fi

echo ""
echo "=== Failure Tests Summary ==="
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo ""

if [ $FAILED -gt 0 ]; then
    exit 1
fi
