package sharedports

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// CacheHarness is one Cache under test and a way to make time pass for it.
type CacheHarness struct {
	// Cache starts empty: the cases use fixed tenants and expect exact
	// misses, so a Redis harness flushes its database (or uses a new one)
	// for each harness.
	Cache appshared.Cache
	// Elapse lets d pass for the cache. A fake advances its clock; a real
	// cache can return at once, because the TTL case polls with a deadline
	// either way (no sleeps in tests).
	Elapse func(t *testing.T, d time.Duration)
}

type cachedItem struct {
	Name  string `json:"name"`
	Cents int64  `json:"cents"`
}

var (
	cacheTenantA = shared.TenantID(shared.NewV7(1_700_000_000_000, [10]byte{0xa}))
	cacheTenantB = shared.TenantID(shared.NewV7(1_700_000_000_000, [10]byte{0xb}))
)

func tenantKey(t *testing.T, tenant shared.TenantID, parts ...string) appshared.Key {
	t.Helper()
	k, err := appshared.TenantKey(tenant, parts...)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// RunCacheContract checks that a Cache stores JSON values per key, expires
// them, treats an entry that no longer fits as a miss, deletes by key and by
// prefix without touching siblings or other tenants, and refuses zero keys,
// non-positive TTLs and values JSON cannot carry.
func RunCacheContract(t *testing.T, newHarness func(t *testing.T) CacheHarness) {
	t.Run("miss on a missing key", func(t *testing.T) { cacheMiss(t, newHarness(t)) })
	t.Run("set then get round-trips JSON", func(t *testing.T) { cacheRoundTrip(t, newHarness(t)) })
	t.Run("set replaces", func(t *testing.T) { cacheReplace(t, newHarness(t)) })
	t.Run("an entry that no longer fits is a miss", func(t *testing.T) { cacheStaleShape(t, newHarness(t)) })
	t.Run("entries expire after their ttl", func(t *testing.T) { cacheExpiry(t, newHarness(t)) })
	t.Run("delete removes one key", func(t *testing.T) { cacheDelete(t, newHarness(t)) })
	t.Run("delete by prefix stays inside the prefix", func(t *testing.T) { cacheDeleteByPrefix(t, newHarness(t)) })
	t.Run("tenants never share entries", func(t *testing.T) { cacheTenantIsolation(t, newHarness(t)) })
	t.Run("refuses invalid input", func(t *testing.T) { cacheInvalidInput(t, newHarness(t)) })
}

func mustGet(t *testing.T, c appshared.Cache, k appshared.Key) (cachedItem, bool) {
	t.Helper()
	var got cachedItem
	found, err := c.Get(context.Background(), k, &got)
	if err != nil {
		t.Fatalf("Get(%s) error = %v", k, err)
	}
	return got, found
}

func mustSet(t *testing.T, c appshared.Cache, k appshared.Key, v cachedItem, ttl time.Duration) {
	t.Helper()
	if err := c.Set(context.Background(), k, v, ttl); err != nil {
		t.Fatalf("Set(%s) error = %v", k, err)
	}
}

func cacheMiss(t *testing.T, h CacheHarness) {
	if _, found := mustGet(t, h.Cache, tenantKey(t, cacheTenantA, "item", "missing")); found {
		t.Error("Get found a key that was never set")
	}
}

func cacheRoundTrip(t *testing.T, h CacheHarness) {
	k := tenantKey(t, cacheTenantA, "item", "1")
	want := cachedItem{Name: "Banco de couro", Cents: 45000}
	mustSet(t, h.Cache, k, want, time.Minute)

	got, found := mustGet(t, h.Cache, k)
	if !found || got != want {
		t.Errorf("Get = %+v, %v; want %+v", got, found, want)
	}
}

func cacheReplace(t *testing.T, h CacheHarness) {
	k := tenantKey(t, cacheTenantA, "item", "1")
	mustSet(t, h.Cache, k, cachedItem{Name: "old"}, time.Minute)
	mustSet(t, h.Cache, k, cachedItem{Name: "new"}, time.Minute)

	if got, _ := mustGet(t, h.Cache, k); got.Name != "new" {
		t.Errorf("Get = %+v, want the second value", got)
	}
}

func cacheStaleShape(t *testing.T, h CacheHarness) {
	// A deploy changed the cached view's type while old entries live on.
	// The read must fall back to Postgres, not fail the store page.
	k := tenantKey(t, cacheTenantA, "item", "1")
	mustSet(t, h.Cache, k, cachedItem{Name: "Banco de couro"}, time.Minute)

	type changedItem struct {
		Name  int   `json:"name"`  // was a string
		Cents int64 `json:"cents"` // still fits, so a careless decode would write it
	}
	changed := changedItem{Name: 7, Cents: 1}
	found, err := h.Cache.Get(context.Background(), k, &changed)
	if err != nil || found {
		t.Errorf("Get into a changed type = %v, %v; want a miss without error", found, err)
	}
	if changed != (changedItem{Name: 7, Cents: 1}) {
		t.Errorf("dst = %+v after a miss, want it untouched", changed)
	}
	if _, found := mustGet(t, h.Cache, k); found {
		t.Error("the entry that did not fit is still cached")
	}
}

func cacheExpiry(t *testing.T, h CacheHarness) {
	const ttl = 200 * time.Millisecond
	k := tenantKey(t, cacheTenantA, "item", "ttl")
	mustSet(t, h.Cache, k, cachedItem{Name: "short"}, ttl)
	if _, found := mustGet(t, h.Cache, k); !found {
		t.Fatal("Get missed right after Set")
	}

	h.Elapse(t, ttl)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if _, found := mustGet(t, h.Cache, k); !found {
			return
		}
		h.Elapse(t, 50*time.Millisecond)
	}
	t.Error("entry still present well after its ttl")
}

