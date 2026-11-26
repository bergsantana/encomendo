package http

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

const adminOrigin = "https://admin.encomendo.com.br"

// authPaths are the nine /auth operations of the contract.
var authPaths = []string{
	"/auth/signup", "/auth/login", "/auth/refresh", "/auth/switch-tenant", "/auth/logout",
	"/auth/password-reset", "/auth/password-reset/confirm", "/auth/invitations/preview", "/auth/invitations/accept",
}

// reached is a fake appidentity.UseCases that only records that a call got
// through, and the deadline it ran under. Every call fails with
// ErrUnauthenticated, which every route answers without more input.
type reached struct {
	mu       sync.Mutex
	calls    int
	deadline time.Time
}

func (u *reached) hit(ctx context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls++
	u.deadline, _ = ctx.Deadline()
	return shared.ErrUnauthenticated
}

func (u *reached) SignUp(ctx context.Context, _ appidentity.SignUpInput) (appidentity.AuthResult, error) {
	return appidentity.AuthResult{}, u.hit(ctx)
}

func (u *reached) Login(ctx context.Context, _ appidentity.LoginInput) (appidentity.AuthResult, error) {
	return appidentity.AuthResult{}, u.hit(ctx)
}

func (u *reached) Refresh(ctx context.Context, _ appidentity.RefreshInput) (appidentity.AuthResult, error) {
	return appidentity.AuthResult{}, u.hit(ctx)
}

func (u *reached) SwitchTenant(ctx context.Context, _ appidentity.SwitchTenantInput) (appidentity.AuthResult, error) {
	return appidentity.AuthResult{}, u.hit(ctx)
}

func (u *reached) Logout(ctx context.Context, _ appidentity.LogoutInput) error { return u.hit(ctx) }

func (u *reached) RequestPasswordReset(ctx context.Context, _ appidentity.PasswordResetInput) error {
	return u.hit(ctx)
}

func (u *reached) ConfirmPasswordReset(ctx context.Context, _ appidentity.ConfirmPasswordResetInput) error {
	return u.hit(ctx)
}

func (u *reached) PreviewInvitation(ctx context.Context, _ string) (appidentity.InvitationPreview, error) {
	return appidentity.InvitationPreview{}, u.hit(ctx)
}

func (u *reached) AcceptInvitation(ctx context.Context, _ appidentity.AcceptInvitationInput) (appidentity.AuthResult, error) {
	return appidentity.AuthResult{}, u.hit(ctx)
}

// countingLimiter refuses every key whose rule is in refuse, and counts
// every call.
type countingLimiter struct {
	mu     sync.Mutex
	refuse map[string]bool
	keys   []string
}

func (l *countingLimiter) Allow(_ context.Context, key string, _ int, _ time.Duration) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.keys = append(l.keys, key)
	for rule := range l.refuse {
		if strings.HasPrefix(key, "g:rl:"+rule+":ip:") {
			return false, time.Minute
		}
	}
	return true, 0
}

