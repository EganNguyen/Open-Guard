import { BaseDetector, GuardRequest, DetectorResult, GuardAction, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind, ThreatLevel, extractStringValues } from '@open-guard/core';

export interface SqlInjectionOptions {
  inspectBody?: boolean;
  inspectQuery?: boolean;
  inspectPath?: boolean;
  sensitivity?: 'low' | 'medium' | 'high';
  customPatterns?: string[];
}

interface Pattern {
  id: string;
  regex: string;
  weight: number;
  description: string;
}

const BUILTIN_PATTERNS: Pattern[] = [
  { id: 'union_select', regex: '(?i)(union\\s+(all\\s+)?select)', weight: 0.9, description: 'UNION SELECT injection' },
  { id: 'comment_bypass', regex: '(?i)(/\\*|\\*/|--\\s|#\\s)', weight: 0.6, description: 'SQL comment sequences' },
  { id: 'boolean_blind', regex: '(?i)(\\bor\\b\\s+\\d+\\s*=\\s*\\d+|\\band\\b\\s+\\d+\\s*=\\s*\\d+)', weight: 0.8, description: 'Boolean blind injection' },
  { id: 'time_blind', regex: '(?i)(sleep\\s*\\(|waitfor\\s+delay|benchmark\\s*\\()', weight: 0.95, description: 'Time-based blind injection' },
  { id: 'stacked_query', regex: '(?i)(;\\s*(drop|insert|update|delete|create|alter)\\b)', weight: 0.95, description: 'Stacked queries' },
  { id: 'quote_escape', regex: '(\'\\s*(or|and)\\s*\'|\'\\s*=\\s*\'|\\\\x27)', weight: 0.7, description: 'Quote manipulation' },
  { id: 'function_calls', regex: '(?i)(char\\s*\\(|ascii\\s*\\(|substring\\s*\\(|hex\\s*\\(|unhex\\s*\\()', weight: 0.5, description: 'SQL function abuse' },
];

const SENSITIVITY_MULTIPLIERS = { low: 0.7, medium: 1.0, high: 1.3 };
const BLOCK_THRESHOLD = 0.8;
const LOG_THRESHOLD = 0.4;

export class SqlInjectionDetector extends BaseDetector {
  private options: Required<SqlInjectionOptions>;
  private patterns: Pattern[];

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      inspectBody: config.options?.inspectBody !== false,
      inspectQuery: config.options?.inspectQuery !== false,
      inspectPath: config.options?.inspectPath !== false,
      sensitivity: (config.options?.sensitivity as 'low' | 'medium' | 'high') || 'medium',
      customPatterns: (config.options?.customPatterns as string[]) || [],
    };
    this.patterns = [
      ...BUILTIN_PATTERNS,
      ...this.options.customPatterns.map((p, i) => ({
        id: `custom_${i}`,
        regex: p,
        weight: 0.8,
        description: 'Custom pattern',
      })),
    ];
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();
    const values: string[] = [];

    if (this.options.inspectQuery && req.query) {
      values.push(...extractStringValues(req.query));
    }
    if (this.options.inspectBody && req.body) {
      values.push(...extractStringValues(req.body));
    }
    if (this.options.inspectPath) {
      values.push(req.path);
    }

    const matchedPatterns: string[] = [];
    let rawScore = 0;

    for (const pattern of this.patterns) {
      const regex = new RegExp(pattern.regex);
      for (const value of values) {
        if (regex.test(value)) {
          matchedPatterns.push(pattern.id);
          rawScore += pattern.weight * SENSITIVITY_MULTIPLIERS[this.options.sensitivity];
          break;
        }
      }
    }

    const score = Math.min(1.0, rawScore);
    const inspectedFields = this.getInspectedFields();

    if (score >= BLOCK_THRESHOLD) {
      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.HIGH,
        score,
        reason: `SQL injection detected: matched ${matchedPatterns.join(', ')}`,
        metadata: { matchedPatterns, inspectedFields, rawScore },
        durationMs: Date.now() - startTime,
      });
    }

    if (score >= LOG_THRESHOLD) {
      return this.buildResult({
        action: GuardAction.LOG_ONLY,
        threatLevel: ThreatLevel.MEDIUM,
        score,
        reason: `Suspicious SQL-like patterns detected: ${matchedPatterns.join(', ')}`,
        metadata: { matchedPatterns, inspectedFields, rawScore },
        durationMs: Date.now() - startTime,
      });
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score,
      reason: 'No SQL injection patterns detected',
      metadata: { matchedPatterns: [], inspectedFields, rawScore: 0 },
      durationMs: Date.now() - startTime,
    });
  }

  private getInspectedFields(): string[] {
    const fields: string[] = [];
    if (this.options.inspectQuery) fields.push('query');
    if (this.options.inspectBody) fields.push('body');
    if (this.options.inspectPath) fields.push('path');
    return fields;
  }
}

export const sqlInjectionDetector = {
  kind: DetectorKind.SQL_INJECTION,
  priority: 20,
};