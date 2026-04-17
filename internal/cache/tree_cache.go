package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type TreeCache interface {
	Get(path string) ([]string, bool)
	Set(path string, values []string)
	Delete(path string)
}

type memoryTreeCache struct {
	ttl   time.Duration
	items map[string]cacheEntry
	mu    sync.RWMutex
}

type cacheEntry struct {
	Values    []string
	ExpiresAt time.Time
}

func NewMemoryTreeCache(ttl time.Duration) TreeCache {
	return &memoryTreeCache{
		ttl:   ttl,
		items: map[string]cacheEntry{},
	}
}

func (c *memoryTreeCache) Get(path string) ([]string, bool) {
	c.mu.RLock()
	entry, ok := c.items[path]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.ExpiresAt) {
		if ok {
			c.Delete(path)
		}
		return nil, false
	}
	out := append([]string(nil), entry.Values...)
	return out, true
}

func (c *memoryTreeCache) Set(path string, values []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[path] = cacheEntry{
		Values:    append([]string(nil), values...),
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

func (c *memoryTreeCache) Delete(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, path)
}

type redisTreeCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisTreeCache(client *redis.Client, ttl time.Duration) TreeCache {
	return &redisTreeCache{client: client, ttl: ttl}
}

func (c *redisTreeCache) Get(path string) ([]string, bool) {
	ctx := context.Background()
	raw, err := c.client.Get(ctx, "tree:"+path).Bytes()
	if err != nil {
		return nil, false
	}
	var values []string
	if json.Unmarshal(raw, &values) != nil {
		return nil, false
	}
	return values, true
}

func (c *redisTreeCache) Set(path string, values []string) {
	ctx := context.Background()
	raw, _ := json.Marshal(values)
	_ = c.client.Set(ctx, "tree:"+path, raw, c.ttl).Err()
}

func (c *redisTreeCache) Delete(path string) {
	_ = c.client.Del(context.Background(), "tree:"+path).Err()
}
