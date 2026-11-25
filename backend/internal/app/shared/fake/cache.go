package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Cache is a Cache in memory. Values go through JSON, as in Redis, so a
// value that would not survive the real cache fails here too. Entries expire
// on the given clock.
type Cache struct {
	mu      sync.Mutex
	clock   appshared.Clock
	entries map[string]cacheEntry
}

type cacheEntry struct {
	data    []byte
	expires time.Time
}

// NewCache returns an empty cache whose TTLs run on clock.
func NewCache(clock appshared.Clock) *Cache {
	return &Cache{clock: clock, entries: map[string]cacheEntry{}}
}

// Get follows the Cache contract.
func (c *Cache) Get(_ context.Context, key appshared.Key, dst any) (bool, error) {
	if key.IsZero() {
		return false, appshared.ErrInvalidCacheKey
	}
	c.mu.Lock()
	e, ok := c.entries[key.String()]
	c.mu.Unlock()
	if !ok || !c.clock.Now().Before(e.expires) {
		return false, nil
	}
	if err := json.Unmarshal(e.data, dst); err != nil {
		return false, fmt.Errorf("decode cached %s: %w (%w)", key, shared.ErrInvalid, err)
	}
	return true, nil
}

// Set follows the Cache contract.
func (c *Cache) Set(_ context.Context, key appshared.Key, value any, ttl time.Duration) error {
	if key.IsZero() {
		return appshared.ErrInvalidCacheKey
	}
	if ttl <= 0 {
		return appshared.ErrInvalidTTL
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode cached %s: %w (%w)", key, shared.ErrInvalid, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key.String()] = cacheEntry{data: data, expires: c.clock.Now().Add(ttl)}
	return nil
}

// Delete follows the Cache contract.
func (c *Cache) Delete(_ context.Context, key appshared.Key) error {
	if key.IsZero() {
		return appshared.ErrInvalidCacheKey
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key.String())
	return nil
}

// DeleteByPrefix follows the Cache contract.
func (c *Cache) DeleteByPrefix(_ context.Context, prefix appshared.Key) error {
	if prefix.IsZero() {
		return appshared.ErrInvalidCacheKey
	}
	p := prefix.String()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if k == p || strings.HasPrefix(k, p+":") {
			delete(c.entries, k)
		}
	}
	return nil
}
