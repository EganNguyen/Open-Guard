import React from 'react';
import ReactDOM from 'react-dom/client';
import { OpenGuardDashboard } from './App';
import { OpenGuardClient } from '@open-guard/sdk';
import './index.css';

declare global {
  interface Window {
    openGuardClient: OpenGuardClient;
  }
}

const client = window.openGuardClient || new OpenGuardClient({
  baseUrl: '/api/guard',
  websocketUrl: `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`,
});

const root = document.getElementById('root');
if (root) {
  ReactDOM.createRoot(root).render(
    <React.StrictMode>
      <OpenGuardDashboard client={client} />
    </React.StrictMode>
  );
}

export { OpenGuardDashboard };