package handler_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

var (
	userID   = shared.UserID(mustID("01936d1e-8b5a-7c2e-9f3a-000000000001"))
	tenantID = shared.TenantID(mustID("01936d1e-8b5a-7c2e-9f3a-000000000002"))
	jtiID    = mustID("01936d1e-8b5a-7c2e-9f3a-000000000003")
	expires  = time.Date(2026, 12, 26, 12, 0, 0, 0, time.UTC)
)

func mustID(s string) shared.ID {
	id, err := shared.ParseID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// session is a staff session in a store, as the use cases return it.
func session() appidentity.AuthResult {
	store := appidentity.TenantSummary{ID: tenantID, Name: "Alta RPM", Slug: "altarpm"}
	return appidentity.AuthResult{
		Session: appidentity.SessionView{
			User:        appidentity.UserView{ID: userID, Name: "Ana", Email: "ana@example.com"},
			Tenant:      store,
			Role:        shared.RoleOwner,
			Permissions: identity.PermissionsOf(shared.RoleOwner),
			Memberships: []appidentity.Membership{{Tenant: store, Role: shared.RoleOwner}},
		},
		AccessToken: "access.token.value", AccessExpiresAt: expires,
		RefreshToken: "the-new-refresh-token", RefreshExpiresAt: expires,
	}
}

// useCases is a fake appidentity.UseCases: every call records its input
// and answers res, preview and err.
type useCases struct {
	res     appidentity.AuthResult
	preview appidentity.InvitationPreview
	err     error
	got     any
	logouts []appidentity.LogoutInput
	// logoutErrs, when set, answers the logout calls in turn.
	logoutErrs []error
}

func (u *useCases) SignUp(_ context.Context, in appidentity.SignUpInput) (appidentity.AuthResult, error) {
	u.got = in
	return u.res, u.err
}

func (u *useCases) Login(_ context.Context, in appidentity.LoginInput) (appidentity.AuthResult, error) {
	u.got = in
	return u.res, u.err
}

func (u *useCases) Refresh(_ context.Context, in appidentity.RefreshInput) (appidentity.AuthResult, error) {
	u.got = in
	return u.res, u.err
}

func (u *useCases) SwitchTenant(_ context.Context, in appidentity.SwitchTenantInput) (appidentity.AuthResult, error) {
	u.got = in
	return u.res, u.err
}

func (u *useCases) Logout(_ context.Context, in appidentity.LogoutInput) error {
	u.got = in
	u.logouts = append(u.logouts, in)
	if len(u.logoutErrs) > 0 {
		err := u.logoutErrs[0]
		u.logoutErrs = u.logoutErrs[1:]
		return err
	}
	return u.err
}

func (u *useCases) RequestPasswordReset(_ context.Context, in appidentity.PasswordResetInput) error {
	u.got = in
	return u.err
}

func (u *useCases) ConfirmPasswordReset(_ context.Context, in appidentity.ConfirmPasswordResetInput) error {
	u.got = in
	return u.err
}

func (u *useCases) PreviewInvitation(_ context.Context, token string) (appidentity.InvitationPreview, error) {
	u.got = token
	return u.preview, u.err
}

func (u *useCases) AcceptInvitation(_ context.Context, in appidentity.AcceptInvitationInput) (appidentity.AuthResult, error) {
	u.got = in
	return u.res, u.err
}

// limiter is a fake ratelimit.Limiter that refuses keys of the rules in
// refuse and records every key it counted.
type limiter struct {
	refuse map[string]bool
	keys   []string
}

func (l *limiter) Allow(_ context.Context, key string, _ int, _ time.Duration) (bool, time.Duration) {
	l.keys = append(l.keys, key)
	for rule := range l.refuse {
		if strings.HasPrefix(key, "g:rl:"+rule+":") {
			return false, 90 * time.Second
		}
	}
	return true, 0
}

// verifier is a fake handler.AccessVerifier that accepts one token.
type verifier struct{}

func (verifier) Verify(_ context.Context, raw string, aud shared.Audience) (appidentity.AccessClaims, error) {
	if raw == "fault" {
		return appidentity.AccessClaims{}, errors.New("auth: verify for unknown audience")
	}
	if raw != "good.access.token" || aud != shared.AudienceBackoffice {
		return appidentity.AccessClaims{}, auth.ErrInvalidToken
	}
	return appidentity.AccessClaims{JTI: jtiID}, nil
}

type fixture struct {
	uc      *useCases
	limiter *limiter
	auth    *handler.Auth
	logs    *bytes.Buffer
}

func newFixture(t *testing.T, cookieName string) *fixture {
	t.Helper()
	cookie, err := handler.NewRefreshCookie(cookieName)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ratelimit.NewPseudonyms([]byte(strings.Repeat("k", ratelimit.MinKeyLength)))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{uc: &useCases{res: session()}, limiter: &limiter{refuse: map[string]bool{}}, logs: &bytes.Buffer{}}
	f.auth, err = handler.NewAuth(handler.AuthDeps{
		UseCases: f.uc, Verifier: verifier{}, Limiter: f.limiter, Pseudonyms: p,
		Cookie: cookie, Log: slog.New(slog.NewTextHandler(f.logs, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestNewAuth_RefusesMissingDependencies(t *testing.T) {
	t.Parallel()

	cookie, _ := handler.NewRefreshCookie(handler.HostRefreshCookie)
	p, _ := ratelimit.NewPseudonyms([]byte(strings.Repeat("k", ratelimit.MinKeyLength)))
	full := handler.AuthDeps{
		UseCases: &useCases{}, Verifier: verifier{}, Limiter: &limiter{}, Pseudonyms: p, Cookie: cookie, Log: slog.New(slog.DiscardHandler),
	}
	if _, err := handler.NewAuth(full); err != nil {
		t.Fatalf("NewAuth refused full dependencies: %v", err)
	}
	for name, change := range map[string]func(*handler.AuthDeps){
		"use cases":  func(d *handler.AuthDeps) { d.UseCases = nil },
		"verifier":   func(d *handler.AuthDeps) { d.Verifier = nil },
		"limiter":    func(d *handler.AuthDeps) { d.Limiter = nil },
		"pseudonyms": func(d *handler.AuthDeps) { d.Pseudonyms = ratelimit.Pseudonyms{} },
		"cookie":     func(d *handler.AuthDeps) { d.Cookie = handler.RefreshCookie{} },
		"logger":     func(d *handler.AuthDeps) { d.Log = nil },
	} {
		d := full
		change(&d)
		if _, err := handler.NewAuth(d); err == nil {
			t.Errorf("NewAuth accepted no %s", name)
		}
	}
}

// call sends body to the handler of path and checks the answer against
// the contract.
func (f *fixture) call(t *testing.T, path, body string, setup ...func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	handlers := map[string]http.HandlerFunc{
		"/auth/signup": f.auth.SignUp, "/auth/login": f.auth.LogIn, "/auth/refresh": f.auth.Refresh,
		"/auth/switch-tenant": f.auth.SwitchTenant, "/auth/logout": f.auth.LogOut,
		"/auth/password-reset": f.auth.RequestPasswordReset, "/auth/password-reset/confirm": f.auth.ConfirmPasswordReset,
		"/auth/invitations/preview": f.auth.PreviewInvitation, "/auth/invitations/accept": f.auth.AcceptInvitation,
	}
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	for _, s := range setup {
		s(r)
	}
	rec := httptest.NewRecorder()
	handlers[path](rec, r)
	checkContract(t, r, rec)
	return rec
}

func withCookie(name, value string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: name, Value: value}) }
}

func code(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var p respond.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("not a problem: %s", rec.Body.String())
	}
	return p.Code
}

func body(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %s", rec.Body.String())
	}
	return m
}

// refreshCookie returns the refresh cookie the answer set, if any.
func refreshCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestNewRefreshCookie_TakesOnlyTheTwoNames(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "refresh", "__Host-other", "__Secure-encomendo_refresh"} {
		if _, err := handler.NewRefreshCookie(name); err == nil {
			t.Errorf("NewRefreshCookie(%q) accepted a name ADR 0010 does not allow", name)
		}
	}
}

