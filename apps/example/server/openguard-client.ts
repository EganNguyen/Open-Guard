import axios from 'axios';

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

export class OpenGuardClient {
  private cache = new Map<string, { allowed: boolean; expiresAt: number }>();
  private baseUrl: string;
  private apiKey: string;
  private cacheTtlMs: number = 60_000; // 60 seconds

  constructor() {
    this.baseUrl = process.env.OPENGUARD_URL || 'http://localhost:8080';
    this.apiKey = process.env.OPENGUARD_API_KEY || '';
    
    if (!this.apiKey) {
      console.warn('[OpenGuard] No OPENGUARD_API_KEY provided. Policy evaluations will fail-closed.');
    }
  }

  async allow(subjectId: string, action: string, resource: string, context?: Record<string, any>): Promise<boolean> {
    const key = `${subjectId}:${action}:${resource}:${JSON.stringify(context || {})}`;
    const cached = this.cache.get(key);
    
    if (cached && Date.now() < cached.expiresAt) {
      return cached.allowed;
    }

    try {
      const response = await axios.post<PolicyEvaluationResponse>(
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
          },
          timeout: 2000, // 2s timeout for policy evaluation
        }
      );

      const allowed = response.data.allowed;
      this.cache.set(key, { 
        allowed, 
        expiresAt: Date.now() + this.cacheTtlMs 
      });

      return allowed;
    } catch (error) {
      console.error('[OpenGuard] Policy evaluation failed:', error instanceof Error ? error.message : error);
      // Fail-closed behavior
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
      await axios.post(
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
          },
          timeout: 1000,
        }
      );
    } catch (error) {
      console.error('[OpenGuard] Event ingestion failed:', error instanceof Error ? error.message : error);
    }
  }
}

export const ogClient = new OpenGuardClient();
