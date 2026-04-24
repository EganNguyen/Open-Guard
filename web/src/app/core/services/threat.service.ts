import { Injectable, inject } from '@angular/core';
import { HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { ApiService } from './api.service';

@Injectable({ providedIn: 'root' })
export class ThreatService {
  private api = inject(ApiService);

  listAlerts(params?: { status?: string; severity?: string }): Observable<any> {
    let httpParams = new HttpParams();
    if (params?.status) httpParams = httpParams.set('status', params.status);
    if (params?.severity) httpParams = httpParams.set('severity', params.severity);
    return this.api.get('/v1/threats/alerts', httpParams);
  }

  acknowledgeAlert(id: string): Observable<any> {
    return this.api.post(`/v1/threats/alerts/${id}/acknowledge`, {});
  }

  resolveAlert(id: string): Observable<any> {
    return this.api.post(`/v1/threats/alerts/${id}/resolve`, {});
  }

  getStats(): Observable<any> {
    return this.api.get('/v1/threats/stats');
  }
}
