import React, { useState } from 'react';
import { X, Plus } from 'lucide-react';

interface AddIpModalProps {
  onClose: () => void;
  onAdd: (ip: string, action: 'block' | 'allowlist', reason?: string) => void;
}

export function AddIpModal({ onClose, onAdd }: AddIpModalProps): JSX.Element {
  const [ip, setIp] = useState('');
  const [action, setAction] = useState<'block' | 'allowlist'>('block');
  const [reason, setReason] = useState('');
  const [error, setError] = useState('');

  const validateIp = (ip: string): boolean => {
    const ipv4Pattern = /^(\d{1,3}\.){3}\d{1,3}$/;
    const ipv6Pattern = /^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$/;
    return ipv4Pattern.test(ip) || ipv6Pattern.test(ip);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!validateIp(ip)) {
      setError('Please enter a valid IP address');
      return;
    }
    onAdd(ip, action, reason || undefined);
    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />
      <div className="relative w-full max-w-md bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6">
        <div className="flex justify-between items-center mb-4">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
            Add IP to Block/Allowlist
          </h2>
          <button onClick={onClose} className="text-gray-500 hover:text-gray-700">
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              IP Address
            </label>
            <input
              type="text"
              value={ip}
              onChange={(e) => {
                setIp(e.target.value);
                setError('');
              }}
              placeholder="192.168.1.1"
              className={`w-full px-3 py-2 border rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white ${
                error ? 'border-red-500' : 'border-gray-300 dark:border-gray-600'
              }`}
            />
            {error && <p className="mt-1 text-sm text-red-500">{error}</p>}
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Action
            </label>
            <div className="flex gap-4">
              <label className="flex items-center gap-2">
                <input
                  type="radio"
                  name="action"
                  value="block"
                  checked={action === 'block'}
                  onChange={() => setAction('block')}
                  className="text-red-600"
                />
                <span className="text-sm text-gray-700 dark:text-gray-300">Block</span>
              </label>
              <label className="flex items-center gap-2">
                <input
                  type="radio"
                  name="action"
                  value="allowlist"
                  checked={action === 'allowlist'}
                  onChange={() => setAction('allowlist')}
                  className="text-green-600"
                />
                <span className="text-sm text-gray-700 dark:text-gray-300">Allowlist</span>
              </label>
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Reason (optional)
            </label>
            <textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Reason for blocking/allowing this IP"
              rows={3}
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
            />
          </div>

          <div className="flex justify-end gap-3 pt-4">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-700 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 flex items-center gap-2"
            >
              <Plus className="w-4 h-4" />
              Add IP
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}