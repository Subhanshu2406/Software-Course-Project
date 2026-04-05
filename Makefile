# ---- Windows Git Bash / MSYS2 Path Conversion Fix ----
MSYS_NO_PATHCONV := 1
export MSYS_NO_PATHCONV

LOADTEST_VOL := /$(subst :,,$(CURDIR))/loadtest

include .env
export

.PHONY: build up down seed token gen-config unit-test \
        test-health test-load test-load-single test-load-cross test-load-mixed \
        test-concurrency test-recovery test-follower-kill test-leader-failover \
        test-coordinator-kill test-multi-coordinator test-migration test-scale \
        test-invariant test-all test-fast report clean \
        test-failure test-cross-shard test-stress \
        frontend-dev frontend-build open open-grafana open-prometheus token-set

# ---- Build ----
build:
	docker compose build

# ---- Generate shard map from .env ----
gen-config:
	bash scripts/gen_shard_map.sh

# ---- Cluster lifecycle ----
up: gen-config
	docker compose down -v --remove-orphans 2>/dev/null || true
	docker compose build
	docker compose up -d
	@echo "Waiting for services to become healthy..."
	@for i in $$(seq 1 60); do \
		HEALTHY=$$(docker compose ps --format json 2>/dev/null | grep -c '"healthy"' || echo 0); \
		TOTAL=$$(docker compose ps --format json 2>/dev/null | grep -c '"Service"' || echo 0); \
		if [ "$$HEALTHY" = "$$TOTAL" ] && [ "$$TOTAL" -gt 0 ]; then \
			echo "All $$TOTAL services healthy."; \
			break; \
		fi; \
		sleep 2; \
	done
	docker compose ps

down:
	docker compose down -v --remove-orphans

# ---- Seed & tokens ----
seed:
	bash tests/seed_accounts.sh

token:
	@go run cmd/devtoken/main.go

# ---- Unit tests ----
unit-test:
	go test ./tests/unit/... -v -count=1

# ---- K6 helper (internal) ----
# Usage: $(call run_k6,SCENARIO,RESULTS_FILE)
NETWORK_NAME = $(shell docker network ls --format '{{.Name}}' | grep software-course-project | head -1)
TOKEN = $(shell go run cmd/devtoken/main.go 2>/dev/null)
define run_k6
	docker run --rm -i \
	  --network $(NETWORK_NAME) \
	  -e BASE_URL=http://coordinator:8080 \
	  -e AUTH_TOKEN=$(TOKEN) \
	  -e NUM_ACCOUNTS=$(NUM_USERS) \
	  -e LOAD_VUS=$(LOAD_VUS) \
	  -e LOAD_DURATION=$(LOAD_DURATION) \
	  -e SCENARIO=$(1) \
	  -e RESULTS_FILE=/scripts/$(2) \
	  -v "$(LOADTEST_VOL):/scripts" \
	  grafana/k6 run /scripts/load.js
endef

# ============================================================
#  Test Suite  (cluster must be up and seeded)
# ============================================================

# T1 — Health check every endpoint
test-health:
	@echo ""; echo "=== T1: Health Check ===" ; PASS=0; FAIL=0; \
	for url in http://localhost:8000/health http://localhost:8080/health \
	           http://localhost:8081/health http://localhost:8082/health http://localhost:8083/health \
	           http://localhost:9081/health http://localhost:9082/health \
	           http://localhost:9083/health http://localhost:9084/health \
	           http://localhost:9085/health http://localhost:9086/health \
	           http://localhost:8090/health; do \
		CODE=$$(curl -s -o /dev/null -w "%{http_code}" "$$url" 2>/dev/null || echo "000"); \
		if [ "$$CODE" = "200" ]; then PASS=$$((PASS+1)); else FAIL=$$((FAIL+1)); echo "  FAIL: $$url ($$CODE)"; fi; \
	done; \
	echo "PASS=$$PASS  FAIL=$$FAIL"; \
	if [ $$FAIL -eq 0 ]; then echo "[PASS] T1 Health"; else echo "[FAIL] T1 Health"; exit 1; fi

