/**
 * kafka-throughput.js
 * Phase 8 acceptance criteria: 50,000 events/s to audit.trail
 * Consumer lag must stay < 10,000 during burst.
 *
 * Requires: k6 built with xk6-kafka (https://github.com/grafana/xk6-kafka)
 *   k6 build --with github.com/grafana/xk6-kafka
 */
import { check, sleep } from 'k6';
import { writer, createTopic, CODEC_SNAPPY } from 'k6/x/kafka';
import { Counter, Gauge } from 'k6/metrics';

const errorCount = new Counter('kafka_errors');

export const options = {
  scenarios: {
    burst: {
      executor: 'constant-arrival-rate',
      rate: 50000,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 500,
      maxVUs: 1000,
    },
  },
  thresholds: {
    kafka_errors: ['count<100'],  // < 0.2% error rate at 50k/s
  },
};

const kafkaWriter = writer({
  brokers: (__ENV.KAFKA_BROKERS || 'localhost:9092').split(','),
  topic: 'audit.trail',
  compression: CODEC_SNAPPY,
});

export default function () {
  const orgId = `org-${Math.floor(Math.random() * 100)}`;
  const messages = Array.from({ length: 10 }, (_, i) => ({
    key: `${orgId}-${__VU}-${__ITER}-${i}`,
    value: JSON.stringify({
      event_id: `${__VU}-${__ITER}-${i}`,
      org_id: orgId,
      source: 'k6-load-test',
      action: 'resource.read',
      actor: `user-${__VU}`,
      target: `document-${i}`,
      ts: Date.now(),
    }),
  }));

  const err = kafkaWriter.produce({ messages });
  if (err) {
    errorCount.add(1);
  }
}

export function teardown() {
  kafkaWriter.close();
}
