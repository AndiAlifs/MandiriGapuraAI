package cache

import (
	"sync"
	"time"
)

type Entry struct {
	Body      []byte
	ExpiresAt time.Time
}

type MemoryCache struct {
	mu      sync.RWMutex
	entries map[string]Entry
	ttl     time.Duration
}

func NewMemoryCache(ttl time.Duration) *MemoryCache {
	return &MemoryCache{
		entries: make(map[string]Entry),
		ttl:     ttl,
	}
}

func (c *MemoryCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}
	copyBody := append([]byte(nil), entry.Body...)
	return copyBody, true
}

func (c *MemoryCache) Set(key string, body []byte) {
	c.mu.Lock()
	c.entries[key] = Entry{
		Body:      append([]byte(nil), body...),
		ExpiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}
