import { BaseDetector, GuardRequest, DetectorResult, GuardAction, ThreatLevel, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind } from '@open-guard/core';

export interface PayloadSizeOptions {
  maxBodyBytes?: number;
  maxUrlLength?: number;
  maxHeaderValueLength?: number;
  skipMethods?: string[];
}

export class PayloadSizeDetector extends BaseDetector {
  private options: Required<PayloadSizeOptions>;

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      maxBodyBytes: (config.options?.maxBodyBytes as number) || 1048576,
      maxUrlLength: (config.options?.maxUrlLength as number) || 2048,
      maxHeaderValueLength: (config.options?.maxHeaderValueLength as number) || 8192,
      skipMethods: (config.options?.skipMethods as string[]) || ['GET', 'HEAD', 'OPTIONS'],
    };
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();

    if (this.options.skipMethods.includes(req.method)) {
      return this.buildResult({
        action: GuardAction.ALLOW,
        threatLevel: ThreatLevel.NONE,
        score: 0,
        reason: 'Method excluded from payload size check',
        metadata: { exceededLimit: null, actualBytes: 0, limitBytes: 0 },
        durationMs: Date.now() - startTime,
      });
    }

    const contentLengthHeader = req.headers['content-length'];
    let bodySize = contentLengthHeader ? parseInt(contentLengthHeader, 10) : 0;

    if (req.body && typeof req.body === 'object') {
      const bodyStr = JSON.stringify(req.body);
      bodySize = new Blob([bodyStr]).size;
    }

    if (bodySize > this.options.maxBodyBytes) {
      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.HIGH,
        score: 1.0,
        reason: `Body size ${bodySize} exceeds limit ${this.options.maxBodyBytes}`,
        metadata: { exceededLimit: 'body', actualBytes: bodySize, limitBytes: this.options.maxBodyBytes },
        durationMs: Date.now() - startTime,
      });
    }

    const urlLength = req.path.length + (req.query ? Object.entries(req.query).reduce((len, [k, v]) => {
      const val = Array.isArray(v) ? v.join(',') : v;
      return len + encodeURIComponent(k).length + encodeURIComponent(val).length + 1;
    }, 0) : 0);

    if (urlLength > this.options.maxUrlLength) {
      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.HIGH,
        score: 1.0,
        reason: `URL length ${urlLength} exceeds limit ${this.options.maxUrlLength}`,
        metadata: { exceededLimit: 'url', actualBytes: urlLength, limitBytes: this.options.maxUrlLength },
        durationMs: Date.now() - startTime,
      });
    }

    for (const [header, value] of Object.entries(req.headers)) {
      if (value && value.length > this.options.maxHeaderValueLength) {
        return this.buildResult({
          action: GuardAction.BLOCK,
          threatLevel: ThreatLevel.HIGH,
          score: 1.0,
          reason: `Header ${header} length ${value.length} exceeds limit ${this.options.maxHeaderValueLength}`,
          metadata: { exceededLimit: 'header', actualBytes: value.length, limitBytes: this.options.maxHeaderValueLength },
          durationMs: Date.now() - startTime,
        });
      }
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score: 0,
      reason: 'Payload size within limits',
      metadata: { exceededLimit: null, actualBytes: bodySize, limitBytes: this.options.maxBodyBytes },
      durationMs: Date.now() - startTime,
    });
  }
}

export const payloadSizeDetector = {
  kind: DetectorKind.PAYLOAD_SIZE,
  priority: 5,
};