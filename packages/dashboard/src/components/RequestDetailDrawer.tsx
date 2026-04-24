import React from 'react';

interface RequestDetailDrawerProps {
  request: {
    id: string;
    ip: string;
    method: string;
    path: string;
    headers: Record<string, string>;
    timestamp: number;
  };
  detectorResults: {
    detectorId: string;
    kind: string;
    action: string;
    score: number;
    reason: string;
  }[];
  onClose: () => void;
}

export function RequestDetailDrawer({ request, detectorResults, onClose }: RequestDetailDrawerProps): JSX.Element {
  const [showRawJson, setShowRawJson] = React.useState(false);

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />
      <div className="relative w-full max-w-2xl bg-white dark:bg-gray-800 shadow-xl overflow-y-auto">
        <div className="sticky top-0 bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex justify-between items-center">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
            Request Details
          </h2>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
          >
            ✕
          </button>
        </div>

        <div className="p-6 space-y-6">
          <section>
            <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">
              Request Metadata
            </h3>
            <dl className="space-y-2">
              <div className="flex">
                <dt className="w-32 text-sm text-gray-500">ID:</dt>
                <dd className="text-sm text-gray-900 dark:text-white font-mono">{request.id}</dd>
              </div>
              <div className="flex">
                <dt className="w-32 text-sm text-gray-500">IP:</dt>
                <dd className="text-sm text-gray-900 dark:text-white">{request.ip}</dd>
              </div>
              <div className="flex">
                <dt className="w-32 text-sm text-gray-500">Method:</dt>
                <dd className="text-sm text-gray-900 dark:text-white">{request.method}</dd>
              </div>
              <div className="flex">
                <dt className="w-32 text-sm text-gray-500">Path:</dt>
                <dd className="text-sm text-gray-900 dark:text-white font-mono">{request.path}</dd>
              </div>
              <div className="flex">
                <dt className="w-32 text-sm text-gray-500">Timestamp:</dt>
                <dd className="text-sm text-gray-900 dark:text-white">
                  {new Date(request.timestamp).toLocaleString()}
                </dd>
              </div>
            </dl>
          </section>

          <section>
            <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">
              Headers
            </h3>
            <pre className="bg-gray-50 dark:bg-gray-900 rounded p-3 text-xs overflow-x-auto">
              {JSON.stringify(request.headers, null, 2)}
            </pre>
          </section>

          <section>
            <div className="flex justify-between items-center mb-2">
              <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400">
                Detector Results
              </h3>
              <button
                onClick={() => setShowRawJson(!showRawJson)}
                className="text-xs text-blue-500 hover:text-blue-600"
              >
                {showRawJson ? 'Hide Raw JSON' : 'Show Raw JSON'}
              </button>
            </div>
            {showRawJson ? (
              <pre className="bg-gray-50 dark:bg-gray-900 rounded p-3 text-xs overflow-x-auto">
                {JSON.stringify({ request, detectorResults }, null, 2)}
              </pre>
            ) : (
              <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                <thead className="bg-gray-50 dark:bg-gray-900">
                  <tr>
                    <th className="px-4 py-2 text-left text-xs font-medium text-gray-500">Detector</th>
                    <th className="px-4 py-2 text-left text-xs font-medium text-gray-500">Action</th>
                    <th className="px-4 py-2 text-left text-xs font-medium text-gray-500">Score</th>
                    <th className="px-4 py-2 text-left text-xs font-medium text-gray-500">Reason</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                  {detectorResults.map((result) => (
                    <tr key={result.detectorId}>
                      <td className="px-4 py-2 text-sm text-gray-900 dark:text-white">{result.detectorId}</td>
                      <td className="px-4 py-2 text-sm">
                        <span className={`px-2 py-1 text-xs rounded ${
                          result.action === 'BLOCK' ? 'bg-red-100 text-red-700' : 'bg-green-100 text-green-700'
                        }`}>
                          {result.action}
                        </span>
                      </td>
                      <td className="px-4 py-2 text-sm text-gray-500">{result.score.toFixed(2)}</td>
                      <td className="px-4 py-2 text-sm text-gray-500 max-w-xs truncate">{result.reason}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}