// The cookie rules of ADR 0010, for both names.
func TestAuth_SessionCookie(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		wantSecure bool
	}{{handler.HostRefreshCookie, true}, {handler.PlainRefreshCookie, false}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.name)
			rec := f.call(t, "/auth/login", `{"email":"ana@example.com","password":"segredo123"}`)

			c := refreshCookie(rec, tt.name)
			if rec.Code != http.StatusOK || c == nil {
				t.Fatalf("answer %d, cookie %v", rec.Code, c)
			}
			if c.Value != "the-new-refresh-token" || !c.HttpOnly || c.Secure != tt.wantSecure ||
				c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" {
				t.Errorf("cookie = %+v; want HttpOnly, SameSite=Strict, Path=/, no Domain, Secure=%v", c, tt.wantSecure)
			}
			if _, ok := body(t, rec)["refreshToken"]; ok {
				t.Error("with the cookie transport the refresh token also went into the body")
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestAuth_BodyTransport(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/auth/signup", "/auth/login", "/auth/invitations/accept"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, handler.HostRefreshCookie)
			rec := f.call(t, path, bodyFor(path, `"refreshTransport":"body"`))

			if rec.Code >= 300 {
				t.Fatalf("answer %d %s", rec.Code, rec.Body.String())
			}
			if got := body(t, rec)["refreshToken"]; got != "the-new-refresh-token" {
				t.Errorf("refreshToken = %v", got)
			}
			if refreshCookie(rec, handler.HostRefreshCookie) != nil {
				t.Error("with the body transport a cookie was set too")
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Error("a session answer with the token in its body may be cached")
			}
		})
	}
}

