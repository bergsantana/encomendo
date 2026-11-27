package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

const (
	// cacheTimeout bounds a single-key call. The cache only saves a
	// Postgres read, so a Redis that hangs must cost less than that read
	// would: the caller gets ErrUnavailable and falls back.
	cacheTimeout = 500 * time.Millisecond
	// deletePrefixTimeout bounds DeleteByPrefix as a whole. It scans many
	// pages, so it gets more room than one key does.
	deletePrefixTimeout = 5 * time.Second
	// scanBatch is the COUNT hint for each SCAN page.
	scanBatch = 500
	// canonicalIDLen is the length of a UUID in its canonical form.
	canonicalIDLen = 36
)

// Cache implements appshared.Cache on Redis. Values are JSON, every entry
// has a TTL, and every key must start with "t:{tenantID}:" or "g:", which
// the adapter checks again because RLS does not protect Redis
// (backend/CLAUDE.md, Redis).
type Cache struct {
	rdb *goredis.Client
	log *slog.Logger
}

var _ appshared.Cache = (*Cache)(nil)

// NewCache returns the cache on c.
func NewCache(c *Client, log *slog.Logger) *Cache {
	return &Cache{rdb: c.rdb, log: log}
}

// Get follows appshared.Cache. A Redis failure returns an error that
// matches shared.ErrUnavailable. A stored value that does not decode into
// dst is a miss, dst stays untouched, and the entry is deleted.
func (c *Cache) Get(ctx context.Context, key appshared.Key, dst any) (bool, error) {
	if err := checkKey(key); err != nil {
		return false, err
	}
	target := reflect.ValueOf(dst)
	if dst == nil || target.Kind() != reflect.Pointer || target.IsNil() {
		return false, fmt.Errorf("decode into %T: %w", dst, appshared.ErrInvalidCacheValue)
	}
	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	data, err := c.rdb.Get(ctx, key.String()).Bytes()
	if errors.Is(err, goredis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, c.unavailable(ctx, "get", err)
	}
	// Decode into a fresh value so a stale entry that half fits never
	// writes part of dst.
	fresh := reflect.New(target.Elem().Type())
	if err := json.Unmarshal(data, fresh.Interface()); err != nil {
		c.dropUndecodable(ctx, key, target.Elem().Type())
		return false, nil
	}
	target.Elem().Set(fresh.Elem())
	return true, nil
}

// dropUndecodable deletes an entry that no longer fits its type, which a
// deploy that changes a cached view can cause. It logs the type only: the
// key names a tenant and the decode error may quote the value.
func (c *Cache) dropUndecodable(ctx context.Context, key appshared.Key, into reflect.Type) {
	c.log.WarnContext(ctx, "cached value does not decode; treated as a miss and dropped", "type", into.String())
	if err := c.rdb.Unlink(ctx, key.String()).Err(); err != nil {
		c.log.WarnContext(ctx, "dropping an undecodable cache entry failed; it expires with its ttl", "err", err)
	}
}

// Set follows appshared.Cache. A Redis failure returns an error that
// matches shared.ErrUnavailable.
func (c *Cache) Set(ctx context.Context, key appshared.Key, value any, ttl time.Duration) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if ttl <= 0 {
		return appshared.ErrInvalidTTL
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode cached value: %w (%w)", appshared.ErrInvalidCacheValue, err)
	}
	// Redis counts expiry in whole milliseconds and refuses 0, so a shorter
	// positive TTL is rounded up instead of failing.
	ttl = max(ttl, time.Millisecond)
	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	if err := c.rdb.Set(ctx, key.String(), data, ttl).Err(); err != nil {
		return c.unavailable(ctx, "set", err)
	}
	return nil
}

// Delete follows appshared.Cache. A Redis failure returns an error that
// matches shared.ErrUnavailable.
func (c *Cache) Delete(ctx context.Context, key appshared.Key) error {
	if err := checkKey(key); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	if err := c.rdb.Unlink(ctx, key.String()).Err(); err != nil {
		return c.unavailable(ctx, "delete", err)
	}
	return nil
}

// DeleteByPrefix follows appshared.Cache. It removes prefix itself, then
// SCANs for prefix+":*" and UNLINKs each page, so Redis is never blocked
// the way KEYS or a huge DEL would block it. Each Redis call gets
// cacheTimeout, so a hung Redis fails on the first one, and the whole
// delete gets deletePrefixTimeout. A Redis failure returns an error that
// matches shared.ErrUnavailable; entries already removed stay removed, and
// the rest expire with their TTL.
func (c *Cache) DeleteByPrefix(ctx context.Context, prefix appshared.Key) error {
	if err := checkKey(prefix); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, deletePrefixTimeout)
	defer cancel()
	p := prefix.String()
	if err := c.unlink(ctx, p); err != nil {
		return c.unavailable(ctx, "delete by prefix", err)
	}
	match := escapeGlob(p) + ":*"
	var cursor uint64
	for {
		keys, next, err := c.scanPage(ctx, cursor, match)
		if err != nil {
			return c.unavailable(ctx, "delete by prefix", err)
		}
		if len(keys) > 0 {
			if err := c.unlink(ctx, keys...); err != nil {
				return c.unavailable(ctx, "delete by prefix", err)
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

func (c *Cache) unlink(ctx context.Context, keys ...string) error {
	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	return c.rdb.Unlink(ctx, keys...).Err()
}

func (c *Cache) scanPage(ctx context.Context, cursor uint64, match string) ([]string, uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	return c.rdb.Scan(ctx, cursor, match, scanBatch).Result()
}

// unavailable logs a Redis failure once, at warn, and wraps it so the use
// case can tell "the cache is down, read from Postgres" from misuse. The
// use case does not log (backend/CLAUDE.md, Logging), so this is the only
// record of an outage. The log line carries the operation and the Redis
// error, never the key or the value. The cause stays in the chain, so a
// cancelled request is still context.Canceled.
func (c *Cache) unavailable(ctx context.Context, op string, err error) error {
	c.log.WarnContext(ctx, "cache unavailable; the caller falls back to Postgres", "op", op, "err", err)
	return fmt.Errorf("cache %s: %w (%w)", op, shared.ErrUnavailable, err)
}

// checkKey refuses the zero Key and any key that does not start with
// "t:{tenantID}:" or "g:". Key can only be built through TenantKey and
// GlobalKey, which produce those prefixes; this is the second lock on the
// door (docs/architecture.md section 4.4).
func checkKey(k appshared.Key) error {
	if k.IsZero() || !hasOwnerPrefix(k.String()) {
		return appshared.ErrInvalidCacheKey
	}
	return nil
}

func hasOwnerPrefix(s string) bool {
	if rest, ok := strings.CutPrefix(s, "g:"); ok {
		return rest != ""
	}
	rest, ok := strings.CutPrefix(s, "t:")
	if !ok || len(rest) <= canonicalIDLen+1 || rest[canonicalIDLen] != ':' {
		return false
	}
	_, err := shared.ParseTenantID(rest[:canonicalIDLen])
	return err == nil
}

// escapeGlob backslash-escapes the characters a SCAN MATCH pattern treats as
// special. Key parts cannot hold them; this keeps a prefix from widening the
// delete if that rule ever loosens.
func escapeGlob(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`\*?[]`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
