//go:build integration

package redis_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/bergsantana/encomendo/backend/internal/adapter/redis"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
	"github.com/bergsantana/encomendo/backend/test/contract/sharedports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// contractDB is the Redis database the contract suite flushes between
// cases. The contract uses fixed tenants, so it gets a database no other
// test touches; the rest of the tests share database 0 with unique tenants.
const contractDB = 1

type cacheEnv struct {
	cache *redis.Cache
	raw   *goredis.Client
}

// newCacheOn returns a cache on url and a raw client on the same server,
// for checking what the adapter really stored.
func newCacheOn(t *testing.T, url string) cacheEnv {
	t.Helper()
	c, err := redis.Open(config.Redis{URL: url})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	opts, err := goredis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	raw := goredis.NewClient(opts)
	t.Cleanup(func() { _ = raw.Close() })
	return cacheEnv{cache: redis.NewCache(c, slog.New(slog.DiscardHandler)), raw: raw}
}

func newCache(t *testing.T) cacheEnv { return newCacheOn(t, redisURL) }

func newTenant() shared.TenantID { return shared.TenantID(pgtest.NewID()) }

func tenantKey(t *testing.T, tenant shared.TenantID, parts ...string) appshared.Key {
	t.Helper()
	k, err := appshared.TenantKey(tenant, parts...)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func globalKey(t *testing.T, parts ...string) appshared.Key {
	t.Helper()
	k, err := appshared.GlobalKey(parts...)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

type view struct {
	Name string `json:"name"`
}

func TestCache_Contract(t *testing.T) {
	env := newCacheOn(t, strings.TrimSuffix(redisURL, "/0")+fmt.Sprintf("/%d", contractDB))
	down := newCacheOn(t, closedPortURL(t)).cache
	sharedports.RunCacheContract(t, func(t *testing.T) sharedports.CacheHarness {
		if err := env.raw.FlushDB(t.Context()).Err(); err != nil {
			t.Fatal(err)
		}
		return sharedports.CacheHarness{
			Cache: env.cache,
			// Real time passes by itself; the TTL case polls with a deadline.
			Elapse: func(*testing.T, time.Duration) {},
			Down:   down,
		}
	})
}

func TestCache_SetStoresJSONWithATTL(t *testing.T) {
	t.Parallel()
	env := newCache(t)
	ctx := t.Context()
	k := tenantKey(t, newTenant(), "catalog", "item", "1")

	if err := env.cache.Set(ctx, k, view{Name: "Banco"}, 10*time.Minute); err != nil {
		t.Fatal(err)
	}

	raw, err := env.raw.Get(ctx, k.String()).Result()
	if err != nil || raw != `{"name":"Banco"}` {
		t.Errorf("stored value = %q, %v; want the JSON", raw, err)
	}
	if ttl := env.raw.PTTL(ctx, k.String()).Val(); ttl <= 9*time.Minute || ttl > 10*time.Minute {
		t.Errorf("PTTL = %v, want about 10 minutes", ttl)
	}
}

func TestCache_SetRoundsASubMillisecondTTLUp(t *testing.T) {
	t.Parallel()
	env := newCache(t)
	k := tenantKey(t, newTenant(), "catalog")

	if err := env.cache.Set(t.Context(), k, view{Name: "x"}, time.Microsecond); err != nil {
		t.Errorf("Set with a 1µs ttl = %v, want nil", err)
	}
}

func TestCache_GetDropsAnEntryThatIsNotJSON(t *testing.T) {
	t.Parallel()
	env := newCache(t)
	ctx := t.Context()
	k := tenantKey(t, newTenant(), "catalog", "item", "1")
	if err := env.raw.Set(ctx, k.String(), "not json", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}

	dst := view{Name: "untouched"}
	found, err := env.cache.Get(ctx, k, &dst)

	if err != nil || found || dst.Name != "untouched" {
		t.Errorf("Get = %v, %v, dst %+v; want a miss that leaves dst alone", found, err, dst)
	}
	if n := env.raw.Exists(ctx, k.String()).Val(); n != 0 {
		t.Error("the undecodable entry is still in Redis")
	}
}

func TestCache_DeleteByPrefixKeepsEverythingOutsideThePrefix(t *testing.T) {
	t.Parallel()
	env := newCache(t)
	ctx := t.Context()
	a, b := newTenant(), newTenant()
	catalog := tenantKey(t, a, "catalog")
	gone := []appshared.Key{catalog, tenantKey(t, a, "catalog", "item", "1"), tenantKey(t, a, "catalog", "kits")}
	kept := []appshared.Key{
		tenantKey(t, a, "catalogue"),
		tenantKey(t, a, "catalog-x"),
		tenantKey(t, b, "catalog"),
		tenantKey(t, b, "catalog", "item", "1"),
		globalKey(t, "catalog", pgtest.NewID().String()),
	}
	for _, k := range append(append([]appshared.Key{}, gone...), kept...) {
		if err := env.cache.Set(ctx, k, view{Name: k.String()}, time.Minute); err != nil {
			t.Fatal(err)
		}
	}

	if err := env.cache.DeleteByPrefix(ctx, catalog); err != nil {
		t.Fatal(err)
	}

	for _, k := range gone {
		if env.raw.Exists(ctx, k.String()).Val() != 0 {
			t.Errorf("%s survived DeleteByPrefix(%s)", k, catalog)
		}
	}
	for _, k := range kept {
		if env.raw.Exists(ctx, k.String()).Val() != 1 {
			t.Errorf("%s was deleted by DeleteByPrefix(%s)", k, catalog)
		}
	}
}

func TestCache_DeleteByPrefixDeletesMoreKeysThanOneScanPage(t *testing.T) {
	t.Parallel()
	env := newCache(t)
	ctx := t.Context()
	tenant := newTenant()
	prefix := tenantKey(t, tenant, "catalog")
	const n = 3000
	_, err := env.raw.Pipelined(ctx, func(p goredis.Pipeliner) error {
		for i := range n {
			p.Set(ctx, fmt.Sprintf("%s:item:%d", prefix, i), "{}", time.Minute)
		}
		p.Set(ctx, tenantKey(t, tenant, "catalogue").String(), "{}", time.Minute)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := env.cache.DeleteByPrefix(ctx, prefix); err != nil {
		t.Fatal(err)
	}

	left, err := scanAll(ctx, env.raw, "t:"+tenant.String()+":*")
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || !strings.HasSuffix(left[0], ":catalogue") {
		t.Errorf("%d keys left after DeleteByPrefix, want only the sibling :catalogue: %v", len(left), firstN(left, 3))
	}
}

func scanAll(ctx context.Context, rdb *goredis.Client, match string) ([]string, error) {
	var out []string
	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, match, 1000).Result()
		if err != nil {
			return nil, err
		}
		out = append(out, keys...)
		if next == 0 {
			return out, nil
		}
		cursor = next
	}
}

func firstN(s []string, n int) []string { return s[:min(n, len(s))] }

// closedPortURL returns the URL of a port nothing listens on, which refuses
// connections at once.
func closedPortURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return "redis://" + addr + "/0"
}

// silentServerURL returns the URL of a server that accepts connections and
// never answers, as a Redis stuck behind a network fault does.
func silentServerURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { close(done); _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() { <-done; _ = conn.Close() }()
		}
	}()
	return "redis://" + l.Addr().String() + "/0"
}

