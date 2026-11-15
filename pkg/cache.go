package pkg

import (
	"sync"
	"time"
)

// Cache - a TTL cache safe for concurrent use. The zero value is ready to use.
type Cache struct {
	mu    sync.Mutex
	items map[string]cacheItem
}

type cacheItem struct {
	value   any
	expires time.Time
}

// Get - returns a cached value if it has not expired.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.items[key]
	if !ok || time.Now().After(item.expires) {
		delete(c.items, key)
		return nil, false
	}
	return item.value, true
}

// Set - caches a value for the given TTL. A non-positive TTL skips caching.
func (c *Cache) Set(key string, value any, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.items == nil {
		c.items = make(map[string]cacheItem)
	}
	c.items[key] = cacheItem{value: value, expires: time.Now().Add(ttl)}
}

// Reset - drops every cached value.
func (c *Cache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = nil
}
