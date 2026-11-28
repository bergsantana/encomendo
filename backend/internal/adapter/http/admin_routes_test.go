package http

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// adminPaths are every admin operation of the contract: the nine store and
// team ones, then the catalog, supplier and milestone 4 ones.
var adminPaths = slices.Concat(tenancyAdminPaths, catalogAdminPaths, supplierAdminPaths, orderAdminPaths)

var tenancyAdminPaths = []struct{ method, path string }{
	{"POST", "/api/v1/admin/tenants"},
	{"GET", "/api/v1/admin/settings"},
	{"PUT", "/api/v1/admin/settings"},
	{"GET", "/api/v1/admin/members"},
	{"DELETE", "/api/v1/admin/members/01936d1e-8b5a-7c2e-9f3a-000000000004"},
	{"PUT", "/api/v1/admin/members/01936d1e-8b5a-7c2e-9f3a-000000000004/role"},
	{"GET", "/api/v1/admin/invitations"},
	{"POST", "/api/v1/admin/invitations"},
	{"DELETE", "/api/v1/admin/invitations/01936d1e-8b5a-7c2e-9f3a-000000000005"},
}

// tokens is a fake verifier: "staff" is a staff token for the backoffice
// audience, "client" a client account's token, which the backoffice
// audience refuses as the real verifier does.
type tokens struct{}

func (tokens) Verify(_ context.Context, raw string, aud shared.Audience) (appidentity.AccessClaims, error) {
	switch {
	case raw == "staff" && aud == shared.AudienceBackoffice:
		p, err := shared.NewBackofficePrincipal(shared.UserID(shared.ID{1}), shared.TenantID(shared.ID{2}), shared.RoleOwner)
		return appidentity.AccessClaims{Principal: p, JTI: shared.ID{3}}, err
	case raw == "client" && aud == shared.AudienceClient:
		// A real client token, which only a client route would take.
		p, err := shared.NewClientPrincipal(shared.ClientAccountID(shared.ID{4}))
		return appidentity.AccessClaims{Principal: p, JTI: shared.ID{5}}, err
	}
	return appidentity.AccessClaims{}, auth.ErrInvalidToken
}

type openDenylist struct{}

func (openDenylist) Denied(context.Context, shared.ID) (bool, error) { return false, nil }

// tenancyReached records that a call got through; every call refuses with
// ErrForbidden, which every admin route answers.
type tenancyReached struct{ calls int }

func (u *tenancyReached) CreateTenant(context.Context, apptenancy.CreateTenantInput) (apptenancy.TenantView, error) {
	u.calls++
	return apptenancy.TenantView{}, shared.ErrForbidden
}

func (u *tenancyReached) Settings(context.Context) (apptenancy.SettingsView, error) {
	u.calls++
	return apptenancy.SettingsView{}, shared.ErrForbidden
}

func (u *tenancyReached) UpdateSettings(context.Context, apptenancy.UpdateSettingsInput) (apptenancy.SettingsView, error) {
	u.calls++
	return apptenancy.SettingsView{}, shared.ErrForbidden
}

func (u *tenancyReached) ChooseStatusWorkflow(context.Context, apptenancy.ChooseWorkflowInput) (apptenancy.SettingsView, error) {
	u.calls++
	return apptenancy.SettingsView{}, shared.ErrForbidden
}

func (u *tenancyReached) Members(context.Context, shared.PageRequest) (shared.Page[apptenancy.MemberView], error) {
	u.calls++
	return shared.Page[apptenancy.MemberView]{}, shared.ErrForbidden
}

func (u *tenancyReached) ChangeRole(context.Context, apptenancy.ChangeRoleInput) (apptenancy.MemberView, error) {
	u.calls++
	return apptenancy.MemberView{}, shared.ErrForbidden
}

func (u *tenancyReached) RemoveMember(context.Context, shared.UserID) error {
	u.calls++
	return shared.ErrForbidden
}

func (u *tenancyReached) Invitations(context.Context, shared.PageRequest) (shared.Page[apptenancy.InvitationView], error) {
	u.calls++
	return shared.Page[apptenancy.InvitationView]{}, shared.ErrForbidden
}

func (u *tenancyReached) InviteMember(context.Context, apptenancy.InviteMemberInput) (apptenancy.InvitationView, error) {
	u.calls++
	return apptenancy.InvitationView{}, shared.ErrForbidden
}

func (u *tenancyReached) RevokeInvitation(context.Context, shared.InvitationID) error {
	u.calls++
	return shared.ErrForbidden
}

func adminRouter(t *testing.T, uc *tenancyReached) http.Handler {
	t.Helper()
	return adminRouterWith(t, uc, activeStores{})
}

// activeStores is a StoreChecker for which every store is active.
type activeStores struct{}

func (activeStores) CheckStore(context.Context) error { return nil }

