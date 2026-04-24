import { Router } from 'express';

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