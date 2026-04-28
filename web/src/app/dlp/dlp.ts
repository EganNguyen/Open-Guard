import { Component, OnInit, signal, inject } from '@angular/core';
import { CommonModule } from '@angular/forms';
import { DlpService, DlpFinding, DlpPolicy } from '../core/services/dlp.service';
import { LucideAngularModule } from 'lucide-angular';

@Component({
  selector: 'app-dlp',
  standalone: true,
  imports: [CommonModule, LucideAngularModule],
  templateUrl: './dlp.html',
  styleUrls: ['./dlp.css'],
})
export class DlpComponent implements OnInit {
  private dlpService = inject(DlpService);

  findings = signal<DlpFinding[]>([]);
  policies = signal<DlpPolicy[]>([]);
  loading = signal(true);
  error = signal('');

  ngOnInit() {
    this.loadData();
  }

  loadData() {
    this.loading.set(true);
    this.dlpService.getPolicies().subscribe({
      next: (policies) => {
        this.policies.set(policies);
        this.loadFindings();
      },
      error: (err) => {
        this.error.set('Failed to load DLP policies');
        this.loading.set(false);
      },
    });
  }

  loadFindings() {
    this.dlpService.getFindings({ limit: 50 }).subscribe({
      next: (res) => {
        this.findings.set(res.items);
        this.loading.set(false);
      },
      error: (err) => {
        this.error.set('Failed to load DLP findings');
        this.loading.set(false);
      },
    });
  }

  togglePolicyMode(policy: DlpPolicy) {
    const newMode = policy.mode === 'monitor' ? 'block' : 'monitor';

    if (newMode === 'block') {
      if (
        !confirm('Warning: Enabling BLOCK mode will reject all matching traffic. Are you sure?')
      ) {
        return;
      }
    }

    this.dlpService.updatePolicy(policy.id, newMode).subscribe({
      next: (updated) => {
        this.policies.update((pols) => pols.map((p) => (p.id === updated.id ? updated : p)));
      },
      error: (err) => {
        alert('Failed to update policy mode');
      },
    });
  }
}
