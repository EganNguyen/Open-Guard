// policy-evaluate.js — k6 load test for POST /policies/evaluate
// Phase 2 Acceptance Criteria: p99 < 30ms under 500 concurrent requests.
//
// Usage:
//   k6 run loadtest/policy-evaluate.js
//
// Requires env var:
//   POLICY_URL  — base URL of the policy service (default: http://localhost:8082)

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Rate } from 'k6/metrics';

const evaluateLatency = new Trend('evaluate_latency', true);
const errorRate = new Rate('error_rate');

export const options = {
  stages: [
    { duration: '30s', target: 100 },  // Ramp up to 100 VUs
    { duration: '1m',  target: 500 },  // Sustain 500 VUs
    { duration: '30s', target: 0   },  // Ramp down
  ],
  thresholds: {
    // p99 must be under 30ms (Phase 2 SLO)
    'http_req_duration{name:evaluate}': ['p(99)<30'],
    'evaluate_latency': ['p(99)<30'],
    'error_rate': ['rate<0.01'], // < 1% errors
  },
};

const BASE_URL = __ENV.POLICY_URL || 'http://localhost:8082';

const evalPayload = JSON.stringify({
  org_id:   '00000000-0000-0000-0000-000000000001',
  user_id:  '00000000-0000-0000-0000-000000000002',
  action:   'data.read',
  resource: '/api/v1/reports',
  user_groups: ['viewers'],
  ip_address: '203.0.113.1',
});

const params = {
  headers: {
    'Content-Type': 'application/json',
    'X-Org-ID':  '00000000-0000-0000-0000-000000000001',
    'X-User-ID': '00000000-0000-0000-0000-000000000002',
  },
  tags: { name: 'evaluate' },
};

export default function () {
  const res = http.post(`${BASE_URL}/policies/evaluate`, evalPayload, params);

  evaluateLatency.add(res.timings.duration);
  errorRate.add(res.status !== 200 && res.status !== 403);

  check(res, {
    'status is 200 or 403': (r) => r.status === 200 || r.status === 403,
    'has permitted field': (r) => JSON.parse(r.body).permitted !== undefined,
  });

  sleep(0.01); // 10ms think time
}
