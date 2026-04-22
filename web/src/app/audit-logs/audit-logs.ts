import { Component, inject, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { PolicyService } from '../core/services/policy.service';
import { AuthService } from '../core/services/auth.service';
import { AuditLog } from '../core/models/audit.model';

@Component({
  selector: 'app-audit-logs',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './audit-logs.html',
  styleUrls: ['./audit-logs.css']
})
export class AuditLogComponent implements OnInit {
  private policyService = inject(PolicyService);
  private authService = inject(AuthService);

  logs = signal<AuditLog[]>([]);
  loading = signal(true);

  ngOnInit() {
    this.loadLogs();
  }

  loadLogs() {
    const user = this.authService.user();
    if (!user) return;

    this.loading.set(true);
    this.policyService.listEvalLogs(user.org_id).subscribe({
      next: (res) => {
        this.logs.set(res.logs);
        this.loading.set(false);
      },
      error: (err) => {
        console.error('Failed to load audit logs', err);
        this.loading.set(false);
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
