package redis

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

// A limit that cannot be checked lets the attempt through and logs it,
// without the key's value, which is a client's IP or an email's HMAC
// (ADR 0021). The window itself runs against a real Redis in
// test/integration/redis.
func TestRateLimiter_RedisDownAllowsAndLogs(t *testing.T) {
	t.Parallel()

	c, err := Open(config.Redis{URL: "redis://127.0.0.1:1/0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	var logs bytes.Buffer
	l := NewRateLimiter(c, sharedfake.NewClock(time.Now()), slog.New(slog.NewJSONHandler(&logs, nil)))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok, wait := l.Allow(ctx, "g:rl:login:ip:203.0.113.7", 1, time.Minute)

	if !ok || wait != 0 {
		t.Errorf("Allow = %v, %v; want allowed", ok, wait)
	}
	if !strings.Contains(logs.String(), `"level":"ERROR"`) || !strings.Contains(logs.String(), `"rule":"login:ip"`) {
		t.Errorf("the failure is not logged at error with its rule: %s", logs.String())
	}
	if strings.Contains(logs.String(), "203.0.113.7") {
		t.Errorf("the client's address reached the log: %s", logs.String())
	}
}

func TestRateLimiter_EndedRequestIsNotLogged(t *testing.T) {
	t.Parallel()

	c, err := Open(config.Redis{URL: "redis://127.0.0.1:1/0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	var logs bytes.Buffer
	l := NewRateLimiter(c, sharedfake.NewClock(time.Now()), slog.New(slog.NewJSONHandler(&logs, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if ok, _ := l.Allow(ctx, "g:rl:login:ip:203.0.113.7", 1, time.Minute); !ok || logs.Len() != 0 {
		t.Errorf("ok = %v, logs = %s; want allowed and silent", ok, logs.String())
	}
}

func TestRuleOf(t *testing.T) {
	t.Parallel()

	for key, want := range map[string]string{
		"g:rl:invite:store-email:a:b": "invite:store-email",
		"g:rl:login:ip:2001:db8::/64": "login:ip",
		"bad":                         "unknown",
	} {
		if got := ruleOf(key); got != want {
			t.Errorf("ruleOf(%q) = %q, want %q", key, got, want)
		}
	}
}

// A Redis that accepts connections and never answers must not hold the
// request: the count gives up after limitTimeout and the attempt passes,
// logged (ADR 0021).
func TestRateLimiter_HungRedisAllowsQuickly(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() }) // held open, never answered
		}
	}()
	c, err := Open(config.Redis{URL: "redis://" + ln.Addr().String() + "/0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	var logs bytes.Buffer
	l := NewRateLimiter(c, sharedfake.NewClock(time.Now()), slog.New(slog.NewJSONHandler(&logs, nil)))

	start := time.Now()
	ok, _ := l.Allow(t.Context(), "g:rl:login:ip:x", 1, time.Minute)
	took := time.Since(start)

	if !ok {
		t.Error("a hung Redis refused the attempt")
	}
	if took > 2*time.Second {
		t.Errorf("Allow took %v, want about %v", took, limitTimeout)
	}
	if !strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Errorf("the hang was not logged: %s", logs.String())
	}
}