func TestCache_RedisDownReturnsUnavailableQuickly(t *testing.T) {
	t.Parallel()
	urls := map[string]string{
		"connection refused": closedPortURL(t),
		"no answer":          silentServerURL(t),
	}
	for name, url := range urls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newCacheOn(t, url).cache
			k := tenantKey(t, newTenant(), "catalog", "item", "1")
			calls := map[string]func(context.Context) error{
				"Get": func(ctx context.Context) error {
					var dst view
					_, err := c.Get(ctx, k, &dst)
					return err
				},
				"Set":            func(ctx context.Context) error { return c.Set(ctx, k, view{Name: "x"}, time.Minute) },
				"Delete":         func(ctx context.Context) error { return c.Delete(ctx, k) },
				"DeleteByPrefix": func(ctx context.Context) error { return c.DeleteByPrefix(ctx, k) },
			}
			for call, fn := range calls {
				start := time.Now()
				err := fn(t.Context())
				if !errors.Is(err, shared.ErrUnavailable) {
					t.Errorf("%s error = %v, want shared.ErrUnavailable", call, err)
				}
				if took := time.Since(start); took > 1500*time.Millisecond {
					t.Errorf("%s took %v, want it bounded by the cache timeout", call, took)
				}
			}
		})
	}
}

func TestCache_MisuseIsNotUnavailable(t *testing.T) {
	t.Parallel()
	// A closed port proves the checks run before any network call: misuse
	// must stay misuse, not turn into "Redis is down".
	c := newCacheOn(t, closedPortURL(t)).cache
	k := tenantKey(t, newTenant(), "catalog")

	err := c.Set(t.Context(), k, view{}, 0)
	if !errors.Is(err, appshared.ErrInvalidTTL) || errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("Set with ttl 0 error = %v, want only ErrInvalidTTL", err)
	}
	err = c.Delete(t.Context(), appshared.Key{})
	if !errors.Is(err, appshared.ErrInvalidCacheKey) || errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("Delete of the zero key error = %v, want only ErrInvalidCacheKey", err)
	}
}

func TestCache_RedisDownIsLoggedOnceWithoutKeyOrValue(t *testing.T) {
	t.Parallel()
	c, err := redis.Open(config.Redis{URL: closedPortURL(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	var logs bytes.Buffer
	cache := redis.NewCache(c, slog.New(slog.NewTextHandler(&logs, nil)))
	tenant := newTenant()
	k := tenantKey(t, tenant, "catalog", "item", "1")

	if err := cache.Set(t.Context(), k, view{Name: "segredo-do-valor"}, time.Minute); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("Set error = %v, want shared.ErrUnavailable", err)
	}

	out := logs.String()
	if n := strings.Count(out, "\n"); n != 1 {
		t.Errorf("%d log lines for one failed call, want 1:\n%s", n, out)
	}
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "op=set") {
		t.Errorf("log = %q, want a warn line naming the operation", out)
	}
	for _, secret := range []string{tenant.String(), "catalog", "segredo-do-valor"} {
		if strings.Contains(out, secret) {
			t.Errorf("log carries %q: %s", secret, out)
		}
	}
}

func TestCache_ACancelledRequestIsNotLoggedAsAnOutage(t *testing.T) {
	t.Parallel()
	c, err := redis.Open(config.Redis{URL: closedPortURL(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	var logs bytes.Buffer
	cache := redis.NewCache(c, slog.New(slog.NewTextHandler(&logs, nil)))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = cache.Delete(ctx, tenantKey(t, newTenant(), "catalog"))

	if !errors.Is(err, shared.ErrUnavailable) || !errors.Is(err, context.Canceled) {
		t.Errorf("Delete error = %v, want ErrUnavailable and context.Canceled", err)
	}
	if logs.Len() != 0 {
		t.Errorf("a cancelled request was logged: %s", logs.String())
	}
}
