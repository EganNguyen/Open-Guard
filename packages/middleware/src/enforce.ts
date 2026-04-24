import {
  GuardAction,
  GuardResponse,
  GuardRequest,
} from '@open-guard/core';
import { globalEventEmitter } from './events';

export interface EnforceOptions {
  request: GuardRequest;
  response: GuardResponse;
  res: {
    status: (code: number) => EnforceOptions['res'];
    setHeader: (name: string, value: string) => EnforceOptions['res'];
    json: (body: unknown) => EnforceOptions['res'];
    end: () => EnforceOptions['res'];
  };
  next: (err?: unknown) => void;
}

export function enforceAction(options: EnforceOptions): void {
  const { request, response, res, next } = options;

  res.setHeader('X-Guard-Request-Id', response.requestId);

  switch (response.action) {
    case GuardAction.ALLOW:
      next();
      break;

    case GuardAction.BLOCK:
      res.setHeader('X-Guard-Blocked-By', response.blockedBy || '');
      res.status(403).json({
        error: 'Blocked by OpenGuard',
        requestId: response.requestId,
        reason: response.results.find(r => r.action === GuardAction.BLOCK)?.reason || 'Request blocked',
      });
      globalEventEmitter.emitGuardBlock({ request, response });
      break;

    case GuardAction.CHALLENGE:
      const challengeToken = generateChallengeToken();
      res.status(429).json({
        error: 'Challenge required',
        challengeToken,
        requestId: response.requestId,
      });
      break;

    case GuardAction.RATE_LIMIT:
      const rateLimitMeta = response.results.find(r => r.action === GuardAction.RATE_LIMIT)?.metadata;
      const ttl = (rateLimitMeta?.resetAt as number)
        ? Math.ceil(((rateLimitMeta.resetAt as number) - Date.now()) / 1000)
        : 60;
      const limit = (rateLimitMeta?.limit as number) || 100;
      const remaining = (rateLimitMeta?.remaining as number) || 0;

      res.setHeader('Retry-After', String(Math.max(1, ttl)));
      res.setHeader('X-RateLimit-Limit', String(limit));
      res.setHeader('X-RateLimit-Remaining', String(remaining));
      res.setHeader('X-RateLimit-Reset', String(Math.floor(Date.now() / 1000 + ttl)));

      res.status(429).json({
        error: 'Rate limit exceeded',
        requestId: response.requestId,
        retryAfter: ttl,
      });
      globalEventEmitter.emitGuardRateLimit({ request, detectorId: response.blockedBy || 'unknown', ttl });
      break;

    case GuardAction.LOG_ONLY:
      next();
      break;

    default:
      next();
  }
}

function generateChallengeToken(): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  let token = '';
  for (let i = 0; i < 32; i++) {
    token += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return token;
}