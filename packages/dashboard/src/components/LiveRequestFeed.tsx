import React from 'react';
import { Shield, Clock, ShieldAlert, ShieldCheck } from 'lucide-react';
import type { GuardStreamEvent } from '@open-guard/sdk';
import clsx from 'clsx';

interface LiveRequestFeedProps {
  events: GuardStreamEvent[];
  maxItems?: number;
}

export function LiveRequestFeed({ events, maxItems = 50 }: LiveRequestFeedProps): JSX.Element {
  const displayEvents = events.slice(0, maxItems);

  const getActionIcon = (type: string) => {
    switch (type) {
      case 'block':
        return ShieldAlert;
      case 'rate_limit':
        return Clock;
      case 'allow':
        return ShieldCheck;
      default:
        return Shield;
    }
  };

  const getActionColor = (type: string) => {
    switch (type) {
      case 'block':
        return 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300';
      case 'rate_limit':
        return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-300';
      case 'allow':
        return 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300';
      case 'challenge':
        return 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300';
      default:
        return 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300';
    }
  };

  const formatTime = (timestamp: number): string => {
    const diff = Date.now() - timestamp;
    if (diff < 60000) return `${Math.floor(diff / 1000)}s ago`;
    if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
    return new Date(timestamp).toLocaleTimeString();
  };

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700">
      <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
          Live Request Feed
        </h3>
      </div>
      <div className="max-h-96 overflow-y-auto">
        {displayEvents.length === 0 ? (
          <div className="p-6 text-center text-gray-500 dark:text-gray-400">
            No events yet. Waiting for incoming requests...
          </div>
        ) : (
          <ul className="divide-y divide-gray-200 dark:divide-gray-700">
            {displayEvents.map((event) => {
              const Icon = getActionIcon(event.type);
              return (
                <li key={event.requestId} className="px-6 py-3 hover:bg-gray-50 dark:hover:bg-gray-700">
                  <div className="flex items-center justify-between">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-xs text-gray-500 dark:text-gray-400">
                          {formatTime(event.timestamp)}
                        </span>
                        <span
                          className={clsx(
                            'inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium',
                            getActionColor(event.type)
                          )}
                        >
                          <Icon className="w-3 h-3" />
                          {event.type.replace('_', ' ')}
                        </span>
                      </div>
                      <div className="mt-1 text-sm text-gray-900 dark:text-white truncate">
                        {event.path}
                      </div>
                      <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                        IP: {event.ip} · {event.detectorId}
                      </div>
                    </div>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}