import axios, { AxiosError } from 'axios';
import crypto from 'crypto';

export interface PolicyEvaluationRequest {
  subject_id: string;
  action: string;
  resource: string;
  context?: Record<string, any>;
}

export interface PolicyEvaluationResponse {
  allowed: boolean;
  reason?: string;
  request_id: string;
}

// Simple Circuit Breaker implementation
class CircuitBreaker {
  private failures = 0;
  private state: 'CLOSED' | 'OPEN' | 'HALF_OPEN' = 'CLOSED';
  private nextAttemptAt = 0;

  constructor(private failureThreshold = 3, private resetTimeoutMs = 10000) {}

  async execute<T>(action: () => Promise<T>): Promise<T> {
    if (this.state === 'OPEN') {
      if (Date.now() > this.nextAttemptAt) {
        this.state = 'HALF_OPEN';
      } else {
        throw new Error('Circuit Breaker is OPEN');
      }
    }

    try {
      const result = await action();
      this.onSuccess();
      return result;
    } catch (error) {
      this.onFailure();
      throw error;
    }
  }

  private onSuccess() {
    this.failures = 0;
    this.state = 'CLOSED';
  }

  private onFailure() {
    this.failures++;
    if (this.failures >= this.failureThreshold) {
      this.state = 'OPEN';
      this.nextAttemptAt = Date.now() + this.resetTimeoutMs;
    }
  }
}

export class OpenGuardClient {
  private cache = new Map<string, { allowed: boolean; expiresAt: number }>();
  private baseUrl: string;
  private apiKey: string;
  private orgId: string;
  private cacheTtlMs: number = 60_000; // 60 seconds
  private gracePeriodMs: number = 60_000; // 60 seconds stale-while-unavailable grace period
  private breaker = new CircuitBreaker(3, 10000); // 3 failures, 10s backoff

  constructor() {
    this.baseUrl = process.env.OPENGUARD_URL || 'http://localhost:8080';
    this.apiKey = process.env.OPENGUARD_API_KEY || '';
    this.orgId = process.env.OPENGUARD_ORG_ID || 'default-org';
    
    if (!this.apiKey) {
      console.warn('[OpenGuard] No OPENGUARD_API_KEY provided. Policy evaluations will fail-closed.');
    }
  }

  async allow(subjectId: string, action: string, resource: string, context?: Record<string, any>): Promise<boolean> {
    // REC-08: orgID-scoped cache key
    const key = `${this.orgId}:${subjectId}:${action}:${resource}:${JSON.stringify(context || {})}`;
    const cached = this.cache.get(key);
    const now = Date.now();
    
    // Fresh cache hit
    if (cached && now < cached.expiresAt) {
      return cached.allowed;
    }

    try {
      const response = await this.breaker.execute(() => 
        axios.post<PolicyEvaluationResponse>(
          `${this.baseUrl}/v1/policy/evaluate`,
          {
            subject_id: subjectId,
            action,
            resource,
            context,
          },
          {
            headers: {
              'Authorization': `Bearer ${this.apiKey}`,
              'Content-Type': 'application/json',
              'X-Org-ID': this.orgId,
            },
            timeout: 2000, // Exactly 2s timeout per REC-11 spec
          }
        )
      );

      const allowed = response.data.allowed;
      this.cache.set(key, { 
        allowed, 
        expiresAt: Date.now() + this.cacheTtlMs 
      });

      return allowed;
    } catch (error) {
      console.error('[OpenGuard] Policy evaluation failed:', error instanceof Error ? error.message : String(error));
      
      // REC-11 / REC-12: Stale-While-Unavailable (Grace Period)
      if (cached && now < cached.expiresAt + this.gracePeriodMs) {
        console.warn(`[OpenGuard] Serving STALE cached decision during outage (grace period active) for key: ${key}`);
        return cached.allowed;
      }

      // Default: Fail-Closed
      return false;
    }
  }

  async ingestEvent(event: {
    type: string;
    actor_id: string;
    resource_id?: string;
    action?: string;
    status?: string;
    payload?: Record<string, any>;
  }) {
    try {
      await this.breaker.execute(() => 
        axios.post(
          `${this.baseUrl}/v1/events/ingest`,
          {
            event_id: crypto.randomUUID(),
            occurred_at: new Date().toISOString(),
            ...event,
          },
          {
            headers: {
              'Authorization': `Bearer ${this.apiKey}`,
              'Content-Type': 'application/json',
              'X-Org-ID': this.orgId,
            },
            timeout: 1000,
          }
        )
      );
    } catch (error) {
      console.error('[OpenGuard] Event ingestion failed:', error instanceof Error ? error.message : String(error));
    }
  }
}

export const ogClient = new OpenGuardClient();
