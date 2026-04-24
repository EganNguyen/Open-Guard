import { DetectorConfig, StoreAdapter, DetectorKind } from '@open-guard/core';
import { BaseDetector } from './base/BaseDetector';
import { RateLimitDetector } from './RateLimitDetector';
import { SqlInjectionDetector } from './SqlInjectionDetector';
import { XssDetector } from './XssDetector';
import { CsrfDetector } from './CsrfDetector';
import { BotDetector } from './BotDetector';
import { GeoBlockDetector } from './GeoBlockDetector';
import { IpReputationDetector } from './IpReputationDetector';
import { PayloadSizeDetector } from './PayloadSizeDetector';
import { AuthBruteForceDetector } from './AuthBruteForceDetector';
import { HeaderInjectionDetector } from './HeaderInjectionDetector';
import { PathTraversalDetector } from './PathTraversalDetector';
import { AnomalyDetector } from './AnomalyDetector';

export interface DetectorEntry {
  id: string;
  kind: DetectorKind;
  class: new (config: DetectorConfig, store: StoreAdapter) => BaseDetector;
}

export const DETECTOR_REGISTRY: Record<string, DetectorEntry> = {
  [DetectorKind.RATE_LIMIT]: {
    id: 'rate-limiter',
    kind: DetectorKind.RATE_LIMIT,
    class: RateLimitDetector,
  },
  [DetectorKind.SQL_INJECTION]: {
    id: 'sql-injection',
    kind: DetectorKind.SQL_INJECTION,
    class: SqlInjectionDetector,
  },
  [DetectorKind.XSS]: {
    id: 'xss',
    kind: DetectorKind.XSS,
    class: XssDetector,
  },
  [DetectorKind.CSRF]: {
    id: 'csrf',
    kind: DetectorKind.CSRF,
    class: CsrfDetector,
  },
  [DetectorKind.BOT]: {
    id: 'bot-detection',
    kind: DetectorKind.BOT,
    class: BotDetector,
  },
  [DetectorKind.GEO_BLOCK]: {
    id: 'geo-block',
    kind: DetectorKind.GEO_BLOCK,
    class: GeoBlockDetector,
  },
  [DetectorKind.IP_REPUTATION]: {
    id: 'ip-reputation',
    kind: DetectorKind.IP_REPUTATION,
    class: IpReputationDetector,
  },
  [DetectorKind.PAYLOAD_SIZE]: {
    id: 'payload-size',
    kind: DetectorKind.PAYLOAD_SIZE,
    class: PayloadSizeDetector,
  },
  [DetectorKind.AUTH_BRUTE_FORCE]: {
    id: 'auth-brute-force',
    kind: DetectorKind.AUTH_BRUTE_FORCE,
    class: AuthBruteForceDetector,
  },
  [DetectorKind.HEADER_INJECTION]: {
    id: 'header-injection',
    kind: DetectorKind.HEADER_INJECTION,
    class: HeaderInjectionDetector,
  },
  [DetectorKind.PATH_TRAVERSAL]: {
    id: 'path-traversal',
    kind: DetectorKind.PATH_TRAVERSAL,
    class: PathTraversalDetector,
  },
  [DetectorKind.ANOMALY]: {
    id: 'anomaly',
    kind: DetectorKind.ANOMALY,
    class: AnomalyDetector,
  },
};

export function createDetector(config: DetectorConfig, store: StoreAdapter): BaseDetector {
  const entry = DETECTOR_REGISTRY[config.kind];
  if (!entry) {
    throw new Error(`Unknown detector kind: ${config.kind}`);
  }
  return new entry.class(config, store);
}

export function getDetectorKind(id: string): DetectorKind | undefined {
  const entry = Object.values(DETECTOR_REGISTRY).find((e) => e.id === id);
  return entry?.kind;
}

export {
  RateLimitDetector,
  SqlInjectionDetector,
  XssDetector,
  CsrfDetector,
  BotDetector,
  GeoBlockDetector,
  IpReputationDetector,
  PayloadSizeDetector,
  AuthBruteForceDetector,
  HeaderInjectionDetector,
  PathTraversalDetector,
  AnomalyDetector,
};