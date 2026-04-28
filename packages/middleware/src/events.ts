import { EventEmitter } from 'events';
import type { GuardResponse, GuardRequest } from '@open-guard/core/types';

export const guardEvents = new EventEmitter();

export interface GuardResultEvent {
  request: GuardRequest;
  response: GuardResponse;
}

export interface GuardErrorEvent {
  detectorId: string;
  error: Error;
  request: GuardRequest;
}

export interface GuardRateLimitEvent {
  request: GuardRequest;
  detectorId: string;
  ttl: number;
}

guardEvents.on('guard:result', (response: GuardResponse) => {
});

guardEvents.on('guard:block', (event: GuardResultEvent) => {
});

guardEvents.on('guard:error', (event: GuardErrorEvent) => {
});

guardEvents.on('guard:ratelimit', (event: GuardRateLimitEvent) => {
});

export function emitGuardResult(response: GuardResponse): void {
  guardEvents.emit('guard:result', response);
}

export function emitGuardBlock(request: GuardRequest, response: GuardResponse): void {
  guardEvents.emit('guard:block', { request, response });
}

export function emitGuardError(detectorId: string, error: Error, request: GuardRequest): void {
  guardEvents.emit('guard:error', { detectorId, error, request });
}

export function emitGuardRateLimit(request: GuardRequest, detectorId: string, ttl: number): void {
  guardEvents.emit('guard:ratelimit', { request, detectorId, ttl });
}

// OpenGuardEventEmitter wraps the raw EventEmitter with typed helper methods.
// This is the preferred API for consumers — do not use guardEvents directly.
export class OpenGuardEventEmitter {
  onGuardResult(cb: (response: GuardResponse) => void): () => void {
    guardEvents.on('guard:result', cb);
    return () => guardEvents.off('guard:result', cb);
  }

  onGuardBlock(cb: (event: GuardResultEvent) => void): () => void {
    guardEvents.on('guard:block', cb);
    return () => guardEvents.off('guard:block', cb);
  }

  onGuardError(cb: (event: GuardErrorEvent) => void): () => void {
    guardEvents.on('guard:error', cb);
    return () => guardEvents.off('guard:error', cb);
  }

  onGuardRateLimit(cb: (event: GuardRateLimitEvent) => void): () => void {
    guardEvents.on('guard:ratelimit', cb);
    return () => guardEvents.off('guard:ratelimit', cb);
  }

  off(event: string, listener: (...args: any[]) => void): void {
    guardEvents.off(event, listener);
  }
}

// Singleton instance — imported as `globalEventEmitter` by consumers.
export const globalEventEmitter = new OpenGuardEventEmitter();
