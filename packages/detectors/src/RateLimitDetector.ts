import { BaseDetector, GuardRequest, DetectorResult, GuardAction, ThreatLevel, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind, scoreToThreatLevel } from '@open-guard/core';

export interface RateLimitOptions {
  windowSeconds?: number;
  maxRequests?: number;
  keyBy?: ('ip' | 'userId' | 'sessionId')[];
  warnAt?: number;
  skipPaths?: string[];
  skipIps?: string[];
}

export class RateLimitDetector extends BaseDetector {
  private options: Required<RateLimitOptions>;

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      windowSeconds: (config.options?.windowSeconds as number) || 60,
      maxRequests: (config.options?.maxRequests as number) || 100,
      keyBy: (config.options?.keyBy as ('ip' | 'userId' | 'sessionId')[]) || ['ip'],
      warnAt: (config.options?.warnAt as number) || 0.8,
      skipPaths: (config.options?.skipPaths as string[]) || [],
      skipIps: (config.options?.skipIps as string[]) || [],
    };
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();

    if (this.shouldSkip(req)) {
      return this.buildResult({
        action: GuardAction.ALLOW,
        threatLevel: ThreatLevel.NONE,
        score: 0,
        reason: 'Request skipped (path or IP allowlisted)',
        durationMs: Date.now() - startTime,
      });
    }

    const key = this.buildKey(req);
    const counter = await this.store.incr(key, this.options.windowSeconds);

    const remaining = Math.max(0, this.options.maxRequests - counter);
    const score = counter / this.options.maxRequests;
    const resetAt = Date.now() + this.options.windowSeconds * 1000;

    if (counter > this.options.maxRequests) {
      return this.buildResult({
        action: GuardAction.RATE_LIMIT,
        threatLevel: ThreatLevel.HIGH,
        score: 1.0,
        reason: `Rate limit exceeded: ${counter} requests in ${this.options.windowSeconds}s window`,
        metadata: { remaining: 0, limit: this.options.maxRequests, resetAt, windowSeconds: this.options.windowSeconds },
        durationMs: Date.now() - startTime,
      });
    }

    if (counter > this.options.maxRequests * this.options.warnAt) {
      return this.buildResult({
        action: GuardAction.LOG_ONLY,
        threatLevel: scoreToThreatLevel(score),
        score,
        reason: `Rate limit warning: ${counter}/${this.options.maxRequests} requests`,
        metadata: { remaining, limit: this.options.maxRequests, resetAt, windowSeconds: this.options.windowSeconds },
        durationMs: Date.now() - startTime,
      });
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score: 0,
      reason: 'Request within rate limit',
      metadata: { remaining, limit: this.options.maxRequests, resetAt, windowSeconds: this.options.windowSeconds },
      durationMs: Date.now() - startTime,
    });
  }

  private shouldSkip(req: GuardRequest): boolean {
    if (this.options.skipPaths.some((p) => this.matchGlob(req.path, p))) {
      return true;
    }
    if (this.options.skipIps.some((ip) => this.matchGlob(req.ip, ip))) {
      return true;
    }
    return false;
  }

  private buildKey(req: GuardRequest): string {
    const parts = this.options.keyBy.map((dim) => {
      switch (dim) {
        case 'ip': return req.ip;
        case 'userId': return req.userId || 'anonymous';
        case 'sessionId': return req.sessionId || 'nosession';
      }
    });
    return this.getStoreKey('rl', this.config.id, ...parts);
  }

  private matchGlob(str: string, pattern: string): boolean {
    const regex = new RegExp(
      '^' + pattern.replace(/\*/g, '.*').replace(/\?/g, '.') + '$'
    );
    return regex.test(str);
  }
}

export const rateLimitDetector = {
  kind: DetectorKind.RATE_LIMIT,
  priority: 10,
};