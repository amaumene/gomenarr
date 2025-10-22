package cache

import (
	"time"

	"github.com/amaumene/gomenarr/internal/platform/config"
	gocache "github.com/patrickmn/go-cache"
)

// MemoryCache implements ports.Cache using in-memory storage
type MemoryCache struct {
	cache             *gocache.Cache
	defaultExpiration time.Duration
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(cfg config.CacheConfig) *MemoryCache {
	return &MemoryCache{
		cache:             gocache.New(cfg.DefaultExpiration, cfg.CleanupInterval),
		defaultExpiration: cfg.DefaultExpiration,
	}
}

func (c *MemoryCache) Get(key string) (interface{}, bool) {
	return c.cache.Get(key)
}

func (c *MemoryCache) Set(key string, value interface{}) {
	c.cache.Set(key, value, c.defaultExpiration)
}

func (c *MemoryCache) SetWithExpiration(key string, value interface{}, expiration time.Duration) {
	c.cache.Set(key, value, expiration)
}

func (c *MemoryCache) Delete(key string) {
	c.cache.Delete(key)
}

func (c *MemoryCache) Clear() {
	c.cache.Flush()
}

func (c *MemoryCache) ItemCount() int {
	return c.cache.ItemCount()
}
