import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter, Routes, Route, Link } from 'react-router-dom';
import { HomePage } from './pages/HomePage';
import { LoginPage } from './pages/LoginPage';
import { AttackSimulator } from './components/AttackSimulator';
import { LiveGuardFeed } from './components/LiveGuardFeed';
import { OpenGuardClient, GuardProvider, installGuardInterceptor } from '@open-guard/sdk';
import './index.css';

const client = new OpenGuardClient({
  baseUrl: '/api/guard',
  websocketUrl: `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`,
  autoReport: true,
});

installGuardInterceptor(client);

const root = document.getElementById('root');
if (root) {
  ReactDOM.createRoot(root).render(
    <React.StrictMode>
      <GuardProvider client={client}>
        <BrowserRouter>
          <div className="min-h-screen bg-gray-50">
            <nav className="bg-gray-900 text-white">
              <div className="max-w-7xl mx-auto px-4 py-4 flex items-center justify-between">
                <div className="flex items-center gap-8">
                  <Link to="/" className="text-xl font-bold">OpenGuard Demo</Link>
                  <div className="flex gap-4 text-sm">
                    <Link to="/" className="hover:text-gray-300">Home</Link>
                    <Link to="/login" className="hover:text-gray-300">Login</Link>
                    <Link to="/guard" className="hover:text-gray-300">Dashboard</Link>
                  </div>
                </div>
              </div>
            </nav>
            <main className="max-w-7xl mx-auto px-4 py-8">
              <Routes>
                <Route path="/" element={<HomePage />} />
                <Route path="/login" element={<LoginPage />} />
              </Routes>
            </main>
          </div>
        </BrowserRouter>
      </GuardProvider>
    </React.StrictMode>
  );
}

export { client };