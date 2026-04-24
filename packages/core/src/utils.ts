import {
  GuardRequest,
  GuardResponse,
  GuardAction,
  ThreatLevel,
  DetectorResult,
  DetectorKind,
  GeoInfo,
} from './types';

export function normalizeRequest(
  raw: unknown,
  platform: 'express' | 'fetch' | 'node'
): GuardRequest {
  switch (platform) {
    case 'express': {
      const req = raw as {
        headers: Record<string, string | string[] | undefined>;
        ip?: string;
        method: string;
        path: string;
        query?: Record<string, string | string[]>;
        body?: unknown;
        params?: Record<string, string>;
        url?: string;
        session?: { id?: string; userId?: string };
        user?: { id?: string };
      };
      return {
        id: generateRequestId(),
        ip: req.ip || 'unknown',
        method: (req.method?.toUpperCase() || 'GET') as GuardRequest['method'],
        path: req.path || req.url?.split('?')[0] || '/',
        query: req.query || {},
        headers: normalizeHeaders(req.headers),
        body: req.body,
        timestamp: Date.now(),
        userId: req.session?.userId || req.user?.id,
        sessionId: req.session?.id,
      };
    }
    case 'fetch': {
      const req = raw as {
        headers: Headers;
        ip?: string;
        method: string;
        url: string;
      };
      const url = new URL(req.url);
      return {
        id: generateRequestId(),
        ip: req.ip || 'unknown',
        method: (req.method?.toUpperCase() || 'GET') as GuardRequest['method'],
        path: url.pathname,
        query: Object.fromEntries(url.searchParams.entries()),
        headers: Object.fromEntries(req.headers.entries()),
        timestamp: Date.now(),
      };
    }
    case 'node': {
      const req = raw as {
        headers: Record<string, string | string[] | undefined>;
        socket?: { remoteAddress?: string };
        method: string;
        url?: string;
      };
      const url = new URL(req.url || '/', 'http://localhost');
      return {
        id: generateRequestId(),
        ip: req.socket?.remoteAddress || 'unknown',
        method: (req.method?.toUpperCase() || 'GET') as GuardRequest['method'],
        path: url.pathname,
        query: Object.fromEntries(url.searchParams.entries()),
        headers: normalizeHeaders(req.headers),
        timestamp: Date.now(),
      };
    }
    default:
      throw new Error(`Unknown platform: ${platform}`);
  }
}

function normalizeHeaders(
  headers: Record<string, string | string[] | undefined> | Headers
): Record<string, string> {
  if (headers instanceof Headers) {
    return Object.fromEntries(headers.entries());
  }
  const result: Record<string, string> = {};
  for (const [key, value] of Object.entries(headers)) {
    if (Array.isArray(value)) {
      result[key.toLowerCase()] = value.join(', ');
    } else if (value !== undefined) {
      result[key.toLowerCase()] = value;
    }
  }
  return result;
}

export function scoreToThreatLevel(score: number): ThreatLevel {
  if (score >= 0.9) return ThreatLevel.CRITICAL;
  if (score >= 0.7) return ThreatLevel.HIGH;
  if (score >= 0.5) return ThreatLevel.MEDIUM;
  if (score >= 0.2) return ThreatLevel.LOW;
  return ThreatLevel.NONE;
}

const ACTION_PRIORITY: Record<GuardAction, number> = {
  [GuardAction.BLOCK]: 5,
  [GuardAction.CHALLENGE]: 4,
  [GuardAction.RATE_LIMIT]: 3,
  [GuardAction.LOG_ONLY]: 2,
  [GuardAction.ALLOW]: 1,
};

export function mergeResults(
  results: DetectorResult[],
  requestId: string
): GuardResponse {
  if (results.length === 0) {
    return {
      requestId,
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      results: [],
      durationMs: 0,
    };
  }

  const sortedByThreat = [...results].sort(
    (a, b) => b.threatLevel.localeCompare(a.threatLevel) || b.score - a.score
  );

  const highestThreatLevel = sortedByThreat[0].threatLevel;

  const sortedByAction = [...results].sort(
    (a, b) => ACTION_PRIORITY[b.action] - ACTION_PRIORITY[a.action]
  );
  const mostRestrictiveAction = sortedByAction[0].action;

  const totalDurationMs = results.reduce(
    (sum, r) => sum + (r.durationMs || 0),
    0
  );

  const blockedBy = results.find((r) => r.action === GuardAction.BLOCK)?.detectorId;

  return {
    requestId,
    action: mostRestrictiveAction,
    threatLevel: highestThreatLevel,
    results,
    durationMs: totalDurationMs,
    blockedBy,
  };
}

export function generateRequestId(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

export function extractStringValues(obj: unknown, prefix = ''): string[] {
  const values: string[] = [];

  function traverse(value: unknown, key: string): void {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value === null || value === undefined) return;

    if (typeof value === 'string') {
      values.push(value);
    } else if (typeof value === 'number' || typeof value === 'boolean') {
      values.push(String(value));
    } else if (Array.isArray(value)) {
      value.forEach((item, i) => traverse(item, `${i}`));
    } else if (typeof value === 'object') {
      for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
        traverse(v, k);
      }
    }
  }

  traverse(obj, '');
  return values;
}