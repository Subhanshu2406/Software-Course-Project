#!/usr/bin/env bash
# docker_up.sh — Starts the cluster and waits for all services to be healthy.
set -e

COMPOSE_PROJECT=$(basename "$(cd "$(dirname "$0")/.." && pwd)" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]//g')

echo "=== Bringing up the cluster ==="
docker compose down -v 2>/dev/null || true
docker compose build
docker compose up -d

echo "Waiting for all services to be healthy..."

SERVICES="shard1 shard2 shard3 shard1a shard1b shard2a shard2b shard3a shard3b coordinator load-monitor"
TIMEOUT=60

for svc in $SERVICES; do
    echo -n "  Waiting for $svc..."
    elapsed=0
    while [ $elapsed -lt $TIMEOUT ]; do
        # Get the port mapping for the service
        port=$(docker compose port "$svc" $(docker compose ps --format json "$svc" 2>/dev/null | head -1 | python3 -c "import sys,json; print(list(json.load(sys.stdin).get('Publishers',[{}])[0:1])[0].get('TargetPort',''))" 2>/dev/null) 2>/dev/null | cut -d: -f2 || true)
        
        # Try using the container's internal health check
        status=$(docker inspect --format='{{.State.Health.Status}}' "$(docker compose ps -q "$svc" 2>/dev/null)" 2>/dev/null || echo "starting")
        
        if [ "$status" = "healthy" ]; then
            echo " OK"
            break
        fi
        
        sleep 2
        elapsed=$((elapsed + 2))
        echo -n "."
    done
    
    if [ $elapsed -ge $TIMEOUT ]; then
        echo " TIMEOUT (may still be starting)"
    fi
done

echo ""
echo "=== Cluster is up ==="
echo ""
echo "Services:"
docker compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}"
