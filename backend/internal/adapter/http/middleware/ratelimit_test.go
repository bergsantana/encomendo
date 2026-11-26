package middleware_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

// limiter is a fake ratelimit.Limiter that allows the first n attempts per
// key and records what it was asked.
type limiter struct {
	n    int
	wait time.Duration
	seen map[string]int
}

func (l *limiter) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, time.Duration) {
	if limit < 1 || window <= 0 {
		panic("not a limit")
	}
	l.seen[key]++
	if l.seen[key] > l.n {
		return false, l.wait
	}
	return true, 0
}

func pseudonyms(t *testing.T) ratelimit.Pseudonyms {
	t.Helper()
	p, err := ratelimit.NewPseudonyms([]byte(strings.Repeat("k", ratelimit.MinKeyLength)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRateLimits_ByIP(t *testing.T) {
	t.Parallel()

	l := &limiter{n: 2, wait: 1500 * time.Millisecond, seen: map[string]int{}}
	ip := ratelimit.NewClientIP(config.DefaultTrustedProxies())
	p := pseudonyms(t)
	reached := 0
	h := middleware.NewRateLimits(l, ip, p, discard()).ByIP(ratelimit.LogInByIP)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached++
	}))
	send := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		r.RemoteAddr = "172.18.0.6:40000"
		r.Header.Set("X-Forwarded-For", "198.51.100.1, 203.0.113.7, 172.18.0.5")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}

	send()
	send()
	rec := send()

	if reached != 2 {
		t.Errorf("the handler ran %d times, want 2", reached)
	}
	if l.seen["g:rl:login:ip:"+p.IP("203.0.113.7")] != 3 {
		t.Errorf("counted keys = %v, want the pseudonym of the client's address behind the proxies", l.seen)
	}
	for k := range l.seen {
		if strings.Contains(k, "203.0.113.7") {
			t.Errorf("the client's address sits in the key %q in clear", k)
		}
	}
	if rec.Code != http.StatusTooManyRequests || problem(t, rec).Code != respond.CodeRateLimited {
		t.Errorf("third answer = %d %s, want 429 rate_limited", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(respond.HeaderRetryAfter); got != "2" {
		t.Errorf("Retry-After = %q, want the wait rounded up to 2", got)
	}
}

func TestRateLimits_ByIP_UnreadablePeerPasses(t *testing.T) {
	t.Parallel()

	l := &limiter{n: 0, seen: map[string]int{}}
	reached := false
	h := middleware.NewRateLimits(l, ratelimit.NewClientIP(nil), pseudonyms(t), discard()).ByIP(ratelimit.LogInByIP)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.RemoteAddr = ""
	h.ServeHTTP(httptest.NewRecorder(), r)

	if !reached || len(l.seen) != 0 {
		t.Errorf("reached = %v, counted %v; want a pass without a count", reached, l.seen)
	}
}

// In production a client that resolves to a private address is logged,
// once: with the exact proxy list of production, Docker's bridge gateway
// handing Caddy every IPv6 client looks like that (ADR 0021).
func TestRateLimits_WarnOnPrivateClients(t *testing.T) {
	t.Parallel()

	prod := []netip.Prefix{netip.MustParsePrefix("172.30.0.10/32"), netip.MustParsePrefix("172.30.0.11/32")}
	send := func(limits *middleware.RateLimits, xff string) {
		h := limits.ByIP(ratelimit.LogInByIP)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		r.RemoteAddr = "172.30.0.10:40000" // Caddy
		r.Header.Set("X-Forwarded-For", xff)
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
	newLimits := func(logs *bytes.Buffer) *middleware.RateLimits {
		return middleware.NewRateLimits(&limiter{n: 100, seen: map[string]int{}}, ratelimit.NewClientIP(prod), pseudonyms(t),
			slog.New(slog.NewTextHandler(logs, nil)))
	}

	var logs bytes.Buffer
	warned := newLimits(&logs).WarnOnPrivateClients()
	send(warned, "203.0.113.7")
	if logs.Len() != 0 {
		t.Errorf("a public client was warned about: %s", logs.String())
	}
	for range 3 {
		send(warned, "172.30.0.1") // the bridge gateway, as Caddy saw it
	}
	if n := strings.Count(logs.String(), "private address"); n != 1 {
		t.Errorf("warned %d times, want once: %s", n, logs.String())
	}

	var quiet bytes.Buffer
	send(newLimits(&quiet), "172.30.0.1")
	if quiet.Len() != 0 {
		t.Errorf("warned with the warning off: %s", quiet.String())
	}
}
