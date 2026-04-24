import React, { useState } from 'react';
import { X, Check } from 'lucide-react';

interface DetectorConfigEditorProps {
  detectorId: string;
  options: Record<string, unknown>;
  onSave: (options: Record<string, unknown>) => void;
  onClose: () => void;
}

export function DetectorConfigEditor({ detectorId, options, onSave, onClose }: DetectorConfigEditorProps): JSX.Element {
  const [localOptions, setLocalOptions] = useState(options);

  const handleChange = (key: string, value: unknown) => {
    setLocalOptions((prev) => ({ ...prev, [key]: value }));
  };

  const handleSave = () => {
    onSave(localOptions);
    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />
      <div className="relative w-full max-w-lg bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6">
        <div className="flex justify-between items-center mb-4">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
            Edit {detectorId} Options
          </h2>
          <button onClick={onClose} className="text-gray-500 hover:text-gray-700">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="space-y-4">
          {Object.entries(localOptions).map(([key, value]) => (
            <div key={key}>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                {key}
              </label>
              {typeof value === 'boolean' ? (
                <select
                  value={String(value)}
                  onChange={(e) => handleChange(key, e.target.value === 'true')}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                >
                  <option value="true">true</option>
                  <option value="false">false</option>
                </select>
              ) : typeof value === 'number' ? (
                <input
                  type="number"
                  value={value}
                  onChange={(e) => handleChange(key, parseFloat(e.target.value))}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                />
              ) : Array.isArray(value) ? (
                <input
                  type="text"
                  value={JSON.stringify(value)}
                  onChange={(e) => {
                    try {
                      handleChange(key, JSON.parse(e.target.value));
                    } catch {}
                  }}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white font-mono text-sm"
                />
              ) : (
                <input
                  type="text"
                  value={String(value)}
                  onChange={(e) => handleChange(key, e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                />
              )}
            </div>
          ))}
        </div>

        <div className="flex justify-end gap-3 mt-6">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-700 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 flex items-center gap-2"
          >
            <Check className="w-4 h-4" />
            Save
          </button>
        </div>
      </div>
    </div>
  );
}