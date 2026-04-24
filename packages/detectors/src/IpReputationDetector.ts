import { BaseDetector, GuardRequest, DetectorResult, GuardAction, ThreatLevel, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind } from '@open-guard/core';

export interface IpReputationOptions {
  externalApiUrl?: string;
  externalApiKey?: string;
  cacheHitsTtlSeconds?: number;
  blockOnApiError?: boolean;
}

interface IpReputationResponse {
  score: number;
  category: string;
  provider?: string;
}

export class IpReputationDetector extends BaseDetector {
  private options: Required<IpReputationOptions>;

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      externalApiUrl: config.options?.externalApiUrl as string || '',
      externalApiKey: config.options?.externalApiKey as string || process.env.GUARD_IP_REPUTATION_API_KEY || '',
      cacheHitsTtlSeconds: (config.options?.cacheHitsTtlSeconds as number) || 3600,
      blockOnApiError: (config.options?.blockOnApiError as boolean) || false,
    };
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();

    const allowlistKey = this.getStoreKey('bl', 'ip', req.ip);
    const blocklistKey = this.getStoreKey('bl', 'ip', req.ip);
    const cachedResult = await this.store.get(this.getStoreKey('ipr', req.ip));

    const allowlistEntry = await this.store.get(this.getStoreKey('wl', 'ip', req.ip));
    if (allowlistEntry) {
      return this.buildResult({
        action: GuardAction.ALLOW,
        threatLevel: ThreatLevel.NONE,
        score: 0,
        reason: 'IP is allowlisted',
        metadata: { source: 'allowlist', cachedResult: false },
        durationMs: Date.now() - startTime,
      });
    }

    const blocklistEntry = await this.store.get(blocklistKey);
    if (blocklistEntry) {
      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.CRITICAL,
        score: 1.0,
        reason: 'IP is blocklisted',
        metadata: { source: 'blocklist', cachedResult: false },
        durationMs: Date.now() - startTime,
      });
    }

    if (this.options.externalApiUrl) {
      if (cachedResult) {
        const cached = JSON.parse(cachedResult) as IpReputationResponse;
        return this.buildResult({
          action: cached.score >= 0.7 ? GuardAction.BLOCK : cached.score >= 0.4 ? GuardAction.CHALLENGE : GuardAction.ALLOW,
          threatLevel: this.scoreToThreatLevel(cached.score),
          score: cached.score,
          reason: `IP reputation check (cached): ${cached.category}`,
          metadata: { source: 'external', externalProvider: cached.provider || 'unknown', cachedResult: true },
          durationMs: Date.now() - startTime,
        });
      }

      try {
        const response = await fetch(this.options.externalApiUrl, {
          headers: {
            'Authorization': `Bearer ${this.options.externalApiKey}`,
            'X-IP': req.ip,
          },
        });

        if (!response.ok) {
          if (this.options.blockOnApiError) {
            return this.buildResult({
              action: GuardAction.BLOCK,
              threatLevel: ThreatLevel.HIGH,
              score: 1.0,
              reason: 'IP reputation API error',
              metadata: { source: 'external', error: 'API request failed' },
              durationMs: Date.now() - startTime,
            });
          }
          return this.buildResult({
            action: GuardAction.ALLOW,
            threatLevel: ThreatLevel.NONE,
            score: 0,
            reason: 'IP reputation API unavailable',
            metadata: { source: 'none', error: 'API request failed' },
            durationMs: Date.now() - startTime,
          });
        }

        const data = await response.json() as IpReputationResponse;
        await this.store.set(this.getStoreKey('ipr', req.ip), JSON.stringify(data), this.options.cacheHitsTtlSeconds);

        return this.buildResult({
          action: data.score >= 0.7 ? GuardAction.BLOCK : data.score >= 0.4 ? GuardAction.CHALLENGE : GuardAction.ALLOW,
          threatLevel: this.scoreToThreatLevel(data.score),
          score: data.score,
          reason: `IP reputation check: ${data.category}`,
          metadata: { source: 'external', externalProvider: data.provider || 'unknown', cachedResult: false },
          durationMs: Date.now() - startTime,
        });
      } catch {
        return this.buildResult({
          action: GuardAction.ALLOW,
          threatLevel: ThreatLevel.NONE,
          score: 0,
          reason: 'IP reputation check failed',
          metadata: { source: 'none', error: 'Network error' },
          durationMs: Date.now() - startTime,
        });
      }
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score: 0,
      reason: 'No IP reputation data available',
      metadata: { source: 'none', cachedResult: false },
      durationMs: Date.now() - startTime,
    });
  }
}

export const ipReputationDetector = {
  kind: DetectorKind.IP_REPUTATION,
  priority: 55,
};