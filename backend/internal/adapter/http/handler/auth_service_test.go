package handler_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/app/identity/fake"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
)

// realAuth is the /auth handlers on the real identity use cases, with the
// use cases' fakes behind them.
func realAuth(t *testing.T) *handler.Auth {
	t.Helper()
	clock := sharedfake.NewClock(time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC))
	members := fake.NewMemberships()
	store := fake.NewStore()
	svc, err := appidentity.NewService(appidentity.Deps{
		Users: store, Refresh: store, ResetCodes: store,
		Memberships: members, Invitations: fake.NewInvitations(members),
		Hasher: fake.NewHasher(), Tokens: fake.NewTokens(), Denylist: fake.NewDenylist(), Mailer: fake.NewMailer(),
		Tx: sharedfake.NewTxManager(), Clock: clock, IDs: sharedfake.NewIDs(clock), Secrets: sharedfake.NewSecrets(),
		ResetCodeKey: []byte(strings.Repeat("r", appidentity.MinResetCodeKeyLength)),
	})
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
	a, err := handler.NewAuth(handler.AuthDeps{
		UseCases: svc, Verifier: verifier{}, Limiter: &limiter{refuse: map[string]bool{}}, Pseudonyms: p,
		Cookie: cookie, Log: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func post(t *testing.T, h http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	// As Ktor sends it (ADR 0013): no Origin, no Sec-Fetch-Site.
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	h(rec, r)
	checkContract(t, r, rec)
	return rec
}

// Logout revokes the whole family from any token of it, even one a refresh
// just rotated, so the new token stops too (ADR 0016; asked by the web
// session).
func TestAuth_LogoutWithAJustRotatedTokenRevokesTheFamily(t *testing.T) {
	t.Parallel()

	a := realAuth(t)
	rec := post(t, a.SignUp, "/auth/signup", `{"name":"Ana","email":"ana@example.com","password":"segredo123","refreshTransport":"body"}`)
	first, _ := body(t, rec)["refreshToken"].(string)
	if rec.Code != http.StatusCreated || first == "" {
		t.Fatalf("sign-up answered %d %s", rec.Code, rec.Body.String())
	}
	rec = post(t, a.Refresh, "/auth/refresh", `{"refreshToken":"`+first+`"}`)
	second, _ := body(t, rec)["refreshToken"].(string)
	if rec.Code != http.StatusOK || second == "" || second == first {
		t.Fatalf("refresh answered %d %s", rec.Code, rec.Body.String())
	}

	if rec := post(t, a.LogOut, "/auth/logout", `{"refreshToken":"`+first+`"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("logout answered %d %s", rec.Code, rec.Body.String())
	}

	if rec := post(t, a.Refresh, "/auth/refresh", `{"refreshToken":"`+second+`"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("the rotated-to token still works after logout: %d %s", rec.Code, rec.Body.String())
	}
}

// The native-client case of ADR 0013 end to end through the handler: body
// transport, Ktor's media type, 200 with the new token in the body.
func TestAuth_NativeClientRefreshes(t *testing.T) {
	t.Parallel()

	a := realAuth(t)
	rec := post(t, a.SignUp, "/auth/signup", `{"name":"Ana","email":"ana@example.com","password":"segredo123","refreshTransport":"body"}`)
	token, _ := body(t, rec)["refreshToken"].(string)

	rec = post(t, a.Refresh, "/auth/refresh", `{"refreshToken":"`+token+`"}`)
	if rec.Code != http.StatusOK || body(t, rec)["refreshToken"] == "" {
		t.Errorf("refresh answered %d %s", rec.Code, rec.Body.String())
	}
}