func cacheDelete(t *testing.T, h CacheHarness) {
	ctx := context.Background()
	k := tenantKey(t, cacheTenantA, "item", "1")
	mustSet(t, h.Cache, k, cachedItem{Name: "x"}, time.Minute)

	if err := h.Cache.Delete(ctx, k); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	if _, found := mustGet(t, h.Cache, k); found {
		t.Error("Get found a deleted key")
	}
	if err := h.Cache.Delete(ctx, k); err != nil {
		t.Errorf("Delete of a missing key error = %v, want nil", err)
	}
}

func cacheDeleteByPrefix(t *testing.T, h CacheHarness) {
	catalog := tenantKey(t, cacheTenantA, "catalog")
	gone := []appshared.Key{catalog, tenantKey(t, cacheTenantA, "catalog", "item", "1"), tenantKey(t, cacheTenantA, "catalog", "kits")}
	kept := []appshared.Key{
		tenantKey(t, cacheTenantA, "catalogue"), // shares the characters, not the prefix
		tenantKey(t, cacheTenantA, "portal"),
		tenantKey(t, cacheTenantB, "catalog", "item", "1"),
	}
	for _, k := range append(append([]appshared.Key{}, gone...), kept...) {
		mustSet(t, h.Cache, k, cachedItem{Name: k.String()}, time.Minute)
	}

	if err := h.Cache.DeleteByPrefix(context.Background(), catalog); err != nil {
		t.Fatalf("DeleteByPrefix error = %v", err)
	}
	for _, k := range gone {
		if _, found := mustGet(t, h.Cache, k); found {
			t.Errorf("%s survived DeleteByPrefix(%s)", k, catalog)
		}
	}
	for _, k := range kept {
		if _, found := mustGet(t, h.Cache, k); !found {
			t.Errorf("%s was deleted by DeleteByPrefix(%s)", k, catalog)
		}
	}
}

func cacheTenantIsolation(t *testing.T, h CacheHarness) {
	a, b := tenantKey(t, cacheTenantA, "portal"), tenantKey(t, cacheTenantB, "portal")
	mustSet(t, h.Cache, a, cachedItem{Name: "A"}, time.Minute)

	if _, found := mustGet(t, h.Cache, b); found {
		t.Error("tenant B read tenant A's entry")
	}
	mustSet(t, h.Cache, b, cachedItem{Name: "B"}, time.Minute)
	if got, _ := mustGet(t, h.Cache, a); got.Name != "A" {
		t.Errorf("tenant A's entry = %+v after tenant B wrote", got)
	}
}

func cacheInvalidInput(t *testing.T, h CacheHarness) {
	ctx := context.Background()
	k := tenantKey(t, cacheTenantA, "item", "1")
	var dst cachedItem
	checks := []struct {
		name string
		err  error
		want error
	}{
		{"get zero key", second(h.Cache.Get(ctx, appshared.Key{}, &dst)), appshared.ErrInvalidCacheKey},
		{"set zero key", h.Cache.Set(ctx, appshared.Key{}, dst, time.Minute), appshared.ErrInvalidCacheKey},
		{"delete zero key", h.Cache.Delete(ctx, appshared.Key{}), appshared.ErrInvalidCacheKey},
		{"delete by zero prefix", h.Cache.DeleteByPrefix(ctx, appshared.Key{}), appshared.ErrInvalidCacheKey},
		{"zero ttl", h.Cache.Set(ctx, k, dst, 0), appshared.ErrInvalidTTL},
		{"negative ttl", h.Cache.Set(ctx, k, dst, -time.Second), appshared.ErrInvalidTTL},
		{"value JSON cannot encode", h.Cache.Set(ctx, k, func() {}, time.Minute), appshared.ErrInvalidCacheValue},
	}
	for _, c := range checks {
		if !errors.Is(c.err, c.want) {
			t.Errorf("%s: error = %v, want %v", c.name, c.err, c.want)
		}
	}

	mustSet(t, h.Cache, k, cachedItem{Name: "x"}, time.Minute)
	if _, err := h.Cache.Get(ctx, k, dst); !errors.Is(err, appshared.ErrInvalidCacheValue) {
		t.Errorf("Get into a non-pointer error = %v, want ErrInvalidCacheValue", err)
	}
	if _, err := h.Cache.Get(ctx, k, nil); !errors.Is(err, appshared.ErrInvalidCacheValue) {
		t.Errorf("Get into nil error = %v, want ErrInvalidCacheValue", err)
	}
}

func second[T any](_ T, err error) error { return err }
