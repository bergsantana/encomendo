package shared

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Cache errors. Each matches shared.ErrInvalid: they are programming
// mistakes, never the result of the cache being down.
var (
	// ErrInvalidCacheKey means a key part is empty or uses a character
	// outside [A-Za-z0-9._-], or the tenant is zero.
	ErrInvalidCacheKey = fmt.Errorf("cache: invalid key: %w", shared.ErrInvalid)
	// ErrInvalidTTL means a TTL is zero or negative. Every entry expires.
	ErrInvalidTTL = fmt.Errorf("cache: ttl must be positive: %w", shared.ErrInvalid)
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
// A cache outage never fails a request: an unreachable cache is a miss for
// Get and a no-op for Set and the deletes, and the adapter logs it. Errors
// are returned only for a zero key, a non-positive TTL, or a value that
// cannot be encoded or decoded as JSON.
type Cache interface {
	// Get decodes the value at key into dst, which must be a pointer, and
	// reports whether it was found and not expired.
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
