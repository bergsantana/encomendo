package middleware_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The authn tests run the real token verifier (adapter/auth), so a forged
// token here is refused for the reason it would be in production. Only the
// denylist is a fake.

const issuer = "https://api.encomendo.com.br"

// tokens issues and verifies access tokens on a clock the test moves.
type tokens struct {
	clock *sharedfake.Clock
	ids   *sharedfake.IDs
	keys  auth.KeySet
	at    *auth.AccessTokens
}

func newTokens(t *testing.T) *tokens {
	t.Helper()
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	key, err := auth.ParseKey(base64.StdEncoding.EncodeToString(seed))
	if err != nil {
		t.Fatal(err)
	}
	keys, err := auth.NewKeySet(key)
	if err != nil {
		t.Fatal(err)
	}
	clock := sharedfake.NewClock(time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC))
	at, err := auth.NewAccessTokens(keys, issuer, clock)
	if err != nil {
		t.Fatal(err)
	}
	return &tokens{clock: clock, ids: sharedfake.NewIDs(clock), keys: keys, at: at}
}

// issue signs a token for p, issued now, and returns it with its jti.
func (k *tokens) issue(t *testing.T, p shared.Principal) (string, shared.ID) {
	t.Helper()
	now := k.clock.Now()
	c := appidentity.AccessClaims{Principal: p, JTI: k.ids.NewID(), IssuedAt: now, ExpiresAt: now.Add(identity.AccessTokenLifetime)}
	raw, err := k.at.Issue(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	return raw, c.JTI
}

func (k *tokens) staff(t *testing.T) shared.Principal {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(shared.UserID(k.ids.NewID()), shared.TenantID(k.ids.NewID()), shared.RoleStaff)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// storeless is a staff user right after sign-up: no store, no role.
func (k *tokens) storeless(t *testing.T) shared.Principal {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(shared.UserID(k.ids.NewID()), shared.TenantID{}, "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (k *tokens) client(t *testing.T) shared.Principal {
	t.Helper()
	p, err := shared.NewClientPrincipal(shared.ClientAccountID(k.ids.NewID()))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// denylist is a fake adapter/redis.Denylist: it answers from denied, or
// fails with err the way the real one does, as (false, err).
type denylist struct {
	denied map[shared.ID]bool
	err    error
}

func (d *denylist) Denied(ctx context.Context, jti shared.ID) (bool, error) {
	if d.err != nil {
		return false, d.err
	}
	return d.denied[jti], ctx.Err()
}

// result is what a request through authn produced.
type result struct {
	rec     *httptest.ResponseRecorder
	reached bool
	caller  shared.Principal
	scope   appshared.Scope
	logs    string
}

// serve sends r through authn for aud and records what the handler saw.
func serve(v middleware.TokenVerifier, d middleware.DenylistReader, aud shared.Audience, r *http.Request) result {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var out result
	h := middleware.NewAuthn(v, d, log).Require(aud)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		out.reached = true
		out.caller, _ = authz.PrincipalFrom(r.Context())
		out.scope = appshared.ScopeFrom(r.Context())
	}))
	out.rec = httptest.NewRecorder()
	h.ServeHTTP(out.rec, r)
	out.logs = logs.String()
	return out
}

func request(authorization ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/members", nil)
	for _, v := range authorization {
		r.Header.Add("Authorization", v)
	}
	return r
}

func problem(t *testing.T, rec *httptest.ResponseRecorder) respond.Problem {
	t.Helper()
	var p respond.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not a problem: %q", rec.Body.String())
	}
	return p
}

