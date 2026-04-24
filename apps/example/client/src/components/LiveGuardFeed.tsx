import React from 'react';
import { Shield, AlertTriangle, Clock, CheckCircle } from 'lucide-react';
import type { GuardStreamEvent } from '@open-guard/sdk';

interface LiveGuardFeedProps {
  events: GuardStreamEvent[];
}

export function LiveGuardFeed({ events }: LiveGuardFeedProps): JSX.Element {
  const getIcon = (type: string) => {
    switch (type) {
      case 'block': return Shield;
      case 'rate_limit': return Clock;
      case 'allow': return CheckCircle;
      default: return AlertTriangle;
    }
  };

  const getColor = (type: string) => {
    switch (type) {
      case 'block': return 'text-red-500 bg-red-100';
      case 'rate_limit': return 'text-yellow-500 bg-yellow-100';
      case 'allow': return 'text-green-500 bg-green-100';
      default: return 'text-blue-500 bg-blue-100';
    }
  };

  const formatTime = (timestamp: number) => {
    return new Date(timestamp).toLocaleTimeString();
  };

  if (events.length === 0) {
    return (
      <div className="bg-white border border-gray-200 rounded-lg p-6 text-center text-gray-500">
        <Shield className="w-8 h-8 mx-auto mb-2 opacity-50" />
        <p>No events yet. Trigger an attack to see results here.</p>
      </div>
    );
  }

  return (
    <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
      <ul className="divide-y divide-gray-100 max-h-96 overflow-y-auto">
        {events.slice(0, 20).map((event, i) => {
          const Icon = getIcon(event.type);
          return (
            <li key={`${event.requestId}-${i}`} className="p-3 hover:bg-gray-50">
              <div className="flex items-center gap-3">
                <div className={`p-2 rounded-lg ${getColor(event.type)}`}>
                  <Icon className="w-4 h-4" />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-gray-500">{formatTime(event.timestamp)}</span>
                    <span className="text-xs font-medium px-1.5 py-0.5 rounded bg-gray-100 text-gray-700">
                      {event.type.replace('_', ' ')}
                    </span>
                  </div>
                  <p className="text-sm text-gray-900 truncate">{event.path}</p>
                  <p className="text-xs text-gray-500">
                    {event.ip} · {event.detectorId}
                  </p>
                </div>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}