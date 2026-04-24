import express from 'express';
import cors from 'cors';
import cookieParser from 'cookie-parser';
import axios from 'axios';
import { openGuard, MemoryStore, globalEventEmitter } from '@open-guard/middleware';
import { GuardAction, GuardRequest, GuardResponse } from '@open-guard/core';
import { WebSocketServer } from 'ws';
import { Issuer, Strategy, Client } from 'openid-client';
import dotenv from 'dotenv';
import { guardConfig } from './guard.config';
import { apiRouter } from './routes/api';
import { guardAdminRouter } from './routes/guard-admin';
import { setupWebSocket } from './websocket';
import { ogClient } from './openguard-client';

dotenv.config();

const app = express();
const PORT = process.env.PORT || 3001;

app.use(cors({
  origin: 'http://localhost:3000',
  credentials: true,
}));
app.use(express.json());
app.use(cookieParser(process.env.SESSION_SECRET || 'og-secret'));

const store = new MemoryStore({ maxKeys: 10000 });

// OIDC Client Setup
let oidcClient: Client;

async function setupOIDC() {
  try {
    const issuer = await Issuer.discover(process.env.OPENGUARD_ISSUER_URL || 'http://localhost:8081');
    oidcClient = new issuer.Client({
      client_id: process.env.OPENGUARD_CLIENT_ID || 'example-app',
      client_secret: process.env.OPENGUARD_CLIENT_SECRET || 'example-app-secret',
      redirect_uris: [`http://localhost:${PORT}/api/callback`],
      response_types: ['code'],
    });
    console.log('[OIDC] Client configured for issuer:', issuer.issuer);
  } catch (error) {
    console.warn('[OIDC] Failed to discover OIDC issuer. Login flow may be unavailable.', error instanceof Error ? error.message : error);
  }
}

setupOIDC();

// Hook into middleware events to ingest to OpenGuard
globalEventEmitter.onGuardResult((response: GuardResponse) => {
  // Ingest all results for audit trail
  ogClient.ingestEvent({
    type: 'request',
    actor_id: 'anonymous', // Would be actual user ID if authenticated
    action: response.action,
    status: response.action === GuardAction.BLOCK ? 'blocked' : 'allowed',
    payload: response,
  });
});

globalEventEmitter.onGuardBlock((event: { request: GuardRequest; response: GuardResponse }) => {
  // Specifically ingest blocks as high-severity threat events
  ogClient.ingestEvent({
    type: 'threat',
    actor_id: 'anonymous',
    action: 'BLOCK',
    status: 'detected',
    payload: {
      request: event.request,
      response: event.response,
    },
  });
});

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

// OIDC Login Routes
app.get('/api/login', (req, res) => {
  if (!oidcClient) return res.status(503).json({ error: 'OIDC client not initialized' });
  
  const url = oidcClient.authorizationUrl({
    scope: 'openid profile email',
    state: Math.random().toString(36).substring(7),
  });
  res.json({ url });
});

app.get('/api/callback', async (req, res) => {
  if (!oidcClient) return res.status(503).send('OIDC client not initialized');
  
  const params = oidcClient.callbackParams(req);
  try {
    const tokenSet = await oidcClient.callback(`http://localhost:${PORT}/api/callback`, params);
    const userinfo = await oidcClient.userinfo(tokenSet.access_token!);
    
    // Set secure cookie
    res.cookie('og_session', tokenSet.access_token, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      maxAge: (tokenSet.expires_in || 3600) * 1000,
    });

    res.redirect('http://localhost:3000/dashboard');
  } catch (err) {
    console.error('Callback error:', err);
    res.redirect('http://localhost:3000/login?error=auth_failed');
  }
});

app.get('/api/user', async (req, res) => {
  const token = req.cookies.og_session;
  if (!token) return res.status(401).json({ error: 'Unauthorized' });

  try {
    // In a real app, you'd verify the JWT or call userinfo
    const userinfo = await oidcClient.userinfo(token);
    res.json(userinfo);
  } catch (error) {
    res.status(401).json({ error: 'Invalid session' });
  }
});

app.get('/api/guard/stats', async (_req, res) => {
  try {
    // Attempt to fetch real stats from OpenGuard if possible, fallback to local mock
    const resp = await axios.get(`${process.env.OPENGUARD_URL}/v1/audit/stats`, {
      headers: { 'Authorization': `Bearer ${process.env.OPENGUARD_API_KEY}` }
    }).catch(() => null);

    if (resp) {
      res.json(resp.data);
    } else {
      res.json({
        totalRequests: 1234,
        blockedRequests: 45,
        rateLimitedRequests: 23,
        challengedRequests: 12,
        topThreats: [],
        topBlockedIps: [],
        timeline: [],
      });
    }
  } catch (error) {
    res.status(500).json({ error: 'Failed to fetch stats' });
  }
});

const server = app.listen(PORT, () => {
  console.log(`OpenGuard example server running on http://localhost:${PORT}`);
});

const wss = new WebSocketServer({ server, path: '/ws' });
setupWebSocket(wss);

export { app, server };