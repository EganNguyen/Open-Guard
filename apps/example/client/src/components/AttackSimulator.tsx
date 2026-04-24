import React, { useState } from 'react';
import { AlertTriangle, Shield, Loader } from 'lucide-react';

interface AttackResult {
  name: string;
  success: boolean;
  status: number;
  message: string;
}

const attacks = [
  {
    name: 'SQL Injection',
    description: 'UNION SELECT attack',
    method: 'GET',
    path: '/api/test/sqli',
    query: { q: "1' UNION SELECT * FROM users--" },
    expectedDetector: 'sql-injection',
  },
  {
    name: 'XSS Payload',
    description: 'Script injection',
    method: 'GET',
    path: '/api/test/xss',
    query: { q: '<script>alert(1)</script>' },
    expectedDetector: 'xss',
  },
  {
    name: 'Rate Limit',
    description: 'Exceed request limit',
    method: 'GET',
    path: '/api/test/rate-limit',
    query: {},
    expectedDetector: 'rate-limiter',
    repeat: 3,
  },
  {
    name: 'Bot UA',
    description: 'Suspicious user agent',
    method: 'GET',
    path: '/api/test/bot',
    query: {},
    expectedDetector: 'bot-detection',
    headers: { 'User-Agent': 'python-requests/2.28.0' },
  },
  {
    name: 'Path Traversal',
    description: 'Directory traversal',
    method: 'GET',
    path: '/api/test/path',
    query: { file: '../../etc/passwd' },
    expectedDetector: 'path-traversal',
  },
  {
    name: 'Large Payload',
    description: 'Oversized request body',
    method: 'POST',
    path: '/api/comment',
    query: {},
    body: { content: 'A'.repeat(2000000) },
    expectedDetector: 'payload-size',
  },
  {
    name: 'Brute Force',
    description: 'Multiple failed logins',
    method: 'POST',
    path: '/api/login',
    query: {},
    body: { username: 'admin', password: 'wrong' },
    expectedDetector: 'auth-brute-force',
    repeat: 6,
  },
];

export function AttackSimulator(): JSX.Element {
  const [results, setResults] = useState<AttackResult[]>([]);
  const [loading, setLoading] = useState<string | null>(null);

  const triggerAttack = async (attack: typeof attacks[0]) => {
    setLoading(attack.name);
    const attackResults: AttackResult[] = [];

    for (let i = 0; i < (attack.repeat || 1); i++) {
      try {
        const params = new URLSearchParams();
        Object.entries(attack.query).forEach(([k, v]) => params.set(k, v));
        const url = `${attack.path}?${params}`;

        const response = await fetch(url, {
          method: attack.method,
          headers: {
            'Content-Type': 'application/json',
            ...attack.headers,
          },
          body: attack.body ? JSON.stringify(attack.body) : undefined,
        });

        attackResults.push({
          name: attack.name,
          success: response.status >= 400,
          status: response.status,
          message: response.status === 403 ? 'Blocked' : response.status === 429 ? 'Rate Limited' : 'Allowed',
        });
      } catch (error) {
        attackResults.push({
          name: attack.name,
          success: true,
          status: 0,
          message: String(error),
        });
      }
    }

    setResults((prev) => [...attackResults, ...prev].slice(0, 20));
    setLoading(null);
  };

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
        {attacks.map((attack) => (
          <button
            key={attack.name}
            onClick={() => triggerAttack(attack)}
            disabled={loading !== null}
            className="p-3 bg-white border border-gray-200 rounded-lg hover:border-red-300 hover:bg-red-50 transition-colors text-left disabled:opacity-50"
          >
            <div className="flex items-center gap-2">
              <AlertTriangle className="w-4 h-4 text-red-500" />
              <span className="text-sm font-medium text-gray-900">{attack.name}</span>
              {loading === attack.name && <Loader className="w-4 h-4 animate-spin" />}
            </div>
            <p className="text-xs text-gray-500 mt-1">{attack.description}</p>
          </button>
        ))}
      </div>

      {results.length > 0 && (
        <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
          <div className="px-4 py-2 bg-gray-50 border-b border-gray-200">
            <h3 className="text-sm font-medium text-gray-700">Results</h3>
          </div>
          <ul className="divide-y divide-gray-100">
            {results.map((result, i) => (
              <li key={i} className="px-4 py-2 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Shield className={`w-4 h-4 ${result.success ? 'text-red-500' : 'text-green-500'}`} />
                  <span className="text-sm">{result.name}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className={`text-xs px-2 py-1 rounded ${
                    result.success ? 'bg-red-100 text-red-700' : 'bg-green-100 text-green-700'
                  }`}>
                    {result.status} - {result.message}
                  </span>
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}