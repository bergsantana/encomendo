package shared

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Cache misuse errors. Like ErrNoTransaction they mean the calling code is
// wrong, never that the cache is down, so they match no domain sentinel and
// reach the API as a logged 500.
var (
	// ErrInvalidCacheKey means a key part is empty or uses a character
	// outside [A-Za-z0-9._-], the tenant is zero, or a method got the zero
	// Key.
	ErrInvalidCacheKey = errors.New("cache: invalid key")
	// ErrInvalidTTL means a TTL is zero or negative. Every entry expires.
	ErrInvalidTTL = errors.New("cache: ttl must be positive")
	// ErrInvalidCacheValue means Set got a value JSON cannot encode, or Get
	// got a destination JSON cannot decode into (nil or not a pointer).
	ErrInvalidCacheValue = errors.New("cache: value cannot go through JSON")
)

// Key is a cache key that always starts with its owner: "t:{tenantID}:" for
// tenant data or "g:" for global presets. RLS does not protect Redis, so the
// only way to build a Key is TenantKey or GlobalKey, and the Redis adapter
// checks the prefix again (backend/CLAUDE.md, Redis).
type Key struct {
	s string
}

// TenantKey returns the key t:{tenant}:{parts...}. It returns
// ErrInvalidCacheKey for a zero tenant, no parts, or a part that is empty
// or holds a character outside [A-Za-z0-9._-]; the separator and Redis glob
// characters are refused so that no part can reach another key's space.
func TenantKey(tenant shared.TenantID, parts ...string) (Key, error) {
	if tenant.IsZero() {
		return Key{}, fmt.Errorf("tenant key without a tenant: %w", ErrInvalidCacheKey)
	}
	return newKey("t:"+tenant.String(), parts)
}

// GlobalKey returns the key g:{parts...} for read-only global data such as
// status workflow presets and templates. Errors as for TenantKey.
func GlobalKey(parts ...string) (Key, error) {
	return newKey("g", parts)
}

func newKey(prefix string, parts []string) (Key, error) {
	if len(parts) == 0 {
		return Key{}, fmt.Errorf("key %s without parts: %w", prefix, ErrInvalidCacheKey)
	}
	for _, p := range parts {
		if !validKeyPart(p) {
			return Key{}, fmt.Errorf("key part %q: %w", p, ErrInvalidCacheKey)
		}
	}
	return Key{s: prefix + ":" + strings.Join(parts, ":")}, nil
}

func validKeyPart(p string) bool {
	if p == "" {
		return false
	}
	for _, r := range p {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

// String returns the full key.
func (k Key) String() string { return k.s }

// IsZero reports whether k was never built, which every Cache method refuses.
func (k Key) IsZero() bool { return k.s == "" }

// Cache stores JSON-encoded values for hot reads that may be slightly stale:
// store page catalog and kits, portal config, item detail, workflow presets.
// Never cache orders, customers or anything that depends on the caller's
// permissions (docs/architecture.md section 4.4).
//
// A cache problem never fails a request. When the cache cannot be reached,
// every method returns an error that matches shared.ErrUnavailable; the use
// case treats it as a miss (Get) or a skipped write (Set and the deletes),
// logs it, and reads from Postgres. A stored value that no longer decodes
// into dst (its type changed in a deploy, or it is corrupt) is a miss
// without an error, and the entry is dropped; the adapter logs that. Misuse
// has its own errors: the zero Key (ErrInvalidCacheKey), a non-positive TTL
// (ErrInvalidTTL), and values or destinations JSON cannot handle
// (ErrInvalidCacheValue).
type Cache interface {
	// Get decodes the value at key into dst, which must be a non-nil
	// pointer, and reports whether it was found, not expired, and decoded.
	// On a miss dst is left untouched, even when a stale entry half fitted.
	Get(ctx context.Context, key Key, dst any) (bool, error)
	// Set stores value at key for ttl, replacing any previous value.
	Set(ctx context.Context, key Key, value any, ttl time.Duration) error
	// Delete removes key. Deleting a missing key is not an error.
	Delete(ctx context.Context, key Key) error
	// DeleteByPrefix removes prefix itself and every key under it
	// (prefix + ":" + anything), and nothing else: deleting t:{id}:catalog
	// leaves t:{id}:catalogue alone. Call it after a write commits.
	DeleteByPrefix(ctx context.Context, prefix Key) error
}
