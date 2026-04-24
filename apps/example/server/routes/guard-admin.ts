import { Router } from 'express';

export const guardAdminRouter = Router();

let guardConfig = {
  mode: 'enforce',
  detectors: [] as Array<{
    id: string;
    enabled: boolean;
    options?: Record<string, unknown>;
  }>,
};

guardAdminRouter.get('/config', (_req, res) => {
  res.json(guardConfig);
});

guardAdminRouter.patch('/config/detectors/:id', (req, res) => {
  const { id } = req.params;
  const { enabled } = req.body as { enabled?: boolean };
  
  if (enabled !== undefined) {
    const detector = guardConfig.detectors.find((d) => d.id === id);
    if (detector) {
      detector.enabled = enabled;
    }
  }
  
  res.json({ success: true });
});

guardAdminRouter.patch('/config/detectors/:id/options', (req, res) => {
  const { id } = req.params;
  const options = req.body as Record<string, unknown>;
  
  const detector = guardConfig.detectors.find((d) => d.id === id);
  if (detector) {
    detector.options = { ...detector.options, ...options };
  }
  
  res.json({ success: true });
});

guardAdminRouter.get('/requests', (_req, res) => {
  res.json({ requests: [], total: 0, page: 1, perPage: 25 });
});

guardAdminRouter.get('/requests/:id', (req, res) => {
  res.json({
    id: req.params.id,
    timestamp: Date.now(),
    ip: '127.0.0.1',
    path: '/',
    method: 'GET',
    results: [],
  });
});

const ipStore = new Map<string, { status: 'blocked' | 'allowlisted'; reason?: string; addedAt: number }>();

guardAdminRouter.get('/ips', (_req, res) => {
  const blocked = Array.from(ipStore.entries())
    .filter(([, v]) => v.status === 'blocked')
    .map(([ip, v]) => ({ ip, ...v }));
  const allowlisted = Array.from(ipStore.entries())
    .filter(([, v]) => v.status === 'allowlisted')
    .map(([ip, v]) => ({ ip, ...v }));
  res.json({ blocked, allowlisted });
});

guardAdminRouter.post('/ips/block', (req, res) => {
  const { ip, reason } = req.body as { ip: string; reason?: string };
  ipStore.set(ip, { status: 'blocked', reason, addedAt: Date.now() });
  res.json({ success: true });
});

guardAdminRouter.post('/ips/allowlist', (req, res) => {
  const { ip } = req.body as { ip: string };
  ipStore.set(ip, { status: 'allowlisted', addedAt: Date.now() });
  res.json({ success: true });
});

guardAdminRouter.delete('/ips/:ip', (req, res) => {
  ipStore.delete(req.params.ip);
  res.json({ success: true });
});

guardAdminRouter.get('/alerts', (_req, res) => {
  res.json({ alerts: [] });
});

guardAdminRouter.post('/alerts', (req, res) => {
  res.json({ success: true, id: Math.random().toString(36).substring(2) });
});

guardAdminRouter.delete('/alerts/:id', (_req, res) => {
  res.json({ success: true });
});