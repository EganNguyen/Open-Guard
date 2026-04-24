import { RequestHandler } from 'express';
import {
  GuardConfig,
  GuardRequest,
  GuardResponse,
  GuardAction,
  normalizeRequest,
  mergeResults,
  generateRequestId,
} from '@open-guard/core';
import { createDetector, BaseDetector } from '@open-guard/detectors';
import { enforceAction } from './enforce';
import { globalEventEmitter } from './events';

export interface OpenGuardOptions extends Partial<GuardConfig> {
  detectors: GuardConfig['detectors'];
  store: GuardConfig['store'];
  mode?: GuardConfig['mode'];
  logger?: GuardConfig['logger'];
  onBlock?: GuardConfig['onBlock'];
}

const DETECTOR_TIMEOUT_MS = 5000;

export function openGuard(config: OpenGuardOptions): RequestHandler {
  const detectors: BaseDetector[] = config.detectors
    .filter(d => d.enabled !== false)
    .sort((a, b) => a.priority - b.priority)
    .map(d => createDetector(d, config.store));

  return async (req, res, next) => {
    const requestId = generateRequestId();
    const startTime = Date.now();

    const guardRequest: GuardRequest = normalizeRequest(req, 'express');
    guardRequest.id = requestId;

    if (config.mode === 'monitor') {
      res.setHeader('X-Guard-Request-Id', requestId);
      return next();
    }

    const results = await Promise.allSettled(
      detectors.map(async (detector) => {
        const timeoutPromise = new Promise<never>((_, reject) => {
          setTimeout(() => reject(new Error(`Detector ${detector.config.id} timed out`)), DETECTOR_TIMEOUT_MS);
        });

        try {
          return await Promise.race([detector.evaluate(guardRequest), timeoutPromise]);
        } catch (error) {
          config.logger?.error?.(`Detector ${detector.config.id} failed`, { error });
          globalEventEmitter.emitGuardError({
            detectorId: detector.config.id,
            error: error instanceof Error ? error : new Error(String(error)),
            request: guardRequest,
          });
          return {
            detectorId: detector.config.id,
            kind: detector.config.kind,
            action: GuardAction.ALLOW,
            threatLevel: 'NONE' as const,
            score: 0,
            reason: `Detector error: ${error instanceof Error ? error.message : 'Unknown error'}`,
            durationMs: DETECTOR_TIMEOUT_MS,
          };
        }
      })
    );

    const detectorResults = results.map(r => {
      if (r.status === 'fulfilled') {
        return r.value;
      }
      return {
        detectorId: 'unknown',
        kind: 'ANOMALY' as const,
        action: GuardAction.ALLOW,
        threatLevel: 'NONE' as const,
        score: 0,
        reason: `Evaluation failed: ${r.reason}`,
        durationMs: 0,
      };
    });

    const response: GuardResponse = mergeResults(detectorResults, requestId);
    response.durationMs = Date.now() - startTime;

    globalEventEmitter.emitGuardResult(response);

    if (response.action === GuardAction.BLOCK && config.onBlock) {
      try {
        config.onBlock(guardRequest, response);
      } catch (error) {
        config.logger?.error?.('onBlock callback failed', { error });
      }
    }

    enforceAction({
      request: guardRequest,
      response,
      res,
      next,
    });
  };
}

export { createDetector, BaseDetector } from '@open-guard/detectors';
export { MemoryStore } from './stores/memory';
export { RedisStore, createRedisStore } from './stores/redis';
export { UpstashStore, createUpstashStore } from './stores/upstash';
export { globalEventEmitter, OpenGuardEventEmitter } from './events';
export * from './enforce';