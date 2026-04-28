import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

export interface AuditEvent {
  id: string;
  org_id: string;
  subject_id: string;
  action: string;
  resource: string;
  effect: 'allow' | 'deny';
  matched_policy_ids: string[];
  cache_hit: boolean;
  latency_ms: number;
  created_at: string;
}

import { SseService } from './sse.service';

@Injectable({
  providedIn: 'root',
})
export class AuditService {
  private http = inject(HttpClient);
  private sse = inject(SseService);
  private apiUrl = `${environment.apiUrl}/audit/v1`;

  listEvents(orgId: string): Observable<{ events: AuditEvent[] }> {
    return this.http.get<{ events: AuditEvent[] }>(`${this.apiUrl}/events?org_id=${orgId}`);
  }

  streamEvents(): Observable<AuditEvent> {
    return this.sse.connect();
  }
}
