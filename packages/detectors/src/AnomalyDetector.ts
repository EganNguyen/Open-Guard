import { BaseDetector, GuardRequest, DetectorResult, GuardAction, ThreatLevel, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind } from '@open-guard/core';

export interface AnomalyOptions {
  baselineWindowSeconds?: number;
  minSamplesForBaseline?: number;
  rateDeviationSigma?: number;
  pathEntropyThreshold?: number;
  enabled?: boolean;
}

interface BaselineStats {
  requestCount: number;
  lastRequestTime: number;
  methodCounts: Record<string, number>;
  pathHistory: string[];
  createdAt: number;
}

interface Signal {
  name: string;
  weight: number;
}

const SIGNALS: Signal[] = [
  { name: 'request_rate_spike', weight: 0.6 },
  { name: 'method_anomaly', weight: 0.4 },
  { name: 'high_path_entropy', weight: 0.7 },
  { name: 'new_ip_burst', weight: 0.5 },
];

const BLOCK_THRESHOLD = 0.7;

export class AnomalyDetector extends BaseDetector {
  private options: Required<AnomalyOptions>;

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      baselineWindowSeconds: (config.options?.baselineWindowSeconds as number) || 3600,
      minSamplesForBaseline: (config.options?.minSamplesForBaseline as number) || 20,
      rateDeviationSigma: (config.options?.rateDeviationSigma as number) || 3.0,
      pathEntropyThreshold: (config.options?.pathEntropyThreshold as number) || 0.9,
      enabled: (config.options?.enabled as boolean) || false,
    };
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();

    if (!this.options.enabled) {
      return this.buildResult({
        action: GuardAction.ALLOW,
        threatLevel: ThreatLevel.NONE,
        score: 0,
        reason: 'Anomaly detection is disabled',
        metadata: { signals: [], baselineSamples: 0 },
        durationMs: Date.now() - startTime,
      });
    }

    const baselineKey = this.getStoreKey('anomaly', 'baseline', req.ip);
    const baselineStr = await this.store.get(baselineKey);
    let baseline: BaselineStats;

    if (baselineStr) {
      baseline = JSON.parse(baselineStr);
    } else {
      baseline = {
        requestCount: 0,
        lastRequestTime: Date.now(),
        methodCounts: {},
        pathHistory: [],
        createdAt: Date.now(),
      };
    }

    baseline.requestCount++;
    baseline.lastRequestTime = Date.now();
    baseline.methodCounts[req.method] = (baseline.methodCounts[req.method] || 0) + 1;
    baseline.pathHistory.push(req.path);
    if (baseline.pathHistory.length > 100) {
      baseline.pathHistory = baseline.pathHistory.slice(-100);
    }

    await this.store.set(baselineKey, JSON.stringify(baseline), this.options.baselineWindowSeconds);

    if (baseline.requestCount < this.options.minSamplesForBaseline) {
      return this.buildResult({
        action: GuardAction.ALLOW,
        threatLevel: ThreatLevel.NONE,
        score: 0,
        reason: `Building baseline: ${baseline.requestCount}/${this.options.minSamplesForBaseline} samples`,
        metadata: { signals: [], baselineSamples: baseline.requestCount },
        durationMs: Date.now() - startTime,
      });
    }

    const signals: string[] = [];
    let rawScore = 0;

    const requestsPerMinute = baseline.requestCount / (this.options.baselineWindowSeconds / 60);
    const timeSinceCreation = (Date.now() - baseline.createdAt) / 60000;
    const expectedRate = baseline.requestCount / Math.max(1, timeSinceCreation);
    const rateDeviation = Math.abs(requestsPerMinute - expectedRate) / Math.max(1, expectedRate);

    if (rateDeviation > this.options.rateDeviationSigma) {
      signals.push('request_rate_spike');
      rawScore += SIGNALS[0].weight;
    }

    const totalMethods = Object.values(baseline.methodCounts).reduce((a, b) => a + b, 0);
    const methodEntropy = this.calculateEntropy(Object.values(baseline.methodCounts).map(c => c / totalMethods));
    if (methodEntropy < 0.3) {
      signals.push('method_anomaly');
      rawScore += SIGNALS[1].weight;
    }

    if (baseline.pathHistory.length > 5) {
      const recentPaths = baseline.pathHistory.slice(-10);
      const uniquePaths = new Set(recentPaths).size;
      const pathEntropy = uniquePaths / recentPaths.length;
      if (pathEntropy > this.options.pathEntropyThreshold) {
        signals.push('high_path_entropy');
        rawScore += SIGNALS[2].weight;
      }
    }

    const score = Math.min(1.0, rawScore);

    if (score >= BLOCK_THRESHOLD) {
      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.HIGH,
        score,
        reason: `Anomaly detected: ${signals.join(', ')}`,
        metadata: { signals, baselineSamples: baseline.requestCount, rateDeviation },
        durationMs: Date.now() - startTime,
      });
    }

    if (score > 0) {
      return this.buildResult({
        action: GuardAction.LOG_ONLY,
        threatLevel: ThreatLevel.LOW,
        score,
        reason: `Suspicious patterns: ${signals.join(', ')}`,
        metadata: { signals, baselineSamples: baseline.requestCount },
        durationMs: Date.now() - startTime,
      });
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score: 0,
      reason: 'Request within normal parameters',
      metadata: { signals: [], baselineSamples: baseline.requestCount },
      durationMs: Date.now() - startTime,
    });
  }

  private calculateEntropy(probs: number[]): number {
    return probs.reduce((entropy, p) => {
      if (p > 0) {
        return entropy - p * Math.log2(p);
      }
      return entropy;
    }, 0);
  }
}

export const anomalyDetector = {
  kind: DetectorKind.ANOMALY,
  priority: 80,
};