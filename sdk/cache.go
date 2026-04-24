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
	mu   sync.RWMutex
	data map[string]cacheEntry
	ttl  time.Duration
	done chan struct{}
}

func newLocalCache(ttl time.Duration) *localCache {
	c := &localCache{
		data: make(map[string]cacheEntry),
		ttl:  ttl,
		done: make(chan struct{}),
	}
	go c.evictionLoop()
	return c
}

func (c *localCache) evictionLoop() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			now := time.Now()
			// Grace period: keep until expiry + 60s (for stale-while-unavailable)
			cutoff := now.Add(-60 * time.Second)
			c.mu.Lock()
			for k, v := range c.data {
				if v.expiresAt.Before(cutoff) {
					delete(c.data, k)
				}
			}
			c.mu.Unlock()
		}
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

func (c *localCache) GetOrStale(key string) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.data[key]
	if !ok {
		return false, false
	}
	if time.Now().Before(entry.expiresAt) {
		return entry.value, true // fresh hit
	}
	// Stale: check grace period (60s after expiry)
	if time.Now().Before(entry.expiresAt.Add(60 * time.Second)) {
		return entry.value, true // grace hit — serve stale during outage
	}
	return false, false // expired beyond grace period
}

func (c *localCache) Set(key string, value bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *localCache) Close() {
	close(c.done)
}
