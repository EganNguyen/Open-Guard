import { Component, OnInit, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute } from '@angular/router';

@Component({
  selector: 'app-connector-detail',
  standalone: true,
  imports: [CommonModule],
  template: `
    <div class="p-6 max-w-6xl mx-auto">
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold text-gray-900">Connector Details</h1>
        <span class="px-3 py-1 bg-green-100 text-green-800 rounded-full text-sm font-medium">Active</span>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div class="lg:col-span-2 space-y-6">
          <div class="bg-white p-6 rounded-lg shadow border">
            <h2 class="text-lg font-semibold mb-4 text-gray-800">Configuration</h2>
            <div class="space-y-3">
              <div>
                <label class="block text-xs font-medium text-gray-500 uppercase">Connector ID</label>
                <div class="mt-1 text-sm font-mono bg-gray-50 p-2 rounded">{{ connectorId() }}</div>
              </div>
            </div>
          </div>

          <div class="bg-white p-6 rounded-lg shadow border">
            <h2 class="text-lg font-semibold mb-4 text-gray-800">Recent Deliveries</h2>
            <div class="text-sm text-gray-500 italic">No recent webhook deliveries found.</div>
          </div>
        </div>

        <div class="space-y-6">
          <div class="bg-white p-6 rounded-lg shadow border">
            <h2 class="text-lg font-semibold mb-4 text-gray-800">Health Metrics</h2>
            <div class="space-y-4">
              <div class="flex justify-between items-center">
                <span class="text-sm text-gray-600">Success Rate</span>
                <span class="text-sm font-bold text-green-600">100%</span>
              </div>
              <div class="flex justify-between items-center">
                <span class="text-sm text-gray-600">Avg Latency</span>
                <span class="text-sm font-bold text-gray-900">45ms</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  `,
})
export class ConnectorDetailComponent implements OnInit {
  private route = inject(ActivatedRoute);
  connectorId = signal<string | null>(null);

  ngOnInit() {
    this.connectorId.set(this.route.snapshot.paramMap.get('id'));
  }
}
