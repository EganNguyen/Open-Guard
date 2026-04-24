import { describe, it, expect, beforeEach } from 'vitest';
import { RateLimitDetector } from '../RateLimitDetector';
import { GuardRequest, DetectorConfig, DetectorKind, GuardAction } from '@open-guard/core';

interface MockStore {
  data: Map<string, { value: number; expiry?: number }>;
}

function createMockStore(): MockStore & { get: (key: string) => Promise<string | null>; set: (key: string, value: string, ttl?: number) => Promise<void>; incr: (key: string, ttl?: number) => Promise<number>; del: (key: string) => Promise<void> } {
  const store: MockStore = { data: new Map() };
  return {
    ...store,
    get: async (key: string) => {
      const entry = store.data.get(key);
      if (!entry) return null;
      if (entry.expiry && entry.expiry < Date.now()) {
        store.data.delete(key);
        return null;
      }
      return String(entry.value);
    },
    set: async (key: string, value: string, ttl?: number) => {
      store.data.set(key, {
        value: parseInt(value, 10),
        expiry: ttl ? Date.now() + ttl * 1000 : undefined,
      });
    },
    incr: async (key: string, ttl?: number) => {
      const existing = store.data.get(key);
      const newValue = existing ? existing.value + 1 : 1;
      store.data.set(key, {
        value: newValue,
        expiry: ttl ? Date.now() + ttl * 1000 : undefined,
      });
      return newValue;
    },
    del: async (key: string) => {
      store.data.delete(key);
    },
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
  id: 'rate-limiter',
  kind: DetectorKind.RATE_LIMIT,
  enabled: true,
  priority: 10,
  options,
});

describe('RateLimitDetector', () => {
  let store: ReturnType<typeof createMockStore>;

  beforeEach(() => {
    store = createMockStore();
  });

  it('allows requests under limit', async () => {
    const detector = new RateLimitDetector(createConfig({ maxRequests: 100 }), store);
    const request = createRequest();
    await store.set('og:rl:rate-limiter:192.168.1.1', '50', 60);
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
    expect(result.score).toBeLessThan(0.8);
  });

  it('logs warning at 80%', async () => {
    const detector = new RateLimitDetector(createConfig({ maxRequests: 100, warnAt: 0.8 }), store);
    const request = createRequest();
    await store.set('og:rl:rate-limiter:192.168.1.1', '82', 60);
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.LOG_ONLY);
    expect(result.score).toBeGreaterThanOrEqual(0.8);
    expect(result.score).toBeLessThan(1.0);
  });

  it('blocks at limit', async () => {
    const detector = new RateLimitDetector(createConfig({ maxRequests: 100 }), store);
    const request = createRequest();
    await store.set('og:rl:rate-limiter:192.168.1.1', '101', 60);
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.RATE_LIMIT);
    expect(result.score).toBe(1.0);
  });

  it('skips configured paths', async () => {
    const detector = new RateLimitDetector(createConfig({ skipPaths: ['/health'] }), store);
    const request = createRequest({ path: '/health' });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
    expect(result.reason).toContain('skipped');
  });

  it('respects IP allowlist', async () => {
    const detector = new RateLimitDetector(createConfig({ skipIps: ['192.168.1.1'] }), store);
    const request = createRequest({ ip: '192.168.1.1' });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
  });

  it('uses userId in key when configured', async () => {
    const detector = new RateLimitDetector(createConfig({ keyBy: ['ip', 'userId'], maxRequests: 5 }), store);
    const request = createRequest({ userId: 'user123' });
    await store.set('og:rl:rate-limiter:192.168.1.1:user123', '3', 60);
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
  });

  it('handles store errors gracefully', async () => {
    const failingStore = {
      ...store,
      incr: async () => { throw new Error('Store error'); },
    };
    const detector = new RateLimitDetector(createConfig(), failingStore);
    const request = createRequest();
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
  });
});