// segment encodes v as a JWT segment.
func segment(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// payload decodes the claims of a signed token.
func payload(t *testing.T, raw string) map[string]any {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(strings.Split(raw, ".")[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(b, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

// noTokenIn fails when any segment of the tokens in values, the whole token
// included, reached the logs (ADR 0019). Segments under 8 bytes are too
// short to tell apart from other text.
func noTokenIn(t *testing.T, logs string, values ...string) {
	t.Helper()
	for _, v := range values {
		_, token, _ := strings.Cut(v, " ")
		for _, part := range append(strings.Split(token, "."), token) {
			if len(part) >= 8 && strings.Contains(logs, part) {
				t.Errorf("part of a token reached the logs: %q in %s", part, logs)
			}
		}
	}
}

// refusalLogged fails unless the refusal's reason is logged at debug, and
// only at debug: a refused token is a client's mistake, not an alarm.
func refusalLogged(t *testing.T, logs string) {
	t.Helper()
	found := false
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not JSON: %s", line)
		}
		if reason, _ := entry["reason"].(string); entry["msg"] == "access token refused" && reason != "" {
			found = entry["level"] == "DEBUG"
		}
	}
	if !found {
		t.Errorf("the refusal's reason is not logged at debug: %s", logs)
	}
}

func TestAuthn_Require_RefusesEveryBadToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		aud  shared.Audience
		// header returns the Authorization values to send, and the denylist.
		header func(t *testing.T, k *tokens) ([]string, *denylist)
	}{
		{"no Authorization header", shared.AudienceBackoffice, func(*testing.T, *tokens) ([]string, *denylist) {
			return nil, &denylist{}
		}},
		{"two Authorization headers", shared.AudienceBackoffice, func(t *testing.T, k *tokens) ([]string, *denylist) {
			a, _ := k.issue(t, k.staff(t))
			b, _ := k.issue(t, k.staff(t))
			return []string{"Bearer " + a, "Bearer " + b}, &denylist{}
		}},
		{"Basic scheme", shared.AudienceBackoffice, func(t *testing.T, k *tokens) ([]string, *denylist) {
			raw, _ := k.issue(t, k.staff(t))
			return []string{"Basic " + raw}, &denylist{}
		}},
		{"Bearer without a token", shared.AudienceBackoffice, func(*testing.T, *tokens) ([]string, *denylist) {
			return []string{"Bearer "}, &denylist{}
		}},
		{"not a token", shared.AudienceBackoffice, func(*testing.T, *tokens) ([]string, *denylist) {
			return []string{"Bearer abc"}, &denylist{}
		}},
		{"expired", shared.AudienceBackoffice, func(t *testing.T, k *tokens) ([]string, *denylist) {
			raw, _ := k.issue(t, k.staff(t))
			k.clock.Advance(identity.AccessTokenLifetime + identity.AccessTokenLeeway + time.Second)
			return []string{"Bearer " + raw}, &denylist{}
		}},
		{"client token on a staff route", shared.AudienceBackoffice, func(t *testing.T, k *tokens) ([]string, *denylist) {
			raw, _ := k.issue(t, k.client(t))
			return []string{"Bearer " + raw}, &denylist{}
		}},
		{"staff token on a client route", shared.AudienceClient, func(t *testing.T, k *tokens) ([]string, *denylist) {
			raw, _ := k.issue(t, k.staff(t))
			return []string{"Bearer " + raw}, &denylist{}
		}},
		{"alg none", shared.AudienceBackoffice, func(t *testing.T, k *tokens) ([]string, *denylist) {
			raw, _ := k.issue(t, k.staff(t))
			header := segment(t, map[string]string{"alg": "none", "typ": "at+jwt", "kid": k.keys.KID()})
			return []string{"Bearer " + header + "." + strings.Split(raw, ".")[1] + "."}, &denylist{}
		}},
		{"unknown kid", shared.AudienceBackoffice, func(t *testing.T, k *tokens) ([]string, *denylist) {
			other := newTokens(t)
			raw, _ := other.issue(t, other.staff(t))
			return []string{"Bearer " + raw}, &denylist{}
		}},
		{"tampered claims", shared.AudienceBackoffice, func(t *testing.T, k *tokens) ([]string, *denylist) {
			raw, _ := k.issue(t, k.staff(t))
			claims := payload(t, raw)
			claims["role"] = string(shared.RoleOwner)
			parts := strings.Split(raw, ".")
			return []string{"Bearer " + parts[0] + "." + segment(t, claims) + "." + parts[2]}, &denylist{}
		}},
		{"denylisted", shared.AudienceBackoffice, func(t *testing.T, k *tokens) ([]string, *denylist) {
			raw, jti := k.issue(t, k.staff(t))
			return []string{"Bearer " + raw}, &denylist{denied: map[shared.ID]bool{jti: true}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			k := newTokens(t)
			values, d := tt.header(t, k)
			got := serve(k.at, d, tt.aud, request(values...))

			if got.reached {
				t.Fatal("the handler ran for a refused token")
			}
			if got.rec.Code != http.StatusUnauthorized || problem(t, got.rec).Code != respond.CodeUnauthenticated {
				t.Errorf("answer = %d %s, want 401 unauthenticated", got.rec.Code, got.rec.Body.String())
			}
			if got.rec.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Errorf("WWW-Authenticate = %q", got.rec.Header().Get("WWW-Authenticate"))
			}
			refusalLogged(t, got.logs)
			noTokenIn(t, got.logs, values...)
		})
	}
}

func TestAuthn_Require_PassesTheCallerAndItsScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		aud       shared.Audience
		scheme    string
		principal func(k *tokens, t *testing.T) shared.Principal
		scope     func(p shared.Principal) appshared.Scope
	}{
		{"staff in a store", shared.AudienceBackoffice, "Bearer", (*tokens).staff, func(p shared.Principal) appshared.Scope {
			return appshared.Scope{TenantID: p.TenantID(), UserID: p.UserID()}
		}},
		{"staff without a store", shared.AudienceBackoffice, "Bearer", (*tokens).storeless, func(p shared.Principal) appshared.Scope {
			return appshared.Scope{UserID: p.UserID()}
		}},
		{"client account", shared.AudienceClient, "Bearer", (*tokens).client, func(p shared.Principal) appshared.Scope {
			return appshared.Scope{ClientAccountID: p.ClientAccountID()}
		}},
		{"scheme in lower case", shared.AudienceBackoffice, "bearer", (*tokens).staff, func(p shared.Principal) appshared.Scope {
			return appshared.Scope{TenantID: p.TenantID(), UserID: p.UserID()}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			k := newTokens(t)
			p := tt.principal(k, t)
			raw, _ := k.issue(t, p)
			got := serve(k.at, &denylist{}, tt.aud, request(tt.scheme+" "+raw))

			if !got.reached {
				t.Fatalf("refused: %d %s", got.rec.Code, got.rec.Body.String())
			}
			if got.caller != p {
				t.Errorf("caller = %+v, want %+v", got.caller, p)
			}
			if want := tt.scope(p); got.scope != want {
				t.Errorf("scope = %+v, want %+v", got.scope, want)
			}
		})
	}
}