// bodyFor is a valid body for a session-starting path, plus extra fields.
func bodyFor(path, extra string) string {
	fields := map[string]string{
		"/auth/signup":             `"name":"Ana","email":"ana@example.com","password":"segredo123"`,
		"/auth/login":              `"email":"ana@example.com","password":"segredo123"`,
		"/auth/invitations/accept": `"token":"invitation-token","password":"segredo123"`,
	}[path]
	if extra != "" {
		fields += "," + extra
	}
	return "{" + fields + "}"
}

func TestAuth_RefreshAnswersTheWayTheTokenCame(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/auth/refresh", "/auth/switch-tenant"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			extra := ""
			if path == "/auth/switch-tenant" {
				extra = `"tenantId":"` + tenantID.String() + `"`
			}
			join := func(fields ...string) string {
				var nonEmpty []string
				for _, f := range fields {
					if f != "" {
						nonEmpty = append(nonEmpty, f)
					}
				}
				return "{" + strings.Join(nonEmpty, ",") + "}"
			}

			f := newFixture(t, handler.HostRefreshCookie)
			rec := f.call(t, path, join(extra), withCookie(handler.HostRefreshCookie, "old-cookie-token"))
			if c := refreshCookie(rec, handler.HostRefreshCookie); rec.Code != http.StatusOK || c == nil || c.Value != "the-new-refresh-token" {
				t.Errorf("cookie in: answer %d, cookie %v; want a new cookie", rec.Code, c)
			}
			if _, ok := body(t, rec)["refreshToken"]; ok {
				t.Error("cookie in, token out in the body: a script could trade the cookie for a token")
			}

			rec = f.call(t, path, join(extra, `"refreshToken":"old-body-token"`))
			if rec.Code != http.StatusOK || body(t, rec)["refreshToken"] != "the-new-refresh-token" ||
				refreshCookie(rec, handler.HostRefreshCookie) != nil || rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("body in: answer %d %s; want the token in the body and no cookie", rec.Code, rec.Body.String())
			}

			rec = f.call(t, path, join(extra, `"refreshToken":"old-body-token"`), withCookie(handler.HostRefreshCookie, "c"))
			if rec.Code != http.StatusBadRequest || code(t, rec) != respond.CodeInvalid {
				t.Errorf("both: answer %d, want 400 invalid", rec.Code)
			}

			// The cookie cannot be traded for a body token by asking for one:
			// these calls have no refreshTransport field.
			rec = f.call(t, path, join(extra, `"refreshTransport":"body"`), withCookie(handler.HostRefreshCookie, "c"))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("cookie with refreshTransport body: answer %d, want 400", rec.Code)
			}

			rec = f.call(t, path, join(extra), withCookie(handler.HostRefreshCookie, "a"), withCookie(handler.HostRefreshCookie, "b"))
			if rec.Code != http.StatusUnauthorized || code(t, rec) != respond.CodeUnauthenticated {
				t.Errorf("two cookies: answer %d, want 401 unauthenticated", rec.Code)
			}
		})
	}
}

