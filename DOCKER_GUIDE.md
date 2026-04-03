# Docker Compose Guide

## Overview

The entire system is containerized and orchestrated with Docker Compose. This guide explains what containers exist, how they communicate, and how to manage them.

---

## Container Architecture

| Container | Image | Port | Purpose | Network |
|-----------|-------|------|---------|---------|
| **zookeeper** | confluentinc/cp-zookeeper:7.5.0 | 2181 | Kafka coordination | Docker network |
| **kafka** | confluentinc/cp-kafka:7.5.0 | 9092 | Event streaming | Docker network |
| **shard1** | ledger-shard | 8081 | Primary shard (partitions 0-9) | Docker network |
| **shard2** | ledger-shard | 8082 | Primary shard (partitions 10-19) | Docker network |
| **shard3** | ledger-shard | 8083 | Primary shard (partitions 20-29) | Docker network |
| **shard1a, 1b** | ledger-shard | 9081, 9082 | Followers for shard1 | Docker network |
| **shard2a, 2b** | ledger-shard | 9083, 9084 | Followers for shard2 | Docker network |
| **shard3a, 3b** | ledger-shard | 9085, 9086 | Followers for shard3 | Docker network |
| **coordinator** | ledger-coordinator | 8080 | Transaction coordinator | Docker network |
| **api-gateway** | ledger-api | 8000 | REST API gateway | Docker network |
| **load-monitor** | ledger-monitor | 8090 | Load balancing & metrics | Docker network |
| **fault-proxy** | ledger-fault-proxy | 6060 | Fault injection (kill/restart) | Docker network |
| **prometheus** | prom/prometheus:v2.51.0 | 9090 | Metrics collection | Docker network |
| **grafana** | grafana/grafana:10.4.0 | 3001 | Dashboards & visualization | Docker network |
| **frontend** | ledger-frontend (nginx) | 3000 | React SPA (via nginx) | Docker network |

---

## Docker Compose Commands

### View Status

```bash
# See all running containers
docker compose ps

# See details (includes IPs, volumes)
docker compose ps --format json | jq

# Check container health
docker compose ps | grep healthy

# See logs for a specific service
docker compose logs coordinator          # Real-time logs from coordinator
docker compose logs frontend --tail 50   # Last 50 lines
docker compose logs -f kafka             # Follow logs (live)
```

### Start/Stop Services

```bash
# Start all services (from docker-compose.yml)
docker compose up -d

# Start specific service
docker compose start frontend
docker compose start coordinator

# Stop all services (keeps volumes, data persists)
docker compose stop

# Stop specific service
docker compose stop frontend
docker compose stop kafka

# Restart service (stop + start)
docker compose restart frontend
docker compose restart coordinator
```

### Remove/Clean Up

```bash
# Stop and remove containers (keep volumes)
docker compose down

# Remove everything including volumes (WARNING: data loss)
docker compose down -v

# Remove everything including volumes AND images
docker compose down -v --rmi local

# Remove orphaned containers
docker compose down --remove-orphans
```

### Build Images

```bash
# Build all images
docker compose build

# Build specific image
docker compose build frontend

# Build with no cache (force rebuild)
docker compose build --no-cache

# View build output
docker compose build --progress=plain
```

### Access Container Shell

```bash
# Open bash shell in running container
docker compose exec coordinator bash
docker compose exec shard1 bash
docker compose exec frontend bash

# Run a single command in container
docker compose exec coordinator curl localhost:8080/health
docker compose exec shard1 curl localhost:8081/health
```

### View Volumes & Data

```bash
# List all volumes
docker volume ls

# View volume details
docker volume inspect software-course-project_prometheus-data
docker volume inspect software-course-project_shard1-data

# Mount volume locally (inspect data)
docker run -v software-course-project_shard1-data:/data -it alpine sh
# Inside container: ls -la /data
```

---

## Network Connectivity

### Docker Network

All containers are on the same **Docker bridge network** (default in docker-compose).

**From inside containers:**
- Services communicate by **container name**: `http://coordinator:8080`, `http://shard1:8081`
- Example: Coordinator can call `http://shard1:8081/health`

**From your host machine (browser/curl):**
- Services are accessed by **localhost + mapped port**: `http://localhost:3000`, `http://localhost:8080`
- Container names DON'T resolve on host
- Example: Browser calls `http://localhost:8080/health` (not `http://coordinator:8080`)

### Port Mappings

Container port ← mapped to → host port

```
Frontend (nginx inside container port 80) → host port 3000
  Browser use: http://localhost:3000

Coordinator (port 8080) → host port 8080
  curl: curl http://localhost:8080/health

Shard1 (port 8081) → host port 8081
  curl: curl http://localhost:8081/health

Grafana (port 3000 inside) → host port 3001
  Browser: http://localhost:3001

Prometheus (port 9090) → host port 9090
  Browser: http://localhost:9090
```

### Frontend to Backend Connectivity

**Problem:** React browser code can't call `http://coordinator:8080` (container name doesn't resolve in browser).

**Solution:** nginx reverse proxy

The frontend container runs nginx which:
1. Serves the React app on port 80 (mapped to host port 3000)
2. Proxies API requests to backend containers by name (which works within Docker network)

```
Browser → nginx (localhost:3000) → coordinator:8080
         (within Docker network, by name works)
```

nginx config (`config/nginx.conf`):
```nginx
location /coordinator/ {
    proxy_pass http://coordinator:8080/;  # Works inside Docker
}
```

React API client calls `http://localhost:3000/coordinator/health`, which nginx converts to `http://coordinator:8080/health` inside the Docker network.

