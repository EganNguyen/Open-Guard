import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  scenarios: {
    constant_request_rate: {
      executor: 'per-vu-iterations',
      vus: 10,
      iterations: 5,
      maxDuration: '5m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<30000'], // 30s completion threshold per spec
    errors: ['rate<0.05'],
  },
};

export default function () {
  const params = {
    headers: {
      'Authorization': `Bearer ${__ENV.TEST_TOKEN}`,
      'Content-Type': 'application/json',
    },
  };

  const payload = JSON.stringify({
    org_id: "org-1",
    framework: "SOC2",
    format: "PDF",
  });

  const res = http.post(`${__ENV.BASE_URL}/v1/compliance/reports/generate`, payload, params);

  errorRate.add(res.status !== 200 && res.status !== 202);
  check(res, {
    'status is 200 or 202': (r) => r.status === 200 || r.status === 202,
    'duration < 30s': (r) => r.timings.duration < 30000,
  });

  sleep(10);
}
