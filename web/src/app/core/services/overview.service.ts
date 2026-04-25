import { Injectable, inject } from '@angular/core';
import { ApiService } from './api.service';
import { Observable, forkJoin, map, catchError, of } from 'rxjs';

export interface DashboardStats {
  connectedApps: number;
  activeSessions: number;
  policyEvaluations: string;
  securityAlerts: number;
}

import { User } from '../models/user.model';
import { Connector } from '../models/connector.model';

export interface DashboardStat {
  label: string;
  value: string;
  trend: string;
  icon: string;
  color: string;
}

export interface Activity {
  type: string;
  user: string;
  action: string;
  time: string;
  icon: string;
}

export interface ServiceHealth {
  name: string;
  status: string;
  color: string;
  dot: string;
}


@Injectable({
  providedIn: 'root'
})
export class OverviewService {
  private api = inject(ApiService);

  getDashboardData(): Observable<{ stats: DashboardStat[], activities: Activity[], health: ServiceHealth[] }> {
    return forkJoin({
      connectors: this.api.get<Connector[]>('/mgmt/connectors').pipe(catchError(() => of([]))),
      users: this.api.get<User[]>('/mgmt/users').pipe(catchError(() => of([]))),
      iamHealth: this.api.get<{ status: string }>('/health/iam').pipe(catchError(() => of({ status: 'DOWN' }))),
      policyHealth: this.api.get<{ status: string }>('/health/policy').pipe(catchError(() => of({ status: 'DOWN' }))),
      auditHealth: this.api.get<{ status: string }>('/health/audit').pipe(catchError(() => of({ status: 'DOWN' }))),
      controlPlaneHealth: this.api.get<{ status: string }>('/health/control-plane').pipe(catchError(() => of({ status: 'DOWN' })))
    }).pipe(
      map(({ connectors, users, iamHealth, policyHealth, auditHealth, controlPlaneHealth }) => {
        // Map real data to the stats format used in HomeComponent
        const stats = [
          { 
            label: 'Connected Apps', 
            value: (connectors || []).length.toString(), 
            trend: (connectors || []).length > 0 ? 'Active' : 'None', 
            icon: 'hub', 
            color: 'text-blue-600' 
          },
          { 
            label: 'Active Users', 
            value: (users || []).length.toString(), 
            trend: '+'+(users || []).length, 
            icon: 'group', 
            color: 'text-green-600' 
          },
          { 
            label: 'Policy Evaluations', 
            value: '0', 
            trend: 'N/A', 
            icon: 'task_alt', 
            color: 'text-purple-600' 
          },
          { 
            label: 'Security Alerts', 
            value: '0', 
            trend: 'Stable', 
            icon: 'warning', 
            color: 'text-red-600' 
          }
        ];

        // Generate activities from real users and connectors
        const activities: Activity[] = [];
        
        // Add connector activities
        (connectors || []).forEach(c => {
          activities.push({
            type: 'connector',
            user: 'system',
            action: `Connector "${c.name}" is active`,
            time: 'Recent',
            icon: 'add_link'
          });
        });

        // Add user activities
        (users || []).forEach(u => {
          activities.push({
            type: 'login',
            user: u.email,
            action: `User "${u.display_name}" registered`,
            time: this.formatDate(u.created_at),
            icon: 'person_add'
          });
        });

        const health = [
          { name: 'Control Plane', status: controlPlaneHealth?.status === 'OK' ? 'Healthy' : 'Degraded', color: controlPlaneHealth?.status === 'OK' ? 'text-green-600' : 'text-red-600', dot: controlPlaneHealth?.status === 'OK' ? 'bg-green-600' : 'bg-red-600' },
          { name: 'IAM Service', status: iamHealth?.status === 'OK' ? 'Healthy' : 'Degraded', color: iamHealth?.status === 'OK' ? 'text-green-600' : 'text-red-600', dot: iamHealth?.status === 'OK' ? 'bg-green-600' : 'bg-red-600' },
          { name: 'Policy Engine', status: policyHealth?.status === 'OK' ? 'Healthy' : 'Degraded', color: policyHealth?.status === 'OK' ? 'text-green-600' : 'text-red-600', dot: policyHealth?.status === 'OK' ? 'bg-green-600' : 'bg-red-600' },
          { name: 'Audit Service', status: auditHealth?.status === 'OK' ? 'Healthy' : 'Degraded', color: auditHealth?.status === 'OK' ? 'text-green-600' : 'text-red-600', dot: auditHealth?.status === 'OK' ? 'bg-green-600' : 'bg-red-600' }
        ];

        return { 
          stats, 
          activities: activities.slice(0, 5),
          health
        };
      })
    );
  }

  private formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    const now = new Date();
    const diff = now.getTime() - date.getTime();
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(minutes / 60);
    const days = Math.floor(hours / 24);

    if (days > 0) return `${days}d ago`;
    if (hours > 0) return `${hours}h ago`;
    if (minutes > 0) return `${minutes}m ago`;
    return 'Just now';
  }
}
