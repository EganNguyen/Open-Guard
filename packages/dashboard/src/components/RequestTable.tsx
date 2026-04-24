import React, { useState } from 'react';
import { Download, ChevronLeft, ChevronRight } from 'lucide-react';

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
}

interface RequestTableProps {
  requests: Request[];
}

export function RequestTable({ requests }: RequestTableProps): JSX.Element {
  const [currentPage, setCurrentPage] = useState(1);
  const [sortField, setSortField] = useState<keyof Request>('timestamp');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');
  const perPage = 25;

  const handleSort = (field: keyof Request) => {
    if (sortField === field) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortField(field);
      setSortOrder('desc');
    }
  };

  const sortedRequests = [...requests].sort((a, b) => {
    const aVal = a[sortField];
    const bVal = b[sortField];
    const order = sortOrder === 'asc' ? 1 : -1;
    return aVal < bVal ? order : -order;
  });

  const totalPages = Math.ceil(sortedRequests.length / perPage);
  const paginatedRequests = sortedRequests.slice(
    (currentPage - 1) * perPage,
    currentPage * perPage
  );

  const exportCSV = () => {
    const headers = ['Timestamp', 'Method', 'Path', 'IP', 'Action', 'Detector', 'Threat Level', 'Duration'];
    const rows = sortedRequests.map((r) => [
      new Date(r.timestamp).toISOString(),
      r.method,
      r.path,
      r.ip,
      r.action,
      r.detector,
      r.threatLevel,
      `${r.duration}ms`,
    ]);
    const csv = [headers.join(','), ...rows.map((r) => r.join(','))].join('\n');
    const blob = new Blob([csv], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `guard-requests-${Date.now()}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const columns: { key: keyof Request; label: string }[] = [
    { key: 'timestamp', label: 'Time' },
    { key: 'method', label: 'Method' },
    { key: 'path', label: 'Path' },
    { key: 'ip', label: 'IP' },
    { key: 'action', label: 'Action' },
    { key: 'detector', label: 'Detector' },
    { key: 'threatLevel', label: 'Threat Level' },
    { key: 'duration', label: 'Duration' },
  ];

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700">
      <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
          Request History
        </h3>
        <button
          onClick={exportCSV}
          className="inline-flex items-center gap-2 px-3 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-600"
        >
          <Download className="w-4 h-4" />
          Export CSV
        </button>
      </div>
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
          <thead className="bg-gray-50 dark:bg-gray-900">
            <tr>
              {columns.map((col) => (
                <th
                  key={col.key}
                  onClick={() => handleSort(col.key)}
                  className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-800"
                >
                  {col.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
            {paginatedRequests.map((req) => (
              <tr key={req.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                  {new Date(req.timestamp).toLocaleString()}
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm">
                  <span className="px-2 py-1 text-xs font-medium rounded bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300">
                    {req.method}
                  </span>
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white truncate max-w-xs">
                  {req.path}
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                  {req.ip}
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm">
                  <span
                    className={`px-2 py-1 text-xs font-medium rounded ${
                      req.action === 'BLOCK'
                        ? 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300'
                        : req.action === 'RATE_LIMIT'
                        ? 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-300'
                        : 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'
                    }`}
                  >
                    {req.action}
                  </span>
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                  {req.detector}
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm">
                  <span
                    className={`px-2 py-1 text-xs font-medium rounded ${
                      req.threatLevel === 'CRITICAL'
                        ? 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300'
                        : req.threatLevel === 'HIGH'
                        ? 'bg-orange-100 text-orange-700 dark:bg-orange-900 dark:text-orange-300'
                        : req.threatLevel === 'MEDIUM'
                        ? 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-300'
                        : 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300'
                    }`}
                  >
                    {req.threatLevel}
                  </span>
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                  {req.duration}ms
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="px-6 py-4 border-t border-gray-200 dark:border-gray-700 flex items-center justify-between">
        <div className="text-sm text-gray-500 dark:text-gray-400">
          Showing {(currentPage - 1) * perPage + 1} to{' '}
          {Math.min(currentPage * perPage, sortedRequests.length)} of{' '}
          {sortedRequests.length} results
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
            disabled={currentPage === 1}
            className="p-2 rounded-lg border border-gray-300 dark:border-gray-600 disabled:opacity-50"
          >
            <ChevronLeft className="w-4 h-4" />
          </button>
          <span className="text-sm text-gray-700 dark:text-gray-300">
            Page {currentPage} of {totalPages}
          </span>
          <button
            onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
            disabled={currentPage === totalPages}
            className="p-2 rounded-lg border border-gray-300 dark:border-gray-600 disabled:opacity-50"
          >
            <ChevronRight className="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>
  );
}