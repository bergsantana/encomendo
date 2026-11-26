package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
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
	ip := ratelimit.NewClientIP(ratelimit.DefaultTrustedProxies())
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
