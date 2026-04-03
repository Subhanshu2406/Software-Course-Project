# Frontend Fix - Session Summary

## Problem
After `docker compose down -v && make build && make up`, opening http://localhost:3000 resulted in a black screen with console errors:
- `Failed to load resource: the server responded with a status of 404 (Not Found) coordinator/metrics:1`
- `TypeError: (r || []).map is not a function`

Additionally, `make frontend-dev` failed with:
- `Error: listen EACCES: permission denied ::1:3000`

## Root Causes

### Issue 1: Missing `/metrics` Endpoint on Coordinator
- **Problem**: Frontend API client called `GET /coordinator/metrics`, but the coordinator only had `/metrics/prometheus`
- **Result**: 404 error, then TypeError when trying to `.map()` on error text

### Issue 2: IPv6 Bind Error on WSL2  
- **Problem**: Vite dev server tried to bind to `::1:3000` (IPv6), which fails on WSL2
- **Result**: `EACCES: permission denied`

## Solutions Implemented

### ✅ Solution 1: Added `/metrics` Endpoint to Coordinator

**File:** `coordinator/consumer/consumer.go`
- Added `handleMetrics()` method that returns JSON metrics (compatible with frontend expectations)
- Added `HandleMetricsDirect()` public wrapper for external mux registration
- Returns: committed_count, aborted_count, tps, uptime_seconds, etc.

**File:** `cmd/coordinator/main.go`
- Registered `/metrics` endpoint on HTTP mux (both Kafka and HTTP modes)

**Endpoints Now Available:**
```
GET /coordinator/metrics        → JSON metrics (NEW)
GET /coordinator/transactions   → Recent transactions (existing)
GET /coordinator/status         → Single transaction status (existing)
POST /coordinator/transfer      → Submit new transfer (existing)
```

### ✅ Solution 2: Fixed Vite IPv6 Issue

**File:** `frontend/vite.config.js`
- Changed `server.host` from undefined (defaults to `::1`) to `127.0.0.1` (IPv4 only)
- Prevents WSL2 permission errors

### ✅ Solution 3: Verified nginx Routes

**File:** `config/nginx.conf`
- Confirmed `location /coordinator/ { proxy_pass http://coordinator/; }`
- Correctly routes `/coordinator/metrics` → coordinator:8080/metrics within Docker network

## Verification

### Test 1: Coordinator Metrics Endpoint (Inside Docker)
```bash
docker compose exec coordinator curl http://localhost:8080/metrics
```
**Result**: Returns JSON with metrics ✅
```json
{
  "shard_id": "coordinator",
  "role": "coordinator",
  "committed_count": 0,
  "aborted_count": 0,
  "tps": 0,
  "uptime_seconds": 45
}
```

### Test 2: Frontend API Routing (Through nginx)
```bash
curl http://localhost:3000/coordinator/metrics
```
**Result**: Returns 200 OK with JSON ✅
- nginx successfully proxies to coordinator
- Status code: 200
- Content-Type: application/json

### Test 3: Frontend Load
```bash
# Open browser
http://localhost:3000
```
**Expected Result**: 
- ✅ Frontend loads (no black screen)
- ✅ Cluster data displays OR error banner shows if services unavailable
- ✅ Check browser F12 → Console for API logs

## What to Do Now

### Step 1: Verify Everything is Running
```bash
docker compose ps
# All containers should show "Up" and preferably "Healthy"
```

### Step 2: Test Frontend in Browser
```bash
# Open http://localhost:3000 in your browser
# (or use: start http://localhost:3000 on Windows)
```

**Expected outcomes:**
- **Best case**: Cluster overview page loads with shard data
- **Good case**: Error banner appears with helpful troubleshooting steps
- **Never again**: Black screen with no feedback

### Step 3: Check Console Logs (F12)
Open Developer Tools (F12) and look for:
- API call logs: `[API GET /coordinator/metrics]` ✅
- No 404 errors anymore
- Any remaining errors will be in the error banner on the page

### Step 4: If Using Dev Server
```bash
make frontend-dev
```

**Now works** because:
- Vite binds to `127.0.0.1:3000` (IPv4) instead of `::1:3000` (IPv6)
- No more "permission denied" errors on WSL2
- Browser: http://localhost:3000
- Hot reload enabled for development

## Files Modified

| File | Change |
|------|--------|
| `coordinator/consumer/consumer.go` | Added `handleMetrics()` and `HandleMetricsDirect()` |
| `cmd/coordinator/main.go` | Registered `/metrics` endpoint (2 places) |
| `frontend/vite.config.js` | Added `host: '127.0.0.1'` to server config |
| `frontend/src/main.jsx` | Already had ErrorBoundary (from previous fix) |
| `frontend/src/api/client.js` | Already had error handling (from previous fix) |
| `frontend/src/context/ClusterContext.jsx` | Already had graceful degradation (from previous fix) |

## Architecture Now

```
Browser (http://localhost:3000)
    ↓
    └── nginx (port 3000)
         ├─ GET /coordinator/metrics
         │   └── (proxy to coordinator:8080/metrics) → ✅ Now returns 200 + JSON
         ├─ GET /shard1/metrics
         │   └── (proxy to shard1:8081/metrics) → ✅ Already working
         ├─ GET /load-monitor/metrics
         │   └── (proxy to load-monitor:8090/metrics) → ✅ Already working
         └─ Static files + React frontend
             └── App → ClusterContext → API calls → displays data or error banner
```

## Quick Reference

| Task | Command |
|------|---------|
| View all containers | `docker compose ps` |
| Open frontend | `start http://localhost:3000` (Windows) |
| | `open http://localhost:3000` (macOS) |
| | `xdg-open http://localhost:3000` (Linux) |
| Check coordinator metrics | `curl http://localhost:3000/coordinator/metrics` |
| Run dev server | `make frontend-dev` |
| View frontend logs | `docker compose logs frontend -f` |
| Full restart | `docker compose down -v && make build && make up` |

## Summary of Fixes

| Issue | Cause | Fix |
|-------|-------|-----|
| 404 on `/coordinator/metrics` | Endpoint didn't exist | Added `/metrics` endpoint returning JSON metrics |
| `map is not a function` | Trying to parse 404 error as data | Fixed by providing actual endpoint |
| IPv6 bind error on `make frontend-dev` | WSL2 + Vite IPv6 default | Set `host: '127.0.0.1'` in vite config |
| Black screen on load | Cascading errors from missing APIs | Error boundary + endpoints now work together |

## Next Steps

1. **Test the frontend**: Open http://localhost:3000
2. **Try make frontend-dev**: For local development with hot reload
3. **Run tests**: `make seed && make test-health && make test-load-single`
4. **View metrics**: Open http://localhost:3001 (Grafana)

All systems should now work together seamlessly! 🎉