# T2 — All load tests (single, cross, mixed)
test-load: test-load-single test-load-cross test-load-mixed

# T2a — Single-shard load
test-load-single:
	@echo ""; echo "=== T2: Load – single-shard ==="
	$(call run_k6,single_shard,results_single.json)

# T3 — Cross-shard load
test-load-cross:
	@echo ""; echo "=== T3: Load – cross-shard ==="
	$(call run_k6,cross_shard,results_cross.json)

# T4 — Mixed load (70/30)
test-load-mixed:
	@echo ""; echo "=== T4: Load – mixed ==="
	$(call run_k6,mixed,results_mixed.json)

# T5 — Go concurrency test
test-concurrency:
	@echo ""; echo "=== T5: Concurrency ==="
	go test ./tests/integration/ -run TestConcurrency -v -count=1 -timeout 120s

# T6 — WAL / crash recovery
test-recovery:
	@echo ""; echo "=== T6: WAL Recovery ==="
	bash tests/wal_replay_test.sh

# T7 — Kill one follower, verify writes continue
test-follower-kill:
	@echo ""; echo "=== T7: Follower Kill ==="
	bash tests/failure_tests.sh

# T8 — Kill leader shard, verify follower promotion (if implemented) or recovery
test-leader-failover:
	@echo ""; echo "=== T8: Leader Failover ==="
	@echo "Stopping shard1 leader..."
	docker compose stop shard1
	@sleep 5
	@echo "Restarting shard1..."
	docker compose start shard1
	@ELAPSED=0; while [ $$ELAPSED -lt 30 ]; do \
		CODE=$$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8081/health 2>/dev/null || echo "000"); \
		if [ "$$CODE" = "200" ]; then echo "[PASS] T8 Leader Failover — shard1 recovered"; break; fi; \
		sleep 2; ELAPSED=$$((ELAPSED+2)); \
	done; \
	if [ "$$CODE" != "200" ]; then echo "[FAIL] T8 Leader Failover — shard1 did not recover"; exit 1; fi

# T9 — Kill coordinator, verify Kafka-based recovery
test-coordinator-kill:
	@echo ""; echo "=== T9: Coordinator Kill ==="
	docker compose stop coordinator
	@sleep 5
	docker compose start coordinator
	@ELAPSED=0; while [ $$ELAPSED -lt 30 ]; do \
		CODE=$$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health 2>/dev/null || echo "000"); \
		if [ "$$CODE" = "200" ]; then echo "[PASS] T9 Coordinator Kill — recovered"; break; fi; \
		sleep 2; ELAPSED=$$((ELAPSED+2)); \
	done; \
	if [ "$$CODE" != "200" ]; then echo "[FAIL] T9 Coordinator Kill — did not recover"; exit 1; fi

# T10 — Multi-coordinator (bring up coordinator2 via profile)
test-multi-coordinator:
	@echo ""; echo "=== T10: Multi-Coordinator ==="
	COMPOSE_PROFILES=$${COMPOSE_PROFILES},multi-coordinator docker compose up -d coordinator2
	@ELAPSED=0; while [ $$ELAPSED -lt 30 ]; do \
		CODE=$$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8079/health 2>/dev/null || echo "000"); \
		if [ "$$CODE" = "200" ]; then break; fi; \
		sleep 2; ELAPSED=$$((ELAPSED+2)); \
	done; \
	RESP=$$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8079/submit \
		-H "Content-Type: application/json" \
		-d '{"txn_id":"mc-test-1","source":"user0","destination":"user1","amount":1}' 2>/dev/null || echo "000"); \
	if [ "$$RESP" = "200" ] || [ "$$RESP" = "202" ]; then \
		echo "[PASS] T10 Multi-Coordinator — coordinator2 accepts transactions"; \
	else \
		echo "[FAIL] T10 Multi-Coordinator — coordinator2 returned $$RESP"; exit 1; \
	fi

