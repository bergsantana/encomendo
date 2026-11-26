package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// TokenVerifier checks an access token for one audience and returns its
// claims (adapter/auth.AccessTokens). A refused token is an error matching
// shared.ErrUnauthenticated whose text names the reason and never any part
// of the token, because authn logs it; any other error is the server's
// fault.
type TokenVerifier interface {
	Verify(ctx context.Context, raw string, aud shared.Audience) (appidentity.AccessClaims, error)
}

// DenylistReader reports whether an access token was stopped before its
// expiry (adapter/redis.Denylist). It returns an error when it cannot tell.
type DenylistReader interface {
	Denied(ctx context.Context, jti shared.ID) (bool, error)
}

// errNoToken is a refused request: a missing or unusable Authorization
// header. The reasons stay out of the error so none reaches the client.
var errNoToken = fmt.Errorf("authn: no bearer token: %w", shared.ErrUnauthenticated)

// Authn authenticates requests by their bearer access token (ADRs 0010,
// 0013 and 0019).
type Authn struct {
	verifier TokenVerifier
	denylist DenylistReader
	log      *slog.Logger
}

// NewAuthn returns the authenticator that verifies tokens with verifier and
// checks them against denylist.
func NewAuthn(verifier TokenVerifier, denylist DenylistReader, log *slog.Logger) *Authn {
	return &Authn{verifier: verifier, denylist: denylist, log: log}
}

// Require returns the middleware that lets a request through only with one
// Authorization header holding a Bearer access token for aud that is not
// denylisted. It puts the token's principal, and the RLS scope that goes
// with it, in the request's context (authz.WithPrincipal).
//
// A missing, bad, expired, wrong-audience or denylisted token answers 401
// unauthenticated. A denylist that cannot be read answers 503 not_ready,
// never 401 and never a pass: a 401 would end every Android session during
// a Redis outage, and a pass would let revoked tokens back in (ADR 0013).
// The refusal's reason is logged at debug; the Authorization header never
// is (ADR 0019).
func (a *Authn) Require(aud shared.Audience) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := a.authenticate(r, aud)
			if err != nil {
				respond.Error(w, r, a.log, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(authz.WithPrincipal(r.Context(), p)))
		})
	}
}

// authenticate returns the principal of r's access token for aud.
func (a *Authn) authenticate(r *http.Request, aud shared.Audience) (shared.Principal, error) {
	ctx := r.Context()
	raw, reason := bearerToken(r.Header)
	if reason != "" {
		a.log.DebugContext(ctx, "access token refused", "reason", reason)
		return shared.Principal{}, errNoToken
	}
	claims, err := a.verifier.Verify(ctx, raw, aud)
	if errors.Is(err, shared.ErrUnauthenticated) {
		// The verifier's text names the reason, never the token (ADR 0019).
		a.log.DebugContext(ctx, "access token refused", "reason", err.Error())
		return shared.Principal{}, err
	}
	if err != nil {
		return shared.Principal{}, fmt.Errorf("authn: verify access token: %w", err)
	}
	if claims.Principal.Audience() != aud {
		// A verifier that answers for another audience is a bug; fail closed.
		return shared.Principal{}, fmt.Errorf("authn: verifier returned a %s principal for a %s route",
			claims.Principal.Audience(), aud)
	}
	// Check the error first: Denied answers (false, err) when Redis fails,
	// and reading the bool alone would let a revoked token through.
	denied, err := a.denylist.Denied(ctx, claims.JTI)
	if err != nil {
		return shared.Principal{}, fmt.Errorf("authn: %w: %w", shared.ErrUnavailable, err)
	}
	if denied {
		a.log.DebugContext(ctx, "access token refused", "reason", "denylisted")
		return shared.Principal{}, fmt.Errorf("authn: access token denylisted: %w", shared.ErrUnauthenticated)
	}
	return claims.Principal, nil
}

// bearerToken returns the token of the request's one Authorization header in
// the Bearer scheme (RFC 6750; the scheme name is case-insensitive), or the
// reason there is none, for the debug log.
func bearerToken(h http.Header) (token, reason string) {
	values := h.Values("Authorization")
	switch {
	case len(values) == 0:
		return "", "no Authorization header"
	case len(values) > 1:
		return "", "more than one Authorization header"
	}
	scheme, token, _ := strings.Cut(values[0], " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return "", "not the Bearer scheme"
	}
	if token == "" {
		return "", "an empty bearer token"
	}
	return token, ""
}
