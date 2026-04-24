import express from 'express';
import cors from 'cors';
import { openGuard, MemoryStore, globalEventEmitter } from '@open-guard/middleware';
import { DetectorKind, GuardAction, GuardResponse, GuardRequest } from '@open-guard/core';
import { WebSocketServer, WebSocket } from 'ws';
import { guardConfig } from './guard.config';
import { apiRouter } from './routes/api';
import { guardAdminRouter } from './routes/guard-admin';
import { setupWebSocket } from './websocket';

const app = express();
const PORT = process.env.PORT || 3001;

app.use(cors());
app.use(express.json());

const store = new MemoryStore({ maxKeys: 10000 });

app.use(openGuard({
  ...guardConfig,
  store,
  logger: {
    info: (msg, meta) => console.log('[INFO]', msg, meta),
    warn: (msg, meta) => console.warn('[WARN]', msg, meta),
    error: (msg, meta) => console.error('[ERROR]', msg, meta),
  },
}));

app.use('/api', apiRouter);
app.use('/api/guard', guardAdminRouter);

app.get('/api/health', (_req, res) => {
  res.json({ status: 'ok', timestamp: Date.now() });
});

app.post('/api/login', (req, res) => {
  const { username, password } = req.body as { username?: string; password?: string };
  if (username === 'admin' && password === 'password') {
    const csrfToken = Math.random().toString(36).substring(2);
    const sessionId = req.headers['x-session-id'] as string || 'sess_' + Math.random().toString(36).substring(2);
    store.set(`og:csrf:${sessionId}`, csrfToken, 3600);
    res.json({ success: true, csrfToken, sessionId });
  } else {
    res.status(401).json({ success: false, error: 'Invalid credentials' });
  }
});

app.get('/api/user', (req, res) => {
  const sessionId = req.headers['x-session-id'] as string;
  if (sessionId) {
    res.json({ id: '1', username: 'admin', email: 'admin@example.com' });
  } else {
    res.status(401).json({ error: 'Unauthorized' });
  }
});

app.get('/api/guard/stats', (_req, res) => {
  res.json({
    totalRequests: 1234,
    blockedRequests: 45,
    rateLimitedRequests: 23,
    challengedRequests: 12,
    topThreats: [],
    topBlockedIps: [],
    timeline: [],
  });
});

const server = app.listen(PORT, () => {
  console.log(`OpenGuard example server running on http://localhost:${PORT}`);
});

const wss = new WebSocketServer({ server, path: '/ws' });
setupWebSocket(wss);

export { app, server };