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