import { EventEmitter } from 'events';
import { GuardResponse, GuardRequest } from '@open-guard/core';

export type GuardEventType = 'guard:result' | 'guard:block' | 'guard:error' | 'guard:ratelimit';

export interface GuardBlockEvent {
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

export class OpenGuardEventEmitter extends EventEmitter {
  emitGuardResult(response: GuardResponse): void {
    this.emit('guard:result', response);
  }

  emitGuardBlock(event: GuardBlockEvent): void {
    this.emit('guard:block', event);
  }

  emitGuardError(event: GuardErrorEvent): void {
    this.emit('guard:error', event);
  }

  emitGuardRateLimit(event: GuardRateLimitEvent): void {
    this.emit('guard:ratelimit', event);
  }

  onGuardResult(handler: (response: GuardResponse) => void): this {
    return this.on('guard:result', handler);
  }

  onGuardBlock(handler: (event: GuardBlockEvent) => void): this {
    return this.on('guard:block', handler);
  }

  onGuardError(handler: (event: GuardErrorEvent) => void): this {
    return this.on('guard:error', handler);
  }

  onGuardRateLimit(handler: (event: GuardRateLimitEvent) => void): this {
    return this.on('guard:ratelimit', handler);
  }
}

export const globalEventEmitter = new OpenGuardEventEmitter();