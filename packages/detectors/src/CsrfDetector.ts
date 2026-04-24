import { BaseDetector, GuardRequest, DetectorResult, GuardAction, ThreatLevel, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind } from '@open-guard/core';

export interface CsrfOptions {
  methods?: string[];
  tokenHeader?: string;
  tokenCookie?: string;
  skipPaths?: string[];
  doubleSubmitCookie?: boolean;
}

export class CsrfDetector extends BaseDetector {
  private options: Required<CsrfOptions>;

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      methods: (config.options?.methods as string[]) || ['POST', 'PUT', 'PATCH', 'DELETE'],
      tokenHeader: (config.options?.tokenHeader as string) || 'x-csrf-token',
      tokenCookie: (config.options?.tokenCookie as string) || '_csrf',
      skipPaths: (config.options?.skipPaths as string[]) || ['/api/auth/login', '/api/webhooks/*'],
      doubleSubmitCookie: (config.options?.doubleSubmitCookie as boolean) || false,
    };
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();

    if (!this.options.methods.includes(req.method)) {
      return this.buildResult({
        action: GuardAction.ALLOW,
        threatLevel: ThreatLevel.NONE,
        score: 0,
        reason: 'Method not subject to CSRF check',
        metadata: { tokenPresent: false, tokenSource: 'none', validationMethod: 'none' },
        durationMs: Date.now() - startTime,
      });
    }

    if (this.shouldSkipPath(req.path)) {
      return this.buildResult({
        action: GuardAction.ALLOW,
        threatLevel: ThreatLevel.NONE,
        score: 0,
        reason: 'Path excluded from CSRF check',
        metadata: { tokenPresent: false, tokenSource: 'none', validationMethod: 'none' },
        durationMs: Date.now() - startTime,
      });
    }

    const headerToken = req.headers[this.options.tokenHeader.toLowerCase()];
    const cookieToken = req.headers['cookie']?.split(';')?.find(c => c.trim().startsWith(`${this.options.tokenCookie}=`))?.split('=')[1];

    if (!headerToken) {
      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.HIGH,
        score: 1.0,
        reason: 'CSRF token missing from headers',
        metadata: { tokenPresent: false, tokenSource: 'none', validationMethod: this.options.doubleSubmitCookie ? 'double_submit' : 'server_session' },
        durationMs: Date.now() - startTime,
      });
    }

    if (this.options.doubleSubmitCookie) {
      if (!cookieToken || !this.constantTimeCompare(headerToken, cookieToken)) {
        return this.buildResult({
          action: GuardAction.BLOCK,
          threatLevel: ThreatLevel.HIGH,
          score: 1.0,
          reason: 'CSRF token mismatch (double-submit cookie)',
          metadata: { tokenPresent: true, tokenSource: 'header', validationMethod: 'double_submit' },
          durationMs: Date.now() - startTime,
        });
      }
    } else {
      const sessionId = req.sessionId;
      if (!sessionId) {
        return this.buildResult({
          action: GuardAction.BLOCK,
          threatLevel: ThreatLevel.HIGH,
          score: 1.0,
          reason: 'No session ID for server-side CSRF validation',
          metadata: { tokenPresent: true, tokenSource: 'header', validationMethod: 'server_session' },
          durationMs: Date.now() - startTime,
        });
      }

      const storedToken = await this.store.get(this.getStoreKey('csrf', sessionId));
      if (!storedToken || !this.constantTimeCompare(headerToken, storedToken)) {
        return this.buildResult({
          action: GuardAction.BLOCK,
          threatLevel: ThreatLevel.HIGH,
          score: 1.0,
          reason: 'CSRF token mismatch',
          metadata: { tokenPresent: true, tokenSource: 'header', validationMethod: 'server_session' },
          durationMs: Date.now() - startTime,
        });
      }
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score: 0,
      reason: 'CSRF token valid',
      metadata: { tokenPresent: true, tokenSource: 'header', validationMethod: this.options.doubleSubmitCookie ? 'double_submit' : 'server_session' },
      durationMs: Date.now() - startTime,
    });
  }

  private shouldSkipPath(path: string): boolean {
    return this.options.skipPaths.some((p) => {
      if (p.includes('*')) {
        const regex = new RegExp('^' + p.replace(/\*/g, '.*') + '$');
        return regex.test(path);
      }
      return path === p;
    });
  }

  private constantTimeCompare(a: string, b: string): boolean {
    if (a.length !== b.length) return false;
    let result = 0;
    for (let i = 0; i < a.length; i++) {
      result |= a.charCodeAt(i) ^ b.charCodeAt(i);
    }
    return result === 0;
  }
}

export const csrfDetector = {
  kind: DetectorKind.CSRF,
  priority: 30,
};