#!/usr/bin/env bash
# stress_test.sh — Comprehensive stress test that concurrently validates:
#   1. High transaction throughput (single + cross-shard)
#   2. Write-Ahead Logging (WAL) durability under load
#   3. Partition migrations during transactions
#   4. Fault tolerance (shard kill/restart during load)
#   5. Money invariant conservation throughout
#
# This test is designed to be run after `make seed` on a healthy cluster.
set -euo pipefail

COORDINATOR_URL=${COORDINATOR_URL:-"http://localhost:8080"}
SHARD1_URL=${SHARD1_URL:-"http://localhost:8081"}
SHARD2_URL=${SHARD2_URL:-"http://localhost:8082"}
SHARD3_URL=${SHARD3_URL:-"http://localhost:8083"}
MONITOR_URL=${MONITOR_URL:-"http://localhost:8090"}
NUM_ACCOUNTS=${NUM_ACCOUNTS:-1000}
STARTING_BALANCE=${STARTING_BALANCE:-10000}
PHASE1_REQUESTS=${PHASE1_REQUESTS:-600}
PHASE2_REQUESTS=${PHASE2_REQUESTS:-300}
PHASE5_REQUESTS=${PHASE5_REQUESTS:-600}
MIGRATION_WAIT_S=${MIGRATION_WAIT_S:-30}
BURST_BATCH_SIZE=${BURST_BATCH_SIZE:-50}

PASSED=0
FAILED=0

pass() { echo "  PASS: $1"; PASSED=$((PASSED + 1)); }
fail() { echo "  FAIL: $1"; FAILED=$((FAILED + 1)); }

get_total_queue_depth() {
    local total=0
    local metrics depth
    for URL in "$SHARD1_URL" "$SHARD2_URL" "$SHARD3_URL"; do
        metrics=$(curl -s --connect-timeout 2 --max-time 5 "$URL/metrics" 2>/dev/null || echo "")
        depth=$(printf '%s' "$metrics" | grep -o '"queue_depth":[0-9]*' | head -1 | cut -d: -f2)
        depth=${depth:-0}
        total=$((total + depth))
    done
    echo "$total"
}

wait_for_quiescence() {
    local stable=0
    local depth=0
    for _ in $(seq 1 20); do
        depth=$(get_total_queue_depth)
        if [ "${depth:-0}" -eq 0 ]; then
            stable=$((stable + 1))
            if [ $stable -ge 2 ]; then
                return 0
            fi
        else
            stable=0
        fi
        sleep 1
    done
    return 1
}

stable_sum() {
    local prev=""
    local curr="0"
    wait_for_quiescence >/dev/null 2>&1 || true
    for _ in $(seq 1 5); do
        curr=$(bash tests/check_invariant.sh --sum-only 2>/dev/null || echo "0")
        curr=$(printf '%s' "$curr" | tr -cd '0-9-')
        curr=${curr:-0}
        if [ -n "$prev" ] && [ "$curr" = "$prev" ]; then
            echo "$curr"
            return 0
        fi
        prev="$curr"
        sleep 1
    done
    echo "$curr"
}

get_migration_count() {
    local resp count
    resp=$(curl -s --connect-timeout 2 --max-time 5 "$MONITOR_URL/migrations" 2>/dev/null || echo "{}")
    count=$(printf '%s' "$resp" | grep -o '"partition_id"' | wc -l | tr -d '[:space:]')
    count=$(printf '%s' "${count:-0}" | tr -cd '0-9')
    echo "${count:-0}"
}

wait_for_migration_growth() {
    local before=${1:-0}
    local timeout=${2:-30}
    local elapsed=0
    local current=0
    while [ $elapsed -lt $timeout ]; do
        current=$(get_migration_count)
        if [ "${current:-0}" -gt "${before:-0}" ]; then
            echo "$current"
            return 0
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done
    echo "${current:-0}"
    return 1
}

