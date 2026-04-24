import React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { GuardProvider } from '@open-guard/sdk';
import { AppRouter } from './router';
import type { OpenGuardClient } from '@open-guard/sdk';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});

interface OpenGuardDashboardProps {
  client: OpenGuardClient;
}

export function OpenGuardDashboard({ client }: OpenGuardDashboardProps): JSX.Element {
  return (
    <QueryClientProvider client={queryClient}>
      <GuardProvider client={client}>
        <AppRouter client={client} />
      </GuardProvider>
    </QueryClientProvider>
  );
}

export default OpenGuardDashboard;