func authRouter(t *testing.T, uc appidentity.UseCases, l *countingLimiter) http.Handler {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	guard, err := middleware.CrossOrigin(adminOrigin, log)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := handler.NewRefreshCookie(handler.HostRefreshCookie)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ratelimit.NewPseudonyms([]byte(strings.Repeat("k", ratelimit.MinKeyLength)))
	if err != nil {
		t.Fatal(err)
	}
	a, err := handler.NewAuth(handler.AuthDeps{UseCases: uc, Verifier: noVerifier{}, Limiter: l, Pseudonyms: p, Cookie: cookie, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(log, Handlers{Health: handler.NewHealth(log, time.Second), Auth: a}, RouterOptions{
		AuthGuard:  guard,
		RateLimits: middleware.NewRateLimits(l, ratelimit.NewClientIP(config.DefaultTrustedProxies()), p, log),
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

// noVerifier refuses every access token.
type noVerifier struct{}

func (noVerifier) Verify(context.Context, string, shared.Audience) (appidentity.AccessClaims, error) {
	return appidentity.AccessClaims{}, shared.ErrUnauthenticated
}

func TestNewRouter_RefusesAuthWithoutItsGuards(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	cookie, _ := handler.NewRefreshCookie(handler.HostRefreshCookie)
	p, _ := ratelimit.NewPseudonyms([]byte(strings.Repeat("k", ratelimit.MinKeyLength)))
	a, err := handler.NewAuth(handler.AuthDeps{
		UseCases: &reached{}, Verifier: noVerifier{}, Limiter: &countingLimiter{}, Pseudonyms: p, Cookie: cookie, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := middleware.CrossOrigin(adminOrigin, log)
	if err != nil {
		t.Fatal(err)
	}
	limits := middleware.NewRateLimits(&countingLimiter{}, ratelimit.NewClientIP(nil), p, log)
	h := Handlers{Health: handler.NewHealth(log, time.Second), Auth: a}
	for name, opts := range map[string]RouterOptions{
		"no cross-origin check": {RateLimits: limits},
		"no rate limits":        {AuthGuard: guard},
	} {
		if _, err := NewRouter(log, h, opts); err == nil {
			t.Errorf("the /auth routes were mounted with %s", name)
		}
	}
	if _, err := NewRouter(log, h, RouterOptions{AuthGuard: guard, RateLimits: limits}); err != nil {
		t.Errorf("NewRouter refused complete options: %v", err)
	}
}

func authRequest(path string, header map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		r.Header.Set(k, v)
	}
	return r
}

// Every /auth route refuses a cross-site browser call before anything else
// runs: no use case, and no rate-limit count, so a cross-site page cannot
// spend its visitors' budgets (ADRs 0010 and 0021).
func TestRouter_AuthRoutesRefuseCrossSiteCallsFirst(t *testing.T) {
	t.Parallel()

	for _, path := range authPaths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			uc, l := &reached{}, &countingLimiter{}
			rec := httptest.NewRecorder()
			authRouter(t, uc, l).ServeHTTP(rec, authRequest(path, map[string]string{"Origin": "https://evil.example"}))

			if rec.Code != http.StatusForbidden || rec.Header().Get("Content-Type") != "application/problem+json" {
				t.Errorf("answer %d %s, want 403 problem+json", rec.Code, rec.Header().Get("Content-Type"))
			}
			if uc.calls != 0 || len(l.keys) != 0 {
				t.Errorf("the use case ran %d times and the limiter counted %v behind a refused origin", uc.calls, l.keys)
			}
		})
	}
}

// A native client, with neither Origin nor Sec-Fetch-Site, reaches every
// /auth handler, under the /auth deadline (ADRs 0013 and 0019).
func TestRouter_AuthRoutesServeNativeClientsUnderTheDeadline(t *testing.T) {
	t.Parallel()

	for _, path := range authPaths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			uc, l := &reached{}, &countingLimiter{}
			before := time.Now()
			rec := httptest.NewRecorder()
			r := authRequest(path, nil)
			// Logout with no token at all has nothing to revoke.
			r.AddCookie(&http.Cookie{Name: handler.HostRefreshCookie, Value: strings.Repeat("A", 43)})
			authRouter(t, uc, l).ServeHTTP(rec, r)

			if uc.calls != 1 {
				t.Fatalf("answer %d %s; the use case was not reached", rec.Code, rec.Body.String())
			}
			if uc.deadline.Before(before.Add(middleware.AuthDeadline)) || uc.deadline.After(time.Now().Add(middleware.AuthDeadline)) {
				t.Errorf("the use case ran under deadline %v, want the /auth deadline", uc.deadline)
			}
		})
	}
}

// Each limited route counts its own per-IP rule, and the two without one
// never count (ADR 0021).
func TestRouter_AuthRoutesCountTheirPerIPRule(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"/auth/signup": "signup", "/auth/login": "login", "/auth/refresh": "refresh",
		"/auth/password-reset": "password-reset", "/auth/password-reset/confirm": "password-reset-confirm",
		"/auth/invitations/preview": "invitation-preview", "/auth/invitations/accept": "invitation-accept",
		"/auth/switch-tenant": "", "/auth/logout": "",
	}
	for path, rule := range want {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			l := &countingLimiter{refuse: map[string]bool{rule: true}}
			rec := httptest.NewRecorder()
			authRouter(t, &reached{}, l).ServeHTTP(rec, authRequest(path, nil))

			var ipKeys []string
			for _, k := range l.keys {
				if strings.Contains(k, ":ip:") {
					ipKeys = append(ipKeys, k)
				}
			}
			if rule == "" {
				if len(ipKeys) != 0 || rec.Code == http.StatusTooManyRequests {
					t.Errorf("counted %v and answered %d on a route without a per-IP limit", ipKeys, rec.Code)
				}
				return
			}
			if len(ipKeys) != 1 || !strings.HasPrefix(ipKeys[0], "g:rl:"+rule+":ip:") || rec.Code != http.StatusTooManyRequests {
				t.Errorf("counted %v and answered %d, want one %s per-IP count refused with 429", ipKeys, rec.Code, rule)
			}
			if rec.Header().Get(respond.HeaderRetryAfter) != "60" {
				t.Errorf("Retry-After = %q, want 60", rec.Header().Get(respond.HeaderRetryAfter))
			}
		})
	}
}

// The per-IP limit runs before the body is read: a limited client gets its
// 429 even for a body that would not decode.
func TestRouter_AuthLimitRunsBeforeTheBody(t *testing.T) {
	t.Parallel()

	l := &countingLimiter{refuse: map[string]bool{"login": true}}
	r := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{not json`))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	authRouter(t, &reached{}, l).ServeHTTP(rec, r)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("answer %d, want 429 before the body is decoded", rec.Code)
	}
}

func TestRouter_AuthFallbacksStayProblems(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method, path string
		wantStatus   int
	}{
		{http.MethodGet, "/auth/login", http.StatusMethodNotAllowed},
		{http.MethodPost, "/auth/nowhere", http.StatusNotFound},
		{http.MethodPost, "/auth", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			authRouter(t, &reached{}, &countingLimiter{}).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus || rec.Header().Get("Content-Type") != "application/problem+json" {
				t.Errorf("answer %d %s, want %d problem+json", rec.Code, rec.Header().Get("Content-Type"), tt.wantStatus)
			}
		})
	}
}
