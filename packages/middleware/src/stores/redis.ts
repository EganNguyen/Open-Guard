import Redis from 'ioredis';
import type { StoreAdapter } from '@open-guard/core/types';

export class RedisStore implements StoreAdapter {
  private client: Redis;
  private keyPrefix: string;

  constructor(options: { client: Redis; keyPrefix?: string }) {
    this.client = options.client;
    this.keyPrefix = options.keyPrefix ?? 'og:';
  }

  private prefixedKey(key: string): string {
    return `${this.keyPrefix}${key}`;
  }

  async get(key: string): Promise<string | null> {
    return this.client.get(this.prefixedKey(key));
  }

  async set(key: string, value: string, ttlSeconds?: number): Promise<void> {
    const fullKey = this.prefixedKey(key);
    if (ttlSeconds) {
      await this.client.setex(fullKey, ttlSeconds, value);
    } else {
      await this.client.set(fullKey, value);
    }
  }

  async incr(key: string, ttlSeconds?: number): Promise<number> {
    const fullKey = this.prefixedKey(key);
    const count = await this.client.incr(fullKey);
    if (ttlSeconds && (await this.client.get(fullKey))) {
      await this.client.expire(fullKey, ttlSeconds);
    }
    return count;
  }

  async del(key: string): Promise<void> {
    await this.client.del(this.prefixedKey(key));
  }
}