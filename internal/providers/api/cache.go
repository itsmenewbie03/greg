package api

import (
	"sync"
)

// InfoCache caches media info responses to avoid redundant API calls
type InfoCache struct {
	mu   sync.RWMutex
	data map[string]*InfoResponse
}

// NewInfoCache creates a new InfoCache
func NewInfoCache() *InfoCache {
	return &InfoCache{
		data: make(map[string]*InfoResponse),
	}
}

// Get retrieves a cached response
func (c *InfoCache) Get(key string) (*InfoResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.data[key]
	return val, ok
}

// Set stores a response in the cache
func (c *InfoCache) Set(key string, val *InfoResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = val
}
