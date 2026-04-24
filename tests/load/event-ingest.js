import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  scenarios: {
    constant_request_rate: {
      executor: 'constant-arrival-rate',
      rate: 20000,
      timeUnit: '1s',
      duration: '5m',
      preAllocatedVUs: 1000,
      maxVUs: 2000,
    },
  },
  thresholds: {
    http_req_duration: ['p(99)<50'],  // spec SLO: 50ms p99
    errors: ['rate<0.01'],             // < 1% error rate
  },
};

export default function () {
  const payload = JSON.stringify({
    event_type: "auth.login.success",
    org_id: "org-1",
    actor: {
      id: "user:123",
      type: "user",
    },
    action: "login",
    resource: "iam:session",
    metadata: {
      ip: "1.2.3.4",
      user_agent: "k6-load-test",
    },
    timestamp: new Date().toISOString(),
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'X-API-Key': __ENV.API_KEY || 'test-api-key',
    },
  };

  const res = http.post(`${__ENV.BASE_URL}/v1/events/ingest`, payload, params);

  errorRate.add(res.status !== 200 && res.status !== 202);
  check(res, {
    'status is 200 or 202': (r) => r.status === 200 || r.status === 202,
    'duration < 50ms': (r) => r.timings.duration < 50,
  });
}
