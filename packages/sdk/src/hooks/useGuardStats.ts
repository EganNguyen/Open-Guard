import { useState, useEffect, useCallback } from 'react';
import type { OpenGuardClient } from '../client';
import type { GuardStats, StatsFilter } from '../types';

export interface UseGuardStatsOptions {
  filters?: StatsFilter;
  refreshIntervalMs?: number;
}

export interface UseGuardStatsResult {
  stats: GuardStats | null;
  loading: boolean;
  error: Error | null;
  refresh: () => Promise<void>;
}

export function useGuardStats(
  client: OpenGuardClient,
  options: UseGuardStatsOptions = {}
): UseGuardStatsResult {
  const { filters = {}, refreshIntervalMs = 30000 } = options;
  const [stats, setStats] = useState<GuardStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchStats = useCallback(async () => {
    try {
      const data = await client.getStats(filters);
      setStats(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('Failed to fetch stats'));
    }
  }, [client, filters]);

  useEffect(() => {
    let mounted = true;
    let intervalId: ReturnType<typeof setInterval>;

    const init = async () => {
      setLoading(true);
      await fetchStats();
      if (mounted) {
        setLoading(false);
      }
    };

    init();

    if (refreshIntervalMs > 0) {
      intervalId = setInterval(() => {
        fetchStats();
      }, refreshIntervalMs);
    }

    return () => {
      mounted = false;
      if (intervalId) {
        clearInterval(intervalId);
      }
    };
  }, [fetchStats, refreshIntervalMs]);

  return { stats, loading, error, refresh: fetchStats };
}