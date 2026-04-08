import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Trend } from 'k6/metrics';
import { sha256 } from 'k6/crypto';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const AUTH_TOKEN = __ENV.AUTH_TOKEN || 'dummy-token';
const NUM_ACCOUNTS = parseInt(__ENV.NUM_ACCOUNTS || '1000');
const LOAD_VUS = parseInt(__ENV.LOAD_VUS || '50');
const LOAD_DURATION = __ENV.LOAD_DURATION || '30s';
const SCENARIO = __ENV.SCENARIO || 'mixed'; // single_shard, cross_shard, or mixed
const NUM_PARTITIONS = 30;
const POW32_MOD_N = Math.pow(2, 32) % NUM_PARTITIONS; // = 16

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
    'single_shard_duration': ['p(99)<2000'],  // 2 seconds for single-shard at 50 VUs
    'cross_shard_duration': ['p(99)<3000'],   // 3 seconds for cross-shard at 50 VUs
    'http_req_failed': ['rate<0.05'],         // <5% error rate
  },
};

const params = {
  headers: {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${AUTH_TOKEN}`,
  },
};

// Replicate Go's SHA-256 based partition mapping:
//   partition = binary.BigEndian.Uint64(sha256(accountID)[:8]) % NUM_PARTITIONS
// Since JS can't handle uint64, we use modular arithmetic:
//   val = hi * 2^32 + lo  →  val % N = ((hi%N * (2^32%N)) % N + lo%N) % N
function getPartition(accountID) {
  const hash = sha256(accountID, 'hex');
  const hi = parseInt(hash.substring(0, 8), 16);
  const lo = parseInt(hash.substring(8, 16), 16);
  return ((hi % NUM_PARTITIONS) * POW32_MOD_N + lo % NUM_PARTITIONS) % NUM_PARTITIONS;
}

function getShardIdx(partition) {
  const perShard = NUM_PARTITIONS / 3;
  return Math.floor(partition / perShard);
}

// Pre-compute shard assignments for all accounts
const shardUsers = [[], [], []]; // shard1, shard2, shard3
for (let i = 0; i < NUM_ACCOUNTS; i++) {
  const partition = getPartition(`user${i}`);
  const idx = getShardIdx(partition);
  shardUsers[idx].push(i);
}

// Pick two accounts that hash to the same shard (verified via SHA-256 partition mapping)
function sameShardPair() {
  const shardIdx = Math.floor(Math.random() * 3);
  const users = shardUsers[shardIdx];
  if (users.length < 2) return [0, 1]; // fallback
  const src = users[Math.floor(Math.random() * users.length)];
  let dst = users[Math.floor(Math.random() * users.length)];
  while (dst === src) dst = users[Math.floor(Math.random() * users.length)];
  return [src, dst];
}

// Pick two accounts that hash to different shards
function crossShardPair() {
  const shard1 = Math.floor(Math.random() * 3);
  let shard2 = (shard1 + 1 + Math.floor(Math.random() * 2)) % 3;
  const users1 = shardUsers[shard1];
  const users2 = shardUsers[shard2];
  if (users1.length === 0 || users2.length === 0) return [0, Math.floor(NUM_ACCOUNTS / 2)];
  const src = users1[Math.floor(Math.random() * users1.length)];
  const dst = users2[Math.floor(Math.random() * users2.length)];
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
