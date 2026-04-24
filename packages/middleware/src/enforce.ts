import type { Response, NextFunction } from 'express';
import type { GuardResponse, GuardRequest } from '@open-guard/core/types';
import { GuardAction } from '@open-guard/core/types';

export function enforceAction(
  response: GuardResponse,
  req: GuardRequest,
  res: Response,
  next: NextFunction
): void {
  res.setHeader('X-Guard-Request-Id', response.requestId);

  switch (response.action) {
    case GuardAction.ALLOW:
      next();
      break;

    case GuardAction.BLOCK:
      res.setHeader('X-Guard-Blocked-By', response.blockedBy || 'unknown');
      res.status(403).json({
        error: 'Blocked by OpenGuard',
        requestId: response.requestId,
        reason: response.results.find((r) => r.action === GuardAction.BLOCK)?.reason,
      });
      break;

    case GuardAction.CHALLENGE:
      res.status(429).json({
        error: 'Challenge required',
        challengeToken: response.results.find((r) => r.action === GuardAction.CHALLENGE)?.metadata?.challengeToken,
      });
      break;

    case GuardAction.RATE_LIMIT:
      const rateLimitResult = response.results.find((r) => r.action === GuardAction.RATE_LIMIT);
      const ttl = (rateLimitResult?.metadata?.resetAt as number) || 0;
      const limit = (rateLimitResult?.metadata?.limit as number) || 0;

      res.setHeader('Retry-After', String(Math.max(0, Math.ceil((ttl - Date.now()) / 1000))));
      res.setHeader('X-RateLimit-Limit', String(limit));
      res.setHeader('X-RateLimit-Remaining', '0');
      res.setHeader('X-RateLimit-Reset', String(Math.ceil(ttl / 1000)));

      res.status(429).json({
        error: 'Rate limit exceeded',
        requestId: response.requestId,
        retryAfter: Math.ceil((ttl - Date.now()) / 1000),
      });
      break;

    case GuardAction.LOG_ONLY:
    default:
      next();
      break;
  }
}