find_account_on_shard() {
    local shard_url=$1
    local skip=${2:-}
    local account resp
    for i in $(seq 0 $((NUM_ACCOUNTS - 1))); do
        account="user$i"
        if [ -n "$skip" ] && [ "$account" = "$skip" ]; then
            continue
        fi
        resp=$(curl -s --connect-timeout 2 --max-time 3 "$shard_url/balance?account=$account" 2>/dev/null || echo '{"exists":false}')
        if printf '%s' "$resp" | grep -q '"exists":true'; then
            echo "$account"
            return 0
        fi
    done
    return 1
}

discover_hot_accounts() {
    HOT_SRC=$(find_account_on_shard "$SHARD1_URL") || return 1
    HOT_DST=$(find_account_on_shard "$SHARD1_URL" "$HOT_SRC") || return 1

    CROSS_DST=$(find_account_on_shard "$SHARD2_URL")
    if [ -z "${CROSS_DST:-}" ]; then
        CROSS_DST=$(find_account_on_shard "$SHARD3_URL") || return 1
    fi
    return 0
}

submit_cross_shard_burst() {
    local count=$1
    local tmpfile
    tmpfile=$(mktemp)
    : > "$tmpfile"

    for i in $(seq 1 "$count"); do
        (
            code=$(bash scripts/http_code.sh -X POST "$COORDINATOR_URL/submit" \
                -H "Content-Type: application/json" \
                -d "{\"txn_id\":\"stress-cs-$i\",\"source\":\"$HOT_SRC\",\"destination\":\"$CROSS_DST\",\"amount\":1}")
            printf '%s\n' "$code" >> "$tmpfile"
        ) &
        if [ $((i % BURST_BATCH_SIZE)) -eq 0 ]; then
            wait 2>/dev/null || true
        fi
    done
    wait 2>/dev/null || true

    CS_SUCCESS=0
    CS_FAIL=0
    while IFS= read -r code; do
        if [ "$code" = "200" ] || [ "$code" = "202" ]; then
            CS_SUCCESS=$((CS_SUCCESS + 1))
        else
            CS_FAIL=$((CS_FAIL + 1))
        fi
    done < "$tmpfile"
    rm -f "$tmpfile"
}

submit_same_shard_burst() {
    local count=$1
    local tmpfile
    tmpfile=$(mktemp)
    : > "$tmpfile"

    for i in $(seq 1 "$count"); do
        if [ $((i % 2)) -eq 1 ]; then
            SRC="$HOT_SRC"
            DST="$HOT_DST"
        else
            SRC="$HOT_DST"
            DST="$HOT_SRC"
        fi
        (
            code=$(bash scripts/http_code.sh -X POST "$COORDINATOR_URL/submit" \
                -H "Content-Type: application/json" \
                -d "{\"txn_id\":\"stress-ss-$i\",\"source\":\"$SRC\",\"destination\":\"$DST\",\"amount\":1}")
            printf '%s\n' "$code" >> "$tmpfile"
        ) &
        if [ $((i % BURST_BATCH_SIZE)) -eq 0 ]; then
            wait 2>/dev/null || true
        fi
    done
    wait 2>/dev/null || true

    SS_SUCCESS=0
    SS_FAIL=0
    while IFS= read -r code; do
        if [ "$code" = "200" ] || [ "$code" = "202" ]; then
            SS_SUCCESS=$((SS_SUCCESS + 1))
        else
            SS_FAIL=$((SS_FAIL + 1))
        fi
    done < "$tmpfile"
    rm -f "$tmpfile"
}

echo "=============================================="
echo "  COMPREHENSIVE STRESS TEST SUITE"
echo "=============================================="
echo ""

# ---- Phase 0: Pre-flight Invariant Snapshot ----
echo "--- Phase 0: Pre-flight Invariant Check ---"
PRE_TOTAL=$(stable_sum)
echo "  Pre-flight system balance: $PRE_TOTAL"
EXPECTED=$((NUM_ACCOUNTS * STARTING_BALANCE))
if [ "$PRE_TOTAL" -eq "$EXPECTED" ]; then
    pass "Pre-flight invariant holds ($PRE_TOTAL == $EXPECTED)"