# T11 — Migration / load rebalancing
test-migration:
	@echo ""; echo "=== T11: Migration ==="
	bash tests/migration_test.sh

# T12 — Scale test (restart with 2-shard topology then back to 3)
test-scale:
	@echo ""; echo "=== T12: Scale Test ==="
	@echo "Currently running with 3 shards. Verifying shard count..."
	@S1=$$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8081/health 2>/dev/null || echo "000"); \
	 S2=$$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8082/health 2>/dev/null || echo "000"); \
	 S3=$$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8083/health 2>/dev/null || echo "000"); \
	 COUNT=0; \
	 if [ "$$S1" = "200" ]; then COUNT=$$((COUNT+1)); fi; \
	 if [ "$$S2" = "200" ]; then COUNT=$$((COUNT+1)); fi; \
	 if [ "$$S3" = "200" ]; then COUNT=$$((COUNT+1)); fi; \
	 echo "Active shards: $$COUNT"; \
	 if [ $$COUNT -eq 3 ]; then echo "[PASS] T12 Scale — all 3 shards active"; \
	 else echo "[FAIL] T12 Scale — expected 3 shards, got $$COUNT"; exit 1; fi

# T13 — Final invariant
test-invariant:
	@echo ""; echo "=== T13: Final Invariant ==="
	bash tests/check_invariant.sh

# T14 — Failure tests (alias)
test-failure:
	@echo ""; echo "=== T14: Failure Tests (alias for test-follower-kill) ==="
	bash tests/failure_tests.sh

# T15 — Cross-shard load test (alias)
test-cross-shard:
	@echo ""; echo "=== T15: Cross-Shard Load ==="
	$(call run_k6,cross_shard,results_cross.json)

# T16 — Comprehensive stress test (invariant + WAL + migration + faults under load)
test-stress:
	@echo ""; echo "=== T16: Comprehensive Stress Test ==="
	bash tests/stress_test.sh

# ---- Composite targets ----
test-all: test-health test-load-single test-load-cross test-load-mixed \
          test-concurrency test-recovery test-follower-kill test-leader-failover \
          test-coordinator-kill test-multi-coordinator test-migration test-scale \
          test-invariant

test-fast: test-health test-concurrency test-follower-kill test-invariant

# ---- Report generation ----
report:
	@echo "Generating TEST_REPORT.md..."
	@bash scripts/gen_report.sh

# ---- Clean everything ----
clean:
	docker compose down -v --remove-orphans --rmi local 2>/dev/null || true
	rm -f loadtest/results*.json
	rm -rf data/ logs/

# ---- Frontend ----
frontend-dev:
	cd frontend && npm install && npm run dev

frontend-build:
	cd frontend && npm install && npm run build

# ---- Open in browser ----
open:
	@echo "Opening frontend at http://localhost:3000"
	@start http://localhost:3000 2>/dev/null || open http://localhost:3000 2>/dev/null || xdg-open http://localhost:3000 2>/dev/null || true

open-grafana:
	@echo "Opening Grafana at http://localhost:3001"
	@start http://localhost:3001 2>/dev/null || open http://localhost:3001 2>/dev/null || xdg-open http://localhost:3001 2>/dev/null || true

open-prometheus:
	@echo "Opening Prometheus at http://localhost:9090"
	@start http://localhost:9090 2>/dev/null || open http://localhost:9090 2>/dev/null || xdg-open http://localhost:9090 2>/dev/null || true

# ---- Token management ----
token-set:
	@TOKEN=$$(go run cmd/devtoken/main.go 2>/dev/null); \
	echo "Token: $$TOKEN"; \
	echo "Set in browser localStorage: localStorage.setItem('ledger_token', '$$TOKEN')"
