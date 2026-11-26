//go:build integration

package redis_test

import (
	"log/slog"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/bergsantana/encomendo/backend/internal/adapter/redis"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

func newRateLimiter(t *testing.T) (*redis.RateLimiter, *sharedfake.Clock) {
	t.Helper()
	c, err := redis.Open(config.Redis{URL: redisURL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	clock := sharedfake.NewClock(time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC))
	return redis.NewRateLimiter(c, clock, slog.New(slog.DiscardHandler)), clock
}

// key is a key no other test uses, since every test shares the container.
func key(rule string) string { return "g:rl:" + rule + ":ip:" + pgtest.NewID().String() }

func TestRateLimiter_SlidingWindowEdges(t *testing.T) {
	t.Parallel()
	l, clock := newRateLimiter(t)
	ctx := t.Context()
	k := key("edges")

	for i := range 3 {
		if ok, _ := l.Allow(ctx, k, 3, time.Minute); !ok {
			t.Fatalf("attempt %d refused under the limit", i+1)
		}
		clock.Advance(10 * time.Second)
	}
	// Attempts at 0, 10 and 20 s; now is 30 s.
	ok, wait := l.Allow(ctx, k, 3, time.Minute)
	if ok || wait != 30*time.Second {
		t.Fatalf("fourth attempt = %v, %v; want refused, 30s until the first leaves", ok, wait)
	}
	clock.Advance(30*time.Second - time.Millisecond)
	if ok, wait := l.Allow(ctx, k, 3, time.Minute); ok || wait != time.Millisecond {
		t.Fatalf("one millisecond early = %v, %v; want refused for 1ms", ok, wait)
	}
	clock.Advance(time.Millisecond)
	if ok, _ := l.Allow(ctx, k, 3, time.Minute); !ok {
		t.Fatal("refused once the first attempt left the window")
	}
	// Attempts now at 10, 20 and 60 s: the next opening is at 70 s.
	if ok, wait := l.Allow(ctx, k, 3, time.Minute); ok || wait != 10*time.Second {
		t.Fatalf("after the slide = %v, %v; want refused for 10s", ok, wait)
	}
}

// Hammering a used-up limit never extends it: refused attempts do not count.
func TestRateLimiter_RefusedAttemptsDoNotCount(t *testing.T) {
	t.Parallel()
	l, clock := newRateLimiter(t)
	ctx := t.Context()
	k := key("hammer")

	if ok, _ := l.Allow(ctx, k, 1, time.Minute); !ok {
		t.Fatal("first attempt refused")
	}
	for range 50 {
		clock.Advance(time.Second)
		if ok, _ := l.Allow(ctx, k, 1, time.Minute); ok {
			t.Fatal("allowed over the limit")
		}
	}
	clock.Advance(10 * time.Second) // 60 s after the only counted attempt
	if ok, _ := l.Allow(ctx, k, 1, time.Minute); !ok {
		t.Fatal("still refused after the window: refused attempts were counted")
	}
}

func TestRateLimiter_AttemptsInTheSameMillisecondBothCount(t *testing.T) {
	t.Parallel()
	l, _ := newRateLimiter(t)
	ctx := t.Context()
	k := key("same-ms")

	for i := range 2 {
		if ok, _ := l.Allow(ctx, k, 2, time.Minute); !ok {
			t.Fatalf("attempt %d refused", i+1)
		}
	}
	if ok, _ := l.Allow(ctx, k, 2, time.Minute); ok {
		t.Fatal("two attempts in one millisecond were counted once")
	}
}

func TestRateLimiter_KeysCountApart(t *testing.T) {
	t.Parallel()
	l, _ := newRateLimiter(t)
	ctx := t.Context()
	a, b := key("apart"), key("apart")

	if ok, _ := l.Allow(ctx, a, 1, time.Minute); !ok {
		t.Fatal("first key refused")
	}
	if ok, _ := l.Allow(ctx, b, 1, time.Minute); !ok {
		t.Fatal("a second key shared the first one's count")
	}
}

// The sorted set expires with its window, so idle keys do not pile up.
func TestRateLimiter_KeyExpiresWithItsWindow(t *testing.T) {
	t.Parallel()
	l, _ := newRateLimiter(t)
	ctx := t.Context()
	k := key("expiry")

	if ok, _ := l.Allow(ctx, k, 5, 2*time.Second); !ok {
		t.Fatal("refused")
	}
	ttl := redisTTL(t, k)
	if ttl <= 0 || ttl > 2*time.Second {
		t.Errorf("TTL = %v, want at most the 2s window", ttl)
	}
}

// redisTTL reads a key's remaining time straight from Redis.
func redisTTL(t *testing.T, key string) time.Duration {
	t.Helper()
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := goredis.NewClient(opts)
	t.Cleanup(func() { _ = rdb.Close() })
	ttl, err := rdb.PTTL(t.Context(), key).Result()
	if err != nil {
		t.Fatal(err)
	}
	return ttl
}
