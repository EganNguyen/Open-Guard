import { BaseDetector, GuardRequest, DetectorResult, GuardAction, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind, ThreatLevel, extractStringValues } from '@open-guard/core';

export interface XssOptions {
  inspectBody?: boolean;
  inspectQuery?: boolean;
  sensitivity?: 'low' | 'medium' | 'high';
  decodeEntities?: boolean;
}

interface Pattern {
  id: string;
  regex: string;
  weight: number;
  description: string;
}

const BUILTIN_PATTERNS: Pattern[] = [
  { id: 'script_tag', regex: '(?i)<script[\\s>]', weight: 0.95, description: 'Script tag' },
  { id: 'event_handler', regex: '(?i)\\bon\\w+\\s*=', weight: 0.8, description: 'Event handler attribute' },
  { id: 'javascript_uri', regex: '(?i)javascript\\s*:', weight: 0.9, description: 'JavaScript URI' },
  { id: 'data_uri', regex: '(?i)data\\s*:[^,]*base64', weight: 0.7, description: 'Data URI with base64' },
  { id: 'vbscript', regex: '(?i)vbscript\\s*:', weight: 0.9, description: 'VBScript URI' },
  { id: 'svg_xss', regex: '(?i)<svg[\\s>].*?(onload|onerror)', weight: 0.95, description: 'SVG with event handler' },
  { id: 'template_injection', regex: '\\{\\{.*?\\}\\}|\\{%.*?%\\}', weight: 0.6, description: 'Template injection' },
  { id: 'encoded_angle', regex: '(%3C|&lt;|&#60;).*?(%3E|&gt;|&#62;)', weight: 0.7, description: 'Encoded angle brackets' },
];

const SENSITIVITY_MULTIPLIERS = { low: 0.7, medium: 1.0, high: 1.3 };
const BLOCK_THRESHOLD = 0.75;
const LOG_THRESHOLD = 0.35;

const HTML_ENTITIES: Record<string, string> = {
  '&lt;': '<',
  '&gt;': '>',
  '&amp;': '&',
  '&quot;': '"',
  '&#60;': '<',
  '&#62;': '>',
  '&#x3C': '<',
  '&#x3E': '>',
};

export class XssDetector extends BaseDetector {
  private options: Required<XssOptions>;
  private patterns: Pattern[];

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      inspectBody: config.options?.inspectBody !== false,
      inspectQuery: config.options?.inspectQuery !== false,
      sensitivity: (config.options?.sensitivity as 'low' | 'medium' | 'high') || 'medium',
      decodeEntities: config.options?.decodeEntities !== false,
    };
    this.patterns = BUILTIN_PATTERNS;
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();
    let values: string[] = [];

    if (this.options.inspectQuery && req.query) {
      values.push(...extractStringValues(req.query));
    }
    if (this.options.inspectBody && req.body) {
      values.push(...extractStringValues(req.body));
    }

    if (this.options.decodeEntities) {
      values = values.map((v) => this.decodeHtmlEntities(v));
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

    if (score >= BLOCK_THRESHOLD) {
      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.HIGH,
        score,
        reason: `XSS payload detected: matched ${matchedPatterns.join(', ')}`,
        metadata: { matchedPatterns, decodedInput: this.options.decodeEntities },
        durationMs: Date.now() - startTime,
      });
    }

    if (score >= LOG_THRESHOLD) {
      return this.buildResult({
        action: GuardAction.LOG_ONLY,
        threatLevel: ThreatLevel.MEDIUM,
        score,
        reason: `Suspicious XSS-like patterns detected: ${matchedPatterns.join(', ')}`,
        metadata: { matchedPatterns },
        durationMs: Date.now() - startTime,
      });
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score: 0,
      reason: 'No XSS patterns detected',
      metadata: { matchedPatterns: [] },
      durationMs: Date.now() - startTime,
    });
  }

  private decodeHtmlEntities(str: string): string {
    let result = str;
    for (const [entity, char] of Object.entries(HTML_ENTITIES)) {
      result = result.replace(new RegExp(entity, 'gi'), char);
    }
    result = result.replace(/%3C/gi, '<').replace(/%3E/gi, '>').replace(/%2F/gi, '/');
    return result;
  }
}

export const xssDetector = {
  kind: DetectorKind.XSS,
  priority: 25,
};