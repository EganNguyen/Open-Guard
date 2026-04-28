import { Injectable, inject } from '@angular/core';
import { ApiService } from './api.service';
import { Observable } from 'rxjs';
import { HttpParams } from '@angular/common/http';

export interface DlpFinding {
  id: string;
  org_id: string;
  event_id: string;
  finding_type: string;
  confidence: number;
  redacted_value: string;
  created_at: string;
}

export interface DlpPolicy {
  id: string;
  org_id: string;
  name: string;
  mode: 'monitor' | 'block';
  patterns: string[];
  created_at: string;
  updated_at: string;
}

export interface DlpFindingsParams {
  limit?: number;
  cursor?: string;
  finding_type?: string;
}

export interface PagedFindings {
  items: DlpFinding[];
  next_cursor: string;
}

@Injectable({ providedIn: 'root' })
export class DlpService {
  private api = inject(ApiService);

  getFindings(params: DlpFindingsParams): Observable<PagedFindings> {
    let httpParams = new HttpParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value) httpParams = httpParams.set(key, value.toString());
    });
    return this.api.get<PagedFindings>('/v1/dlp/findings', httpParams);
  }

  getPolicies(): Observable<DlpPolicy[]> {
    return this.api.get<DlpPolicy[]>('/v1/dlp/policies');
  }

  createPolicy(policy: Partial<DlpPolicy>): Observable<DlpPolicy> {
    return this.api.post<DlpPolicy>('/v1/dlp/policies', policy);
  }

  updatePolicy(id: string, mode: 'monitor' | 'block'): Observable<DlpPolicy> {
    return this.api.patch<DlpPolicy>(`/v1/dlp/policies/${id}`, { mode });
  }
}
