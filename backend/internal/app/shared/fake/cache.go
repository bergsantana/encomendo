package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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
	down    bool
}

type cacheEntry struct {
	data    []byte
	expires time.Time
}

// NewCache returns an empty cache whose TTLs run on clock.
func NewCache(clock appshared.Clock) *Cache {
	return &Cache{clock: clock, entries: map[string]cacheEntry{}}
}

// SetDown makes every method that passes the misuse checks return an error
// matching shared.ErrUnavailable, as the Redis adapter does when Redis cannot
// be reached, so use case tests can cover the fallback to Postgres. The
// entries are kept for when the cache comes back.
func (c *Cache) SetDown(down bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.down = down
}

var errDown = fmt.Errorf("cache is down: %w", shared.ErrUnavailable)

// Get follows the Cache contract. It decodes into a fresh value and copies
// it to dst only on success, so a stale entry never half-writes dst.
func (c *Cache) Get(_ context.Context, key appshared.Key, dst any) (bool, error) {
	if key.IsZero() {
		return false, appshared.ErrInvalidCacheKey
	}
	target := reflect.ValueOf(dst)
	if dst == nil || target.Kind() != reflect.Pointer || target.IsNil() {
		return false, fmt.Errorf("decode into %T: %w", dst, appshared.ErrInvalidCacheValue)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.down {
		return false, errDown
	}
	e, ok := c.entries[key.String()]
	if !ok || !c.clock.Now().Before(e.expires) {
		return false, nil
	}
	fresh := reflect.New(target.Elem().Type())
	if err := json.Unmarshal(e.data, fresh.Interface()); err != nil {
		// The entry no longer fits its type: a miss, and it goes away.
		delete(c.entries, key.String())
		return false, nil
	}
	target.Elem().Set(fresh.Elem())
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
		return fmt.Errorf("encode cached %s: %w (%w)", key, appshared.ErrInvalidCacheValue, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.down {
		return errDown
	}
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
	if c.down {
		return errDown
	}
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
	if c.down {
		return errDown
	}
	for k := range c.entries {
		if k == p || strings.HasPrefix(k, p+":") {
			delete(c.entries, k)
		}
	}
	return nil
}
