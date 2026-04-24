import React, { useState } from 'react';
import { Plus } from 'lucide-react';
import { CreateAlertModal } from '../components/CreateAlertModal';

interface AlertRule {
  id: string;
  name: string;
  metric: string;
  threshold: number;
  windowMinutes: number;
  channel: string;
  enabled: boolean;
}

export function AlertsPage(): JSX.Element {
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [alerts] = useState<AlertRule[]>([
    {
      id: '1',
      name: 'High Block Rate',
      metric: 'blocked_rate',
      threshold: 10,
      windowMinutes: 5,
      channel: 'email',
      enabled: true,
    },
  ]);

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Alert Rules</h1>
          <p className="text-gray-500 dark:text-gray-400">
            Configure alert thresholds and notification channels
          </p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
        >
          <Plus className="w-4 h-4" />
          Create Alert
        </button>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
          <thead className="bg-gray-50 dark:bg-gray-900">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Metric</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Threshold</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Window</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Channel</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
            {alerts.map((alert) => (
              <tr key={alert.id}>
                <td className="px-6 py-4 text-sm text-gray-900 dark:text-white">{alert.name}</td>
                <td className="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">{alert.metric}</td>
                <td className="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">{alert.threshold}%</td>
                <td className="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">{alert.windowMinutes}m</td>
                <td className="px-6 py-4 text-sm text-gray-500 dark:text-gray-400 capitalize">{alert.channel}</td>
                <td className="px-6 py-4">
                  <span className={`px-2 py-1 text-xs rounded ${alert.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-700'}`}>
                    {alert.enabled ? 'Active' : 'Disabled'}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {showCreateModal && (
        <CreateAlertModal onClose={() => setShowCreateModal(false)} />
      )}
    </div>
  );
}