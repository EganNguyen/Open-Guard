import type { OpenGuardClient } from './client';
import type { ClientGuardEvent } from './types';

let originalFetch: typeof fetch | null = null;
let activeClient: OpenGuardClient | null = null;

export function installGuardInterceptor(client: OpenGuardClient): () => void {
  if (activeClient) {
    restoreFetch();
  }

  activeClient = client;
  originalFetch = window.fetch;

  window.fetch = async function fetchWithGuard(
    input: RequestInfo | URL,
    init?: RequestInit
  ): Promise<Response> {
    const request = new Request(input, init);
    const url = request.url;

    try {
      const response = await originalFetch!.call(window, request);

      if (response.status === 403 || response.status === 429) {
        const event: ClientGuardEvent = {
          type: response.status === 403 ? 'fetch_blocked' : 'challenge_shown',
          url,
          statusCode: response.status,
          timestamp: Date.now(),
        };

        client.report(event);

        const challengeToken = response.headers.get('X-Guard-Challenge-Token');
        if (challengeToken && response.status === 429) {
          const challenge = {
            token: challengeToken,
            type: 'captcha' as const,
            expiresAt: Date.now() + 60000,
          };
          await client.resolveChallenge(response);
        }
      }

      return response;
    } catch (error) {
      client.report({
        type: 'manual_report',
        url,
        statusCode: 0,
        timestamp: Date.now(),
      });
      throw error;
    }
  };

  return () => {
    restoreFetch();
  };
}

function restoreFetch(): void {
  if (originalFetch) {
    window.fetch = originalFetch;
    originalFetch = null;
    activeClient = null;
  }
}

export function uninstallGuardInterceptor(): void {
  restoreFetch();
}