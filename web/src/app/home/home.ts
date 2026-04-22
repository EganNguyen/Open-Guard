import { Component, inject, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { OverviewService } from '../core/services/overview.service';

@Component({
  selector: 'app-home',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './home.html',
  styleUrls: ['./home.css']
})
export class HomeComponent implements OnInit {
  private overviewService = inject(OverviewService);
  
  stats = signal<any[]>([]);
  recentActivities = signal<any[]>([]);
  systemHealth = signal<any[]>([]);
  loading = signal(true);

  ngOnInit() {
    this.overviewService.getDashboardData().subscribe({
      next: (data) => {
        this.stats.set(data.stats);
        this.recentActivities.set(data.activities);
        this.systemHealth.set(data.health);
        this.loading.set(false);
      },
      error: (err) => {
        console.error('Failed to fetch dashboard data', err);
        this.loading.set(false);
      }
    });
  }
}
