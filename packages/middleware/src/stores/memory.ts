import type { StoreAdapter } from '@open-guard/core/types';

interface MemoryStoreEntry {
  value: string;
  expiresAt?: number;
}

export class MemoryStore implements StoreAdapter {
  private store: Map<string, MemoryStoreEntry> = new Map();
  private maxKeys: number;

  constructor(options: { maxKeys?: number } = {}) {
    this.maxKeys = options.maxKeys ?? 10000;
  }

  async get(key: string): Promise<string | null> {
    const entry = this.store.get(key);
    if (!entry) return null;
    if (entry.expiresAt && entry.expiresAt < Date.now()) {
      this.store.delete(key);
      return null;
    }
    return entry.value;
  }

  async set(key: string, value: string, ttlSeconds?: number): Promise<void> {
    if (this.store.size >= this.maxKeys && !this.store.has(key)) {
      const firstKey = this.store.keys().next().value;
      if (firstKey) this.store.delete(firstKey);
    }

    const entry: MemoryStoreEntry = { value };
    if (ttlSeconds) {
      entry.expiresAt = Date.now() + ttlSeconds * 1000;
    }
    this.store.set(key, entry);
  }

  async incr(key: string, ttlSeconds?: number): Promise<number> {
    const current = await this.get(key);
    const count = current ? parseInt(current, 10) + 1 : 1;
    await this.set(key, String(count), ttlSeconds);
    return count;
  }

  async del(key: string): Promise<void> {
    this.store.delete(key);
  }

  clear(): void {
    this.store.clear();
  }

  size(): number {
    return this.store.size;
  }
}