import { Component, inject, OnInit, signal, OnDestroy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { AuditService, AuditEvent } from '../core/services/audit.service';
import { AuthService } from '../core/services/auth.service';
import { Subscription } from 'rxjs';

@Component({
  selector: 'app-audit-logs',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './audit-logs.html',
  styleUrls: ['./audit-logs.css']
})
export class AuditLogComponent implements OnInit, OnDestroy {
  private auditService = inject(AuditService);
  private authService = inject(AuthService);
  private streamSubscription?: Subscription;

  logs = signal<any[]>([]);
  loading = signal(true);

  ngOnInit() {
    this.loadLogs();
  }

  ngOnDestroy() {
    this.streamSubscription?.unsubscribe();
  }

  loadLogs() {
    const user = this.authService.user();
    if (!user) return;

    this.loading.set(true);
    this.auditService.listEvents(user.org_id).subscribe({
      next: (res) => {
        this.logs.set(res.events);
        this.loading.set(false);
        this.startStreaming(user.org_id);
      },
      error: (err) => {
        console.error('Failed to load audit logs', err);
        this.loading.set(false);
      }
    });
  }

  startStreaming(orgId: string) {
    this.streamSubscription = this.auditService.streamEvents(orgId).subscribe({
      next: (data) => {
        const event = JSON.parse(data);
        this.logs.update(prev => [event, ...prev.slice(0, 49)]);
      }
    });
  }

  getStatusClass(effect: string): string {
    return effect === 'allow' 
      ? 'bg-green-100 text-green-700' 
      : 'bg-red-100 text-red-700';
  }

  getCacheClass(cacheHit: boolean): string {
    return cacheHit 
      ? 'bg-blue-50 text-blue-600' 
      : 'bg-gray-100 text-gray-600';
  }
}
