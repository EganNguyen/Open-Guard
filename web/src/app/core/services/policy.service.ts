import { Injectable, inject } from '@angular/core';
import { ApiService } from './api.service';
import { Observable } from 'rxjs';
import { Policy, EvaluateRequest, EvaluateResponse } from '../models/policy.model';
import { HttpParams } from '@angular/common/http';
import { AuditLog } from '../models/audit.model';

@Injectable({
  providedIn: 'root'
})
export class PolicyService {
  private api = inject(ApiService);

  listPolicies(orgId: string): Observable<{ policies: Policy[], total: number }> {
    const params = new HttpParams().set('org_id', orgId);
    return this.api.get<{ policies: Policy[], total: number }>('/v1/policies', params);
  }

  getPolicy(id: string, orgId: string): Observable<Policy> {
    const params = new HttpParams().set('org_id', orgId);
    return this.api.get<Policy>(`/v1/policies/${id}`, params);
  }

  createPolicy(policy: Partial<Policy>): Observable<Policy> {
    return this.api.post<Policy>('/v1/policies', policy);
  }

  updatePolicy(id: string, policy: Partial<Policy>): Observable<Policy> {
    return this.api.put<Policy>(`/v1/policies/${id}`, policy);
  }

  deletePolicy(id: string, orgId: string): Observable<any> {
    const params = new HttpParams().set('org_id', orgId);
    return this.api.delete<any>(`/v1/policies/${id}`, params);
  }

  evaluate(request: EvaluateRequest): Observable<EvaluateResponse> {
    return this.api.post<EvaluateResponse>('/v1/policy/evaluate', request);
  }

  listEvalLogs(orgId: string, limit: number = 100): Observable<{ logs: AuditLog[], total: number }> {
    const params = new HttpParams()
      .set('org_id', orgId)
      .set('limit', limit.toString());
    return this.api.get<{ logs: AuditLog[], total: number }>('/v1/policy/eval-logs', params);
  }
}
