import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

export interface AuditEvent {
  id: string;
  org_id: string;
  source: string;
  action: string;
  actor: string;
  target: string;
  data: any;
  timestamp: string;
}

@Injectable({
  providedIn: 'root'
})
export class AuditService {
  private http = inject(HttpClient);
  private apiUrl = `${environment.apiUrl}/audit/v1`;

  listEvents(orgId: string): Observable<{ events: AuditEvent[] }> {
    return this.http.get<{ events: AuditEvent[] }>(`${this.apiUrl}/events?org_id=${orgId}`);
  }

  streamEvents(orgId: string): Observable<string> {
    return new Observable(observer => {
      const eventSource = new EventSource(`${this.apiUrl}/events/stream?org_id=${orgId}`);
      
      eventSource.onmessage = event => {
        observer.next(event.data);
      };

      eventSource.onerror = error => {
        observer.error(error);
      };

      return () => {
        eventSource.close();
      };
    });
  }
}
