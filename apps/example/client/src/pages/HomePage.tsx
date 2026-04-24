import React from 'react';
import { AttackSimulator } from '../components/AttackSimulator';
import { LiveGuardFeed } from '../components/LiveGuardFeed';
import { useGuardStream } from '@open-guard/sdk';
import { client } from '../main';

export function HomePage(): JSX.Element {
  const { events } = useGuardStream(client);

  return (
    <div className="space-y-8">
      <div className="text-center">
        <h1 className="text-4xl font-bold text-gray-900 mb-4">
          OpenGuard Security Demo
        </h1>
        <p className="text-lg text-gray-600 max-w-2xl mx-auto">
          Test the security guards by triggering different attack types.
          Watch how OpenGuard detects and blocks malicious requests in real-time.
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-4">Attack Simulator</h2>
          <AttackSimulator />
        </div>
        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-4">Live Guard Feed</h2>
          <LiveGuardFeed events={events} />
        </div>
      </div>

      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
        <h2 className="text-xl font-semibold text-gray-900 mb-4">Quick Test Commands</h2>
        <div className="space-y-2 font-mono text-sm bg-gray-100 p-4 rounded-lg">
          <p><span className="text-gray-500"># SQL Injection:</span> curl "http://localhost:3001/api/test/sqli?q=1'+UNION+SELECT+*+FROM+users--"</p>
          <p><span className="text-gray-500"># XSS:</span> curl "http://localhost:3001/api/test/xss?q=%3Cscript%3Ealert(1)%3C/script%3E"</p>
          <p><span className="text-gray-500"># Rate Limit:</span> for i in {1..10}; do curl http://localhost:3001/api/test/rate-limit; done</p>
          <p><span className="text-gray-500"># Bot Detection:</span> curl -H "User-Agent: python-requests/2.28" http://localhost:3001/api/test/bot</p>
          <p><span className="text-gray-500"># Path Traversal:</span> curl "http://localhost:3001/api/test/path?file=../../etc/passwd"</p>
        </div>
      </div>
    </div>
  );
}