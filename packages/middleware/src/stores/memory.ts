import { StoreAdapter } from '@open-guard/core';

export interface MemoryStoreOptions {
  maxKeys?: number;
}

export class MemoryStore implements StoreAdapter {
  private data: Map<string, string>;
  private expiry: Map<string, number>;
  private maxKeys: number;
  private cleanupTimer?: ReturnType<typeof setTimeout>;

  constructor(options: MemoryStoreOptions = {}) {
    this.data = new Map();
    this.expiry = new Map();
    this.maxKeys = options.maxKeys || 10000;
    this.startCleanup();
  }

  async get(key: string): Promise<string | null> {
    const expiryTime = this.expiry.get(key);
    if (expiryTime && expiryTime < Date.now()) {
      this.data.delete(key);
      this.expiry.delete(key);
      return null;
    }
    return this.data.get(key) ?? null;
  }

  async set(key: string, value: string, ttlSeconds?: number): Promise<void> {
    if (this.data.size >= this.maxKeys && !this.data.has(key)) {
      this.evictLRU();
    }
    this.data.set(key, value);
    if (ttlSeconds) {
      this.expiry.set(key, Date.now() + ttlSeconds * 1000);
    }
  }

  async incr(key: string, ttlSeconds?: number): Promise<number> {
    const current = await this.get(key);
    const newValue = current ? parseInt(current, 10) + 1 : 1;
    this.data.set(key, String(newValue));
    if (ttlSeconds) {
      this.expiry.set(key, Date.now() + ttlSeconds * 1000);
    } else {
      this.expiry.delete(key);
    }
    return newValue;
  }

  async del(key: string): Promise<void> {
    this.data.delete(key);
    this.expiry.delete(key);
  }

  async keys(pattern?: string): Promise<string[]> {
    const allKeys = Array.from(this.data.keys());
    if (!pattern) return allKeys;
    const regex = new RegExp('^' + pattern.replace(/\*/g, '.*') + '$');
    return allKeys.filter(k => regex.test(k));
  }

  async clear(): Promise<void> {
    this.data.clear();
    this.expiry.clear();
  }

  size(): number {
    return this.data.size;
  }

  private evictLRU(): void {
    const keys = Array.from(this.data.keys());
    if (keys.length > 0) {
      const keyToRemove = keys[0];
      this.data.delete(keyToRemove);
      this.expiry.delete(keyToRemove);
    }
  }

  private startCleanup(): void {
    this.cleanupTimer = setInterval(() => {
      const now = Date.now();
      for (const [key, expiryTime] of this.expiry.entries()) {
        if (expiryTime < now) {
          this.data.delete(key);
          this.expiry.delete(key);
        }
      }
    }, 60000);
  }

  destroy(): void {
    if (this.cleanupTimer) {
      clearInterval(this.cleanupTimer);
    }
  }
}