// Only the configured name is read: a planted plain cookie in production
// is ignored, not turned into a forced logout (ADR 0010).
func TestAuth_ReadsOnlyTheConfiguredCookie(t *testing.T) {
	t.Parallel()

	f := newFixture(t, handler.HostRefreshCookie)
	f.call(t, "/auth/refresh", `{}`, withCookie(handler.PlainRefreshCookie, "planted"),
		withCookie(handler.HostRefreshCookie, "real"))

	if in, ok := f.uc.got.(appidentity.RefreshInput); !ok || in.RefreshToken != "real" {
		t.Errorf("the use case got %+v, want the configured cookie's token only", f.uc.got)
	}
}

// A failure behind refresh is a 5xx, never a 401 that would log every
// Android user out (ADR 0013).
func TestAuth_RefreshStoreFailureIsNot401(t *testing.T) {
	t.Parallel()

	f := newFixture(t, handler.HostRefreshCookie)
	f.uc.err = errors.New("postgres: connection refused")
	rec := f.call(t, "/auth/refresh", `{"refreshToken":"t"}`)
	if rec.Code < 500 {
		t.Errorf("answer %d, want a 5xx", rec.Code)
	}
	f.uc.err = fmt.Errorf("read token: %w", shared.ErrUnavailable)
	rec = f.call(t, "/auth/refresh", `{"refreshToken":"t"}`)
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get(respond.HeaderRetryAfter) == "" {
		t.Errorf("answer %d, want 503 with Retry-After", rec.Code)
	}
}

