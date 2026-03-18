import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8000';
const AUTH_TOKEN = __ENV.AUTH_TOKEN || 'dummy-token';

export const options = {
  vus: 100,
  duration: '30s',
  thresholds: {
    'http_req_duration': ['p(99)<500'], // 99% of requests must complete below 500ms
    'http_req_failed': ['rate<0.01'],   // error rate must be less than 1%
  },
};

export default function () {
  const srcIdx = Math.floor(Math.random() * 100);
  let dstIdx = Math.floor(Math.random() * 100);
  if (srcIdx === dstIdx) { dstIdx = (dstIdx + 1) % 100; }

  const payload = JSON.stringify({
    TxnID: `txn-k6-${__VU}-${__ITER}`,
    Source: `user${srcIdx}`,
    Destination: `user${dstIdx}`,
    Amount: Math.floor(Math.random() * 100) + 1,
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${AUTH_TOKEN}`,
    },
  };

  const res = http.post(`${BASE_URL}/submit`, payload, params);

  check(res, {
    'is status 202 Accepted': (r) => r.status === 202,
  });

  // Short sleep to throttle slightly if needed
  sleep(0.01);
}
