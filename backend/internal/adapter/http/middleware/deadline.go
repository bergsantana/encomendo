package middleware

import (
	"context"
	"net/http"
	"time"
)

// AuthDeadline bounds every /auth/* request (ADR 0019). It must stay longer
// than auth.DefaultMaxHashWait (3 seconds) plus one hash at the default
// costs (well under a second), so that a busy hasher answers ErrBusy, a 503
// with Retry-After, before the deadline cuts in. It must stay shorter than
// the Android app's 30-second request timeout (ADR 0012), so the app gets
// that answer instead of timing out on its own.
const AuthDeadline = 10 * time.Second

// Deadline returns the middleware that ends each request's context after d.
// Work that respects the context then stops with context.DeadlineExceeded,
// which the problem mapping answers as 503 not_ready. It does not write a
// response itself: the handler still answers, with the error it got.
func Deadline(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