func adminRouterWith(t *testing.T, uc *tenancyReached, stores middleware.StoreChecker) http.Handler {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	p, err := ratelimit.NewPseudonyms([]byte(strings.Repeat("k", ratelimit.MinKeyLength)))
	if err != nil {
		t.Fatal(err)
	}
	ten, err := handler.NewTenancy(handler.TenancyDeps{UseCases: uc, Limiter: &countingLimiter{}, Pseudonyms: p, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	cat, err := handler.NewCatalog(handler.CatalogDeps{UseCases: catalogReached{n: &uc.calls}, Limiter: &countingLimiter{}, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	sup, err := handler.NewSuppliers(handler.SuppliersDeps{UseCases: suppliersReached{n: &uc.calls}, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	h := orderHandlers(t, &uc.calls)
	h.Health, h.Tenancy, h.Catalog, h.Suppliers = handler.NewHealth(log, time.Second), ten, cat, sup
	router, err := NewRouter(log, h,
		RouterOptions{Authn: middleware.NewAuthn(tokens{}, openDenylist{}, log), Stores: stores})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func adminRequest(method, path, token string) *http.Request {
	body := ""
	switch {
	case method == "POST" && path == "/api/v1/admin/tenants":
		body = `{"name":"Alta RPM","slug":"altarpm"}`
	case method == "PUT" && path == "/api/v1/admin/settings":
		body = `{"stockEnabled":true,"productionEnabled":false,"purchaseBeforeDeposit":false,"depositPercent":50,` +
			`"assemblyDays":3,"safetyMarginDays":2,"whatsappPhone":null,"version":1}`
	case method == "POST" && path == "/api/v1/admin/invitations":
		body = `{"email":"a@b.co","role":"staff"}`
	case method == "PUT" && strings.HasSuffix(path, "/role"):
		body = `{"role":"staff"}`
	default:
		if body = newOrderBody(method, path); body == "" {
			body, _ = newCatalogBody(method, path)
		}
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

// Every admin route has authn for the backoffice audience: no token and a
// client account's token are both 401, and nothing runs behind them.
func TestRouter_AdminRoutesNeedAStaffToken(t *testing.T) {
	t.Parallel()

	for _, rt := range adminPaths {
		for _, token := range []string{"", "client", "forged"} {
			t.Run(rt.method+" "+rt.path+" "+token, func(t *testing.T) {
				t.Parallel()

				uc := &tenancyReached{}
				rec := httptest.NewRecorder()
				adminRouter(t, uc).ServeHTTP(rec, adminRequest(rt.method, rt.path, token))

				if rec.Code != http.StatusUnauthorized || uc.calls != 0 {
					t.Errorf("answer %d with %d use-case calls, want 401 and none", rec.Code, uc.calls)
				}
			})
		}
	}
}

// adminPaths must list every admin route, or a new one would escape the
// authn checks above.
func TestRouter_AdminPathsListEveryRoute(t *testing.T) {
	t.Parallel()

	all := Handlers{
		Tenancy: &handler.Tenancy{}, Catalog: &handler.Catalog{}, Suppliers: &handler.Suppliers{}, Workflows: &handler.Workflows{},
		Customers: &handler.Customers{}, Orders: &handler.Orders{}, Today: &handler.Today{}, Finance: &handler.Finance{},
	}
	if n := len(adminRoutes(all)); len(adminPaths) != n || n != 9+29+4+23 {
		t.Fatalf("adminPaths lists %d routes, the router mounts %d", len(adminPaths), n)
	}
}

func TestRouter_AdminRoutesServeStaff(t *testing.T) {
	t.Parallel()

	for _, rt := range adminPaths {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			t.Parallel()

			uc := &tenancyReached{}
			rec := httptest.NewRecorder()
			adminRouter(t, uc).ServeHTTP(rec, adminRequest(rt.method, rt.path, "staff"))

			if uc.calls != 1 || rec.Code != http.StatusForbidden {
				t.Errorf("answer %d with %d use-case calls, want the use case's 403", rec.Code, uc.calls)
			}
		})
	}
}

func TestNewRouter_RefusesAdminWithoutAuthn(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	p, _ := ratelimit.NewPseudonyms([]byte(strings.Repeat("k", ratelimit.MinKeyLength)))
	ten, err := handler.NewTenancy(handler.TenancyDeps{UseCases: &tenancyReached{}, Limiter: &countingLimiter{}, Pseudonyms: p, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	h := Handlers{Health: handler.NewHealth(log, time.Second), Tenancy: ten}
	for name, opts := range map[string]RouterOptions{
		"no authn":                 {Stores: activeStores{}},
		"no suspended-store check": {Authn: middleware.NewAuthn(tokens{}, openDenylist{}, log)},
	} {
		if _, err := NewRouter(log, h, opts); err == nil {
			t.Errorf("the admin routes were mounted with %s", name)
		}
	}
}

// suspended is a StoreChecker for a suspended store that counts its calls.
type suspended struct{ calls *int }

func (s suspended) CheckStore(context.Context) error {
	*s.calls++
	return tenancy.ErrTenantSuspended
}

// Every admin route that acts in a store refuses a suspended one before
// its use case runs; creating a store does not need one (ADR 0022).
func TestRouter_AdminRoutesRefuseASuspendedStore(t *testing.T) {
	t.Parallel()

	for _, rt := range adminPaths {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			t.Parallel()

			uc, checks := &tenancyReached{}, 0
			rec := httptest.NewRecorder()
			adminRouterWith(t, uc, suspended{calls: &checks}).ServeHTTP(rec, adminRequest(rt.method, rt.path, "staff"))

			if rt.method == "POST" && rt.path == "/api/v1/admin/tenants" {
				if checks != 0 || uc.calls != 1 {
					t.Errorf("creating a store ran %d checks and %d use cases, want none and one", checks, uc.calls)
				}
				return
			}
			if checks != 1 || uc.calls != 0 || rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"tenant_suspended"`) {
				t.Errorf("answer %d %s after %d checks and %d use cases; want 403 tenant_suspended and no use case",
					rec.Code, rec.Body.String(), checks, uc.calls)
			}
		})
	}
}

// The check runs after authn: without a valid token it never runs.
func TestRouter_StoreCheckRunsAfterAuthn(t *testing.T) {
	t.Parallel()

	checks := 0
	rec := httptest.NewRecorder()
	adminRouterWith(t, &tenancyReached{}, suspended{calls: &checks}).ServeHTTP(rec, adminRequest("GET", "/api/v1/admin/settings", ""))
	if rec.Code != http.StatusUnauthorized || checks != 0 {
		t.Errorf("answer %d after %d checks, want 401 before any check", rec.Code, checks)
	}
}
