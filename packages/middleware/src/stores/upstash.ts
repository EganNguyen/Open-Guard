import type { StoreAdapter } from '@open-guard/core/types';

export class UpstashStore implements StoreAdapter {
  private url: string;
  private token: string;

  constructor(options: { url: string; token: string }) {
    this.url = options.url;
    this.token = options.token;
  }

  private async request(command: string[]): Promise<unknown> {
    const response = await fetch(this.url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${this.token}`,
      },
      body: JSON.stringify(command),
    });

    if (!response.ok) {
      throw new Error(`Upstash error: ${response.status}`);
    }

    const result = await response.json();
    return result;
  }

  async get(key: string): Promise<string | null> {
    const result = await this.request(['get', key]);
    return (result as { result: string | null })?.result ?? null;
  }

  async set(key: string, value: string, ttlSeconds?: number): Promise<void> {
    if (ttlSeconds) {
      await this.request(['set', key, value, 'ex', ttlSeconds]);
    } else {
      await this.request(['set', key, value]);
    }
  }

  async incr(key: string, ttlSeconds?: number): Promise<number> {
    const result = await this.request(['incr', key]);
    const value = (result as { result: string })?.result;
    const count = parseInt(value || '0', 10);
    
    if (ttlSeconds) {
      await this.request(['expire', key, ttlSeconds]);
    }
    
    return count;
  }

  async del(key: string): Promise<void> {
    await this.request(['del', key]);
  }
}