//go:build e2e

package e2e_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// A staff token of one store reaches nothing of another through any admin
// route; the database half is the RLS suite (test/integration/rls).
func TestIsolation_AnotherStoresTokenReadsNothing(t *testing.T) {
	t.Parallel()

	a := signUp(t, "Dona A")
	a.store = a.newStore(t, "Loja A")
	pendingEmail := unique("pendente") + "@teste.encomendo.local"
	inv := must(t, call(t, "POST", "/api/v1/admin/invitations", map[string]any{"email": pendingEmail, "role": "viewer"}, a.access), 201, "A invites")
	invID := inv.str("id")

	must(t, call(t, "PUT", "/api/v1/admin/settings", settingsWith(70, 1), a.access), 200, "A saves its settings")

	b := signUp(t, "Dono B")
	storeB := b.newStore(t, "Loja B")

	members := must(t, call(t, "GET", "/api/v1/admin/members", nil, b.access), 200, "B lists members")
	if !strings.Contains(asJSON(t, members.Body), b.email) {
		t.Fatalf("B's own team list misses B, so the check below proves nothing: %v", members.Body)
	}
	if strings.Contains(asJSON(t, members.Body), a.userID) || strings.Contains(asJSON(t, members.Body), a.email) {
		t.Errorf("B's team list shows A's owner: %v", members.Body)
	}
	if got := must(t, call(t, "GET", "/api/v1/admin/settings", nil, b.access), 200, "B reads settings"); got.Body["depositPercent"] != float64(50) {
		t.Errorf("B reads depositPercent %v, want its own default 50, not A's 70", got.Body["depositPercent"])
	}
	must(t, call(t, "PUT", "/api/v1/admin/settings", settingsWith(10, 1), b.access), 200, "B saves its settings")
	// The way a token could cross stores: switching into one B is not in.
	if got := call(t, "POST", "/auth/switch-tenant", map[string]any{"tenantId": a.store, "refreshToken": b.refresh}, ""); got.Status != 403 {
		t.Errorf("B switching into A's store = %d %v, want 403", got.Status, got.Body)
	}
	if got := must(t, call(t, "POST", "/auth/refresh", map[string]any{"refreshToken": b.refresh}, ""), 200, "B's session after the refused switch"); got.Body["tenant"].(map[string]any)["id"] != storeB {
		t.Errorf("B's session moved: %v", got.Body["tenant"])
	}
	invites := must(t, call(t, "GET", "/api/v1/admin/invitations", nil, b.access), 200, "B lists invitations")
	if strings.Contains(asJSON(t, invites.Body), invID) || strings.Contains(asJSON(t, invites.Body), pendingEmail) {
		t.Errorf("B's invitations show A's: %v", invites.Body)
	}
	for _, c := range []struct{ method, path string }{
		{"DELETE", "/api/v1/admin/members/" + a.userID},
		{"PUT", "/api/v1/admin/members/" + a.userID + "/role"},
		{"DELETE", "/api/v1/admin/invitations/" + invID},
	} {
		var body any
		if c.method == "PUT" {
			body = map[string]any{"role": "viewer"}
		}
		if got := call(t, c.method, c.path, body, b.access); got.Status != 404 || got.str("code") != "not_found" {
			t.Errorf("B's %s %s = %d %v, want 404 not_found", c.method, c.path, got.Status, got.Body)
		}
	}
	// A's invitation and settings are untouched.
	still := must(t, call(t, "GET", "/api/v1/admin/invitations", nil, a.access), 200, "A lists invitations")
	if !strings.Contains(asJSON(t, still.Body), invID) {
		t.Errorf("A's invitation is gone after B's attempts: %v", still.Body)
	}
	if got := must(t, call(t, "GET", "/api/v1/admin/settings", nil, a.access), 200, "A reads settings"); got.Body["depositPercent"] != float64(70) {
		t.Errorf("A's depositPercent = %v after B saved its own, want 70", got.Body["depositPercent"])
	}
}

