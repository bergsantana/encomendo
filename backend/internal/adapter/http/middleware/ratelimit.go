package middleware

import (
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
)

// RateLimits applies per-IP rules (ADR 0021).
type RateLimits struct {
	limiter    ratelimit.Limiter
	ip         ratelimit.ClientIP
	pseudonyms ratelimit.Pseudonyms
	log        *slog.Logger
}

// NewRateLimits returns the per-IP limits that count with limiter, find the
// client with ip, and key it by its pseudonym.
func NewRateLimits(limiter ratelimit.Limiter, ip ratelimit.ClientIP, pseudonyms ratelimit.Pseudonyms, log *slog.Logger) *RateLimits {
	return &RateLimits{limiter: limiter, ip: ip, pseudonyms: pseudonyms, log: log}
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
			value := l.ip.Value(r)
			if value == "" {
				l.log.ErrorContext(r.Context(), "rate limit skipped: no client address", "rule", rule.Name)
				next.ServeHTTP(w, r)
				return
			}
			key := rule.Key(l.pseudonyms.IP(value))
			if ok, wait := l.limiter.Allow(r.Context(), key, rule.Limit, rule.Window); !ok {
				respond.RateLimited(w, r, wait)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
