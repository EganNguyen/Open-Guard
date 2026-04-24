import React, { useState } from 'react';
import { Search, Plus } from 'lucide-react';
import { IpStatusTable } from '../components/IpStatusTable';
import { AddIpModal } from '../components/AddIpModal';
import { useIpStore } from '../stores/useIpStore';

export function IpManagementPage(): JSX.Element {
  const [showAddModal, setShowAddModal] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const { blockIp, allowlistIp, fetchIps } = useIpStore();

  React.useEffect(() => {
    fetchIps();
  }, [fetchIps]);

  const handleAdd = async (ip: string, action: 'block' | 'allowlist', reason?: string) => {
    if (action === 'block') {
      await blockIp(ip, reason);
    } else {
      await allowlistIp(ip);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">IP Management</h1>
          <p className="text-gray-500 dark:text-gray-400">
            Manage blocked and allowlisted IP addresses
          </p>
        </div>
        <button
          onClick={() => setShowAddModal(true)}
          className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
        >
          <Plus className="w-4 h-4" />
          Add IP
        </button>
      </div>

      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
        <input
          type="text"
          placeholder="Search IPs..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="w-full pl-10 pr-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white"
        />
      </div>

      <IpStatusTable />

      {showAddModal && (
        <AddIpModal
          onClose={() => setShowAddModal(false)}
          onAdd={handleAdd}
        />
      )}
    </div>
  );
}