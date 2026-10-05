package monitor

import (
	"sync"
	"time"
)

// cache - a tiny TTL cache for the computed stats. Every page load and API
// call recomputes the charts from the whole history of every host, which is
// expensive; the results are valid until the next check anyway.
type cache struct {
	mu    sync.Mutex
	items map[string]cacheItem
}

type cacheItem struct {
	value   any
	expires time.Time
}

func (c *cache) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.items[key]
	if !ok || time.Now().After(item.expires) {
		delete(c.items, key)
		return nil, false
	}
	return item.value, true
}

func (c *cache) set(key string, value any, ttl time.Duration) {
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

// reset - drops every cached value (a check produced new data or the config changed)
func (c *cache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = nil
}
