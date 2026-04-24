import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  scenarios: {
    constant_request_rate: {
      executor: 'constant-arrival-rate',
      rate: 1000,
      timeUnit: '1s',
      duration: '5m',
      preAllocatedVUs: 100,
      maxVUs: 200,
    },
  },
  thresholds: {
    http_req_duration: ['p(99)<100'], // spec SLO: 100ms p99
    errors: ['rate<0.01'],             // < 1% error rate
  },
};

export default function () {
  const params = {
    headers: {
      'Authorization': `Bearer ${__ENV.TEST_TOKEN}`,
    },
  };

  const res = http.get(`${__ENV.BASE_URL}/audit/events?limit=50`, params);

  errorRate.add(res.status !== 200);
  check(res, {
    'status is 200': (r) => r.status === 200,
    'duration < 100ms': (r) => r.timings.duration < 100,
  });

  sleep(0.5);
}
