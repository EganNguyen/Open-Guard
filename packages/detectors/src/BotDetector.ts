import { BaseDetector, GuardRequest, DetectorResult, GuardAction, ThreatLevel, StoreAdapter } from './base/BaseDetector';
import { DetectorConfig, DetectorKind } from '@open-guard/core';

export interface BotOptions {
  blockKnownBots?: boolean;
  allowKnownGoodBots?: boolean;
  challengeSuspicious?: boolean;
  minHeaderCount?: number;
  suspiciousUaPatterns?: string[];
}

interface Signal {
  name: string;
  score: number;
}

const KNOWN_BAD_BOT_PATTERNS = [
  '(?i)(scrapy|python-requests|go-http-client|curl/|wget/|libwww-perl)',
  '(?i)(masscan|nikto|nmap|sqlmap|dirbuster|gobuster)',
  '(?i)(zgrab|nuclei|httpie)',
];

const KNOWN_GOOD_BOTS = [
  'Googlebot', 'Bingbot', 'Slurp', 'DuckDuckBot', 'Baiduspider', 'facebookexternalhit', 'Twitterbot',
];

const BLOCK_THRESHOLD = 0.9;
const CHALLENGE_THRESHOLD = 0.5;
const LOG_THRESHOLD = 0.3;

export class BotDetector extends BaseDetector {
  private options: Required<BotOptions>;

  constructor(config: DetectorConfig, store: StoreAdapter) {
    super(config, store);
    this.options = {
      blockKnownBots: (config.options?.blockKnownBots as boolean) || false,
      allowKnownGoodBots: (config.options?.allowKnownGoodBots as boolean) || true,
      challengeSuspicious: (config.options?.challengeSuspicious as boolean) || true,
      minHeaderCount: (config.options?.minHeaderCount as number) || 4,
      suspiciousUaPatterns: (config.options?.suspiciousUaPatterns as string[]) || [],
    };
  }

  async evaluate(req: GuardRequest): Promise<DetectorResult> {
    const startTime = Date.now();
    const signals: Signal[] = [];
    const headerCount = Object.keys(req.headers).length;
    const userAgent = req.headers['user-agent'] || '';

    if (!userAgent) {
      signals.push({ name: 'missing_ua', score: 0.8 });
    }

    for (const pattern of KNOWN_BAD_BOT_PATTERNS) {
      if (new RegExp(pattern).test(userAgent)) {
        signals.push({ name: 'known_bad_bot', score: 0.95 });
        break;
      }
    }

    if (this.options.allowKnownGoodBots) {
      for (const bot of KNOWN_GOOD_BOTS) {
        if (userAgent.includes(bot)) {
          signals.push({ name: 'known_good_bot', score: -0.5 });
          break;
        }
      }
    }

    if (headerCount < this.options.minHeaderCount) {
      signals.push({ name: 'low_header_count', score: 0.4 });
    }

    if (!req.headers['accept']) {
      signals.push({ name: 'missing_accept', score: 0.3 });
    }

    if (!req.headers['accept-language']) {
      signals.push({ name: 'missing_accept_language', score: 0.25 });
    }

    if (userAgent.includes('HeadlessChrome') || userAgent.includes('Phantom')) {
      signals.push({ name: 'headless_markers', score: 0.7 });
    }

    for (const pattern of this.options.suspiciousUaPatterns) {
      if (new RegExp(pattern).test(userAgent)) {
        signals.push({ name: 'suspicious_ua_pattern', score: 0.6 });
        break;
      }
    }

    let score = signals.reduce((sum, s) => sum + s.score, 0);
    score = Math.min(1.0, Math.max(0.0, score));

    const matchedSignals = signals.map((s) => s.name);

    if (score >= BLOCK_THRESHOLD && this.options.blockKnownBots) {
      return this.buildResult({
        action: GuardAction.BLOCK,
        threatLevel: ThreatLevel.CRITICAL,
        score,
        reason: 'Known malicious bot detected',
        metadata: { matchedSignals, headerCount, userAgent: userAgent.substring(0, 100) },
        durationMs: Date.now() - startTime,
      });
    }

    if (score >= CHALLENGE_THRESHOLD && this.options.challengeSuspicious) {
      return this.buildResult({
        action: GuardAction.CHALLENGE,
        threatLevel: score >= 0.7 ? ThreatLevel.HIGH : ThreatLevel.MEDIUM,
        score,
        reason: `Suspicious bot-like behavior: ${matchedSignals.join(', ')}`,
        metadata: { matchedSignals, headerCount },
        durationMs: Date.now() - startTime,
      });
    }

    if (score >= LOG_THRESHOLD) {
      return this.buildResult({
        action: GuardAction.LOG_ONLY,
        threatLevel: ThreatLevel.LOW,
        score,
        reason: `Bot-like signals detected: ${matchedSignals.join(', ')}`,
        metadata: { matchedSignals, headerCount },
        durationMs: Date.now() - startTime,
      });
    }

    return this.buildResult({
      action: GuardAction.ALLOW,
      threatLevel: ThreatLevel.NONE,
      score: 0,
      reason: 'Request appears to be from a legitimate user',
      metadata: { matchedSignals: [], headerCount },
      durationMs: Date.now() - startTime,
    });
  }
}

export const botDetector = {
  kind: DetectorKind.BOT,
  priority: 40,
};