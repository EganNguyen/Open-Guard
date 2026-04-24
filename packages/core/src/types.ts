export enum GuardAction {
  ALLOW = 'ALLOW',
  BLOCK = 'BLOCK',
  CHALLENGE = 'CHALLENGE',
  LOG_ONLY = 'LOG_ONLY',
  RATE_LIMIT = 'RATE_LIMIT',
}

export enum ThreatLevel {
  NONE = 'NONE',
  LOW = 'LOW',
  MEDIUM = 'MEDIUM',
  HIGH = 'HIGH',
  CRITICAL = 'CRITICAL',
}

export enum DetectorKind {
  RATE_LIMIT = 'RATE_LIMIT',
  SQL_INJECTION = 'SQL_INJECTION',
  XSS = 'XSS',
  CSRF = 'CSRF',
  BOT = 'BOT',
  ANOMALY = 'ANOMALY',
  GEO_BLOCK = 'GEO_BLOCK',
  IP_REPUTATION = 'IP_REPUTATION',
  PAYLOAD_SIZE = 'PAYLOAD_SIZE',
  AUTH_BRUTE_FORCE = 'AUTH_BRUTE_FORCE',
  HEADER_INJECTION = 'HEADER_INJECTION',
  PATH_TRAVERSAL = 'PATH_TRAVERSAL',
}

export enum RuleOperator {
  AND = 'AND',
  OR = 'OR',
  NOT = 'NOT',
}

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'OPTIONS' | 'HEAD';

export interface GeoInfo {
  country?: string;
  region?: string;
  city?: string;
  lat?: number;
  lon?: number;
  asn?: string;
}

export interface GuardRequest {
  id: string;
  ip: string;
  method: HttpMethod;
  path: string;
  query?: Record<string, string | string[]>;
  headers: Record<string, string>;
  body?: unknown;
  timestamp: number;
  userId?: string;
  sessionId?: string;
  geo?: GeoInfo;
}

export interface DetectorResult {
  detectorId: string;
  kind: DetectorKind;
  action: GuardAction;
  threatLevel: ThreatLevel;
  score: number;
  reason: string;
  metadata?: Record<string, unknown>;
  durationMs?: number;
}

export interface GuardResponse {
  requestId: string;
  action: GuardAction;
  threatLevel: ThreatLevel;
  results: DetectorResult[];
  durationMs: number;
  blockedBy?: string;
}

export type GuardMode = 'enforce' | 'monitor' | 'dry_run';

export interface DetectorConfig {
  id: string;
  kind: DetectorKind;
  enabled: boolean;
  priority: number;
  options?: Record<string, unknown>;
}

export interface GuardConfig {
  mode: GuardMode;
  detectors: DetectorConfig[];
  store: StoreAdapter;
  logger?: LoggerAdapter;
  onBlock?: (req: GuardRequest, res: GuardResponse) => void;
}

export interface StoreAdapter {
  get(key: string): Promise<string | null>;
  set(key: string, value: string, ttlSeconds?: number): Promise<void>;
  incr(key: string, ttlSeconds?: number): Promise<number>;
  del(key: string): Promise<void>;
}

export interface LoggerAdapter {
  info(msg: string, meta?: Record<string, unknown>): void;
  warn(msg: string, meta?: Record<string, unknown>): void;
  error(msg: string, meta?: Record<string, unknown>): void;
}