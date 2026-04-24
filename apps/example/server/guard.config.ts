import { DetectorConfig, DetectorKind } from '@open-guard/core';

export const guardConfig = {
  mode: process.env.GUARD_MODE as 'enforce' | 'monitor' | 'dry_run' || 'enforce',
  detectors: [
    {
      id: 'payload-size',
      kind: DetectorKind.PAYLOAD_SIZE,
      enabled: true,
      priority: 5,
      options: {
        maxBodyBytes: 1048576,
        maxUrlLength: 2048,
      },
    },
    {
      id: 'rate-limiter',
      kind: DetectorKind.RATE_LIMIT,
      enabled: true,
      priority: 10,
      options: {
        windowSeconds: 60,
        maxRequests: 100,
        keyBy: ['ip'],
        skipPaths: ['/api/health'],
      },
    },
    {
      id: 'auth-brute-force',
      kind: DetectorKind.AUTH_BRUTE_FORCE,
      enabled: true,
      priority: 15,
      options: {
        watchPaths: ['/api/login'],
        maxAttemptsPerIp: 5,
        windowSeconds: 300,
      },
    },
    {
      id: 'path-traversal',
      kind: DetectorKind.PATH_TRAVERSAL,
      enabled: true,
      priority: 18,
      options: {
        inspectQuery: true,
      },
    },
    {
      id: 'sql-injection',
      kind: DetectorKind.SQL_INJECTION,
      enabled: true,
      priority: 20,
      options: {
        sensitivity: 'medium',
      },
    },
    {
      id: 'header-injection',
      kind: DetectorKind.HEADER_INJECTION,
      enabled: true,
      priority: 22,
      options: {
        allowedHosts: ['localhost:3001', 'example.openguard.dev'],
      },
    },
    {
      id: 'xss',
      kind: DetectorKind.XSS,
      enabled: true,
      priority: 25,
      options: {
        sensitivity: 'medium',
      },
    },
    {
      id: 'csrf',
      kind: DetectorKind.CSRF,
      enabled: true,
      priority: 30,
      options: {
        skipPaths: ['/api/login', '/api/webhooks/*'],
      },
    },
    {
      id: 'bot-detection',
      kind: DetectorKind.BOT,
      enabled: true,
      priority: 40,
      options: {
        blockKnownBots: false,
        challengeSuspicious: true,
      },
    },
    {
      id: 'geo-block',
      kind: DetectorKind.GEO_BLOCK,
      enabled: false,
      priority: 50,
      options: {
        blockedCountries: [],
      },
    },
    {
      id: 'ip-reputation',
      kind: DetectorKind.IP_REPUTATION,
      enabled: true,
      priority: 55,
      options: {},
    },
    {
      id: 'anomaly',
      kind: DetectorKind.ANOMALY,
      enabled: false,
      priority: 80,
      options: {
        baselineWindowSeconds: 3600,
        minSamplesForBaseline: 20,
      },
    },
  ] as DetectorConfig[],
};