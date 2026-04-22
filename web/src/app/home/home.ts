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
export class HomeComponent {
  private overviewService = inject(OverviewService);
  
  dashboardData$ = this.overviewService.getDashboardData();


}
