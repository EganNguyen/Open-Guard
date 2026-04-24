import React from 'react';
import { PieChart, Pie, Cell, ResponsiveContainer, Legend, Tooltip } from 'recharts';

const COLORS = ['#10b981', '#f59e0b', '#ef4444', '#3b82f6', '#8b5cf6'];

export function ThreatsPage(): JSX.Element {
  const [topThreats] = useState([
    { name: 'Rate Limit', value: 45 },
    { name: 'SQL Injection', value: 20 },
    { name: 'XSS', value: 15 },
    { name: 'Bot Detection', value: 12 },
    { name: 'Path Traversal', value: 8 },
  ]);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Threat Analysis</h1>
        <p className="text-gray-500 dark:text-gray-400">
          Aggregated threat statistics by detector type
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="bg-white dark:bg-gray-800 rounded-lg p-6 shadow-sm border border-gray-200 dark:border-gray-700">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
            Threat Distribution
          </h3>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={topThreats}
                  cx="50%"
                  cy="50%"
                  innerRadius={60}
                  outerRadius={80}
                  paddingAngle={5}
                  dataKey="value"
                  label={({ name, percent }) => `${name} ${(percent * 100).toFixed(0)}%`}
                >
                  {topThreats.map((_, index) => (
                    <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
                  ))}
                </Pie>
                <Tooltip />
                <Legend />
              </PieChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div className="bg-white dark:bg-gray-800 rounded-lg p-6 shadow-sm border border-gray-200 dark:border-gray-700">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
            Top Threats
          </h3>
          <table className="min-w-full">
            <thead>
              <tr className="text-left text-xs font-medium text-gray-500 uppercase">
                <th className="pb-2">Detector</th>
                <th className="pb-2">Count</th>
                <th className="pb-2">%</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {topThreats.map((threat, i) => (
                <tr key={threat.name}>
                  <td className="py-2 text-sm text-gray-900 dark:text-white">{threat.name}</td>
                  <td className="py-2 text-sm text-gray-500 dark:text-gray-400">{threat.value}</td>
                  <td className="py-2 text-sm text-gray-500 dark:text-gray-400">
                    <div className="w-24 bg-gray-200 rounded-full h-2">
                      <div
                        className="h-2 rounded-full"
                        style={{ width: `${(threat.value / topThreats.reduce((a, b) => a + b.value, 0)) * 100}%`, backgroundColor: COLORS[i] }}
                      />
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}