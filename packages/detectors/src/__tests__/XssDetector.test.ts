import { describe, it, expect, beforeEach } from 'vitest';
import { XssDetector } from '../XssDetector';
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
  ...overrides,
});

const createConfig = (options: Record<string, unknown> = {}): DetectorConfig => ({
  id: 'xss',
  kind: DetectorKind.XSS,
  enabled: true,
  priority: 10,
  options,
});

describe('XssDetector', () => {
  let store: ReturnType<typeof createMockStore>;

  beforeEach(() => {
    store = createMockStore();
  });

  it('detects script tag', async () => {
    const detector = new XssDetector(createConfig(), store);
    const request = createRequest({ query: { search: '<script>alert(1)</script>' } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.BLOCK);
  });

  it('detects event handler', async () => {
    const detector = new XssDetector(createConfig(), store);
    const request = createRequest({ query: { img: '<img onerror=alert(1)>' } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.BLOCK);
  });

  it('detects javascript URI', async () => {
    const detector = new XssDetector(createConfig(), store);
    const request = createRequest({ query: { link: 'javascript:alert(1)' } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.BLOCK);
  });

  it('allows clean input', async () => {
    const detector = new XssDetector(createConfig(), store);
    const request = createRequest({ query: { search: 'hello world' } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
    expect(result.score).toBe(0);
  });

  it('detects template injection', async () => {
    const detector = new XssDetector(createConfig(), store);
    const request = createRequest({ query: { template: '{{exploit}}' } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.LOG_ONLY);
  });

  it('respects inspectBody option', async () => {
    const detector = new XssDetector(createConfig({ inspectBody: false }), store);
    const request = createRequest({ body: { xss: '<script>' } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
  });

  it('respects sensitivity option', async () => {
    const detector = new XssDetector(createConfig({ sensitivity: 'high' }), store);
    const request = createRequest({ query: { search: '<img onerror=x>' } });
    const result = await detector.evaluate(request);
    expect(result.score).toBeGreaterThan(0);
  });
});