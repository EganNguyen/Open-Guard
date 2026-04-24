import { StoreAdapter } from '@open-guard/core';

interface RedisClient {
  get(key: string): Promise<string | null>;
  set(key: string, value: string, mode?: string, duration?: number): Promise<string>;
  incr(key: string): Promise<number>;
  expire(key: string, seconds: number): Promise<number>;
  del(key: string): Promise<number>;
}

export interface RedisStoreOptions {
  keyPrefix?: string;
}

export class RedisStore implements StoreAdapter {
  private client: RedisClient;
  private keyPrefix: string;

  constructor(client: RedisClient, options: RedisStoreOptions = {}) {
    this.client = client;
    this.keyPrefix = options.keyPrefix || 'og:';
  }

  private prefixKey(key: string): string {
    return `${this.keyPrefix}${key}`;
  }

  async get(key: string): Promise<string | null> {
    return this.client.get(this.prefixKey(key));
  }

  async set(key: string, value: string, ttlSeconds?: number): Promise<void> {
    if (ttlSeconds) {
      await this.client.set(this.prefixKey(key), value, 'EX', ttlSeconds);
    } else {
      await this.client.set(this.prefixKey(key), value);
    }
  }

  async incr(key: string, ttlSeconds?: number): Promise<number> {
    const prefixedKey = this.prefixKey(key);
    const newValue = await this.client.incr(prefixedKey);
    if (ttlSeconds) {
      await this.client.expire(prefixedKey, ttlSeconds);
    }
    return newValue;
  }

  async del(key: string): Promise<void> {
    await this.client.del(this.prefixKey(key));
  }
}

let redisClientInstance: RedisClient | null = null;

export async function createRedisStore(
  url: string,
  options: RedisStoreOptions = {}
): Promise<RedisStore> {
  try {
    const Redis = await import('ioredis');
    const client = new Redis.default(url);
    redisClientInstance = client;
    return new RedisStore(client, options);
  } catch {
    throw new Error('ioredis is required for Redis store. Install it with: npm install ioredis');
  }
}

export function getRedisClient(): RedisClient | null {
  return redisClientInstance;
}

export async function disconnectRedis(): Promise<void> {
  if (redisClientInstance && 'disconnect' in redisClientInstance) {
    await (redisClientInstance as { disconnect: () => void }).disconnect();
  }
  redisClientInstance = null;
}