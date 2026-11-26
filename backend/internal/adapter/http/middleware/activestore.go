package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
)

// StoreChecker refuses a request whose selected store may not be used
// (apptenancy.Service.CheckStore): tenancy.ErrTenantSuspended for a
// suspended one, any other error when the status cannot be read.
type StoreChecker interface {
	CheckStore(ctx context.Context) error
}

// ActiveStore returns the middleware that refuses every request whose
// selected store is suspended, with 403 tenant_suspended, at once: the
// status is read on every request, uncached (ADR 0022). It runs after authn,
// which puts the caller in the context. A status that cannot be read fails
// the request, never passes it.
func ActiveStore(checker StoreChecker, log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := checker.CheckStore(r.Context()); err != nil {
				respond.Error(w, r, log, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
