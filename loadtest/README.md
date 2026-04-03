# Load Testing with k6

## Prerequisites

- [k6](https://k6.io/docs/getting-started/installation/) installed, **or** Docker

## Running with Docker (recommended)

```bash
# From the project root, with the cluster already running:
docker run --rm -i \
  --network software-course-project_ledger-net \
  -e BASE_URL=http://api-gateway:8000 \
  -e AUTH_TOKEN=<your-jwt-token> \
  -e NUM_ACCOUNTS=1000 \
  -v "$(pwd)/loadtest:/scripts" \
  grafana/k6 run /scripts/load.js
```

## Running locally

```bash
# Generate a dev token first:
go run cmd/devtoken/main.go

# Run k6:
k6 run \
  -e BASE_URL=http://localhost:8000 \
  -e AUTH_TOKEN=<token> \
  -e NUM_ACCOUNTS=1000 \
  loadtest/load.js
```

## Scenarios

| Scenario       | VUs | Duration | p99 Target |
|---------------|-----|----------|------------|
| single_shard  | 50  | 30s      | < 200 ms   |
| cross_shard   | 50  | 30s      | < 500 ms   |

Both scenarios run simultaneously (100 total VUs).

## Output

Results are printed to stdout and written to `loadtest/results.json` with:
- Per-scenario count, p99, and average latency
- Total requests and error rate

## Tuning

Override via environment variables:

| Variable       | Default                  | Description             |
|---------------|--------------------------|-------------------------|
| `BASE_URL`    | `http://localhost:8000`  | API Gateway URL         |
| `AUTH_TOKEN`  | `dummy-token`            | JWT Bearer token        |
| `NUM_ACCOUNTS`| `1000`                   | Number of user accounts |
