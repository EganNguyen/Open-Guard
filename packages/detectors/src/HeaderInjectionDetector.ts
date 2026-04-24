import { BaseDetector, GuardRequest, DetectorResult, GuardAction, ThreatLevel, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind, extractStringValues } from '@open-guard/core';

export interface HeaderInjectionOptions {
  allowedHosts?: string[];
  inspectQuery?: boolean;
  inspectBody?: boolean;
}

interface Pattern {
  id: string;
  regex: string;
  weight: number;
  description: string;
}

const BUILTIN_PATTERNS: Pattern[] = [
  { id: 'crlf', regex: '(%0d%0a|%0a%0d|\\r\\n|\\r|\\n)', weight: 0.95, description: 'CRLF injection' },
  { id: 'null_byte', regex: '(%00|\\x00)', weight: 0.8, description: 'Null byte injection' },
  { id: 'header_split', regex: '(%0d|%0a|\\\\r|\\\\n)', weight: 0.85, description: 'HTTP header split' },
];

const BLOCK_THRESHOLD = 0.7;

export class HeaderInjectionDetector extends BaseDetector {
  private options: Required<HeaderInjectionOptions>;
  private patterns: Pattern[];

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      allowedHosts: (config.options?.allowedHosts as string[]) || [],
      inspectQuery: config.options?.inspectQuery !== false,
      inspectBody: (config.options?.inspectBody as boolean) || false,
    };
    this.patterns = BUILTIN_PATTERNS;
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();

    if (this.options.allowedHosts.length > 0) {
      const host = req.headers['host'] || req.headers['x-forwarded-host'] || '';
      if (!this.options.allowedHosts.some((h) => host.includes(h))) {
        return this.buildResult({
          action: GuardAction.BLOCK,
          threatLevel: ThreatLevel.HIGH,
          score: 1.0,
          reason: `Host header ${host} not in allowed list`,
          metadata: { host, source: 'host_validation' },
          durationMs: Date.now() - startTime,
        });
      }
    }

    const matchedPatterns: string[] = [];
    let rawScore = 0;

    for (const [header, value] of Object.entries(req.headers)) {
      if (typeof value === 'string') {
        for (const pattern of this.patterns) {
          if (new RegExp(pattern.regex).test(value)) {
            matchedPatterns.push(`${pattern.id}:${header}`);
            rawScore += pattern.weight;
            break;
          }
        }
      }
    }

    if (this.options.inspectQuery && req.query) {
      const queryValues = extractStringValues(req.query);
      for (const value of queryValues) {
        for (const pattern of this.patterns) {
          if (new RegExp(pattern.regex).test(value)) {
            matchedPatterns.push(`${pattern.id}:query`);
            rawScore += pattern.weight;
            break;
          }
        }
      }
    }

    if (this.options.inspectBody && req.body) {
      const bodyValues = extractStringValues(req.body);
      for (const value of bodyValues) {
        for (const pattern of this.patterns) {
          if (new RegExp(pattern.regex).test(value)) {
            matchedPatterns.push(`${pattern.id}:body`);
            rawScore += pattern.weight;
            break;
          }
        }
      }
    }

    const score = Math.min(1.0, rawScore);

    if (score >= BLOCK_THRESHOLD) {
      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.HIGH,
        score,
        reason: `Header injection detected: ${matchedPatterns.join(', ')}`,
        metadata: { matchedPatterns, rawScore },
        durationMs: Date.now() - startTime,
      });
    }

    if (score > 0) {
      return this.buildResult({
        action: GuardAction.LOG_ONLY,
        threatLevel: ThreatLevel.MEDIUM,
        score,
        reason: `Suspicious header patterns: ${matchedPatterns.join(', ')}`,
        metadata: { matchedPatterns, rawScore },
        durationMs: Date.now() - startTime,
      });
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score: 0,
      reason: 'No header injection patterns detected',
      metadata: { matchedPatterns: [] },
      durationMs: Date.now() - startTime,
    });
  }
}

export const headerInjectionDetector = {
  kind: DetectorKind.HEADER_INJECTION,
  priority: 22,
};