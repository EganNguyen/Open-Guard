import { describe, it, expect, beforeEach } from 'vitest';
import { PayloadSizeDetector } from '../PayloadSizeDetector';
import { GuardRequest, DetectorConfig, DetectorKind, GuardAction } from '@open-guard/core';

function createMockStore() {
  return {
    get: async () => null,
    set: async () => {},
    incr: async () => 1,
    del: async () => {},
  };
}

const createRequest = (overrides: Partial<GuardRequest> = {}): GuardRequest => ({
  id: 'test-id',
  ip: '192.168.1.1',
  method: 'GET',
  path: '/api/test',
  headers: { 'user-agent': 'test' },
  timestamp: Date.now(),
  contentLength: 1000,
  ...overrides,
});

const createConfig = (options: Record<string, unknown> = {}): DetectorConfig => ({
  id: 'payload-size',
  kind: DetectorKind.PAYLOAD_SIZE,
  enabled: true,
  priority: 30,
  options,
});

describe('PayloadSizeDetector', () => {
  let store: ReturnType<typeof createMockStore>;

  beforeEach(() => {
    store = createMockStore();
  });

  it('allows payload under limit', async () => {
    const detector = new PayloadSizeDetector(createConfig({ maxSizeBytes: 10000 }), store);
    const request = createRequest({ contentLength: 5000 });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
  });

  it('blocks payload over limit', async () => {
    const detector = new PayloadSizeDetector(createConfig({ maxSizeBytes: 10000 }), store);
    const request = createRequest({ contentLength: 15000 });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.BLOCK);
  });

  it('allows missing content-length', async () => {
    const detector = new PayloadSizeDetector(createConfig({ maxSizeBytes: 10000 }), store);
    const request = createRequest({ contentLength: undefined });
    delete request.contentLength;
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
  });

  it('respects threshold option', async () => {
    const detector = new PayloadSizeDetector(createConfig({ maxSizeBytes: 5000, logThresholdPercent: 50 }), store);
    const request = createRequest({ contentLength: 3000 });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.LOG_ONLY);
  });
});