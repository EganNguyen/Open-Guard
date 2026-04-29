import { Component } from '@angular/core';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-org-settings',
  standalone: true,
  imports: [CommonModule],
  template: `
    <div class="p-6 max-w-4xl mx-auto">
      <h1 class="text-2xl font-bold mb-6">Organization Settings</h1>
      
      <div class="grid grid-cols-1 gap-6">
        <div class="bg-white p-6 rounded-lg shadow border">
          <h2 class="text-xl font-semibold mb-4">MFA Configuration</h2>
          <p class="text-gray-600 mb-4">Manage multi-factor authentication requirements for your organization.</p>
          <button class="bg-indigo-600 text-white px-4 py-2 rounded hover:bg-indigo-700">
            Configure TOTP
          </button>
        </div>

        <div class="bg-white p-6 rounded-lg shadow border">
          <h2 class="text-xl font-semibold mb-4">SIEM Integration</h2>
          <p class="text-gray-600 mb-4">Configure webhooks to stream audit logs to your SIEM provider.</p>
          <button class="border border-indigo-600 text-indigo-600 px-4 py-2 rounded hover:bg-indigo-50">
            Add Webhook
          </button>
        </div>
      </div>
    </div>
  `,
})
export class OrgSettingsComponent {}