func TestAuth_Answers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, path, body string
		err              error
		wantStatus       int
		wantCode         string
	}{
		{"sign-up", "/auth/signup", bodyFor("/auth/signup", ""), nil, 201, ""},
		{"sign-up, email taken", "/auth/signup", bodyFor("/auth/signup", ""), identity.ErrEmailTaken, 409, respond.CodeEmailTaken},
		{"sign-up, bad input", "/auth/signup", bodyFor("/auth/signup", ""), shared.NewValidationError(shared.FieldError{Field: "email", Message: "x"}), 400, respond.CodeInvalid},
		{"sign-up, hasher busy", "/auth/signup", bodyFor("/auth/signup", ""), auth.ErrBusy, 503, respond.CodeNotReady},
		{"sign-up, unknown field", "/auth/signup", `{"name":"Ana","email":"a@b.co","password":"x","admin":true}`, nil, 400, respond.CodeInvalid},
		{"sign-up, bad transport", "/auth/signup", bodyFor("/auth/signup", `"refreshTransport":"carrier-pigeon"`), nil, 400, respond.CodeInvalid},
		{"login", "/auth/login", bodyFor("/auth/login", ""), nil, 200, ""},
		{"login, wrong password", "/auth/login", bodyFor("/auth/login", ""), identity.ErrInvalidCredentials, 401, respond.CodeInvalidCredentials},
		{"login, hasher busy", "/auth/login", bodyFor("/auth/login", ""), auth.ErrBusy, 503, respond.CodeNotReady},
		{"login, bad tenant id", "/auth/login", bodyFor("/auth/login", `"tenantId":"nope"`), nil, 400, respond.CodeInvalid},
		{"refresh, refused", "/auth/refresh", `{"refreshToken":"t"}`, shared.ErrUnauthenticated, 401, respond.CodeUnauthenticated},
		{"switch", "/auth/switch-tenant", `{"tenantId":"` + tenantID.String() + `","refreshToken":"t"}`, nil, 200, ""},
		{"switch, not a member", "/auth/switch-tenant", `{"tenantId":"` + tenantID.String() + `","refreshToken":"t"}`, shared.ErrForbidden, 403, respond.CodeForbidden},
		{"switch, bad tenant id", "/auth/switch-tenant", `{"tenantId":"nope","refreshToken":"t"}`, nil, 400, respond.CodeInvalid},
		{"logout", "/auth/logout", `{}`, nil, 204, ""},
		{"password reset", "/auth/password-reset", `{"email":"ana@example.com"}`, nil, 202, ""},
		{"confirm reset", "/auth/password-reset/confirm", `{"email":"ana@example.com","code":"123456","password":"segredo123"}`, nil, 204, ""},
		{"confirm reset, wrong code", "/auth/password-reset/confirm", `{"email":"ana@example.com","code":"123456","password":"segredo123"}`, identity.ErrInvalidCode, 400, respond.CodeInvalidCode},
		{"confirm reset, hasher busy", "/auth/password-reset/confirm", `{"email":"ana@example.com","code":"123456","password":"segredo123"}`, auth.ErrBusy, 503, respond.CodeNotReady},
		{"preview", "/auth/invitations/preview", `{"token":"t"}`, nil, 200, ""},
		{"preview, not pending", "/auth/invitations/preview", `{"token":"t"}`, tenancy.ErrInvitationNotPending, 404, respond.CodeNotFound},
		{"accept", "/auth/invitations/accept", bodyFor("/auth/invitations/accept", ""), nil, 200, ""},
		{"accept, wrong password", "/auth/invitations/accept", bodyFor("/auth/invitations/accept", ""), identity.ErrInvalidCredentials, 401, respond.CodeInvalidCredentials},
		{"accept, email taken", "/auth/invitations/accept", bodyFor("/auth/invitations/accept", ""), identity.ErrEmailTaken, 409, respond.CodeEmailTaken},
		{"accept, hasher busy", "/auth/invitations/accept", bodyFor("/auth/invitations/accept", ""), auth.ErrBusy, 503, respond.CodeNotReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, handler.HostRefreshCookie)
			f.uc.err = tt.err
			f.uc.preview = appidentity.InvitationPreview{
				Tenant: appidentity.TenantSummary{ID: tenantID, Name: "Alta RPM", Slug: "altarpm"},
				Email:  "bia@example.com", Role: shared.RoleStaff, ExpiresAt: expires,
			}
			rec := f.call(t, tt.path, tt.body)

			if rec.Code != tt.wantStatus {
				t.Fatalf("answer %d %s, want %d", rec.Code, rec.Body.String(), tt.wantStatus)
			}
			if tt.wantCode != "" && code(t, rec) != tt.wantCode {
				t.Errorf("code = %s, want %s", code(t, rec), tt.wantCode)
			}
		})
	}
}

func TestAuth_SessionWithoutAStoreHasNoTenantOrRole(t *testing.T) {
	t.Parallel()

	f := newFixture(t, handler.HostRefreshCookie)
	f.uc.res.Session = appidentity.SessionView{
		User:        appidentity.UserView{ID: userID, Name: "Ana", Email: "ana@example.com"},
		Permissions: []identity.Permission{}, Memberships: []appidentity.Membership{},
	}
	rec := f.call(t, "/auth/signup", bodyFor("/auth/signup", ""))
	got := body(t, rec)
	if _, ok := got["tenant"]; ok {
		t.Error("tenant present without a store")
	}
	if _, ok := got["role"]; ok {
		t.Error("role present without a store")
	}
	if got["permissions"] == nil || got["memberships"] == nil {
		t.Errorf("permissions and memberships must be empty lists, never null: %v", got)
	}
}

func TestAuth_LoginPassesTheStoreHint(t *testing.T) {
	t.Parallel()

	f := newFixture(t, handler.HostRefreshCookie)
	f.call(t, "/auth/login", bodyFor("/auth/login", `"tenantId":"`+tenantID.String()+`"`))
	if in, ok := f.uc.got.(appidentity.LoginInput); !ok || in.TenantID != tenantID || in.Email != "ana@example.com" {
		t.Errorf("use case got %+v", f.uc.got)
	}
}

// tok is a well-formed refresh token, different for each b.
func tok(b byte) string { return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32)) }

