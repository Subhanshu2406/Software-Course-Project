#!/usr/bin/env bash
# check_invariant.sh — Verifies money conservation across all shards.
# Usage: ./tests/check_invariant.sh [--sum-only]
set -e

SHARD1_URL=${SHARD1_URL:-"http://localhost:8081"}
SHARD2_URL=${SHARD2_URL:-"http://localhost:8082"}
SHARD3_URL=${SHARD3_URL:-"http://localhost:8083"}
NUM_ACCOUNTS=${NUM_ACCOUNTS:-1000}
STARTING_BALANCE=${STARTING_BALANCE:-10000}
SUM_ONLY=false

for arg in "$@"; do
    if [ "$arg" = "--sum-only" ]; then SUM_ONLY=true; fi
done

if [ "$SUM_ONLY" = false ]; then
    echo "=== Money Conservation Invariant Check ==="
fi

TOTAL=0

for i in $(seq 0 $((NUM_ACCOUNTS - 1))); do
    ACCOUNT="user$i"
    for URL in $SHARD1_URL $SHARD2_URL $SHARD3_URL; do
        RESP=$(curl -s "$URL/balance?account=$ACCOUNT" 2>/dev/null || echo '{"exists":false}')
        EXISTS=$(echo "$RESP" | grep -o '"exists":true' || echo "")
        if [ -n "$EXISTS" ]; then
            BAL=$(echo "$RESP" | grep -o '"balance":[0-9-]*' | cut -d: -f2)
            if [ -n "$BAL" ]; then
                TOTAL=$((TOTAL + BAL))
            fi
            break
        fi
    done
    
    if [ "$SUM_ONLY" = false ] && [ $(( (i + 1) % 200 )) -eq 0 ]; then
        echo "  Checked $((i + 1))/$NUM_ACCOUNTS accounts (running total: $TOTAL)..."
    fi
done

BANK_TOTAL=0
for URL in $SHARD1_URL $SHARD2_URL $SHARD3_URL; do
    RESP=$(curl -s "$URL/balance?account=__bank__" 2>/dev/null || echo '{"exists":false}')
    EXISTS=$(echo "$RESP" | grep -o '"exists":true' || echo "")
    if [ -n "$EXISTS" ]; then
        BAL=$(echo "$RESP" | grep -o '"balance":[0-9-]*' | cut -d: -f2)
        if [ -n "$BAL" ]; then
            BANK_TOTAL=$((BANK_TOTAL + BAL))
        fi
    fi
done

GRAND_TOTAL=$((TOTAL + BANK_TOTAL))

if [ "$SUM_ONLY" = true ]; then
    echo "$GRAND_TOTAL"
    exit 0
fi

# Expected: __bank__ initial per shard = NUM_ACCOUNTS * STARTING_BALANCE
# Total bank initial = 3 * NUM_ACCOUNTS * STARTING_BALANCE
# After seeding all users: user balances = NUM_ACCOUNTS * STARTING_BALANCE
# Bank remaining = 3 * (N*SB) - N*SB = 2 * N * SB
# Grand total = user + bank = N*SB + 2*N*SB = 3*N*SB
EXPECTED=$((NUM_ACCOUNTS * STARTING_BALANCE * 3))

echo ""
echo "=== Results ==="
echo "Total user balances:  $TOTAL"
echo "Total bank balances:  $BANK_TOTAL"
echo "Grand total:          $GRAND_TOTAL"
echo "Expected:             $EXPECTED"
DELTA=$((GRAND_TOTAL - EXPECTED))
echo "Delta:                $DELTA"
echo ""

if [ $DELTA -eq 0 ]; then
    echo "[PASS] Invariant holds: $GRAND_TOTAL == $EXPECTED"
else
    echo "[FAIL] Invariant violated: expected $EXPECTED, got $GRAND_TOTAL, delta=$DELTA"
    exit 1
fi
