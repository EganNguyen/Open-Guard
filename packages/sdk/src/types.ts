import { ThreatLevel, GuardAction, DetectorKind } from '@open-guard/core';

export interface ClientGuardEvent {
  type: 'fetch_blocked' | 'challenge_shown' | 'challenge_passed' | 'manual_report';
  url: string;
  statusCode: number;
  requestId?: string;
  timestamp: number;
}

export interface GuardStreamEvent {
  type: 'block' | 'allow' | 'rate_limit' | 'challenge' | 'error';
  requestId: string;
  ip: string;
  path: string;
  detectorId: string;
  threatLevel: ThreatLevel;
  timestamp: number;
}

export interface ThreatSummary {
  detectorId: string;
  kind: DetectorKind;
  count: number;
  percentage: number;
}

export interface IpSummary {
  ip: string;
  count: number;
  blockedCount: number;
}

export interface TimelinePoint {
  timestamp: number;
  total: number;
  blocked: number;
  rateLimited: number;
  challenged: number;
  allowed: number;
}

export interface GuardStats {
  totalRequests: number;
  blockedRequests: number;
  rateLimitedRequests: number;
  challengedRequests: number;
  topThreats: ThreatSummary[];
  topBlockedIps: IpSummary[];
  timeline: TimelinePoint[];
}

export interface StatsFilter {
  from?: number;
  to?: number;
  detectorIds?: string[];
  actions?: GuardAction[];
}

export interface ChallengePayload {
  token: string;
  type: 'captcha' | 'totp' | 'email_otp';
  expiresAt: number;
}

export interface OpenGuardClientOptions {
  baseUrl: string;
  apiKey?: string;
  websocketUrl?: string;
  autoReport?: boolean;
  onChallenge?: (challenge: ChallengePayload) => Promise<string>;
}

export type Unsubscribe = () => void;