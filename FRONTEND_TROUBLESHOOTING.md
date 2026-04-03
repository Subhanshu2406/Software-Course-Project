# Frontend Black Screen Troubleshooting

## What Happened

When you first opened http://localhost:3000, the app went black because:

1. **API Calls Failed**: The React frontend tried to fetch cluster data (shard metrics, coordinator health, etc.) on startup
2. **No Error Boundary**: The app had no error handler, so unhandled promise rejections caused the entire UI to crash
3. **No User Feedback**: Without an error boundary, the user saw a black screen with no hint of what went wrong

## What We Fixed

### 1. **Error Boundary Component** (`frontend/src/components/ErrorBoundary.jsx`)
- Catches React rendering errors and component crashes
- Displays helpful error messages instead of a blank screen
- Provides troubleshooting steps directly in the UI
- Has reload/dismiss buttons for recovery

### 2. **Improved API Error Handling** (`frontend/src/api/client.js`)
- Better logging of failed API calls
- Distinguishes between network errors and HTTP errors
- Provides context about what went wrong

### 3. **Resilient ClusterContext** (`frontend/src/context/ClusterContext.jsx`)
- Individual shard/service failures no longer crash the app
- Failed services mark themselves as "offline" instead of throwing
- Separate try-catch for each service (coordinator, load-monitor, each shard)
- Added `isLoading` state for better UX
- Console logs failures for debugging

### 4. **ClusterOverview Error Display** (`frontend/src/pages/ClusterOverview.jsx`)
- Shows loading spinner while connecting
- Displays error banner if connectivity issues occur
- Shows troubleshooting steps integrated in the UI
- Can continue showing stale data even if some services fail

## How to Verify

### Test 1: Frontend is Running
```bash
curl http://localhost:3000
# Should return HTML (not empty or error)
```

### Test 2: Open in Browser
```
http://localhost:3000
```
**Expected:** Frontend loads with either cluster data or an error banner with helpful guidance

### Test 3: Check Backend Services
```bash
docker compose ps
# All containers should show "Up" status

curl http://localhost:8080/health     # Coordinator
curl http://localhost:8081/health     # Shard1
curl http://localhost:8082/health     # Shard2
curl http://localhost:8083/health     # Shard3
```

### Test 4: Monitor Console Logs
Open browser → F12 → Console tab
- Should see API call logs: `[API GET /shard1/metrics]`
- If failures, you'll see: `[API GET /shard1/metrics] Failed: Network error...`

## If You Still See Black Screen

### Step 1: Check Browser Console (F12)
Look for any JavaScript errors. If you see:
- **"Access to XMLHttpRequest...denied"** → CORS issue
- **"Failed to connect to coordinator"** → Backend not responding
- **"Network error"** → Container networking issue

### Step 2: Verify All Containers are Healthy
```bash
docker compose ps
```
Every container should show:
- **STATUS:** "Up X minutes" (not "Exited" or "Unhealthy")
- **HEALTH:** Ideally "Healthy" (green checkmark)

**If any container is not healthy:**
```bash
# Check its logs
docker compose logs coordinator
docker compose logs shard1
docker compose logs kafka

# Or restart it
docker compose restart coordinator
docker compose restart frontend
```

### Step 3: Check Container Networking
Ensure containers can reach each other:
```bash
# From frontend container, try to reach coordinator
docker compose exec frontend curl http://coordinator:8080/health

# From coordinator, try to reach a shard
docker compose exec coordinator curl http://shard1:8081/health
```

**If above commands fail:**
```bash
# Full restart might be needed
docker compose down
docker compose up -d

# Then wait for all to be healthy
docker compose ps | grep healthy
```

### Step 4: Clear Browser Cache
```bash
# In browser: Ctrl+Shift+Delete (or Cmd+Shift+Delete on Mac)
# Or hard refresh: Ctrl+Shift+R (or Cmd+Shift+R on Mac)
```

### Step 5: Check Frontend Build
Frontend code is built into the nginx container. If changes don't appear:

```bash
# Rebuild frontend
cd frontend && npm run build

# Restart container with new build
docker compose restart frontend
```

## Common Issues & Fixes

| Issue | Cause | Fix |
|-------|-------|-----|
| Black screen, no error | Service unavailable | `docker compose ps` verify all healthy |
| Error banner shows | Backend service offline | `docker compose logs coordinator` check logs |
| Port 3000 in use | Another process using port | `docker compose stop && docker compose up -d` |
| API calls 404 | nginx routing issue | `docker compose logs frontend` check nginx config |
| Stale data shown | Some services offline but app working | This is OK - shows partial data gracefully |

## Network Diagram

```
Browser (localhost:3000)
    ↓
    └── nginx (port 80 inside container, 3000 mapped to host)
         ├─ /coordinator/ → http://coordinator:8080       (Docker network name)
         ├─ /shard1/ → http://shard1:8081                 (within Docker network)
         ├─ /shard2/ → http://shard2:8082
         ├─ /shard3/ → http://shard3:8083
         └─ /load-monitor/ → http://load-monitor:8090
            (These container names only work inside Docker, not from browser)
```

**Important:** Browser can only use `localhost:PORT` or IP addresses. Container names like `coordinator:8080` don't resolve in the browser—that's why nginx is needed to proxy requests.

## Testing the Fix

Try this workflow:

```bash
# Terminal 1: Monitor cluster
docker compose ps

# Terminal 2: Open frontend and watch browser console
open http://localhost:3000
# Or: Windows: start http://localhost:3000
# Or: Linux: xdg-open http://localhost:3000
# Then press F12 → Console tab

# Terminal 3: Seed data and watch frontend update
docker compose exec coordinator curl -X POST http://localhost:8080/migrate/accounts
docker compose exec coordinator curl http://localhost:8080/health

# You should see data appear in frontend, or see helpful error if service fails
```

## Architecture

```
┌─ Docker Container Network ─────────────────────────────────────────────┐
│                                                                           │
│  ┌──────────────┐                                    ┌─────────────────┐ │
│  │  Frontend    │                                    │   Coordinator   │ │
│  │   (nginx)    │◄──────────/coordinator/────────► │  (Go API :8080) │ │
│  │  (:80→:3000) │                                    └─────────────────┘ │
│  └──────────────┘                                                        │
│         ▲                                                                  │
│         │ /shard1/../../3/                                               │
│         │                                                                  │
│  ┌──────▼──────────────────────────────────────────────────────────────┐ │
│  │                          Shard Cluster                              │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐                           │ │
│  │  │ Shard 1  │  │ Shard 2  │  │ Shard 3  │                           │ │
│  │  │ (:8081)  │  │ (:8082)  │  │ (:8083)  │                           │ │
│  │  └──────────┘  └──────────┘  └──────────┘                           │ │
│  │                          (Partition Map)                            │ │
│  └──────────────────────────────────────────────────────────────────┘ │
│                                                                           │
└─────────────────────────────────────────────────────────────────────────┘
```

## Quick Reference

| Task | Command |
|------|---------|
| View all containers | `docker compose ps` |
| Rebuild & restart frontend | `npm run build && docker compose restart frontend` |
| View logs | `docker compose logs frontend -f` |
| Execute command in container | `docker compose exec coordinator curl :8080/health` |
| Full clean restart | `docker compose down -v && docker compose up -d` |

