import { WebSocketServer, WebSocket } from 'ws';
import { globalEventEmitter, OpenGuardEventEmitter } from '@open-guard/middleware';

const clients = new Set<WebSocket>();

export function setupWebSocket(wss: WebSocketServer): void {
  wss.on('connection', (ws) => {
    clients.add(ws);

    console.log('WebSocket client connected. Total:', clients.size);

    const onResult = (response: unknown) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({
          type: 'guard:result',
          data: response,
          timestamp: Date.now(),
        }));
      }
    };

    const onBlock = (event: { request: unknown; response: unknown }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({
          type: 'guard:block',
          data: event,
          timestamp: Date.now(),
        }));
      }
    };

    globalEventEmitter.onGuardResult(onResult);
    globalEventEmitter.onGuardBlock(onBlock);

    ws.on('message', (message) => {
      try {
        const data = JSON.parse(message.toString());
        if (data.type === 'ping') {
          ws.send(JSON.stringify({ type: 'pong', timestamp: Date.now() }));
        }
      } catch {
        console.warn('Invalid WebSocket message');
      }
    });

    ws.on('close', () => {
      clients.delete(ws);
      globalEventEmitter.off('guard:result', onResult);
      globalEventEmitter.off('guard:block', onBlock);
      console.log('WebSocket client disconnected. Total:', clients.size);
    });

    ws.on('error', (error) => {
      console.error('WebSocket error:', error);
    });
  });

  console.log('WebSocket server setup complete');
}

export function broadcastToClients(message: unknown): void {
  const payload = JSON.stringify(message);
  clients.forEach((client) => {
    if (client.readyState === WebSocket.OPEN) {
      client.send(payload);
    }
  });
}