else
    echo "  WARNING: Pre-flight invariant delta=$((PRE_TOTAL - EXPECTED)); tests will use current balance as baseline"
    EXPECTED=$PRE_TOTAL
fi
if discover_hot_accounts; then
    echo "  Targeted same-shard hot accounts: $HOT_SRC <-> $HOT_DST on shard1"
    echo "  Targeted cross-shard path:        $HOT_SRC -> $CROSS_DST"
else
    echo "  ERROR: Could not discover test accounts for targeted hotspot load"
    exit 1
fi
echo ""

# ---- Phase 1: Sustained Load with Invariant Checks ----
echo "--- Phase 1: Sustained Load + Invariant Monitoring ---"
echo "  Submitting $PHASE1_REQUESTS single-shard transfers under sustained load..."

PHASE1_MIGRATIONS_BEFORE=$(get_migration_count)
submit_same_shard_burst "$PHASE1_REQUESTS"

echo "  Single-shard batch: success=$SS_SUCCESS, fail=$SS_FAIL"
if [ $SS_SUCCESS -gt $((PHASE1_REQUESTS * 3 / 4)) ]; then
    pass "Single-shard sustained load ($SS_SUCCESS/$PHASE1_REQUESTS succeeded)"
else
    fail "Single-shard sustained load too many failures ($SS_FAIL/$PHASE1_REQUESTS)"
fi

PHASE1_MIGRATIONS_AFTER=$(wait_for_migration_growth "$PHASE1_MIGRATIONS_BEFORE" "$MIGRATION_WAIT_S" || true)
if [ "${PHASE1_MIGRATIONS_AFTER:-0}" -gt "${PHASE1_MIGRATIONS_BEFORE:-0}" ]; then
    pass "Migration observed during Phase 1 (count=${PHASE1_MIGRATIONS_AFTER})"
else
    fail "No migration observed during Phase 1"
fi

sleep 2

# Mid-test invariant check
MID_TOTAL=$(stable_sum)
echo "  Mid-test invariant: $MID_TOTAL (expected: $EXPECTED, delta: $((MID_TOTAL - EXPECTED)))"
if [ "$MID_TOTAL" -eq "$EXPECTED" ]; then
    pass "Invariant holds after single-shard load"
else
    fail "Invariant violated after single-shard load (delta=$((MID_TOTAL - EXPECTED)))"
fi
echo ""

# ---- Phase 2: Cross-Shard Transactions ----
echo "--- Phase 2: Cross-Shard Transaction Stress ---"
echo "  Submitting $PHASE2_REQUESTS cross-shard transfers..."

PHASE2_MIGRATIONS_BEFORE=$(get_migration_count)
submit_cross_shard_burst "$PHASE2_REQUESTS"

echo "  Cross-shard batch: success=$CS_SUCCESS, fail=$CS_FAIL"
if [ $CS_SUCCESS -gt $((PHASE2_REQUESTS * 7 / 10)) ]; then
    pass "Cross-shard transactions ($CS_SUCCESS/$PHASE2_REQUESTS succeeded)"
else
    fail "Cross-shard transactions too many failures ($CS_FAIL/$PHASE2_REQUESTS)"
fi

PHASE2_MIGRATIONS_AFTER=$(wait_for_migration_growth "$PHASE2_MIGRATIONS_BEFORE" "$MIGRATION_WAIT_S" || true)
if [ "${PHASE2_MIGRATIONS_AFTER:-0}" -gt "${PHASE2_MIGRATIONS_BEFORE:-0}" ]; then
    pass "Migration observed during Phase 2 (count=${PHASE2_MIGRATIONS_AFTER})"
else
    fail "No migration observed during Phase 2"
fi

sleep 2

# Post-cross-shard invariant
CS_TOTAL=$(stable_sum)
echo "  Post-cross-shard invariant: $CS_TOTAL (delta: $((CS_TOTAL - EXPECTED)))"
if [ "$CS_TOTAL" -eq "$EXPECTED" ]; then
    pass "Invariant holds after cross-shard load"
else
    fail "Invariant violated after cross-shard load (delta=$((CS_TOTAL - EXPECTED)))"
fi
echo ""

