import { Component, OnInit, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ThreatService } from '../core/services/threat.service';
import { finalize } from 'rxjs';

import { ThreatAlert, ThreatStats } from '../core/models/threat.model';


@Component({
  selector: 'app-threats',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './threats.html',
  styleUrls: ['./threats.css']
})
export class ThreatsComponent implements OnInit {
  private threatService = inject(ThreatService);
  
  alerts = signal<ThreatAlert[]>([]);
  loading = signal(false);
  error = signal<string | null>(null);

  stats = signal({
    total: 0,
    critical: 0,
    open: 0,
    resolved: 0
  });

  ngOnInit(): void {
    this.fetchThreats();
  }

  fetchThreats(): void {
    this.loading.set(true);
    this.threatService.listAlerts()
      .pipe(finalize(() => this.loading.set(false)))
      .subscribe({
        next: (alerts) => {
          const data = alerts || [];
          this.alerts.set(data);
          this.calculateStats(data);
        },
        error: (err) => {
          console.error('Failed to fetch threats', err);
          this.error.set('Failed to load threat data. Please try again later.');
        }
      });
  }

  private calculateStats(alerts: ThreatAlert[]): void {
    const stats = {
      total: alerts.length,
      critical: alerts.filter(a => a.severity === 'CRITICAL' || a.severity === 'HIGH').length,
      open: alerts.filter(a => a.status === 'OPEN').length,
      resolved: alerts.filter(a => a.status === 'RESOLVED').length
    };
    this.stats.set(stats);
  }

  getThreatLevelClass(level: string): string {
    switch (level) {
      case 'CRITICAL': return 'bg-red-100 text-red-800 border-red-200';
      case 'HIGH': return 'bg-orange-100 text-orange-800 border-orange-200';
      case 'MEDIUM': return 'bg-yellow-100 text-yellow-800 border-yellow-200';
      default: return 'bg-blue-100 text-blue-800 border-blue-200';
    }
  }

  getActionClass(action: string): string {
    switch (action) {
      case 'BLOCK':
      case 'DENY': return 'text-red-600 font-bold';
      case 'RATE_LIMIT': return 'text-orange-600 font-bold';
      case 'CHALLENGE': return 'text-blue-600 font-bold';
      default: return 'text-green-600';
    }
  }
}
