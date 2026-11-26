package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

var (
	memberID     = shared.UserID(mustID("01936d1e-8b5a-7c2e-9f3a-000000000004"))
	invitationID = shared.InvitationID(mustID("01936d1e-8b5a-7c2e-9f3a-000000000005"))
	workflowID   = shared.WorkflowID(mustID("01936d1e-8b5a-7c2e-9f3a-000000000006"))
	joined       = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
)

// tenancyCases is a fake apptenancy.UseCases answering fixed views or err.
type tenancyCases struct {
	err  error
	got  any
	page shared.PageRequest
}

func (u *tenancyCases) CreateTenant(_ context.Context, in apptenancy.CreateTenantInput) (apptenancy.TenantView, error) {
	u.got = in
	return apptenancy.TenantView{ID: tenantID, Name: "Alta RPM", Slug: "altarpm", CreatedAt: joined}, u.err
}

func (u *tenancyCases) Settings(context.Context) (apptenancy.SettingsView, error) {
	return settingsView(), u.err
}

func (u *tenancyCases) UpdateSettings(_ context.Context, in apptenancy.UpdateSettingsInput) (apptenancy.SettingsView, error) {
	u.got = in
	return settingsView(), u.err
}

func (u *tenancyCases) Members(_ context.Context, page shared.PageRequest) (shared.Page[apptenancy.MemberView], error) {
	u.page = page
	return shared.Page[apptenancy.MemberView]{Items: []apptenancy.MemberView{member()}, NextCursor: "next"}, u.err
}

func (u *tenancyCases) ChangeRole(_ context.Context, in apptenancy.ChangeRoleInput) (apptenancy.MemberView, error) {
	u.got = in
	return member(), u.err
}

func (u *tenancyCases) RemoveMember(_ context.Context, user shared.UserID) error {
	u.got = user
	return u.err
}

func (u *tenancyCases) Invitations(_ context.Context, page shared.PageRequest) (shared.Page[apptenancy.InvitationView], error) {
	u.page = page
	return shared.Page[apptenancy.InvitationView]{Items: []apptenancy.InvitationView{invitation()}}, u.err
}

func (u *tenancyCases) InviteMember(_ context.Context, in apptenancy.InviteMemberInput) (apptenancy.InvitationView, error) {
	u.got = in
	return invitation(), u.err
}

func (u *tenancyCases) RevokeInvitation(_ context.Context, id shared.InvitationID) error {
	u.got = id
	return u.err
}

func settingsView() apptenancy.SettingsView {
	return apptenancy.SettingsView{
		Editable: tenancy.Editable{DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2},
		Currency: "BRL", Workflow: tenancy.WorkflowRef{ID: workflowID, Version: 1, Name: "Encomenda com montagem"}, Version: 3,
	}
}

func member() apptenancy.MemberView {
	return apptenancy.MemberView{UserID: memberID, Name: "Bia", Email: "bia@example.com", Role: shared.RoleStaff, JoinedAt: joined}
}

func invitation() apptenancy.InvitationView {
	return apptenancy.InvitationView{ID: invitationID, Email: "caio@example.com", Role: shared.RoleStaff, InvitedAt: joined, ExpiresAt: joined.Add(7 * 24 * time.Hour)}
}

type tenancyFixture struct {
	uc      *tenancyCases
	limiter *limiter
	h       *handler.Tenancy
}