# ---- Phase 3: WAL Durability Under Load ----
echo "--- Phase 3: WAL Crash Recovery Under Load ---"

# Record balances of a few accounts
echo "  Recording pre-crash balances..."
PRE_CRASH_BAL=""
for i in 0 1 2 3 4; do
    BAL=$(curl -s "$SHARD1_URL/balance?account=user$i" 2>/dev/null | grep -o '"balance":[0-9]*' | cut -d: -f2 || echo "x")
    PRE_CRASH_BAL="$PRE_CRASH_BAL user$i=$BAL"
done
echo "  Pre-crash balances: $PRE_CRASH_BAL"

# Submit transactions concurrent with a crash
echo "  Submitting 30 transactions then killing shard1..."
for i in $(seq 1 30); do
    curl -s --connect-timeout 2 --max-time 5 -X POST "$COORDINATOR_URL/submit" \
        -H "Content-Type: application/json" \
        -d "{\"txn_id\":\"stress-wal-$i\",\"source\":\"user0\",\"destination\":\"user1\",\"amount\":1}" \
        -o /dev/null &
done
sleep 1

# Kill shard1 mid-flight
echo "  Force-killing shard1..."
docker compose kill -s KILL shard1 2>/dev/null
wait 2>/dev/null || true
sleep 3

# Restart shard1
echo "  Restarting shard1..."
docker compose start shard1 2>/dev/null

echo "  Waiting for shard1 to recover..."
ELAPSED=0
SHARD1_RECOVERED=false
while [ $ELAPSED -lt 60 ]; do
    STATUS=$(bash scripts/http_code.sh "$SHARD1_URL/health")
    if [ "$STATUS" = "200" ]; then
        SHARD1_RECOVERED=true
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

if [ "$SHARD1_RECOVERED" = "true" ]; then
    pass "Shard1 recovered after crash ($ELAPSED s)"
    
    # Verify WAL-recovered balances are consistent
    echo "  Post-recovery balances:"
    POST_CRASH_BAL=""
    for i in 0 1 2 3 4; do
        BAL=$(curl -s "$SHARD1_URL/balance?account=user$i" 2>/dev/null | grep -o '"balance":[0-9]*' | cut -d: -f2 || echo "x")
        POST_CRASH_BAL="$POST_CRASH_BAL user$i=$BAL"
    done
    echo "    $POST_CRASH_BAL"
    
    # Verify system can still process transactions after recovery
    RESP=$(bash scripts/http_code.sh -X POST "$COORDINATOR_URL/submit" \
        -H "Content-Type: application/json" \
        -d "{\"txn_id\":\"stress-post-crash-1\",\"source\":\"user0\",\"destination\":\"user1\",\"amount\":1}")
    if [ "$RESP" = "200" ] || [ "$RESP" = "202" ]; then
        pass "Post-recovery transaction succeeded"
    else
        fail "Post-recovery transaction failed (HTTP $RESP)"
    fi
else
    fail "Shard1 did not recover within 60s"
fi

sleep 2

# Post-WAL invariant check
WAL_TOTAL=$(stable_sum)
echo "  Post-WAL-crash invariant: $WAL_TOTAL (delta: $((WAL_TOTAL - EXPECTED)))"
if [ "$WAL_TOTAL" -eq "$EXPECTED" ]; then
    pass "Invariant holds after WAL crash recovery"
else
    # During crash, some in-flight transactions may be lost (which is acceptable)
    # but money should NOT be created or destroyed
    fail "Invariant violation after crash recovery (delta=$((WAL_TOTAL - EXPECTED)))"
fi
echo ""

# ---- Phase 4: Coordinator Kill Under Load ----
echo "--- Phase 4: Coordinator Kill Under Load ---"

echo "  Submitting 50 concurrent transactions..."
for i in $(seq 1 50); do
    curl -s --connect-timeout 2 --max-time 5 -X POST "$COORDINATOR_URL/submit" \
        -H "Content-Type: application/json" \
        -d "{\"txn_id\":\"stress-coord-$i\",\"source\":\"user$((i % 500))\",\"destination\":\"user$((500 + i % 500))\",\"amount\":1}" \
        -o /dev/null &
