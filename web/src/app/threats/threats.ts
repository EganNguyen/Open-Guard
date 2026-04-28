import {
  Component,
  OnInit,
  inject,
  signal,
  viewChild,
  ElementRef,
  AfterViewInit,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { ThreatService } from '../core/services/threat.service';
import { finalize } from 'rxjs';
import { Chart, ChartConfiguration, registerables } from 'chart.js';

import { ThreatAlert } from '../core/models/threat.model';

Chart.register(...registerables);

@Component({
  selector: 'app-threats',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './threats.html',
  styleUrls: ['./threats.css'],
})
export class ThreatsComponent implements OnInit, AfterViewInit {
  private threatService = inject(ThreatService);

  severityCanvas = viewChild<ElementRef<HTMLCanvasElement>>('severityChart');
  trendCanvas = viewChild<ElementRef<HTMLCanvasElement>>('trendChart');

  severityChart?: Chart;
  trendChart?: Chart;

  alerts = signal<ThreatAlert[]>([]);
  loading = signal(false);
  error = signal<string | null>(null);

  stats = signal({
    total: 0,
    critical: 0,
    open: 0,
    resolved: 0,
  });

  ngOnInit(): void {
    this.fetchThreats();
  }

  ngAfterViewInit(): void {
    this.initCharts();
  }

  fetchThreats(): void {
    this.loading.set(true);
    this.threatService
      .listAlerts()
      .pipe(finalize(() => this.loading.set(false)))
      .subscribe({
        next: (alerts) => {
          const data = alerts || [];
          this.alerts.set(data);
          this.calculateStats(data);
          this.updateCharts(data);
        },
        error: (err) => {
          console.error('Failed to fetch threats', err);
          this.error.set('Failed to load threat data. Please try again later.');
        },
      });
  }

  private initCharts(): void {
    const severityCtx = this.severityCanvas()?.nativeElement.getContext('2d');
    const trendCtx = this.trendCanvas()?.nativeElement.getContext('2d');

    if (severityCtx) {
      this.severityChart = new Chart(severityCtx, {
        type: 'doughnut',
        data: {
          labels: ['Critical', 'High', 'Medium', 'Low'],
          datasets: [
            {
              data: [0, 0, 0, 0],
              backgroundColor: ['#ef4444', '#f97316', '#eab308', '#3b82f6'],
              borderWidth: 0,
            },
          ],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: {
            legend: { position: 'bottom' },
          },
        },
      });
    }

    if (trendCtx) {
      this.trendChart = new Chart(trendCtx, {
        type: 'line',
        data: {
          labels: [],
          datasets: [
            {
              label: 'Alerts',
              data: [],
              borderColor: '#6366f1',
              backgroundColor: 'rgba(99, 102, 241, 0.1)',
              fill: true,
              tension: 0.4,
            },
          ],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: {
            legend: { display: false },
          },
          scales: {
            y: { beginAtZero: true, ticks: { stepSize: 1 } },
            x: { grid: { display: false } },
          },
        },
      });
    }
  }

  private updateCharts(alerts: ThreatAlert[]): void {
    if (this.severityChart) {
      const counts = { CRITICAL: 0, HIGH: 0, MEDIUM: 0, LOW: 0 };
      alerts.forEach((a) => counts[a.severity]++);
      this.severityChart.data.datasets[0].data = [
        counts.CRITICAL,
        counts.HIGH,
        counts.MEDIUM,
        counts.LOW,
      ];
      this.severityChart.update();
    }

    if (this.trendChart) {
      // Group by date for the last 7 days
      const days = [...Array(7)]
        .map((_, i) => {
          const d = new Date();
          d.setDate(d.getDate() - i);
          return d.toISOString().split('T')[0];
        })
        .reverse();

      const dayCounts = days.map(
        (day) => alerts.filter((a) => a.created_at.startsWith(day)).length,
      );

      this.trendChart.data.labels = days.map((d) => d.split('-').slice(1).join('/'));
      this.trendChart.data.datasets[0].data = dayCounts;
      this.trendChart.update();
    }
  }

  private calculateStats(alerts: ThreatAlert[]): void {
    const stats = {
      total: alerts.length,
      critical: alerts.filter((a) => a.severity === 'CRITICAL' || a.severity === 'HIGH').length,
      open: alerts.filter((a) => a.status === 'OPEN').length,
      resolved: alerts.filter((a) => a.status === 'RESOLVED').length,
    };
    this.stats.set(stats);
  }

  getThreatLevelClass(level: string): string {
    switch (level) {
      case 'CRITICAL':
        return 'bg-red-100 text-red-800 border-red-200';
      case 'HIGH':
        return 'bg-orange-100 text-orange-800 border-orange-200';
      case 'MEDIUM':
        return 'bg-yellow-100 text-yellow-800 border-yellow-200';
      default:
        return 'bg-blue-100 text-blue-800 border-blue-200';
    }
  }

  getActionClass(action: string): string {
    switch (action) {
      case 'BLOCK':
      case 'DENY':
        return 'text-red-600 font-bold';
      case 'RATE_LIMIT':
        return 'text-orange-600 font-bold';
      case 'CHALLENGE':
        return 'text-blue-600 font-bold';
      default:
        return 'text-green-600';
    }
  }
}