func TestAuth_LogOut(t *testing.T) {
	t.Parallel()

	bearer := func(v string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+v) }
	}
	cleared := func(t *testing.T, rec *httptest.ResponseRecorder, name string) {
		t.Helper()
		if c := refreshCookie(rec, name); c == nil || c.MaxAge >= 0 {
			t.Errorf("the cookie was not cleared: %v", c)
		}
	}
	t.Run("cookie and a valid access token", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, handler.HostRefreshCookie)
		rec := f.call(t, "/auth/logout", `{}`, withCookie(handler.HostRefreshCookie, tok(1)), bearer("good.access.token"))
		if rec.Code != http.StatusNoContent || len(f.uc.logouts) != 1 ||
			f.uc.logouts[0].RefreshToken != tok(1) || f.uc.logouts[0].AccessJTI != jtiID {
			t.Errorf("answer %d, revoked %+v", rec.Code, f.uc.logouts)
		}
		cleared(t, rec, handler.HostRefreshCookie)
	})
	t.Run("nothing presented", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, handler.HostRefreshCookie)
		rec := f.call(t, "/auth/logout", `{}`)
		if rec.Code != http.StatusNoContent || len(f.uc.logouts) != 0 {
			t.Errorf("answer %d, revoked %+v; want 204 and no work", rec.Code, f.uc.logouts)
		}
		cleared(t, rec, handler.HostRefreshCookie)
	})
	t.Run("a bad access token is ignored", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, handler.HostRefreshCookie)
		rec := f.call(t, "/auth/logout", `{"refreshToken":"`+tok(1)+`"}`, bearer("forged"))
		if rec.Code != http.StatusNoContent || len(f.uc.logouts) != 1 || !f.uc.logouts[0].AccessJTI.IsZero() {
			t.Errorf("answer %d, revoked %+v", rec.Code, f.uc.logouts)
		}
	})
	t.Run("a verifier fault is logged and logout goes on", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, handler.HostRefreshCookie)
		rec := f.call(t, "/auth/logout", `{"refreshToken":"`+tok(1)+`"}`, bearer("fault"))
		if rec.Code != http.StatusNoContent || len(f.uc.logouts) != 1 || !f.uc.logouts[0].AccessJTI.IsZero() {
			t.Errorf("answer %d, revoked %+v", rec.Code, f.uc.logouts)
		}
		if !strings.Contains(f.logs.String(), "logout could not check its access token") {
			t.Errorf("the fault was not logged: %s", f.logs.String())
		}
	})
	t.Run("only an access token", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, handler.HostRefreshCookie)
		rec := f.call(t, "/auth/logout", `{}`, bearer("good.access.token"))
		if rec.Code != http.StatusNoContent || len(f.uc.logouts) != 1 || f.uc.logouts[0].AccessJTI != jtiID {
			t.Errorf("answer %d, revoked %+v", rec.Code, f.uc.logouts)
		}
	})
	// Logout always works: every presented token is revoked, a planted one
	// being only the planter's own session (ADR 0010, amended in step 2.3).
	t.Run("two cookies are both revoked", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, handler.HostRefreshCookie)
		rec := f.call(t, "/auth/logout", `{}`, withCookie(handler.HostRefreshCookie, tok(1)), withCookie(handler.HostRefreshCookie, tok(2)))
		if rec.Code != http.StatusNoContent || len(f.uc.logouts) != 2 ||
			f.uc.logouts[0].RefreshToken != tok(1) || f.uc.logouts[1].RefreshToken != tok(2) {
			t.Errorf("answer %d, revoked %+v; want 204 and both", rec.Code, f.uc.logouts)
		}
		cleared(t, rec, handler.HostRefreshCookie)
	})
	t.Run("a cookie and a body token are both revoked, the access token once", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, handler.HostRefreshCookie)
		rec := f.call(t, "/auth/logout", `{"refreshToken":"`+tok(2)+`"}`, withCookie(handler.HostRefreshCookie, tok(1)), bearer("good.access.token"))
		if rec.Code != http.StatusNoContent || len(f.uc.logouts) != 2 ||
			f.uc.logouts[0].RefreshToken != tok(1) || f.uc.logouts[1].RefreshToken != tok(2) ||
			f.uc.logouts[0].AccessJTI != jtiID || !f.uc.logouts[1].AccessJTI.IsZero() {
			t.Errorf("answer %d, revoked %+v", rec.Code, f.uc.logouts)
		}
	})
	// Each token costs a transaction that outlives the request, so a call
	// stuffed with cookies does a bounded amount of work.
	t.Run("malformed, repeated and surplus tokens are dropped", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, handler.HostRefreshCookie)
		setup := []func(*http.Request){withCookie(handler.HostRefreshCookie, "malformed"), withCookie(handler.HostRefreshCookie, tok(1))}
		for i := range 100 {
			setup = append(setup, withCookie(handler.HostRefreshCookie, tok(byte(i%50+1))))
		}
		rec := f.call(t, "/auth/logout", `{}`, setup...)
		if rec.Code != http.StatusNoContent || len(f.uc.logouts) != 4 || f.uc.logouts[0].RefreshToken != tok(1) {
			t.Errorf("answer %d, %d revokes; want 204 and at most 4, the first one first", rec.Code, len(f.uc.logouts))
		}
	})
	t.Run("a store failure is a 5xx and keeps the cookie", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, handler.HostRefreshCookie)
		f.uc.err = errors.New("postgres down")
		rec := f.call(t, "/auth/logout", `{}`, withCookie(handler.HostRefreshCookie, tok(1)))
		if rec.Code != http.StatusInternalServerError || refreshCookie(rec, handler.HostRefreshCookie) != nil {
			t.Errorf("answer %d; the cookie must stay when nothing was revoked, so a retry can revoke it", rec.Code)
		}
	})
	t.Run("a failure after a first revoke still keeps the cookie", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, handler.HostRefreshCookie)
		f.uc.logoutErrs = []error{nil, errors.New("postgres down")}
		rec := f.call(t, "/auth/logout", `{"refreshToken":"`+tok(2)+`"}`, withCookie(handler.HostRefreshCookie, tok(1)))
		if rec.Code != http.StatusInternalServerError || len(f.uc.logouts) != 2 || refreshCookie(rec, handler.HostRefreshCookie) != nil {
			t.Errorf("answer %d after %d revokes; want a 5xx and the cookie kept", rec.Code, len(f.uc.logouts))
		}
	})
	t.Run("the plain cookie clears without Secure", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, handler.PlainRefreshCookie)
		rec := f.call(t, "/auth/logout", `{}`, withCookie(handler.PlainRefreshCookie, tok(1)))
		if c := refreshCookie(rec, handler.PlainRefreshCookie); c == nil || c.MaxAge >= 0 || c.Secure || !c.HttpOnly {
			t.Errorf("clearing cookie = %+v", c)
		}
	})
}

