import React from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { OverviewPage } from './pages/OverviewPage';
import { RequestsPage } from './pages/RequestsPage';
import { ThreatsPage } from './pages/ThreatsPage';
import { IpManagementPage } from './pages/IpManagementPage';
import { ConfigPage } from './pages/ConfigPage';
import { AlertsPage } from './pages/AlertsPage';
import { Sidebar } from './components/Sidebar';
import type { OpenGuardClient } from '@open-guard/sdk';

interface AppRouterProps {
  client: OpenGuardClient;
}

export function AppRouter({ client }: AppRouterProps): JSX.Element {
  return (
    <BrowserRouter>
      <div className="flex h-screen bg-gray-50 dark:bg-gray-900">
        <Sidebar />
        <main className="flex-1 overflow-y-auto p-6">
          <Routes>
            <Route path="/" element={<OverviewPage client={client} />} />
            <Route path="/requests" element={<RequestsPage />} />
            <Route path="/threats" element={<ThreatsPage />} />
            <Route path="/ips" element={<IpManagementPage />} />
            <Route path="/config" element={<ConfigPage />} />
            <Route path="/alerts" element={<AlertsPage />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  );
}