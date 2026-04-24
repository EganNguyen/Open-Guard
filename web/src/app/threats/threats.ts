import { Component, OnInit, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { HttpClient } from '@angular/common/http';
import { finalize } from 'rxjs';

interface ThreatEvent {
  id: string;
  timestamp: string;
  type: string;
  source_ip: string;
  path: string;
  action: string;
  detector_id: string;
  threat_level: 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
  metadata: any;
}

@Component({
  selector: 'app-threats',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './threats.html',
  styleUrls: ['./threats.css']
})
export class ThreatsComponent implements OnInit {
  private http = inject(HttpClient);
  
  threats = signal<ThreatEvent[]>([]);
  loading = signal(false);
  error = signal<string | null>(null);

  stats = signal({
    total: 0,
    critical: 0,
    blocked: 0,
    rateLimited: 0
  });

  ngOnInit(): void {
    this.fetchThreats();
  }

  fetchThreats(): void {
    this.loading.set(true);
    this.http.get<{events: ThreatEvent[]}>('http://localhost:8080/v1/events')
      .pipe(finalize(() => this.loading.set(false)))
      .subscribe({
        next: (res) => {
          this.threats.set(res.events || []);
          this.calculateStats(res.events || []);
        },
        error: (err) => {
          console.error('Failed to fetch threats', err);
          this.error.set('Failed to load threat data. Please try again later.');
        }
      });
  }

  private calculateStats(events: ThreatEvent[]): void {
    const stats = {
      total: events.length,
      critical: events.filter(e => e.threat_level === 'CRITICAL' || e.threat_level === 'HIGH').length,
      blocked: events.filter(e => e.action === 'BLOCK' || e.action === 'DENY').length,
      rateLimited: events.filter(e => e.action === 'RATE_LIMIT').length
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