func newTenancyFixture(t *testing.T) *tenancyFixture {
	t.Helper()
	p, err := ratelimit.NewPseudonyms([]byte(strings.Repeat("k", ratelimit.MinKeyLength)))
	if err != nil {
		t.Fatal(err)
	}
	f := &tenancyFixture{uc: &tenancyCases{}, limiter: &limiter{refuse: map[string]bool{}}}
	f.h, err = handler.NewTenancy(handler.TenancyDeps{UseCases: f.uc, Limiter: f.limiter, Pseudonyms: p, Log: discardLog()})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// call sends method path to its handler as caller, and checks the answer
// against the contract.
func (f *tenancyFixture) call(t *testing.T, caller shared.Principal, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, h := range map[string]http.HandlerFunc{
		"POST /api/v1/admin/tenants":                      f.h.CreateTenant,
		"GET /api/v1/admin/settings":                      f.h.Settings,
		"PUT /api/v1/admin/settings":                      f.h.UpdateSettings,
		"GET /api/v1/admin/members":                       f.h.Members,
		"DELETE /api/v1/admin/members/{userId}":           f.h.RemoveMember,
		"PUT /api/v1/admin/members/{userId}/role":         f.h.ChangeRole,
		"GET /api/v1/admin/invitations":                   f.h.Invitations,
		"POST /api/v1/admin/invitations":                  f.h.InviteMember,
		"DELETE /api/v1/admin/invitations/{invitationId}": f.h.RevokeInvitation,
	} {
		mux.HandleFunc(pattern, h)
	}
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r = r.WithContext(authz.WithPrincipal(r.Context(), caller))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	checkContract(t, r, rec)
	return rec
}

func staffIn(t *testing.T, role shared.Role) shared.Principal {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(userID, tenantID, role)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func storeless(t *testing.T) shared.Principal {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(userID, shared.TenantID{}, "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

const settingsBody = `{"stockEnabled":true,"productionEnabled":false,"purchaseBeforeDeposit":false,` +
	`"depositPercent":40,"assemblyDays":3,"safetyMarginDays":2,"whatsappPhone":null,"version":3}`

func TestTenancy_Answers(t *testing.T) {
	t.Parallel()

	members := "/api/v1/admin/members/" + memberID.String()
	invitations := "/api/v1/admin/invitations/" + invitationID.String()
	tests := []struct {
		name, method, path, body string
		err                      error
		wantStatus               int
		wantCode                 string
	}{
		{"create store", "POST", "/api/v1/admin/tenants", `{"name":"Alta RPM","slug":"altarpm"}`, nil, 201, ""},
		{"create store, slug taken", "POST", "/api/v1/admin/tenants", `{"name":"Alta RPM","slug":"altarpm"}`, tenancy.ErrSlugTaken, 409, respond.CodeSlugTaken},
		{"create store, bad input", "POST", "/api/v1/admin/tenants", `{"name":"","slug":"x"}`, shared.NewValidationError(shared.FieldError{Field: "slug", Message: "x"}), 400, respond.CodeInvalid},
		{"settings", "GET", "/api/v1/admin/settings", "", nil, 200, ""},
		{"settings, no store", "GET", "/api/v1/admin/settings", "", authz.ErrTenantRequired, 403, respond.CodeTenantRequired},
		{"update settings", "PUT", "/api/v1/admin/settings", settingsBody, nil, 200, ""},
		{"update settings, stale version", "PUT", "/api/v1/admin/settings", settingsBody, shared.ErrVersionConflict, 409, respond.CodeVersionConflict},
		{"update settings, missing fields", "PUT", "/api/v1/admin/settings", `{"version":3}`, nil, 400, respond.CodeInvalid},
		{"update settings, phone of the wrong type", "PUT", "/api/v1/admin/settings", strings.Replace(settingsBody, `"whatsappPhone":null`, `"whatsappPhone":5`, 1), nil, 400, respond.CodeInvalid},
		{"update settings, not the owner", "PUT", "/api/v1/admin/settings", settingsBody, shared.ErrForbidden, 403, respond.CodeForbidden},
		{"members", "GET", "/api/v1/admin/members?limit=10", "", nil, 200, ""},
		{"members, forged cursor", "GET", "/api/v1/admin/members?cursor=forged", "", shared.NewValidationError(shared.FieldError{Field: "cursor", Message: "Cursor inválido."}), 400, respond.CodeInvalid},
		{"members, bad limit", "GET", "/api/v1/admin/members?limit=lots", "", nil, 400, respond.CodeInvalid},
		{"members, limit too large", "GET", "/api/v1/admin/members?limit=101", "", nil, 400, respond.CodeInvalid},
		{"remove member", "DELETE", members, "", nil, 204, ""},
		{"remove the owner", "DELETE", members, "", tenancy.ErrOwnerFixed, 403, respond.CodeForbidden},
		{"remove someone gone", "DELETE", members, "", shared.ErrNotFound, 404, respond.CodeNotFound},
		{"remove a malformed id", "DELETE", "/api/v1/admin/members/nope", "", nil, 404, respond.CodeNotFound},
		{"change role", "PUT", members + "/role", `{"role":"admin"}`, nil, 200, ""},
		{"change role, bad role", "PUT", members + "/role", `{"role":"admin"}`, shared.NewValidationError(shared.FieldError{Field: "role", Message: "x"}), 400, respond.CodeInvalid},
		{"change role, someone gone", "PUT", members + "/role", `{"role":"admin"}`, shared.ErrNotFound, 404, respond.CodeNotFound},
		{"invitations", "GET", "/api/v1/admin/invitations", "", nil, 200, ""},
		{"invite", "POST", "/api/v1/admin/invitations", `{"email":"caio@example.com","role":"staff"}`, nil, 201, ""},
		{"invite, already a member", "POST", "/api/v1/admin/invitations", `{"email":"caio@example.com","role":"staff"}`, tenancy.ErrAlreadyMember, 409, respond.CodeAlreadyMember},
		{"invite, email did not go", "POST", "/api/v1/admin/invitations", `{"email":"caio@example.com","role":"staff"}`, apptenancy.ErrEmailUnavailable, 503, respond.CodeEmailUnavailable},
		{"revoke invitation", "DELETE", invitations, "", nil, 204, ""},
		{"revoke invitation, not pending", "DELETE", invitations, "", tenancy.ErrInvitationNotPending, 404, respond.CodeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newTenancyFixture(t)
			f.uc.err = tt.err
			rec := f.call(t, staffIn(t, shared.RoleOwner), tt.method, tt.path, tt.body)

			if rec.Code != tt.wantStatus {
				t.Fatalf("answer %d %s, want %d", rec.Code, rec.Body.String(), tt.wantStatus)
			}
			if tt.wantCode != "" && code(t, rec) != tt.wantCode {
				t.Errorf("code = %s, want %s", code(t, rec), tt.wantCode)
			}
		})
	}
}

func TestTenancy_PassesInput(t *testing.T) {
	t.Parallel()

	f := newTenancyFixture(t)
	f.call(t, staffIn(t, shared.RoleOwner), "PUT", "/api/v1/admin/settings",
		`{"stockEnabled":true,"productionEnabled":false,"purchaseBeforeDeposit":true,"depositPercent":40,"assemblyDays":4,"safetyMarginDays":1,"whatsappPhone":"5592991234567","version":3}`)
	in, _ := f.uc.got.(apptenancy.UpdateSettingsInput)
	want := tenancy.Editable{StockEnabled: true, PurchaseBeforeDeposit: true, DepositPercent: 40, AssemblyDays: 4, SafetyMarginDays: 1, WhatsAppPhone: "5592991234567"}
	if in.Settings != want || in.Version != 3 {
		t.Errorf("use case got %+v", f.uc.got)
	}

	f.call(t, staffIn(t, shared.RoleOwner), "GET", "/api/v1/admin/members?cursor=abc&limit=10", "")
	if f.uc.page.Cursor() != "abc" || f.uc.page.Limit() != 10 {
		t.Errorf("page = %q, %d", f.uc.page.Cursor(), f.uc.page.Limit())
	}
	f.call(t, staffIn(t, shared.RoleOwner), "PUT", "/api/v1/admin/members/"+memberID.String()+"/role", `{"role":"viewer"}`)
	if in, _ := f.uc.got.(apptenancy.ChangeRoleInput); in.UserID != memberID || in.Role != "viewer" {
		t.Errorf("use case got %+v", f.uc.got)
	}
}

// whatsappPhone may be null, but not missing: null clears the number, and
// a missing field would clear it by accident.
func TestTenancy_UpdateSettingsNeedsThePhoneField(t *testing.T) {
	t.Parallel()

	f := newTenancyFixture(t)
	rec := f.call(t, staffIn(t, shared.RoleOwner), "PUT", "/api/v1/admin/settings", strings.Replace(settingsBody, `"whatsappPhone":null,`, "", 1))
	var p respond.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || rec.Code != 400 || len(p.Errors) != 1 || p.Errors[0].Field != "whatsappPhone" {
		t.Errorf("answer %d %s, want 400 naming whatsappPhone only", rec.Code, rec.Body.String())
	}
	f.call(t, staffIn(t, shared.RoleOwner), "PUT", "/api/v1/admin/settings", settingsBody)
	if in, _ := f.uc.got.(apptenancy.UpdateSettingsInput); in.Settings.WhatsAppPhone != "" || in.Version != 3 {
		t.Errorf("null phone gave %+v, want no number", f.uc.got)
	}
}

func TestTenancy_SettingsWithoutAPhoneIsNull(t *testing.T) {
	t.Parallel()

	rec := newTenancyFixture(t).call(t, staffIn(t, shared.RoleOwner), "GET", "/api/v1/admin/settings", "")
	if got, ok := body(t, rec)["whatsappPhone"]; !ok || got != nil {
		t.Errorf("whatsappPhone = %v (present %v), want null", got, ok)
	}
}

// The invitation link is only in the email, never in an answer (ADR 0011):
// an invitation has exactly the fields the contract lists.
func TestTenancy_InvitationAnswersCarryNoLink(t *testing.T) {
	t.Parallel()

	want := "email,expiresAt,id,invitedAt,role"
	keys := func(m map[string]any) string {
		var ks []string
		for k := range m {
			ks = append(ks, k)
		}
		slices.Sort(ks)
		return strings.Join(ks, ",")
	}
	f := newTenancyFixture(t)
	created := body(t, f.call(t, staffIn(t, shared.RoleOwner), "POST", "/api/v1/admin/invitations", `{"email":"caio@example.com","role":"staff"}`))
	if got := keys(created); got != want {
		t.Errorf("created invitation has %s, want %s", got, want)
	}
	page := body(t, f.call(t, staffIn(t, shared.RoleOwner), "GET", "/api/v1/admin/invitations", ""))
	items, _ := page["items"].([]any)
	for _, item := range items {
		if got := keys(item.(map[string]any)); got != want {
			t.Errorf("listed invitation has %s, want %s", got, want)
		}
	}
}

// The daily caps of ADR 0021, each a 429 with Retry-After before the use
// case. The invitation caps run per user, then per store and email, then
// per store, so a refusal stops the counting there.
func TestTenancy_DailyCaps(t *testing.T) {
	t.Parallel()

	invite := `{"email":"caio@example.com","role":"staff"}`
	tests := []struct {
		name, refuse, path, body string
		wantCounted              int
	}{
		{"create store, per user", "create-tenant:user", "/api/v1/admin/tenants", `{"name":"Alta RPM","slug":"altarpm"}`, 1},
		{"invite, per user", "invite:user", "/api/v1/admin/invitations", invite, 1},
		{"invite, per store and email", "invite:store-email", "/api/v1/admin/invitations", invite, 2},
		{"invite, per store", "invite:store", "/api/v1/admin/invitations", invite, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newTenancyFixture(t)
			f.limiter.refuse[tt.refuse] = true
			rec := f.call(t, staffIn(t, shared.RoleOwner), "POST", tt.path, tt.body)
			if rec.Code != http.StatusTooManyRequests || rec.Header().Get(respond.HeaderRetryAfter) != "90" || f.uc.got != nil {
				t.Errorf("answer %d, use case got %v; want 429 with Retry-After before the use case", rec.Code, f.uc.got)
			}
			if len(f.limiter.keys) != tt.wantCounted {
				t.Errorf("counted %v, want %d counts and none after the refusal", f.limiter.keys, tt.wantCounted)
			}
		})
	}
}

func TestTenancy_InviteCountsEveryCapKeyedRight(t *testing.T) {
	t.Parallel()

	f := newTenancyFixture(t)
	f.call(t, staffIn(t, shared.RoleAdmin), "POST", "/api/v1/admin/invitations", `{"email":"Caio@Example.com","role":"staff"}`)
	if len(f.limiter.keys) != 3 {
		t.Fatalf("counted %v, want the three caps", f.limiter.keys)
	}
	want := []string{
		"g:rl:invite:user:" + userID.String(),
		"g:rl:invite:store-email:" + tenantID.String() + ":",
		"g:rl:invite:store:" + tenantID.String(),
	}
	for i, prefix := range want {
		if !strings.HasPrefix(f.limiter.keys[i], prefix) {
			t.Errorf("count %d = %q, want prefix %q", i, f.limiter.keys[i], prefix)
		}
	}
	if strings.Contains(strings.ToLower(f.limiter.keys[1]), "caio") {
		t.Errorf("the email sits in a key in clear: %q", f.limiter.keys[1])
	}
}

// A call that cannot send mail is not counted: a bad email or role is a
// 400 from the use case, and must not spend the store's budget.
func TestTenancy_InviteDoesNotCountBadInput(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"bad email": `{"email":"not an email","role":"staff"}`,
		"bad role":  `{"email":"caio@example.com","role":"owner"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newTenancyFixture(t)
			f.uc.err = shared.NewValidationError(shared.FieldError{Field: "email", Message: "x"})
			f.call(t, staffIn(t, shared.RoleOwner), "POST", "/api/v1/admin/invitations", body)
			if len(f.limiter.keys) != 0 {
				t.Errorf("counted %v for input the use case refuses", f.limiter.keys)
			}
		})
	}
}

// A caller whose token cannot manage the team, or who has no store, is
// not counted: the use case refuses them, and their refused calls must not
// spend the store's budget.
func TestTenancy_InviteDoesNotCountCallersWhoCannotInvite(t *testing.T) {
	t.Parallel()

	for name, caller := range map[string]func(*testing.T) shared.Principal{
		"viewer":   func(t *testing.T) shared.Principal { return staffIn(t, shared.RoleViewer) },
		"staff":    func(t *testing.T) shared.Principal { return staffIn(t, shared.RoleStaff) },
		"no store": storeless,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newTenancyFixture(t)
			f.uc.err = shared.ErrForbidden
			f.call(t, caller(t), "POST", "/api/v1/admin/invitations", `{"email":"caio@example.com","role":"staff"}`)
			if len(f.limiter.keys) != 0 {
				t.Errorf("counted %v for a caller who cannot invite", f.limiter.keys)
			}
		})
	}
}

func TestNewTenancy_RefusesMissingDependencies(t *testing.T) {
	t.Parallel()

	p, _ := ratelimit.NewPseudonyms([]byte(strings.Repeat("k", ratelimit.MinKeyLength)))
	full := handler.TenancyDeps{UseCases: &tenancyCases{}, Limiter: &limiter{}, Pseudonyms: p, Log: discardLog()}
	if _, err := handler.NewTenancy(full); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*handler.TenancyDeps){
		"use cases":  func(d *handler.TenancyDeps) { d.UseCases = nil },
		"limiter":    func(d *handler.TenancyDeps) { d.Limiter = nil },
		"pseudonyms": func(d *handler.TenancyDeps) { d.Pseudonyms = ratelimit.Pseudonyms{} },
		"logger":     func(d *handler.TenancyDeps) { d.Log = nil },
	} {
		d := full
		change(&d)
		if _, err := handler.NewTenancy(d); err == nil {
			t.Errorf("NewTenancy accepted no %s", name)
		}
	}
}
