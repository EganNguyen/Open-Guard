import { BaseDetector, GuardRequest, DetectorResult, GuardAction, ThreatLevel, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind } from '@open-guard/core';

export interface AuthBruteForceOptions {
  watchPaths?: string[];
  watchMethods?: string[];
  maxAttemptsPerIp?: number;
  maxAttemptsPerUser?: number;
  windowSeconds?: number;
  lockoutSeconds?: number;
  progressiveLockout?: boolean;
  failureStatusCodes?: number[];
}

export class AuthBruteForceDetector extends BaseDetector {
  private options: Required<AuthBruteForceOptions>;
  private lockoutCounter: Map<string, number> = new Map();

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      watchPaths: (config.options?.watchPaths as string[]) || ['/auth/login', '/api/login', '/sign-in'],
      watchMethods: (config.options?.watchMethods as string[]) || ['POST'],
      maxAttemptsPerIp: (config.options?.maxAttemptsPerIp as number) || 10,
      maxAttemptsPerUser: (config.options?.maxAttemptsPerUser as number) || 5,
      windowSeconds: (config.options?.windowSeconds as number) || 300,
      lockoutSeconds: (config.options?.lockoutSeconds as number) || 900,
      progressiveLockout: (config.options?.progressiveLockout as boolean) || true,
      failureStatusCodes: (config.options?.failureStatusCodes as number[]) || [401, 403],
    };
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();

    if (!this.options.watchPaths.includes(req.path) || !this.options.watchMethods.includes(req.method)) {
      return this.buildResult({
        action: GuardAction.ALLOW,
        threatLevel: ThreatLevel.NONE,
        score: 0,
        reason: 'Path/method not monitored for brute force',
        metadata: { attemptsIp: 0, attemptsUser: 0, lockoutRemainingSeconds: 0 },
        durationMs: Date.now() - startTime,
      });
    }

    const ipKey = this.getStoreKey('bf', this.config.id, 'ip', req.ip);
    const ipAttempts = await this.store.incr(ipKey, this.options.windowSeconds);

    let attemptsUser = 0;
    if (req.userId) {
      const userKey = this.getStoreKey('bf', this.config.id, 'user', req.userId);
      attemptsUser = await this.store.incr(userKey, this.options.windowSeconds);
    }

    const lockoutKey = this.getStoreKey('bf', this.config.id, 'lockout', req.ip);
    const lockoutExpiry = await this.store.get(lockoutKey);
    const lockoutRemaining = lockoutExpiry ? parseInt(lockoutExpiry, 10) : 0;

    if (lockoutRemaining > 0) {
      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.HIGH,
        score: 1.0,
        reason: `IP locked out for ${lockoutRemaining} seconds`,
        metadata: { attemptsIp: ipAttempts, attemptsUser, lockoutRemainingSeconds: lockoutRemaining },
        durationMs: Date.now() - startTime,
      });
    }

    if (ipAttempts > this.options.maxAttemptsPerIp || attemptsUser > this.options.maxAttemptsPerUser) {
      const offenseKey = this.getStoreKey('bf', this.config.id, 'offenses', req.ip);
      const offenseCount = await this.store.incr(offenseKey, this.options.windowSeconds);
      let lockoutDuration = this.options.lockoutSeconds;

      if (this.options.progressiveLockout && offenseCount > 1) {
        lockoutDuration = this.options.lockoutSeconds * Math.pow(2, offenseCount - 1);
      }

      await this.store.set(lockoutKey, String(Math.floor(Date.now() / 1000 + lockoutDuration)), lockoutDuration);

      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.CRITICAL,
        score: 1.0,
        reason: `Brute force detected: ${ipAttempts} attempts from IP, ${attemptsUser} for user`,
        metadata: { attemptsIp: ipAttempts, attemptsUser, lockoutRemainingSeconds: lockoutDuration },
        durationMs: Date.now() - startTime,
      });
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score: 0,
      reason: 'No brute force detected',
      metadata: { attemptsIp: ipAttempts, attemptsUser, lockoutRemainingSeconds: 0 },
      durationMs: Date.now() - startTime,
    });
  }

  async handleFailedAuth(req: GuardRequest): Promise<void> {
    if (!this.options.watchPaths.includes(req.path) || !this.options.watchMethods.includes(req.method)) {
      return;
    }

    const ipKey = this.getStoreKey('bf', this.config.id, 'ip', req.ip);
    await this.store.incr(ipKey, this.options.windowSeconds);

    if (req.userId) {
      const userKey = this.getStoreKey('bf', this.config.id, 'user', req.userId);
      await this.store.incr(userKey, this.options.windowSeconds);
    }
  }
}

export const authBruteForceDetector = {
  kind: DetectorKind.AUTH_BRUTE_FORCE,
  priority: 15,
};