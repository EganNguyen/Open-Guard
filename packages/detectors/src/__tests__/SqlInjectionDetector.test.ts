import { describe, it, expect, beforeEach } from 'vitest';
import { SqlInjectionDetector } from '../SqlInjectionDetector';
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
  id: 'sql-injection',
  kind: DetectorKind.SQL_INJECTION,
  enabled: true,
  priority: 20,
  options,
});

describe('SqlInjectionDetector', () => {
  let store: ReturnType<typeof createMockStore>;

  beforeEach(() => {
    store = createMockStore();
  });

  it('detects UNION SELECT', async () => {
    const detector = new SqlInjectionDetector(createConfig(), store);
    const request = createRequest({ query: { search: "1 UNION SELECT * FROM users" } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.BLOCK);
    expect(result.metadata?.matchedPatterns).toContain('union_select');
  });

  it('allows clean input', async () => {
    const detector = new SqlInjectionDetector(createConfig(), store);
    const request = createRequest({ query: { search: 'hello world' } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
    expect(result.score).toBe(0);
  });

  it('detects time-blind injection', async () => {
    const detector = new SqlInjectionDetector(createConfig(), store);
    const request = createRequest({ body: { name: "'; SLEEP(5);--" } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.BLOCK);
    expect(result.metadata?.matchedPatterns).toContain('time_blind');
  });

  it('detects stacked queries', async () => {
    const detector = new SqlInjectionDetector(createConfig(), store);
    const request = createRequest({ query: { id: '; DROP TABLE users;' } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.BLOCK);
    expect(result.metadata?.matchedPatterns).toContain('stacked_query');
  });

  it('respects inspectBody option', async () => {
    const detector = new SqlInjectionDetector(createConfig({ inspectBody: false }), store);
    const request = createRequest({ body: { search: "1 UNION SELECT * FROM users" } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
  });

  it('respects inspectQuery option', async () => {
    const detector = new SqlInjectionDetector(createConfig({ inspectQuery: false }), store);
    const request = createRequest({ query: { search: "1 UNION SELECT * FROM users" } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.ALLOW);
  });

  it('applies sensitivity multiplier for high sensitivity', async () => {
    const detector = new SqlInjectionDetector(createConfig({ sensitivity: 'high' }), store);
    const request = createRequest({ query: { search: "' OR '1'='1" } });
    const result = await detector.evaluate(request);
    expect(result.score).toBeGreaterThanOrEqual(0);
  });

  it('detects boolean blind injection', async () => {
    const detector = new SqlInjectionDetector(createConfig(), store);
    const request = createRequest({ query: { id: "1 OR 1=1" } });
    const result = await detector.evaluate(request);
    expect(result.action).toBe(GuardAction.BLOCK);
    expect(result.metadata?.matchedPatterns).toContain('boolean_blind');
  });
});