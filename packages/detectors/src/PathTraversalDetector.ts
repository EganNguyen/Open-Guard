import { BaseDetector, GuardRequest, DetectorResult, GuardAction, ThreatLevel, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind, extractStringValues } from '@open-guard/core';

export interface PathTraversalOptions {
  inspectQuery?: boolean;
  decodePercent?: boolean;
}

interface Pattern {
  id: string;
  regex: string;
  weight: number;
  description: string;
}

const BUILTIN_PATTERNS: Pattern[] = [
  { id: 'dotdot_slash', regex: '\\.\\./|\\.\\.\\\\', weight: 0.9, description: 'Dot-dot-slash traversal' },
  { id: 'encoded_dotdot', regex: '(%2e%2e%2f|%2e%2e/|\\.%2e/|%2e\\./)', weight: 0.95, description: 'Encoded dot-dot' },
  { id: 'double_encoded', regex: '(%252e%252e)', weight: 0.95, description: 'Double-encoded traversal' },
  { id: 'absolute_path', regex: '^/?(etc/passwd|etc/shadow|windows/system32)', weight: 0.99, description: 'Absolute path traversal' },
];

const BLOCK_THRESHOLD = 0.7;

export class PathTraversalDetector extends BaseDetector {
  private options: Required<PathTraversalOptions>;
  private patterns: Pattern[];

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      inspectQuery: config.options?.inspectQuery !== false,
      decodePercent: config.options?.decodePercent !== false,
    };
    this.patterns = BUILTIN_PATTERNS;
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();

    let path = req.path;
    if (this.options.decodePercent) {
      path = this.decodePercent(path);
      for (const key of Object.keys(req.query || {})) {
        const val = Array.isArray(req.query?.[key]) 
          ? (req.query![key] as string[]).map(v => this.decodePercent(v))
          : this.decodePercent(req.query?.[key] as string || '');
        if (this.checkTraversal(key) || this.checkTraversal(val as string)) {
          return this.buildResult({
            action: GuardAction.BLOCK,
            threatLevel: ThreatLevel.HIGH,
            score: 1.0,
            reason: 'Path traversal detected in query parameters',
            metadata: { matchedPatterns: ['query_traversal'], path },
            durationMs: Date.now() - startTime,
          });
        }
      }
    }

    const matchedPatterns: string[] = [];
    let rawScore = 0;

    for (const pattern of this.patterns) {
      if (new RegExp(pattern.regex).test(path)) {
        matchedPatterns.push(pattern.id);
        rawScore += pattern.weight;
      }
    }

    if (this.options.inspectQuery && req.query) {
      const queryValues = extractStringValues(req.query);
      for (const value of queryValues) {
        const decodedValue = this.options.decodePercent ? this.decodePercent(value) : value;
        for (const pattern of this.patterns) {
          if (new RegExp(pattern.regex).test(decodedValue)) {
            matchedPatterns.push(`${pattern.id}:query`);
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
        reason: `Path traversal detected: ${matchedPatterns.join(', ')}`,
        metadata: { matchedPatterns, path, rawScore },
        durationMs: Date.now() - startTime,
      });
    }

    if (score > 0) {
      return this.buildResult({
        action: GuardAction.LOG_ONLY,
        threatLevel: ThreatLevel.LOW,
        score,
        reason: `Suspicious path patterns: ${matchedPatterns.join(', ')}`,
        metadata: { matchedPatterns, path },
        durationMs: Date.now() - startTime,
      });
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score: 0,
      reason: 'No path traversal patterns detected',
      metadata: { matchedPatterns: [], path },
      durationMs: Date.now() - startTime,
    });
  }

  private decodePercent(str: string): string {
    try {
      return decodeURIComponent(str);
    } catch {
      return str;
    }
  }

  private checkTraversal(str: string): boolean {
    return /\.\.[/\\]/.test(str) || /%2e%2e/i.test(str);
  }
}

export const pathTraversalDetector = {
  kind: DetectorKind.PATH_TRAVERSAL,
  priority: 18,
};