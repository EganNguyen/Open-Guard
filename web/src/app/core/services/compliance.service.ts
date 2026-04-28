import { Injectable, inject } from '@angular/core';
import { ApiService } from './api.service';
import { Observable, interval } from 'rxjs';
import { switchMap, takeWhile, filter } from 'rxjs/operators';

export interface ComplianceReport {
  id: string;
  framework: 'GDPR' | 'SOC2' | 'HIPAA';
  status: 'pending' | 'processing' | 'ready' | 'failed';
  period_start: string;
  period_end: string;
  created_at: string;
  download_url?: string;
}

export interface ReportParams {
  framework: string;
  period_start: string;
  period_end: string;
}

@Injectable({ providedIn: 'root' })
export class ComplianceService {
  private api = inject(ApiService);

  generateReport(params: ReportParams): Observable<ComplianceReport> {
    return this.api.post<ComplianceReport>('/v1/compliance/reports', params);
  }

  getReport(id: string): Observable<ComplianceReport> {
    return this.api.get<ComplianceReport>(`/v1/compliance/reports/${id}`);
  }

  listReports(): Observable<ComplianceReport[]> {
    return this.api.get<ComplianceReport[]>('/v1/compliance/reports');
  }

  // Poll report status until ready or failed
  pollReport(id: string): Observable<ComplianceReport> {
    return interval(3000).pipe(
      switchMap(() => this.getReport(id)),
      takeWhile((r) => r.status === 'pending' || r.status === 'processing', true),
      filter((r) => r.status === 'ready' || r.status === 'failed'),
    );
  }
}
