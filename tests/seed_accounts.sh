#!/usr/bin/env bash
# seed_accounts.sh — Seeds NUM_ACCOUNTS accounts (user0..userN) with STARTING_BALANCE via coordinator.
set -e

NUM_ACCOUNTS=${NUM_ACCOUNTS:-1000}
STARTING_BALANCE=${STARTING_BALANCE:-10000}
COORDINATOR_URL=${COORDINATOR_URL:-"http://localhost:8080"}
SHARD1_URL=${SHARD1_URL:-"http://localhost:8081"}
SHARD2_URL=${SHARD2_URL:-"http://localhost:8082"}
SHARD3_URL=${SHARD3_URL:-"http://localhost:8083"}

# Generate a JWT token
echo "Generating auth token..."
AUTH_TOKEN=$(go run ./cmd/devtoken 2>/dev/null)
if [ -z "$AUTH_TOKEN" ]; then
    echo "ERROR: Failed to generate auth token"
    exit 1
fi
export AUTH_TOKEN
echo "Auth token generated."

TOTAL_BALANCE=$((NUM_ACCOUNTS * STARTING_BALANCE))
BANK_BALANCE=$TOTAL_BALANCE

echo "=== Seeding Phase ==="
echo "Accounts: $NUM_ACCOUNTS"
echo "Starting balance per account: $STARTING_BALANCE"
echo "Total system balance: $TOTAL_BALANCE"
echo ""

# Step 1: Create __bank__ on all shards with large balance
echo "Creating __bank__ account on all shards..."
for SHARD_URL in $SHARD1_URL $SHARD2_URL $SHARD3_URL; do
    curl -s -X POST "$SHARD_URL/create-account" \
        -H "Content-Type: application/json" \
        -d "{\"account_id\":\"__bank__\",\"balance\":$BANK_BALANCE}" \
        -o /dev/null -w "" || true
done
echo "  __bank__ seeded on all shards."

# Step 2: Create user accounts on all shards
echo "Creating user accounts on all shards..."
for i in $(seq 0 $((NUM_ACCOUNTS - 1))); do
    ACCOUNT="user$i"
    for SHARD_URL in $SHARD1_URL $SHARD2_URL $SHARD3_URL; do
        curl -s -X POST "$SHARD_URL/create-account" \
            -H "Content-Type: application/json" \
            -d "{\"account_id\":\"$ACCOUNT\",\"balance\":0}" \
            -o /dev/null -w "" || true
    done
    
    if [ $(( (i + 1) % 100 )) -eq 0 ]; then
        echo "  Created $((i + 1))/$NUM_ACCOUNTS accounts..."
    fi
done

# Step 3: Transfer starting balance from __bank__ to each user via coordinator
echo ""
echo "Funding accounts via coordinator..."
SUCCESS=0
FAIL=0

for i in $(seq 0 $((NUM_ACCOUNTS - 1))); do
    ACCOUNT="user$i"
    RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$COORDINATOR_URL/submit" \
        -H "Content-Type: application/json" \
        -d "{\"txn_id\":\"seed-$ACCOUNT\",\"source\":\"__bank__\",\"destination\":\"$ACCOUNT\",\"amount\":$STARTING_BALANCE}")
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -1)
    
    if [ "$HTTP_CODE" -ge 200 ] && [ "$HTTP_CODE" -lt 300 ]; then
        SUCCESS=$((SUCCESS + 1))
    else
        FAIL=$((FAIL + 1))
    fi
    
    if [ $(( (i + 1) % 100 )) -eq 0 ]; then
        echo "  Funded $((i + 1))/$NUM_ACCOUNTS accounts (success=$SUCCESS, fail=$FAIL)..."
    fi
done

echo ""
echo "=== Seed Complete ==="
echo "Success: $SUCCESS"
echo "Failed:  $FAIL"
echo "Total seeded balance: \$$(( SUCCESS * STARTING_BALANCE ))"