// settingsWith is a full settings update.
func settingsWith(deposit, version int) map[string]any {
	return map[string]any{
		"stockEnabled": false, "productionEnabled": false, "purchaseBeforeDeposit": false,
		"depositPercent": deposit, "assemblyDays": 3, "safetyMarginDays": 2, "whatsappPhone": nil, "version": version,
	}
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// tokenSigner signs access tokens with the development key the stack runs
// with, for the stack's issuer.
func tokenSigner(t *testing.T) func(keys auth.KeySet, p shared.Principal, at time.Time) string {
	t.Helper()
	return func(keys auth.KeySet, p shared.Principal, at time.Time) string {
		at = at.Truncate(time.Second)
		tokens, err := auth.NewAccessTokens(keys, issuer, sharedfake.NewClock(at))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := tokens.Issue(context.Background(), appidentity.AccessClaims{
			Principal: p, JTI: shared.ID{byte(at.Unix()), byte(at.Nanosecond()), 9}, IssuedAt: at, ExpiresAt: at.Add(identity.AccessTokenLifetime),
		})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
}

func devKeys(t *testing.T) auth.KeySet {
	t.Helper()
	keys, err := auth.LoadKeys(auth.DevSigningSeed, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

// badTokens are access tokens every protected route must refuse with 401:
// expired, tampered, alg none, signed by an unknown key, and a client
// account's token on staff routes. They are signed with the development
// key the stack runs with, so only the property under test is wrong.
func badTokens(t *testing.T) map[string]string {
	t.Helper()
	keys := devKeys(t)
	now := time.Now()
	staff, err := shared.NewBackofficePrincipal(shared.UserID(shared.ID{1}), shared.TenantID(shared.ID{2}), shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	client, err := shared.NewClientPrincipal(shared.ClientAccountID(shared.ID{3}))
	if err != nil {
		t.Fatal(err)
	}
	sign := tokenSigner(t)
	other, err := auth.LoadKeys("q0Fcl2nSQ0n6X3zW1T8Vq0Fcl2nSQ0n6X3zW1T8Vq0E=", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	valid := sign(keys, staff, now)
	parts := strings.Split(valid, ".")
	claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(claims), `"owner"`, `"viewer"`, 1))) + "." + parts[2]
	none := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"at+jwt","kid":"`+keys.KID()+`"}`)) + "." + parts[1] + "."
	return map[string]string{
		"expired":        sign(keys, staff, now.Add(-time.Hour)),
		"tampered":       tampered,
		"alg none":       none,
		"unknown kid":    sign(other, staff, now),
		"wrong audience": sign(keys, client, now),
	}
}

// Every operation of the contract that needs a staff token refuses a bad
// one with 401. The list comes from openapi.yaml, so a new route cannot
// skip the check, and a route in the contract that is not served fails it
// with a 404.
func TestTokens_EveryProtectedRouteRefusesABadToken(t *testing.T) {
	t.Parallel()

	// The control: a good token from the same signer is accepted, so the
	// 401s below come from what is wrong with each token, not from a key
	// or issuer the stack does not use.
	s := signUp(t, "Dona Controle")
	store := s.newStore(t, "Loja Controle")
	userID, err := shared.ParseID(s.userID)
	if err != nil {
		t.Fatal(err)
	}
	storeID, err := shared.ParseID(store)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := shared.NewBackofficePrincipal(shared.UserID(userID), shared.TenantID(storeID), shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	must(t, call(t, "GET", "/api/v1/admin/settings", nil, tokenSigner(t)(devKeys(t), owner, time.Now())), 200, "a good token from the test's signer")

	tokens := badTokens(t)
	ops := protectedOperations(t)
	if len(ops) < 9 {
		t.Fatalf("found %d protected operations, want at least the nine admin ones", len(ops))
	}
	for _, op := range ops {
		t.Run(op.method+" "+op.path, func(t *testing.T) {
			if op.unserved != "" {
				// Written ahead of the server: it must not be served yet, so
				// the mark goes the moment it is (see the spec's "Operations
				// not served yet").
				if got := call(t, op.method, op.url, op.body, tokens["expired"]); got.Status != 404 {
					t.Errorf("marked x-unserved (%s) but answered %d %v: serve it fully and remove the mark", op.unserved, got.Status, got.Body)
				}
				return
			}
			for name, token := range tokens {
				got := call(t, op.method, op.url, op.body, token)
				if got.Status != 401 || got.str("code") != "unauthenticated" {
					t.Errorf("with a %s token = %d %v, want 401 unauthenticated", name, got.Status, got.Body)
				}
			}
		})
	}
}

// operation is one protected operation of the contract, ready to call.
type operation struct {
	method, path, url string
	body              any
	// unserved is the operation's x-unserved mark: the plan step that will
	// serve it, or "" for an operation the server serves today.
	unserved string
}

// protectedOperations returns every operation of openapi.yaml that needs a
// staff token, with an id in each path parameter and {} for each body.
// Operations marked x-unserved come back with their mark set.
func protectedOperations(t *testing.T) []operation {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("../../../docs/api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	id := "01936d1e-8b5a-7c2e-9f3a-00000000abcd"
	var out []operation
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			if !needsBearer(op) {
				continue
			}
			o := operation{method: method, path: path, url: pathParam.ReplaceAllString(path, id)}
			if mark, ok := op.Extensions["x-unserved"].(string); ok {
				o.unserved = mark
			}
			if op.RequestBody != nil {
				o.body = map[string]any{}
			}
			out = append(out, o)
		}
	}
	return out
}

// pathParam matches a path template parameter such as {itemId}.
var pathParam = regexp.MustCompile(`\{[^/{}]+\}`)

// needsBearer reports whether op requires a bearer token: it lists
// bearerAuth and no empty requirement (an optional token, as logout).
func needsBearer(op *openapi3.Operation) bool {
	if op.Security == nil {
		return false
	}
	bearer := false
	for _, req := range *op.Security {
		if len(req) == 0 {
			return false
		}
		if _, ok := req["bearerAuth"]; ok {
			bearer = true
		}
	}
	return bearer
}

// A store suspended by SQL is refused at once on its admin routes and on
// switch, and listed as suspended at login (ADR 0022).
func TestSuspendedStore(t *testing.T) {
	t.Parallel()

	s := signUp(t, "Dona Pausada")
	store := s.newStore(t, "Loja Pausada")
	must(t, call(t, "GET", "/api/v1/admin/settings", nil, s.access), 200, "settings before")

	suspend(t, store)

	// The very next request on every tenant-scoped admin route, with the
	// token issued before the suspension. Creating a store needs none.
	for _, op := range protectedOperations(t) {
		// Unserved operations are checked in the token test instead.
		if op.path == "/api/v1/admin/tenants" || op.unserved != "" {
			continue
		}
		if a := call(t, op.method, op.url, op.body, s.access); a.Status != 403 || a.str("code") != "tenant_suspended" {
			t.Errorf("%s %s after the suspension = %d %v, want 403 tenant_suspended", op.method, op.url, a.Status, a.Body)
		}
	}
	if a := call(t, "POST", "/auth/switch-tenant", map[string]any{"tenantId": store, "refreshToken": s.refresh}, ""); a.Status != 403 || a.str("code") != "tenant_suspended" {
		t.Errorf("switch = %d %v, want 403 tenant_suspended", a.Status, a.Body)
	}
	login := must(t, call(t, "POST", "/auth/login", map[string]any{"email": s.email, "password": s.password}, ""), 200, "login")
	ms, _ := login.Body["memberships"].([]any)
	if _, selected := login.Body["tenant"]; selected || len(ms) != 1 || ms[0].(map[string]any)["status"] != "suspended" {
		t.Errorf("login = %v, want no store selected and the store listed as suspended", login.Body)
	}
}
