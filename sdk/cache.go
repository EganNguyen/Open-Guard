package sdk

import (
	"sync"
	"time"
)

type cacheEntry struct {
	value     bool
	expiresAt time.Time
}

type localCache struct {
	mu    sync.RWMutex
	data  map[string]cacheEntry
	ttl   time.Duration
}

func newLocalCache(ttl time.Duration) *localCache {
	return &localCache{
		data: make(map[string]cacheEntry),
		ttl:  ttl,
	}
}

func (c *localCache) Get(key string) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return false, false
	}
	return entry.value, true
}

func (c *localCache) Set(key string, value bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}
