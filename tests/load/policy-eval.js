import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  scenarios: {
    constant_request_rate: {
      executor: 'constant-arrival-rate',
      rate: 10000,
      timeUnit: '1s',
      duration: '5m',
      preAllocatedVUs: 500,
      maxVUs: 1000,
    },
  },
  thresholds: {
    http_req_duration: ['p(99)<30'],  // spec SLO: 30ms p99
    errors: ['rate<0.01'],             // < 1% error rate
  },
};

export default function () {
  const payload = JSON.stringify({
    subject: "user:123",
    action: "task:read",
    resource: "task:456",
    context: {
      org_id: "org-1",
    }
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${__ENV.TEST_TOKEN}`,
    },
  };

  const res = http.post(`${__ENV.BASE_URL}/v1/policy/evaluate`, payload, params);

  errorRate.add(res.status !== 200);
  check(res, {
    'status is 200': (r) => r.status === 200,
    'duration < 30ms': (r) => r.timings.duration < 30,
  });
}
