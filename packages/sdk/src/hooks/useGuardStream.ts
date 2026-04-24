import { useState, useEffect, useCallback, useRef } from 'react';
import type { OpenGuardClient } from '../client';
import type { GuardStreamEvent } from '../types';

export interface UseGuardStreamOptions {
  onEvent?: (event: GuardStreamEvent) => void;
}

export interface UseGuardStreamResult {
  connected: boolean;
  events: GuardStreamEvent[];
  clearEvents: () => void;
}

export function useGuardStream(
  client: OpenGuardClient,
  options: UseGuardStreamOptions = {}
): UseGuardStreamResult {
  const [connected, setConnected] = useState(false);
  const [events, setEvents] = useState<GuardStreamEvent[]>([]);
  const maxEvents = 50;
  const onEventRef = useRef(options.onEvent);

  useEffect(() => {
    onEventRef.current = options.onEvent;
  }, [options.onEvent]);

  useEffect(() => {
    const unsubscribe = client.subscribe((event) => {
      setConnected(true);
      setEvents(prev => {
        const newEvents = [event, ...prev].slice(0, maxEvents);
        return newEvents;
      });
      onEventRef.current?.(event);
    });

    return () => {
      unsubscribe();
      setConnected(false);
    };
  }, [client]);

  const clearEvents = useCallback(() => {
    setEvents([]);
  }, []);

  return { connected, events, clearEvents };
}