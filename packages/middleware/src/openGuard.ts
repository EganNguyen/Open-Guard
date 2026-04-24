import type { Request, Response, NextFunction, RequestHandler } from 'express';
import type { GuardConfig, GuardRequest, GuardResponse, DetectorConfig, DetectorResult } from '@open-guard/core/types';
import { normalizeRequest, mergeResults, generateRequestId } from '@open-guard/core/utils';
import { GuardAction } from '@open-guard/core/types';
import { createDetector } from './detectorFactory.js';
import { enforceAction } from './enforce.js';
import { guardEvents } from './events.js';

export function openGuard(config: GuardConfig): RequestHandler {
  const detectors = config.detectors
    .filter((d) => d.enabled)
    .sort((a, b) => a.priority - b.priority)
    .map((d) => createDetector(d, config.store));

  const timeoutMs = 5000;

  return async (req: Request, res: Response, next: NextFunction) => {
    const requestId = generateRequestId();
    const startTime = Date.now();

    let guardRequest: GuardRequest;
    try {
      guardRequest = normalizeRequest(req, 'express');
      guardRequest.id = requestId;
    } catch (error) {
      config.logger?.error('Failed to normalize request', { error });
      return next();
    }

    const results: DetectorResult[] = [];

    const detectorPromises = detectors.map(async (detector) => {
      const detectorStart = Date.now();
      try {
        const result = await Promise.race([
          detector.evaluate(guardRequest),
          new Promise<never>((_, reject) =>
            setTimeout(() => reject(new Error('Detector timeout')), timeoutMs)
          ),
        ]);
        result.durationMs = Date.now() - detectorStart;
        return result;
      } catch (error) {
        config.logger?.error(`Detector ${detector.id} error`, { error, detectorId: detector.id });
        return {
          detectorId: detector.id,
          kind: detector.kind,
          action: GuardAction.ALLOW,
          threatLevel: 'NONE' as const,
          score: 0,
          reason: `Error: ${error instanceof Error ? error.message : 'Unknown error'}`,
          durationMs: Date.now() - detectorStart,
        };
      }
    });

    try {
      const resolvedResults = await Promise.all(detectorPromises);
      results.push(...resolvedResults);
    } catch (error) {
      config.logger?.error('Detector pipeline error', { error });
    }

    const guardResponse = mergeResults(results, requestId);
    guardResponse.durationMs = Date.now() - startTime;

    guardEvents.emit('guard:result', guardResponse);
    if (guardResponse.action === GuardAction.BLOCK) {
      guardEvents.emit('guard:block', { request: guardRequest, response: guardResponse });
    }

    if (config.mode === 'dry_run') {
      (req as Record<string, unknown>).guardResponse = guardResponse;
      return next();
    }

    if (guardResponse.action === GuardAction.BLOCK) {
      guardEvents.emit('guard:block', { request: guardRequest, response: guardResponse });
      if (config.onBlock) {
        config.onBlock(guardRequest, guardResponse);
      }
    }

    enforceAction(guardResponse, req, res, next);
  };
}

export interface DetectorInstance {
  id: string;
  kind: string;
  evaluate(req: GuardRequest): Promise<DetectorResult>;
}

function createDetector(config: DetectorConfig, store: GuardConfig['store']): DetectorInstance {
  return {
    id: config.id,
    kind: config.kind,
    async evaluate(req: GuardRequest): Promise<DetectorResult> {
      return {
        detectorId: config.id,
        kind: config.kind as any,
        action: GuardAction.ALLOW,
        threatLevel: 'NONE' as any,
        score: 0,
        reason: 'No implementation',
      };
    },
  };
}