import React from 'react';
import { Activity, ShieldX, Clock, AlertTriangle } from 'lucide-react';
import type { GuardStats } from '@open-guard/sdk';
import clsx from 'clsx';

interface KpiCardsProps {
  stats: GuardStats | null;
  loading: boolean;
}

export function KpiCards({ stats, loading }: KpiCardsProps): JSX.Element {
  if (loading || !stats) {
    return (
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {[...Array(4)].map((_, i) => (
          <div key={i} className="bg-white dark:bg-gray-800 rounded-lg p-6 animate-pulse">
            <div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-1/2 mb-2" />
            <div className="h-8 bg-gray-200 dark:bg-gray-700 rounded w-3/4" />
          </div>
        ))}
      </div>
    );
  }

  const cards = [
    {
      label: 'Total Requests',
      value: stats.totalRequests.toLocaleString(),
      icon: Activity,
      color: 'text-blue-500',
    },
    {
      label: 'Blocked',
      value: stats.blockedRequests.toLocaleString(),
      icon: ShieldX,
      color: 'text-red-500',
    },
    {
      label: 'Rate Limited',
      value: stats.rateLimitedRequests.toLocaleString(),
      icon: Clock,
      color: 'text-yellow-500',
    },
    {
      label: 'Challenged',
      value: stats.challengedRequests.toLocaleString(),
      icon: AlertTriangle,
      color: 'text-blue-500',
    },
  ];

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
      {cards.map((card) => (
        <div
          key={card.label}
          className="bg-white dark:bg-gray-800 rounded-lg p-6 shadow-sm border border-gray-200 dark:border-gray-700"
        >
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-gray-500 dark:text-gray-400">
              {card.label}
            </span>
            <card.icon className={clsx('w-5 h-5', card.color)} />
          </div>
          <div className="mt-2 text-3xl font-bold text-gray-900 dark:text-white">
            {card.value}
          </div>
        </div>
      ))}
    </div>
  );
}