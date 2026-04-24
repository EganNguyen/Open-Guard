import { StoreAdapter } from '@open-guard/core';

export interface UpstashStoreOptions {
  url?: string;
  token?: string;
  keyPrefix?: string;
}

export class UpstashStore implements StoreAdapter {
  private url: string;
  private token: string;
  private keyPrefix: string;

  constructor(options: UpstashStoreOptions = {}) {
    this.url = options.url || process.env.UPSTASH_REDIS_REST_URL || '';
    this.token = options.token || process.env.UPSTASH_REDIS_REST_TOKEN || '';
    this.keyPrefix = options.keyPrefix || 'og:';
  }

  private async request(command: string[]): Promise<unknown> {
    const response = await fetch(`${this.url}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${this.token}`,
      },
      body: JSON.stringify(command),
    });

    if (!response.ok) {
      throw new Error(`Upstash request failed: ${response.statusText}`);
    }

    return response.json();
  }

  private prefixKey(key: string): string {
    return `${this.keyPrefix}${key}`;
  }

  async get(key: string): Promise<string | null> {
    try {
      const result = await this.request(['GET', this.prefixKey(key)]) as { result: string | null };
      return result.result;
    } catch {
      return null;
    }
  }

  async set(key: string, value: string, ttlSeconds?: number): Promise<void> {
    const prefixedKey = this.prefixKey(key);
    if (ttlSeconds) {
      await this.request(['SET', prefixedKey, value, 'EX', String(ttlSeconds)]);
    } else {
      await this.request(['SET', prefixedKey, value]);
    }
  }

  async incr(key: string, ttlSeconds?: number): Promise<number> {
    const prefixedKey = this.prefixKey(key);
    const result = await this.request(['INCR', prefixedKey]) as { result: number };
    if (ttlSeconds) {
      await this.request(['EXPIRE', prefixedKey, String(ttlSeconds)]);
    }
    return result.result;
  }

  async del(key: string): Promise<void> {
    await this.request(['DEL', this.prefixKey(key)]);
  }
}

export async function createUpstashStore(options: UpstashStoreOptions = {}): Promise<UpstashStore> {
  const url = options.url || process.env.UPSTASH_REDIS_REST_URL;
  const token = options.token || process.env.UPSTASH_REDIS_REST_TOKEN;

  if (!url || !token) {
    throw new Error('UPSTASH_REDIS_REST_URL and UPSTASH_REDIS_REST_TOKEN are required');
  }

  return new UpstashStore({ ...options, url, token });
}