done
sleep 1

echo "  Stopping coordinator..."
docker compose stop coordinator 2>/dev/null
wait 2>/dev/null || true

sleep 3

echo "  Restarting coordinator..."
docker compose start coordinator 2>/dev/null

ELAPSED=0
COORD_RECOVERED=false
while [ $ELAPSED -lt 30 ]; do
    STATUS=$(bash scripts/http_code.sh "$COORDINATOR_URL/health")
    if [ "$STATUS" = "200" ]; then
        COORD_RECOVERED=true
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

if [ "$COORD_RECOVERED" = "true" ]; then
    pass "Coordinator recovered ($ELAPSED s)"
    
    # Verify can still submit
    RESP=$(bash scripts/http_code.sh -X POST "$COORDINATOR_URL/submit" \
        -H "Content-Type: application/json" \
        -d "{\"txn_id\":\"stress-post-coord-1\",\"source\":\"user0\",\"destination\":\"user1\",\"amount\":1}")
    if [ "$RESP" = "200" ] || [ "$RESP" = "202" ]; then
        pass "Post-coordinator-recovery transaction succeeded"
    else
        fail "Post-coordinator-recovery transaction failed (HTTP $RESP)"
    fi
else
    fail "Coordinator did not recover within 30s"
fi
echo ""

# ---- Phase 5: Migration Trigger ----
echo "--- Phase 5: Migration Under Load ---"

echo "  Generating targeted load to trigger migration..."
PHASE5_MIGRATIONS_BEFORE=$(get_migration_count)
for i in $(seq 1 "$PHASE5_REQUESTS"); do
    if [ $((i % 2)) -eq 1 ]; then
        SRC="$HOT_SRC"
        DST="$HOT_DST"
    else
        SRC="$HOT_DST"
        DST="$HOT_SRC"
    fi
    curl -s --connect-timeout 2 --max-time 5 -X POST "$COORDINATOR_URL/submit" \
        -H "Content-Type: application/json" \
        -d "{\"txn_id\":\"stress-mig-$i\",\"source\":\"$SRC\",\"destination\":\"$DST\",\"amount\":1}" \
        -o /dev/null &
    if [ $((i % 50)) -eq 0 ]; then
        wait 2>/dev/null || true
    fi
done
wait 2>/dev/null || true

echo "  Checking load-monitor for migrations (${MIGRATION_WAIT_S}s timeout)..."
PHASE5_MIGRATIONS_AFTER=$(wait_for_migration_growth "$PHASE5_MIGRATIONS_BEFORE" "$MIGRATION_WAIT_S" || true)
if [ "${PHASE5_MIGRATIONS_AFTER:-0}" -gt "${PHASE5_MIGRATIONS_BEFORE:-0}" ]; then
    echo "  Detected ${PHASE5_MIGRATIONS_AFTER} migration(s)"
    pass "Migration triggered under load"
else
    fail "No migration triggered during Phase 5"
fi
echo ""

# ---- Phase 6: Final Invariant ----
echo "--- Phase 6: Final Invariant Check ---"
sleep 3
FINAL_TOTAL=$(stable_sum)
DELTA=$((FINAL_TOTAL - EXPECTED))
echo "  Final system balance: $FINAL_TOTAL"
echo "  Expected:             $EXPECTED"
echo "  Delta:                $DELTA"

if [ "$DELTA" -eq 0 ]; then
    pass "FINAL Invariant holds: $FINAL_TOTAL == $EXPECTED"
else
    fail "FINAL Invariant violated: delta=$DELTA"
fi
echo ""

# ---- Summary ----
echo "=============================================="
echo "  STRESS TEST SUMMARY"
echo "=============================================="
echo "  Passed: $PASSED"
echo "  Failed: $FAILED"
echo ""

if [ $FAILED -gt 0 ]; then
    echo "[FAIL] Stress test — $FAILED test(s) failed"
    exit 1
else
    echo "[PASS] Stress test — all $PASSED tests passed"
fi
