import type {
  ClientGuardEvent,
  GuardStreamEvent,
  GuardStats,
  StatsFilter,
  ChallengePayload,
  OpenGuardClientOptions,
  Unsubscribe,
} from './types';

export class OpenGuardClient {
  private baseUrl: string;
  private apiKey?: string;
  private websocketUrl?: string;
  private autoReport: boolean;
  private onChallenge?: (challenge: ChallengePayload) => Promise<string>;
  private ws?: WebSocket;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 1000;
  private subscriptions: ((event: GuardStreamEvent) => void)[] = [];
  private pingInterval?: ReturnType<typeof setInterval>;
  private isUnmounted = false;

  constructor(options: OpenGuardClientOptions) {
    this.baseUrl = options.baseUrl;
    this.apiKey = options.apiKey;
    this.websocketUrl = options.websocketUrl;
    this.autoReport = options.autoReport !== false;
    this.onChallenge = options.onChallenge;
  }

  async report(event: ClientGuardEvent): Promise<void> {
    try {
      const response = await fetch(`${this.baseUrl}/report`, {
        method: 'POST',
        headers: this.getHeaders(),
        body: JSON.stringify(event),
      });
      if (!response.ok) {
        console.warn('Failed to report guard event:', response.statusText);
      }
    } catch (error) {
      console.warn('Failed to report guard event:', error);
    }
  }

  subscribe(handler: (event: GuardStreamEvent) => void): Unsubscribe {
    this.subscriptions.push(handler);

    if (!this.ws && this.websocketUrl) {
      this.connectWebSocket();
    }

    return () => {
      this.subscriptions = this.subscriptions.filter(h => h !== handler);
      if (this.subscriptions.length === 0 && this.ws) {
        this.disconnectWebSocket();
      }
    };
  }

  private connectWebSocket(): void {
    if (!this.websocketUrl || this.isUnmounted) return;

    try {
      this.ws = new WebSocket(this.websocketUrl);

      if (this.apiKey) {
        this.ws.auth = this.apiKey;
      }

      this.ws.onopen = () => {
        this.reconnectAttempts = 0;
        this.startPing();
      };

      this.ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as GuardStreamEvent;
          this.subscriptions.forEach(handler => {
            try {
              handler(data);
            } catch {
              console.warn('Handler threw an error');
            }
          });
        } catch {
          console.warn('Failed to parse WebSocket message');
        }
      };

      this.ws.onclose = () => {
        this.stopPing();
        if (!this.isUnmounted) {
          this.scheduleReconnect();
        }
      };

      this.ws.onerror = () => {
        console.warn('WebSocket connection error');
      };
    } catch {
      console.warn('Failed to create WebSocket connection');
    }
  }

  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      return;
    }

    const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts);
    this.reconnectAttempts++;

    setTimeout(() => {
      if (!this.isUnmounted && this.subscriptions.length > 0) {
        this.connectWebSocket();
      }
    }, delay);
  }

  private disconnectWebSocket(): void {
    if (this.ws) {
      this.ws.close();
      this.ws = undefined;
    }
    this.stopPing();
  }

  private startPing(): void {
    this.pingInterval = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: 'ping' }));
      }
    }, 30000);
  }

  private stopPing(): void {
    if (this.pingInterval) {
      clearInterval(this.pingInterval);
      this.pingInterval = undefined;
    }
  }

  async resolveChallenge(response: Response): Promise<boolean> {
    try {
      const body = await response.json() as { challengeToken?: string; error?: string };
      const challengeToken = body.challengeToken;

      if (!challengeToken || !this.onChallenge) {
        return false;
      }

      const challenge: ChallengePayload = {
        token: challengeToken,
        type: 'captcha',
        expiresAt: Date.now() + 60000,
      };

      const resolvedToken = await this.onChallenge(challenge);

      const verifyResponse = await fetch(`${this.baseUrl}/verify-challenge`, {
        method: 'POST',
        headers: this.getHeaders(),
        body: JSON.stringify({ challengeToken, resolvedToken }),
      });

      return verifyResponse.ok;
    } catch {
      return false;
    }
  }

  async getStats(filters?: StatsFilter): Promise<GuardStats> {
    try {
      const params = new URLSearchParams();
      if (filters?.from) params.set('from', String(filters.from));
      if (filters?.to) params.set('to', String(filters.to));
      if (filters?.detectorIds?.length) params.set('detectorIds', filters.detectorIds.join(','));
      if (filters?.actions?.length) params.set('actions', filters.actions.join(','));

      const response = await fetch(`${this.baseUrl}/stats?${params}`, {
        headers: this.getHeaders(),
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch stats: ${response.statusText}`);
      }

      return response.json();
    } catch (error) {
      console.error('Failed to fetch guard stats:', error);
      return {
        totalRequests: 0,
        blockedRequests: 0,
        rateLimitedRequests: 0,
        challengedRequests: 0,
        topThreats: [],
        topBlockedIps: [],
        timeline: [],
      };
    }
  }

  async blockIp(ip: string, reason?: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/ips/block`, {
      method: 'POST',
      headers: this.getHeaders(),
      body: JSON.stringify({ ip, reason }),
    });

    if (!response.ok) {
      throw new Error(`Failed to block IP: ${response.statusText}`);
    }
  }

  async allowlistIp(ip: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/ips/allowlist`, {
      method: 'POST',
      headers: this.getHeaders(),
      body: JSON.stringify({ ip }),
    });

    if (!response.ok) {
      throw new Error(`Failed to allowlist IP: ${response.statusText}`);
    }
  }

  private getHeaders(): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (this.apiKey) {
      headers['Authorization'] = `Bearer ${this.apiKey}`;
    }
    return headers;
  }

  destroy(): void {
    this.isUnmounted = true;
    this.disconnectWebSocket();
    this.subscriptions = [];
  }
}

export default OpenGuardClient;