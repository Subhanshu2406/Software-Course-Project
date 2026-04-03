# Windows Quick Start

If you're on **Windows with PowerShell**, use the Windows-native `make.bat` wrapper instead of the Unix `make` command.

## Quick Start

```powershell
# Ensure you're in the project directory
cd C:\path\to\Software-Course-Project

# Start the cluster (generates shard map, builds, and starts all services)
make.bat up

# Stop the cluster
make.bat down

# Full cleanup
make.bat clean

# Seed accounts
make.bat seed

# Generate JWT token
make.bat token

# Open frontend
make.bat open

# Open Grafana dashboard
make.bat open-grafana

# Open Prometheus metrics
make.bat open-prometheus
```

## Available Commands

```powershell
make.bat build              # Build Docker images
make.bat up                 # Start the cluster
make.bat down               # Stop the cluster
make.bat clean              # Full cleanup (delete everything)
make.bat seed               # Seed accounts
make.bat token              # Generate JWT token
make.bat token-set          # Generate token + setup instructions
make.bat open               # Open frontend in browser
make.bat open-grafana       # Open Grafana in browser
make.bat open-prometheus    # Open Prometheus in browser
make.bat frontend-dev       # Run frontend dev server (hot reload)
make.bat frontend-build     # Build frontend production bundle
make.bat unit-test          # Run unit tests
make.bat test-health        # Test all health endpoints
```

## Alternative: Use PowerShell Functions Directly

If `make.bat` doesn't work, you can use the PowerShell functions directly:

```powershell
# Load the functions
. .\make.ps1

# Then use them
make-up
make-down
make-seed
make-token
make-test-health
```

## Alternative: Use Git Bash (if installed)

If you have Git for Windows installed:

```bash
# Launch Git Bash
git bash

# Then use regular make commands
make up
make down
make seed
```

## What Each Target Does

| Command | Description |
|---------|-------------|
| `make.bat up` | Generates shard map → cleans previous cluster → builds images → starts all services → waits for health checks |
| `make.bat down` | Stops all containers, removes volumes and orphans |
| `make.bat clean` | Full cleanup: down + remove images + delete data/logs |
| `make.bat seed` | Creates `__bank__` account on all shards; creates 1000 user accounts (`user0`-`user999`) |
| `make.bat token` | Generates a JWT token for API requests |
| `make.bat test-health` | Curls all 12 health endpoints and reports pass/fail |

## Ports

- **Frontend**: http://localhost:3000
- **Grafana**: http://localhost:3001
- **Prometheus**: http://localhost:9090
- **API Gateway**: http://localhost:8000
- **Coordinator**: http://localhost:8080
- **Shard 1/2/3**: http://localhost:8081/8082/8083
- **Load Monitor**: http://localhost:8090
- **Fault Proxy**: http://localhost:6060

## Troubleshooting

**Q: Permission denied on make.bat**
- Right-click `make.bat` → Properties → Unblock

**Q: 'powershell' not found**
- Make sure PowerShell is in PATH: Start → search "PowerShell" → right-click → "Run as administrator" → verify `powershell --version`

**Q: Docker not found**
- Install Docker Desktop from https://www.docker.com/products/docker-desktop

**Q: Commands timeout or hang**
- Check Docker Desktop is running: Ctrl+Shift+Esc → look for Docker in processes
- Try `docker compose ps` to verify Docker works

**Q: Port already in use**
- Edit `docker-compose.yml` and change the port mappings (e.g., `3000:80` → `3001:80`)

---

See [START_GUIDE.md](START_GUIDE.md) for full detailed instructions.
