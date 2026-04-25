import { Injectable, inject, NgZone } from '@angular/core';
import { Observable, Subject } from 'rxjs';
import { environment } from '../../../environments/environment';
import { AuthService } from './auth.service';

@Injectable({
  providedIn: 'root'
})
export class SseService {
  private zone = inject(NgZone);
  private auth = inject(AuthService);
  private apiUrl = environment.apiUrl;
  private eventSource: EventSource | null = null;
  private eventSubject = new Subject<any>();

  /**
   * Connects to the audit event stream for the current organization.
   * Implements reconnection logic and zone-awareness for Angular change detection.
   */
  connect(): Observable<any> {
    const orgId = this.auth.getCurrentOrgId();
    if (!orgId) {
      console.warn('SseService: No OrgID available for SSE connection');
      return this.eventSubject.asObservable();
    }

    if (this.eventSource) {
      this.eventSource.close();
    }

    // Pass org_id via header or query param if needed. 
    // Our backend expects it in context (from JWT) or X-Org-ID header.
    // EventSource doesn't support custom headers natively without a polyfill.
    // We'll use a query param or rely on the HttpOnly cookie for auth.
    const url = `${this.apiUrl}/audit/v1/events/stream?org_id=${orgId}`;
    
    this.eventSource = new EventSource(url, { withCredentials: true });

    this.eventSource.onmessage = (event) => {
      this.zone.run(() => {
        try {
          const data = JSON.parse(event.data);
          this.eventSubject.next(data);
        } catch (e) {
          this.eventSubject.next(event.data);
        }
      });
    };

    this.eventSource.onerror = (error) => {
      this.zone.run(() => {
        console.error('SseService: EventSource error', error);
        this.eventSubject.error(error);
        // EventSource will automatically attempt reconnection by default.
      });
    };

    return this.eventSubject.asObservable();
  }

  disconnect(): void {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
  }
}
