import React, { useState } from 'react';
import { Pencil, X } from 'lucide-react';
import type { DetectorConfig } from '@open-guard/core';
import { useGuardConfigStore } from '../stores/useGuardConfigStore';

export function DetectorList(): JSX.Element {
  const { detectors, setDetectorEnabled } = useGuardConfigStore();

  if (detectors.length === 0) {
    return (
      <div className="text-center py-12 text-gray-500 dark:text-gray-400">
        No detectors configured. Load configuration from server.
      </div>
    );
  }

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700">
      <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
          Detectors
        </h3>
      </div>
      <ul className="divide-y divide-gray-200 dark:divide-gray-700">
        {detectors.map((detector) => (
          <li key={detector.id} className="px-6 py-4 flex items-center justify-between">
            <div className="flex items-center gap-4">
              <label className="relative inline-flex items-center cursor-pointer">
                <input
                  type="checkbox"
                  className="sr-only peer"
                  checked={detector.enabled}
                  onChange={(e) => setDetectorEnabled(detector.id, e.target.checked)}
                />
                <div className="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-blue-300 dark:peer-focus:ring-blue-800 rounded-full peer dark:bg-gray-700 peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all dark:border-gray-600 peer-checked:bg-blue-600"></div>
              </label>
              <div>
                <div className="text-sm font-medium text-gray-900 dark:text-white">
                  {detector.id}
                </div>
                <div className="text-xs text-gray-500 dark:text-gray-400">
                  Priority: {detector.priority} · {detector.kind}
                </div>
              </div>
            </div>
            <button className="p-2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
              <Pencil className="w-4 h-4" />
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}