import React, { useEffect } from 'react';
import { Save } from 'lucide-react';
import { DetectorList } from '../components/DetectorList';
import { useGuardConfigStore } from '../stores/useGuardConfigStore';

export function ConfigPage(): JSX.Element {
  const { dirty, save, fetchConfig } = useGuardConfigStore();

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Configuration</h1>
          <p className="text-gray-500 dark:text-gray-400">
            Manage detector settings and guard mode
          </p>
        </div>
        {dirty && (
          <button
            onClick={save}
            className="inline-flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700"
          >
            <Save className="w-4 h-4" />
            Save Changes
          </button>
        )}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
        <div className="lg:col-span-3">
          <DetectorList />
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-lg p-6 shadow-sm border border-gray-200 dark:border-gray-700">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
            Guard Mode
          </h3>
          <div className="space-y-2">
            {(['enforce', 'monitor', 'dry_run'] as const).map((mode) => (
              <label key={mode} className="flex items-center gap-2">
                <input
                  type="radio"
                  name="mode"
                  value={mode}
                  className="text-blue-600"
                />
                <span className="text-sm text-gray-700 dark:text-gray-300 capitalize">{mode.replace('_', ' ')}</span>
              </label>
            ))}
          </div>
          <p className="mt-4 text-xs text-gray-500">
            <strong>Enforce:</strong> Block requests per detector rules<br />
            <strong>Monitor:</strong> Log only, don't block<br />
            <strong>Dry Run:</strong> No action, passive logging
          </p>
        </div>
      </div>
    </div>
  );
}