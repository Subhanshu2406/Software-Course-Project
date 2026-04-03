#!/usr/bin/env bash
# gen_shard_map.sh — Generates config/shard_map.json from .env parameters.
# Output format matches coordinator/shardmap.shardMapData: {"partitions":{"0":{...},...}}
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

# Source .env if it exists
if [ -f "$ROOT_DIR/.env" ]; then
    set -a
    source "$ROOT_DIR/.env"
    set +a
fi

NUM_SHARDS=${NUM_SHARDS:-3}
TOTAL_PARTITIONS=${TOTAL_PARTITIONS:-30}

if [ $((TOTAL_PARTITIONS % NUM_SHARDS)) -ne 0 ]; then
    echo "ERROR: TOTAL_PARTITIONS ($TOTAL_PARTITIONS) must be divisible by NUM_SHARDS ($NUM_SHARDS)"
    exit 1
fi

PARTITIONS_PER_SHARD=$((TOTAL_PARTITIONS / NUM_SHARDS))
SHARD_PORTS=(8081 8082 8083)

mkdir -p "$ROOT_DIR/config"

# Build the partitions map: each partition → {shard_id, address, role}
echo "{" > "$ROOT_DIR/config/shard_map.json"
echo '  "partitions": {' >> "$ROOT_DIR/config/shard_map.json"

FIRST=true
for s in $(seq 1 $NUM_SHARDS); do
    SHARD_NAME="shard${s}"
    PORT=${SHARD_PORTS[$((s-1))]}
    START=$(( (s - 1) * PARTITIONS_PER_SHARD ))
    END=$(( s * PARTITIONS_PER_SHARD - 1 ))

    for p in $(seq $START $END); do
        if [ "$FIRST" = true ]; then
            FIRST=false
        else
            echo "," >> "$ROOT_DIR/config/shard_map.json"
        fi
        printf '    "%d": {"shard_id": "%s", "address": "%s:%d", "role": "PRIMARY"}' \
            "$p" "$SHARD_NAME" "$SHARD_NAME" "$PORT" >> "$ROOT_DIR/config/shard_map.json"
    done
done

echo "" >> "$ROOT_DIR/config/shard_map.json"
echo "  }" >> "$ROOT_DIR/config/shard_map.json"
echo "}" >> "$ROOT_DIR/config/shard_map.json"

# Also copy to root for local use
cp "$ROOT_DIR/config/shard_map.json" "$ROOT_DIR/shard_map.json"

echo "Generated shard_map.json: $NUM_SHARDS shards, $TOTAL_PARTITIONS partitions ($PARTITIONS_PER_SHARD per shard)"
