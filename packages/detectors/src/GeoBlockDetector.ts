import { BaseDetector, GuardRequest, DetectorResult, GuardAction, ThreatLevel, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind } from '@open-guard/core';

export interface GeoBlockOptions {
  blockedCountries?: string[];
  allowedCountries?: string[];
  challengedCountries?: string[];
  actionOnMissingGeo?: 'ALLOW' | 'BLOCK' | 'LOG_ONLY';
}

export class GeoBlockDetector extends BaseDetector {
  private options: Required<GeoBlockOptions>;

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      blockedCountries: (config.options?.blockedCountries as string[]) || [],
      allowedCountries: (config.options?.allowedCountries as string[]) || [],
      challengedCountries: (config.options?.challengedCountries as string[]) || [],
      actionOnMissingGeo: (config.options?.actionOnMissingGeo as 'ALLOW' | 'BLOCK' | 'LOG_ONLY') || 'ALLOW',
    };
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();

    if (!req.geo || !req.geo.country) {
      const action = this.options.actionOnMissingGeo;
      return this.buildResult({
        action: action === 'ALLOW' ? GuardAction.ALLOW : action === 'BLOCK' ? GuardAction.BLOCK : GuardAction.LOG_ONLY,
        threatLevel: action === 'BLOCK' ? ThreatLevel.MEDIUM : ThreatLevel.NONE,
        score: action === 'BLOCK' ? 1.0 : 0,
        reason: `Geo data unavailable, action: ${action}`,
        metadata: { country: null, action: action },
        durationMs: Date.now() - startTime,
      });
    }

    const country = req.geo.country.toUpperCase();

    if (this.options.allowedCountries.length > 0) {
      if (!this.options.allowedCountries.map(c => c.toUpperCase()).includes(country)) {
        return this.buildResult({
          action: GuardAction.BLOCK,
          threatLevel: ThreatLevel.HIGH,
          score: 1.0,
          reason: `Country ${country} not in allowlist`,
          metadata: { country, action: 'blocked_allowlist' },
          durationMs: Date.now() - startTime,
        });
      }
    }

    if (this.options.blockedCountries.map(c => c.toUpperCase()).includes(country)) {
      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.HIGH,
        score: 1.0,
        reason: `Country ${country} is blocked`,
        metadata: { country, action: 'blocked' },
        durationMs: Date.now() - startTime,
      });
    }

    if (this.options.challengedCountries.map(c => c.toUpperCase()).includes(country)) {
      return this.buildResult({
        action: GuardAction.CHALLENGE,
        threatLevel: ThreatLevel.MEDIUM,
        score: 0.6,
        reason: `Country ${country} requires challenge`,
        metadata: { country, action: 'challenge' },
        durationMs: Date.now() - startTime,
      });
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score: 0,
      reason: `Country ${country} allowed`,
      metadata: { country, action: 'allowed' },
      durationMs: Date.now() - startTime,
    });
  }
}

export const geoBlockDetector = {
  kind: DetectorKind.GEO_BLOCK,
  priority: 50,
};