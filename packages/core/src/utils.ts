import { v4 as uuidv4 } from 'uuid';
import type { GuardRequest, GuardResponse, DetectorResult, GuardAction, ThreatLevel, HttpMethod } from './types.js';
import { GuardAction, ThreatLevel } from './types.js';

export function generateRequestId(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  return uuidv4();
}

export function normalizeRequest(raw: unknown, platform: 'express' | 'fetch' | 'node'): GuardRequest {
  const id = generateRequestId();
  const timestamp = Date.now();

  if (platform === 'express') {
    return normalizeExpressRequest(raw as Record<string, unknown>, id, timestamp);
  } else if (platform === 'fetch') {
    return normalizeFetchRequest(raw as Request, id, timestamp);
  } else {
    return normalizeNodeRequest(raw as Record<string, unknown>, id, timestamp);
  }
}

function normalizeExpressRequest(req: Record<string, unknown>, id: string, timestamp: number): GuardRequest {
  const headers: Record<string, string> = {};
  const reqHeaders = req.headers as Record<string, unknown> | undefined;
  if (reqHeaders) {
    for (const [key, value] of Object.entries(reqHeaders)) {
      headers[key] = String(value);
    }
  }

  return {
    id,
    ip: (req.ip as string) || (req.connection as Record<string, unknown>)?.remoteAddress as string || 'unknown',
    method: (req.method as HttpMethod) || 'GET',
    path: req.path as string || '/',
    query: req.query as Record<string, string | string[]> | undefined,
    headers,
    body: req.body,
    timestamp,
    userId: req.userId as string | undefined,
    sessionId: req.sessionId as string | undefined,
  };
}

function normalizeFetchRequest(req: Request, id: string, timestamp: number): GuardRequest {
  const headers: Record<string, string> = {};
  req.headers.forEach((value, key) => {
    headers[key] = value;
  });

  return {
    id,
    ip: 'unknown',
    method: req.method as HttpMethod,
    path: new URL(req.url).pathname,
    query: Object.fromEntries(new URL(req.url).searchParams) as Record<string, string>,
    headers,
    timestamp,
  };
}

function normalizeNodeRequest(req: Record<string, unknown>, id: string, timestamp: number): GuardRequest {
  const headers: Record<string, string | string[] | undefined> = (req.headers as Record<string, unknown>) || {};
  const normalizedHeaders: Record<string, string> = {};

  for (const [key, value] of Object.entries(headers)) {
    if (Array.isArray(value)) {
      normalizedHeaders[key] = value.join(', ');
    } else if (value !== undefined) {
      normalizedHeaders[key] = String(value);
    }
  }

  return {
    id,
    ip: ((req.socket as Record<string, unknown>)?.remoteAddress as string) || 'unknown',
    method: (req.method as HttpMethod) || 'GET',
    path: req.url as string || '/',
    headers: normalizedHeaders,
    body: req.body,
    timestamp,
  };
}

export function scoreToThreatLevel(score: number): ThreatLevel {
  if (score >= 0.9) return ThreatLevel.CRITICAL;
  if (score >= 0.7) return ThreatLevel.HIGH;
  if (score >= 0.5) return ThreatLevel.MEDIUM;
  if (score >= 0.2) return ThreatLevel.LOW;
  return ThreatLevel.NONE;
}

const ACTION_RANK: Record<GuardAction, number> = {
  [GuardAction.BLOCK]: 5,
  [GuardAction.CHALLENGE]: 4,
  [GuardAction.RATE_LIMIT]: 3,
  [GuardAction.LOG_ONLY]: 2,
  [GuardAction.ALLOW]: 1,
};

const THREAT_RANK: Record<ThreatLevel, number> = {
  [ThreatLevel.CRITICAL]: 5,
  [ThreatLevel.HIGH]: 4,
  [ThreatLevel.MEDIUM]: 3,
  [ThreatLevel.LOW]: 2,
  [ThreatLevel.NONE]: 1,
};

export function mergeResults(results: DetectorResult[], requestId: string): GuardResponse {
  let action: GuardAction = GuardAction.ALLOW;
  let threatLevel: ThreatLevel = ThreatLevel.NONE;
  let blockedBy: string | undefined;

  let highestThreatRank = 0;
  let highestActionRank = 0;

  for (const result of results) {
    const threatRank = THREAT_RANK[result.threatLevel];
    const actionRank = ACTION_RANK[result.action];

    if (threatRank > highestThreatRank || (threatRank === highestThreatRank && actionRank > highestActionRank)) {
      highestThreatRank = threatRank;
      highestActionRank = actionRank;
      threatLevel = result.threatLevel;
      action = result.action;
      blockedBy = result.detectorId;
    }
  }

  const durationMs = results.reduce((sum, r) => sum + (r.durationMs || 0), 0);

  return {
    requestId,
    action,
    threatLevel,
    results,
    durationMs,
    blockedBy,
  };
}