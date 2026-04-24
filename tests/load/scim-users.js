import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  scenarios: {
    constant_request_rate: {
      executor: 'constant-arrival-rate',
      rate: 500,
      timeUnit: '1s',
      duration: '5m',
      preAllocatedVUs: 50,
      maxVUs: 100,
    },
  },
  thresholds: {
    http_req_duration: ['p(99)<500'], // spec SLO: 500ms p99
    errors: ['rate<0.01'],             // < 1% error rate
  },
};

export default function () {
  const params = {
    headers: {
      'Authorization': `Bearer ${__ENV.SCIM_TOKEN}`,
      'Accept': 'application/scim+json',
    },
  };

  const res = http.get(`${__ENV.BASE_URL}/v1/scim/v2/Users?startIndex=1&count=10`, params);

  errorRate.add(res.status !== 200);
  check(res, {
    'status is 200': (r) => r.status === 200,
    'duration < 500ms': (r) => r.timings.duration < 500,
  });

  sleep(1);
}
