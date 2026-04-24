import React from 'react';
import { KpiCards } from '../components/KpiCards';
import { ThreatTimeline } from '../components/ThreatTimeline';
import { LiveRequestFeed } from '../components/LiveRequestFeed';
import { useGuardStats, useGuardStream } from '@open-guard/sdk';
import { useLiveFeedStore } from '../stores/useLiveFeedStore';
import type { OpenGuardClient } from '@open-guard/sdk';

interface OverviewPageProps {
  client: OpenGuardClient;
}

export function OverviewPage({ client }: OverviewPageProps): JSX.Element {
  const { stats, loading } = useGuardStats(client, { refreshIntervalMs: 30000 });
  const { events } = useGuardStream(client);
  const { push } = useLiveFeedStore();

  React.useEffect(() => {
    if (events.length > 0) {
      push(events[0]);
    }
  }, [events, push]);

  const [range, setRange] = React.useState<'1h' | '6h' | '24h' | '7d'>('6h');

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Dashboard</h1>
        <p className="text-gray-500 dark:text-gray-400">Real-time security monitoring overview</p>
      </div>

      <KpiCards stats={stats} loading={loading} />

      <div className="flex gap-2">
        {(['1h', '6h', '24h', '7d'] as const).map((r) => (
          <button
            key={r}
            onClick={() => setRange(r)}
            className={`px-3 py-1.5 text-sm rounded-lg ${
              range === r
                ? 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300'
                : 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300'
            }`}
          >
            {r}
          </button>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <ThreatTimeline timeline={stats?.timeline || []} range={range} />
        <LiveRequestFeed events={events} />
      </div>
    </div>
  );
}