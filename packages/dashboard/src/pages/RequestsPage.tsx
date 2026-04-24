import React, { useState } from 'react';
import { RequestTable } from '../components/RequestTable';
import { RequestDetailDrawer } from '../components/RequestDetailDrawer';

interface Request {
  id: string;
  timestamp: number;
  method: string;
  path: string;
  ip: string;
  action: string;
  detector: string;
  threatLevel: string;
  duration: number;
  headers?: Record<string, string>;
}

export function RequestsPage(): JSX.Element {
  const [selectedRequest, setSelectedRequest] = useState<Request | null>(null);
  const [requests] = useState<Request[]>([
    {
      id: '1',
      timestamp: Date.now() - 60000,
      method: 'GET',
      path: '/api/users',
      ip: '192.168.1.1',
      action: 'ALLOW',
      detector: '-',
      threatLevel: 'NONE',
      duration: 12,
    },
    {
      id: '2',
      timestamp: Date.now() - 120000,
      method: 'POST',
      path: '/api/login',
      ip: '10.0.0.1',
      action: 'BLOCK',
      detector: 'auth-brute-force',
      threatLevel: 'CRITICAL',
      duration: 45,
    },
  ]);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Request History</h1>
        <p className="text-gray-500 dark:text-gray-400">
          Browse and filter all guard-evaluated requests
        </p>
      </div>

      <RequestTable
        requests={requests}
      />

      {selectedRequest && (
        <RequestDetailDrawer
          request={selectedRequest}
          detectorResults={[]}
          onClose={() => setSelectedRequest(null)}
        />
      )}
    </div>
  );
}