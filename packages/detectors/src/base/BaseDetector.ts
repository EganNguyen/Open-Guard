import {
  GuardRequest,
  DetectorResult,
  DetectorConfig,
  DetectorKind,
  GuardAction,
  ThreatLevel,
  StoreAdapter,
  scoreToThreatLevel,
} from '@open-guard/core';

export abstract class BaseDetector {
  protected config: DetectorConfig;
  protected store: StoreAdapter;
  protected enabled: boolean;

  constructor(config: DetectorConfig, store: StoreAdapter) {
    this.config = config;
    this.store = store;
    this.enabled = config.enabled !== false;
  }

  abstract evaluate(req: GuardRequest): Promise<DetectorResult>;

  protected buildResult(partial: Partial<DetectorResult>): DetectorResult {
    const startTime = partial.durationMs ? Date.now() - partial.durationMs : Date.now();
    
    return {
      detectorId: this.config.id,
      kind: this.config.kind,
      action: partial.action ?? GuardAction.ALLOW,
      threatLevel: partial.threatLevel ?? ThreatLevel.NONE,
      score: partial.score ?? 0,
      reason: partial.reason ?? 'No threat detected',
      metadata: partial.metadata,
      durationMs: Date.now() - startTime,
    };
  }

  protected getStoreKey(namespace: string, ...parts: string[]): string {
    return `og:${namespace}:${parts.join(':')}`;
  }

  public isEnabled(): boolean {
    return this.enabled;
  }

  protected scoreToThreatLevel(score: number): ThreatLevel {
    return scoreToThreatLevel(score);
  }
}

export { GuardRequest, DetectorResult, DetectorConfig, DetectorKind, GuardAction, ThreatLevel, StoreAdapter };
export { scoreToThreatLevel } from '@open-guard/core';