// A denylist that cannot be read fails closed: 503, never a pass, and never
// a 401, which would end every Android session during an outage (ADR 0013).
func TestAuthn_Require_UnreadableDenylistIs503(t *testing.T) {
	t.Parallel()

	k := newTokens(t)
	raw, _ := k.issue(t, k.staff(t))
	got := serve(k.at, &denylist{err: errors.New("dial tcp redis:6379: connection refused")}, shared.AudienceBackoffice,
		request("Bearer "+raw))

	if got.reached {
		t.Fatal("the handler ran although the denylist could not be read")
	}
	if got.rec.Code != http.StatusServiceUnavailable || problem(t, got.rec).Code != respond.CodeNotReady {
		t.Errorf("answer = %d %s, want 503 not_ready", got.rec.Code, got.rec.Body.String())
	}
	if got.rec.Header().Get(respond.HeaderRetryAfter) == "" {
		t.Error("no Retry-After on the 503")
	}
	noTokenIn(t, got.logs, "Bearer "+raw)
}

func TestAuthn_Require_ClientGoneWhileCheckingIsNotAFailure(t *testing.T) {
	t.Parallel()

	k := newTokens(t)
	raw, _ := k.issue(t, k.staff(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := serve(k.at, &denylist{}, shared.AudienceBackoffice, request("Bearer "+raw).WithContext(ctx))

	if got.reached || got.rec.Code != respond.StatusClientClosedRequest {
		t.Errorf("reached = %v, status = %d, want %d", got.reached, got.rec.Code, respond.StatusClientClosedRequest)
	}
}

// An audience other than the two is a wiring bug: a 500, never a 401 that
// would send every client into a refresh loop (ADR 0019).
func TestAuthn_Require_UnknownAudienceIs500(t *testing.T) {
	t.Parallel()

	k := newTokens(t)
	raw, _ := k.issue(t, k.staff(t))
	got := serve(k.at, &denylist{}, shared.Audience("admin"), request("Bearer "+raw))

	if got.reached || got.rec.Code != http.StatusInternalServerError {
		t.Errorf("reached = %v, status = %d, want a 500", got.reached, got.rec.Code)
	}
	noTokenIn(t, got.logs, "Bearer "+raw)
}

// verifierFunc adapts a function to middleware.TokenVerifier.
type verifierFunc func(ctx context.Context, raw string, aud shared.Audience) (appidentity.AccessClaims, error)

func (f verifierFunc) Verify(ctx context.Context, raw string, aud shared.Audience) (appidentity.AccessClaims, error) {
	return f(ctx, raw, aud)
}

func TestAuthn_Require_RefusesAPrincipalOfAnotherAudience(t *testing.T) {
	t.Parallel()

	k := newTokens(t)
	client := k.client(t)
	lying := verifierFunc(func(context.Context, string, shared.Audience) (appidentity.AccessClaims, error) {
		return appidentity.AccessClaims{Principal: client, JTI: k.ids.NewID()}, nil
	})
	got := serve(lying, &denylist{}, shared.AudienceBackoffice, request("Bearer token"))

	if got.reached || got.rec.Code != http.StatusInternalServerError {
		t.Errorf("reached = %v, status = %d, want a 500", got.reached, got.rec.Code)
	}
}
