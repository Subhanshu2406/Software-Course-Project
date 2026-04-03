import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const AUTH_TOKEN = __ENV.AUTH_TOKEN || 'dummy-token';
const NUM_ACCOUNTS = parseInt(__ENV.NUM_ACCOUNTS || '1000');
const LOAD_VUS = parseInt(__ENV.LOAD_VUS || '50');
const LOAD_DURATION = __ENV.LOAD_DURATION || '30s';
const SCENARIO = __ENV.SCENARIO || 'mixed'; // single_shard, cross_shard, or mixed

// Custom metrics
const singleShardTxns = new Counter('single_shard_txns');
const crossShardTxns = new Counter('cross_shard_txns');
const singleShardDuration = new Trend('single_shard_duration', true);
const crossShardDuration = new Trend('cross_shard_duration', true);

// Build scenarios dynamically based on SCENARIO env var
function buildScenarios() {
  const s = {};
  if (SCENARIO === 'single_shard' || SCENARIO === 'mixed') {
    s.single_shard = {
      executor: 'constant-vus',
      vus: SCENARIO === 'mixed' ? Math.round(LOAD_VUS * 0.7) : LOAD_VUS,
      duration: LOAD_DURATION,
      exec: 'singleShard',
      tags: { scenario: 'single_shard' },
    };
  }
  if (SCENARIO === 'cross_shard' || SCENARIO === 'mixed') {
    s.cross_shard = {
      executor: 'constant-vus',
      vus: SCENARIO === 'mixed' ? Math.round(LOAD_VUS * 0.3) : LOAD_VUS,
      duration: LOAD_DURATION,
      exec: 'crossShard',
      tags: { scenario: 'cross_shard' },
    };
  }
  return s;
}

export const options = {
  scenarios: buildScenarios(),
  thresholds: {
    'single_shard_duration': ['p(99)<200'],
    'cross_shard_duration': ['p(99)<500'],
    'http_req_failed': ['rate<0.05'],
  },
};

const params = {
  headers: {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${AUTH_TOKEN}`,
  },
};

// Pick two accounts that hash to the same shard (within same 333-account range)
// Accounts 0-332 → shard1, 333-665 → shard2, 666-999 → shard3 (approx)
function sameShardPair() {
  const shardSize = Math.floor(NUM_ACCOUNTS / 3);
  const shardIdx = Math.floor(Math.random() * 3);
  const base = shardIdx * shardSize;
  const src = base + Math.floor(Math.random() * shardSize);
  let dst = base + Math.floor(Math.random() * shardSize);
  if (src === dst) dst = base + ((dst - base + 1) % shardSize);
  return [src, dst];
}

// Pick two accounts that hash to different shards
function crossShardPair() {
  const shardSize = Math.floor(NUM_ACCOUNTS / 3);
  const shard1 = Math.floor(Math.random() * 3);
  let shard2 = (shard1 + 1 + Math.floor(Math.random() * 2)) % 3;
  const src = shard1 * shardSize + Math.floor(Math.random() * shardSize);
  const dst = shard2 * shardSize + Math.floor(Math.random() * shardSize);
  return [src, dst];
}

export function singleShard() {
  const [srcIdx, dstIdx] = sameShardPair();

  const payload = JSON.stringify({
    txn_id: `txn-k6-ss-${__VU}-${__ITER}`,
    source: `user${srcIdx}`,
    destination: `user${dstIdx}`,
    amount: 1,
  });

  const res = http.post(`${BASE_URL}/submit`, payload, params);

  check(res, {
    'single-shard accepted': (r) => r.status === 200 || r.status === 202,
  });
  singleShardTxns.add(1);
  singleShardDuration.add(res.timings.duration);

  sleep(0.01);
}

export function crossShard() {
  const [srcIdx, dstIdx] = crossShardPair();

  const payload = JSON.stringify({
    txn_id: `txn-k6-cs-${__VU}-${__ITER}`,
    source: `user${srcIdx}`,
    destination: `user${dstIdx}`,
    amount: 1,
  });

  const res = http.post(`${BASE_URL}/submit`, payload, params);

  check(res, {
    'cross-shard accepted': (r) => r.status === 200 || r.status === 202,
  });
  crossShardTxns.add(1);
  crossShardDuration.add(res.timings.duration);

  sleep(0.01);
}

export function handleSummary(data) {
  const ss = data.metrics.single_shard_duration;
  const cs = data.metrics.cross_shard_duration;
  const reqs = data.metrics.http_reqs;
  const dur = data.metrics.http_req_duration;

  const summary = {
    scenario: SCENARIO,
    single_shard: {
      count: ss ? ss.values.count : 0,
      p50: ss ? ss.values['p(50)'] : 0,
      p95: ss ? ss.values['p(95)'] : 0,
      p99: ss ? ss.values['p(99)'] : 0,
      avg: ss ? ss.values.avg : 0,
    },
    cross_shard: {
      count: cs ? cs.values.count : 0,
      p50: cs ? cs.values['p(50)'] : 0,
      p95: cs ? cs.values['p(95)'] : 0,
      p99: cs ? cs.values['p(99)'] : 0,
      avg: cs ? cs.values.avg : 0,
    },
    total_requests: reqs ? reqs.values.count : 0,
    tps: reqs ? reqs.values.rate : 0,
    error_rate: data.metrics.http_req_failed ? data.metrics.http_req_failed.values.rate : 0,
    overall_p50: dur ? dur.values['p(50)'] : 0,
    overall_p95: dur ? dur.values['p(95)'] : 0,
    overall_p99: dur ? dur.values['p(99)'] : 0,
  };

  const outFile = __ENV.RESULTS_FILE || '/scripts/results.json';

  return {
    stdout: JSON.stringify(summary, null, 2) + '\n',
    [outFile]: JSON.stringify(summary, null, 2),
  };
}
