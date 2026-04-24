import { Router } from 'express';
import { ogClient } from '../openguard-client';

export const apiRouter = Router();

apiRouter.get('/test/sqli', (req, res) => {
  const q = (req.query.q as string) || '';
  res.json({ echo: q });
});

apiRouter.get('/test/xss', (req, res) => {
  const q = (req.query.q as string) || '';
  res.json({ echo: q });
});

apiRouter.get('/test/rate-limit', (_req, res) => {
  res.json({ message: 'Rate limit test endpoint' });
});

apiRouter.get('/test/bot', (_req, res) => {
  res.json({ message: 'Bot detection test endpoint' });
});

apiRouter.get('/test/path', (req, res) => {
  const file = (req.query.file as string) || '';
  res.json({ requested: file });
});

apiRouter.post('/api/comment', (req, res) => {
  const { content } = req.body as { content?: string };
  res.json({ success: true, content });
});

apiRouter.get('/admin/users', async (req, res) => {
  // Demo: Call OpenGuard to see if user is allowed to read users
  // In a real app, subjectId would come from the session (req.cookies.og_session)
  const subjectId = 'user_123'; 
  
  const allowed = await ogClient.allow(subjectId, 'read', 'users');
  
  if (allowed) {
    res.json({
      success: true,
      users: [
        { id: '1', name: 'Alice' },
        { id: '2', name: 'Bob' },
      ]
    });
  } else {
    res.status(403).json({
      success: false,
      error: 'Forbidden: OpenGuard policy denied access to resource "users"',
    });
  }
});