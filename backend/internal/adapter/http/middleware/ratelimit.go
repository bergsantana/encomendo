package middleware

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
)

// RateLimits applies per-IP rules (ADR 0021).
type RateLimits struct {
	limiter    ratelimit.Limiter
	ip         ratelimit.ClientIP
	pseudonyms ratelimit.Pseudonyms
	log        *slog.Logger
	// warnPrivate logs once when a client resolves to a private address.
	warnPrivate bool
	warnOnce    sync.Once
}

// NewRateLimits returns the per-IP limits that count with limiter, find the
// client with ip, and key it by its pseudonym.
func NewRateLimits(limiter ratelimit.Limiter, ip ratelimit.ClientIP, pseudonyms ratelimit.Pseudonyms, log *slog.Logger) *RateLimits {
	return &RateLimits{limiter: limiter, ip: ip, pseudonyms: pseudonyms, log: log}
}

// WarnOnPrivateClients makes the limits log a warning, once, when a client
// resolves to a private, loopback or link-local address. A real client in
// production never has one, so it means something on the way collapses
// clients onto one address, such as Docker's bridge gateway handing Caddy
// every IPv6 client, and one key would limit all of them (ADR 0021).
// Development stacks reach the API from such addresses, so they leave it
// off.
func (l *RateLimits) WarnOnPrivateClients() *RateLimits {
	l.warnPrivate = true
	return l
}

// ByIP returns the middleware that counts each request under rule, keyed
// on the client's address behind the proxies, and answers 429 rate_limited
// with Retry-After once the rule is used up (ADRs 0013 and 0021). It runs
// before the body is read. On /auth routes it must run after the
// cross-origin check, so a cross-site page cannot spend its visitors'
// budgets. A request whose peer address cannot be read is let through and
// logged: net/http always sets it, so only a misused handler gets there.
func (l *RateLimits) ByIP(rule ratelimit.Rule) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			addr, ok := l.ip.Of(r)
			if !ok {
				l.log.ErrorContext(r.Context(), "rate limit skipped: no client address", "rule", rule.Name)
				next.ServeHTTP(w, r)
				return
			}
			if l.warnPrivate && (addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast()) {
				l.warnOnce.Do(func() {
					l.log.WarnContext(r.Context(), "a client resolved to a private address: clients may share one rate-limit key; check TRUSTED_PROXIES and the proxies' X-Forwarded-For")
				})
			}
			key := rule.Key(l.pseudonyms.IP(ratelimit.ValueOf(addr)))
			if ok, wait := l.limiter.Allow(r.Context(), key, rule.Limit, rule.Window); !ok {
				respond.RateLimited(w, r, wait)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
