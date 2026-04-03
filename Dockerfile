# Multi-stage Dockerfile for ledger-service binaries.
# Use build arg BINARY to select which cmd/ to build.
#
# Usage:
#   docker build --build-arg BINARY=shard -t ledger-shard .
#   docker build --build-arg BINARY=api -t ledger-api .
#   docker build --build-arg BINARY=coordinator -t ledger-coordinator .
#   docker build --build-arg BINARY=load-monitor -t ledger-monitor .

FROM golang:1.25-alpine AS builder

ARG BINARY=shard

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy all source
COPY . .

# Build the selected binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /app ./cmd/${BINARY}

# ------- Runtime -------
FROM alpine:3.19

RUN apk --no-cache add ca-certificates curl

WORKDIR /app

COPY --from=builder /app /app/app
COPY config/ /app/config/

# Create data directory for WAL and state files
RUN mkdir -p /app/data

ENTRYPOINT ["/app/app"]
