import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  scenarios: {
    constant_request_rate: {
      executor: 'constant-arrival-rate',
      rate: 2000,
      timeUnit: '1s',
      duration: '5m',
      preAllocatedVUs: 200,
      maxVUs: 400,
    },
  },
  thresholds: {
    http_req_duration: ['p(99)<150'],  // spec SLO: 150ms p99
    errors: ['rate<0.01'],              // < 1% error rate
  },
};

export default function () {
  const payload = JSON.stringify({
    email: `user-${Math.floor(Math.random() * 1000)}@test.com`,
    password: 'testpassword',
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
    },
  };

  const res = http.post(`${__ENV.BASE_URL}/auth/login`, payload, params);

  errorRate.add(res.status !== 200 && res.status !== 401);
  check(res, {
    'status is 200 or 401': (r) => r.status === 200 || r.status === 401,
    'duration < 150ms': (r) => r.timings.duration < 150,
  });

  sleep(0.1);
}