---

## Common Issues & Solutions

### Issue: Black screen in frontend

**Cause:** Frontend can't reach backend services (API calls failing)

**Check:**
```bash
# From inside frontend container
docker compose exec frontend curl http://coordinator:8080/health

# From host (should get address resolution error)
curl http://coordinator:8080/health  # ❌ Won't work (container name)
curl http://localhost:8080/health    # ✅ Works (mapped port)
```

**Fix:**
- Ensure all backend services are healthy: `docker compose ps | grep healthy`
- Check nginx logs: `docker compose logs frontend`
- Check browser console logs (F12 → Network tab) for failed requests
- Restart frontend: `docker compose restart frontend`

### Issue: Port already in use

**Cause:** Another service on system using the port

**Check:**
```bash
# See what's using port 3000
lsof -i :3000                              # macOS/Linux
netstat -ano | findstr :3000                # Windows
```

**Fix:**
- Stop other services using the port
- OR change port mapping in `docker-compose.yml`:
  ```yaml
  frontend:
    ports:
      - "3001:80"  # Use 3001 instead of 3000
  ```

### Issue: Services can't reach each other

**Cause:** Network or DNS issue

**Check:**
```bash
# Test if coordinator is reachable from shard1
docker compose exec shard1 ping coordinator

# Try curl
docker compose exec shard1 curl http://coordinator:8080/health

# Check Docker networks
docker network ls
docker network inspect software-course-project_default
```

**Fix:**
- Ensure all services are on same network
- Restart Docker: `docker compose restart`
- Full reset: `docker compose down -v && docker compose up -d`

### Issue: Data/volumes not persisting

**Cause:** Volume not mounted or named volumes deleted

**Check:**
```bash
# See what's in volumes
docker volume ls | grep shard
docker volume inspect software-course-project_shard1-data
```

**Fix:**
- Don't use `docker compose down -v` (deletes volumes)
- Use `docker compose down` instead (keeps volumes)
- Use `docker compose down --remove-orphans` to clean up stale containers

### Issue: Out of memory

**Cause:** Too many containers, excessive logging

**Check:**
```bash
# See memory usage
docker stats

# Check container size
docker compose ps --format "table {{.Names}}\t{{.Size}}"
```

**Fix:**
- Stop non-essential containers: `docker compose stop shard2a shard2b`
- Reduce log retention in docker-compose.yml:
  ```yaml
  logging:
    driver: "json-file"
    options:
      max-size: "10m"
      max-file: "3"
  ```

---

## Workflow Examples

### Starting from Scratch

```bash
# Full cleanup
docker compose down -v --remove-orphans

# Rebuild
docker compose build

# Start
docker compose up -d

# Wait for health
docker compose ps | grep healthy
# (repeat every 5s until all show "Healthy")

# Check all endpoints
curl http://localhost:8000/health  # API Gateway
curl http://localhost:8080/health  # Coordinator
curl http://localhost:8081/health  # Shard 1
curl http://localhost:8090/health  # Load Monitor
```

### Stopping and Restarting (preserves data)

```bash
# Stop all
docker compose stop

# Later: Restart all
docker compose start

# Or restart just one service
docker compose restart coordinator
docker compose restart frontend
```

### Debugging a Service

```bash
# View logs
docker compose logs coordinator --tail 100

# Access shell
docker compose exec coordinator bash
cd /app
./app --help
exit

# Exit shell
exit
```

### Testing Inter-Container Communication

```bash
# Test frontend can reach coordinator
docker compose exec frontend curl http://coordinator:8080/health

# Test coordinator can reach shard
docker compose exec coordinator curl http://shard1:8081/health

# Test from host (should FAIL - container names don't resolve)
curl http://coordinator:8080/health  # ❌ Error
# But this works (mapped ports)
curl http://localhost:8080/health    # ✅ OK
```

### Viewing Service Configuration

```bash
# See full config that will be used
docker compose config

# See specific service
docker compose config | grep -A 20 'coordinator:'
```

---

## Performance & Monitoring

### Monitor in Real-Time

```bash
# Watch container stats
docker stats

# Watch logs
docker compose logs -f coordinator

# Watch specific service startup
docker compose logs frontend --tail 50
```

### Check Resource Usage

```bash
# Disk space used by containers
docker system df

# Disk space per image
docker images --format "table {{.Repository}}\t{{.Size}}"

# Prune unused resources
docker system prune          # Remove dangling images/containers
docker system prune -a       # Remove all unused (WARNING)
```

### Access Metrics

```bash
# Prometheus metrics (raw)
curl http://localhost:9090/api/v1/query?query=shard_tps

# Grafana dashboards
open http://localhost:3001

# Prometheus UI
open http://localhost:9090
```

---

## Useful Docker Compose Profiles

The docker-compose.yml uses profiles for optional services:

```bash
# Start with all profiles
docker compose --profile followers --profile multi-coordinator up -d

# Start only default services (no followers)
docker compose up -d

# Enable specific profile
COMPOSE_PROFILES=followers docker compose up -d
```

---

## Reference: All docker compose Commands

| Command | Purpose |
|---------|---------|
| `docker compose up -d` | Start all services in background |
| `docker compose down` | Stop and remove containers |
| `docker compose ps` | List running containers |
| `docker compose logs <service>` | View logs |
| `docker compose exec <service> <cmd>` | Run command in container |
| `docker compose build` | Build images |
| `docker compose start` | Start stopped containers |
| `docker compose stop` | Stop running containers |
| `docker compose restart <service>` | Restart service |
| `docker compose config` | Show full config |
| `docker compose stats` | Live resource usage |

