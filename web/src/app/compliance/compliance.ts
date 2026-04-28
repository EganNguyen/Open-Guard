import { Component, OnInit, signal, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ComplianceService, ComplianceReport } from '../core/services/compliance.service';
import { LucideAngularModule } from 'lucide-angular';

@Component({
  selector: 'app-compliance',
  standalone: true,
  imports: [CommonModule, FormsModule, LucideAngularModule],
  templateUrl: './compliance.html',
  styleUrls: ['./compliance.css'],
})
export class ComplianceComponent implements OnInit {
  private complianceService = inject(ComplianceService);

  reports = signal<ComplianceReport[]>([]);
  loading = signal(true);
  generating = signal(false);

  // Wizard state
  wizardStep = signal(1);
  selectedFramework = signal<'GDPR' | 'SOC2' | 'HIPAA'>('GDPR');
  dateRange = {
    start: '',
    end: '',
  };

  ngOnInit() {
    this.loadReports();
  }

  loadReports() {
    this.loading.set(true);
    this.complianceService.listReports().subscribe({
      next: (reports) => {
        this.reports.set(reports);
        this.loading.set(false);
      },
      error: (err) => {
        console.error('Failed to load reports', err);
        this.loading.set(false);
      },
    });
  }

  startWizard() {
    this.wizardStep.set(1);
    this.dateRange = {
      start: new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString().split('T')[0],
      end: new Date().toISOString().split('T')[0],
    };
  }

  generateReport() {
    this.generating.set(true);
    this.complianceService
      .generateReport({
        framework: this.selectedFramework(),
        period_start: this.dateRange.start,
        period_end: this.dateRange.end,
      })
      .subscribe({
        next: (report) => {
          this.reports.update((rs) => [report, ...rs]);
          this.pollReportStatus(report.id);
          this.wizardStep.set(1); // Reset wizard
          this.generating.set(false);
        },
        error: (err) => {
          alert('Failed to trigger report generation');
          this.generating.set(false);
        },
      });
  }

  pollReportStatus(id: string) {
    this.complianceService.pollReport(id).subscribe({
      next: (updated) => {
        this.reports.update((rs) => rs.map((r) => (r.id === updated.id ? updated : r)));
      },
    });
  }

  getFrameworkIcon(framework: string) {
    switch (framework) {
      case 'GDPR':
        return 'shield_check';
      case 'SOC2':
        return 'verified';
      case 'HIPAA':
        return 'medical_services';
      default:
        return 'description';
    }
  }
}