// Every per-email rule of ADR 0021 runs in its handler, keyed on the
// email's pseudonym, never the email itself, and answers 429 with
// Retry-After.
func TestAuth_PerEmailLimits(t *testing.T) {
	t.Parallel()

	tests := []struct{ rule, path, body string }{
		{"signup", "/auth/signup", bodyFor("/auth/signup", "")},
		{"login", "/auth/login", bodyFor("/auth/login", "")},
		{"password-reset", "/auth/password-reset", `{"email":"ana@example.com"}`},
		{"password-reset-confirm", "/auth/password-reset/confirm", `{"email":"ana@example.com","code":"123456","password":"segredo123"}`},
	}
	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, handler.HostRefreshCookie)
			f.limiter.refuse[tt.rule] = true
			rec := f.call(t, tt.path, tt.body)

			if rec.Code != http.StatusTooManyRequests || code(t, rec) != respond.CodeRateLimited || rec.Header().Get(respond.HeaderRetryAfter) != "90" {
				t.Errorf("answer %d %v, want 429 with Retry-After 90", rec.Code, rec.Header())
			}
			if f.uc.got != nil {
				t.Error("the use case ran past the limit")
			}
			if len(f.limiter.keys) != 1 || !strings.HasPrefix(f.limiter.keys[0], "g:rl:"+tt.rule+":email:") ||
				strings.Contains(f.limiter.keys[0], "ana") {
				t.Errorf("counted %v, want one email key holding a pseudonym", f.limiter.keys)
			}
		})
	}
}

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }
