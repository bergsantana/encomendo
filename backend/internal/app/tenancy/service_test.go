package tenancy_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/app/tenancy/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

var (
	start   = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	errBoom = errors.New("postgres is down")
)

type harness struct {
	svc       *apptenancy.Service
	lookup    *apptenancy.Lookup
	store     *fake.Store
	users     *fake.Users
	workflows *fake.Workflows
	mailer    *fake.Mailer
	events    *sharedfake.Events
	tx        *sharedfake.TxManager
	clock     *sharedfake.Clock
	ids       *sharedfake.IDs
	secrets   *sharedfake.Secrets
	preset    tenancy.WorkflowRef
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	clock := sharedfake.NewClock(start)
	ids := sharedfake.NewIDs(clock)
	h := &harness{
		store: fake.NewStore(), users: fake.NewUsers(), mailer: fake.NewMailer(), events: sharedfake.NewEvents(),
		tx: sharedfake.NewTxManager(), clock: clock, ids: ids, secrets: sharedfake.NewSecrets(),
		preset: tenancy.WorkflowRef{ID: shared.WorkflowID(ids.NewID()), Version: 1, Name: "Encomenda com montagem"},
	}
	h.workflows = fake.NewWorkflows(h.preset)
	deps := apptenancy.Deps{
		Tenants: h.store, Members: h.store, Invitations: h.store,
		Users: h.users, Workflows: h.workflows, Mailer: h.mailer,
		Tx: h.tx, Clock: h.clock, IDs: h.ids, Secrets: h.secrets, Events: h.events,
		Slugs: tenancy.NewSlugPolicy("painel", "api-encomendo"),
	}
	h.svc = apptenancy.NewService(deps)
	h.lookup = apptenancy.NewLookup(deps)
	return h
}

var _ apptenancy.UseCases = (*apptenancy.Service)(nil)

// user adds a staff account to the directory.
func (h *harness) user(t *testing.T, name, email string) shared.UserID {
	t.Helper()
	id := shared.UserID(h.ids.NewID())
	h.users.Add(apptenancy.UserSummary{ID: id, Name: name, Email: email})
	return id
}

// as returns a context of a staff user acting in tenant with role, as the
// authn middleware builds it.
func as(t *testing.T, user shared.UserID, tenant shared.TenantID, role shared.Role) context.Context {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(user, tenant, role)
	if err != nil {
		t.Fatal(err)
	}
	return authz.WithPrincipal(context.Background(), p)
}

// newStore creates a store owned by a new user and returns both.
func (h *harness) newStore(t *testing.T, slug string) (shared.TenantID, shared.UserID) {
	t.Helper()
	owner := h.user(t, "Dona "+slug, slug+"@ex.com")
	tv, err := h.svc.CreateTenant(as(t, owner, shared.TenantID{}, ""), apptenancy.CreateTenantInput{Name: "Loja " + slug, Slug: slug})
	if err != nil {
		t.Fatalf("CreateTenant(%s): %v", slug, err)
	}
	return tv.ID, owner
}

// join adds user to tenant with role, directly in the store.
func (h *harness) join(t *testing.T, tenant shared.TenantID, user shared.UserID, role shared.Role) {
	t.Helper()
	ctx := appshared.WithScope(context.Background(), appshared.Scope{TenantID: tenant})
	err := h.tx.WithinTx(ctx, func(ctx context.Context) error {
		inv, err := tenancy.NewInvitation(tenancy.InvitationSpec{
			ID: shared.InvitationID(h.ids.NewID()), Tenant: tenant, Email: mustEmail(t, "x@ex.com"), Role: role,
			Hash: shared.Secret{9}.Hash(), InvitedBy: user,
		}, h.clock.Now())
		if err != nil {
			return err
		}
		m, err := tenancy.NewMembershipFrom(inv, user, h.clock.Now())
		if err != nil {
			return err
		}
		return h.store.InsertMembership(ctx, m)
	})
	if err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Second) // members join in a clear order
}

func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a ValidationError", err)
	}
	var names []string
	for _, f := range ve.Fields {
		names = append(names, f.Field)
	}
	return names
}

// ---- CreateTenant ----

func TestCreateTenant_MakesTheCallerOwnerWithDefaults(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	owner := h.user(t, "Ana", "ana@ex.com")

	tv, err := h.svc.CreateTenant(as(t, owner, shared.TenantID{}, ""), apptenancy.CreateTenantInput{Name: " Alta RPM ", Slug: "altarpm"})
	if err != nil {
		t.Fatal(err)
	}

	if tv.Name != "Alta RPM" || tv.Slug != "altarpm" || !tv.CreatedAt.Equal(start) || tv.ID.IsZero() {
		t.Errorf("tenant = %+v", tv)
	}
	m, ok := h.store.Member(tv.ID, owner)
	if !ok || m.Role != shared.RoleOwner {
		t.Errorf("owner membership = %+v, %v", m, ok)
	}
	// The new store's own scope, with the creator for the audit trail
	// (ADR 0009).
	if got := h.tx.Scopes(); !slices.Equal(got, []appshared.Scope{{TenantID: tv.ID, UserID: owner}}) {
		t.Errorf("scopes = %+v", got)
	}
	s, err := h.svc.Settings(as(t, owner, tv.ID, shared.RoleOwner))
	if err != nil {
		t.Fatal(err)
	}
	if s.Workflow != h.preset || s.Editable.DepositPercent != 50 || s.Currency != "BRL" || s.Version != 1 {
		t.Errorf("settings = %+v; want the defaults and the default workflow", s)
	}
	if got := h.events.Names(); !slices.Equal(got, []string{"tenancy.tenant_created"}) {
		t.Errorf("events = %v", got)
	}
	if e, _ := h.events.Published()[0].(tenancy.TenantCreated); e.Owner != owner || e.Slug != "altarpm" || h.events.Records()[0].TenantID != tv.ID {
		t.Errorf("event = %+v", h.events.Records()[0])
	}
}

func TestCreateTenant_WorksFromAnotherStoreToo(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	first, owner := h.newStore(t, "primeira")

	tv, err := h.svc.CreateTenant(as(t, owner, first, shared.RoleOwner), apptenancy.CreateTenantInput{Name: "Segunda", Slug: "segunda"})

	if err != nil || tv.ID == first {
		t.Fatalf("CreateTenant from a store: %+v, %v", tv, err)
	}
}

func TestCreateTenant_Refuses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.newStore(t, "altarpm")
	owner := h.user(t, "Bia", "bia@ex.com")
	ctx := as(t, owner, shared.TenantID{}, "")

	_, taken := h.svc.CreateTenant(ctx, apptenancy.CreateTenantInput{Name: "Outra", Slug: "altarpm"})
	_, bad := h.svc.CreateTenant(ctx, apptenancy.CreateTenantInput{Name: "", Slug: "Alta RPM"})
	_, reserved := h.svc.CreateTenant(ctx, apptenancy.CreateTenantInput{Name: "Painel", Slug: "painel"})
	_, anonymous := h.svc.CreateTenant(context.Background(), apptenancy.CreateTenantInput{Name: "X", Slug: "xyz"})
	client, _ := shared.NewClientPrincipal(shared.ClientAccountID(h.ids.NewID()))
	_, asClient := h.svc.CreateTenant(authz.WithPrincipal(context.Background(), client), apptenancy.CreateTenantInput{Name: "X", Slug: "xyz"})

	if !errors.Is(taken, tenancy.ErrSlugTaken) {
		t.Errorf("taken slug: %v, want ErrSlugTaken", taken)
	}
	if got := fieldsOf(t, bad); !slices.Equal(got, []string{"name", "slug"}) {
		t.Errorf("bad input: fields %v", got)
	}
	if got := fieldsOf(t, reserved); !slices.Equal(got, []string{"slug"}) {
		t.Errorf("the admin host's label: fields %v", got)
	}
	if !errors.Is(anonymous, shared.ErrUnauthenticated) || !errors.Is(asClient, shared.ErrForbidden) {
		t.Errorf("no caller: %v; client: %v", anonymous, asClient)
	}
}

func TestCreateTenant_FailuresStayServerErrors(t *testing.T) {
	t.Parallel()

	for name, breakIt := range map[string]func(h *harness){
		"workflow":   func(h *harness) { h.workflows.Fail(errBoom) },
		"tenant":     func(h *harness) { h.store.Fail("InsertTenant", errBoom) },
		"settings":   func(h *harness) { h.store.Fail("InsertSettings", errBoom) },
		"membership": func(h *harness) { h.store.Fail("InsertMembership", errBoom) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			owner := h.user(t, "Ana", "ana@ex.com")
			breakIt(h)
			_, err := h.svc.CreateTenant(as(t, owner, shared.TenantID{}, ""), apptenancy.CreateTenantInput{Name: "X", Slug: "xyz"})
			if !errors.Is(err, errBoom) || errors.Is(err, shared.ErrConflict) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

// ---- Settings ----

func TestSettings_EveryRoleReadsThem(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	store, _ := h.newStore(t, "altarpm")
	viewer := h.user(t, "Vi", "vi@ex.com")
	h.join(t, store, viewer, shared.RoleViewer)

	s, err := h.svc.Settings(as(t, viewer, store, shared.RoleViewer))

	if err != nil || s.Version != 1 {
		t.Errorf("viewer reading settings: %+v, %v", s, err)
	}
	if _, err := h.svc.Settings(as(t, viewer, shared.TenantID{}, "")); !errors.Is(err, authz.ErrTenantRequired) {
		t.Errorf("no store selected: %v, want ErrTenantRequired", err)
	}
}

func TestUpdateSettings(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	store, owner := h.newStore(t, "altarpm")
	ctx := as(t, owner, store, shared.RoleOwner)
	next := tenancy.Editable{StockEnabled: true, DepositPercent: 30, AssemblyDays: 5, SafetyMarginDays: 1, WhatsAppPhone: "5592991234567"}

	saved, err := h.svc.UpdateSettings(ctx, apptenancy.UpdateSettingsInput{Settings: next, Version: 1})
	if err != nil {
		t.Fatal(err)
	}

	if saved.Editable != next || saved.Version != 2 {
		t.Errorf("saved = %+v; want the new settings at version 2", saved)
	}
	if read, _ := h.svc.Settings(ctx); read != saved {
		t.Errorf("read back %+v, want %+v", read, saved)
	}
	if _, err := h.svc.UpdateSettings(ctx, apptenancy.UpdateSettingsInput{Settings: next, Version: 1}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("stale version: %v, want ErrVersionConflict", err)
	}
	_, err = h.svc.UpdateSettings(ctx, apptenancy.UpdateSettingsInput{Settings: tenancy.Editable{DepositPercent: 101}, Version: 2})
	if got := fieldsOf(t, err); !slices.Equal(got, []string{"depositPercent"}) {
		t.Errorf("invalid: fields %v", got)
	}
	if ev, ok := h.events.Published()[len(h.events.Published())-1].(tenancy.SettingsUpdated); !ok || ev.Version != 2 || ev.By != owner {
		t.Errorf("last event = %+v; want settings_updated at version 2 by the owner", h.events.Published()[len(h.events.Published())-1])
	}
}

// TestSettings_AMissingRowIsABug: every store has settings, so their
// absence is a 500, never a 404 the apps would read as "no such store".
func TestSettings_AMissingRowIsABug(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	store, owner := h.newStore(t, "altarpm")
	h.store.Fail("SettingsOf", shared.ErrNotFound)
	ctx := as(t, owner, store, shared.RoleOwner)

	_, readErr := h.svc.Settings(ctx)
	_, saveErr := h.svc.UpdateSettings(ctx, apptenancy.UpdateSettingsInput{Version: 1})

	for name, err := range map[string]error{"read": readErr, "update": saveErr} {
		if err == nil || errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%s: %v, want an unmatched error", name, err)
		}
	}
}

func TestUpdateSettings_OnlyTheOwner(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	store, _ := h.newStore(t, "altarpm")
	admin := h.user(t, "Adm", "adm@ex.com")
	h.join(t, store, admin, shared.RoleAdmin)

	_, err := h.svc.UpdateSettings(as(t, admin, store, shared.RoleAdmin), apptenancy.UpdateSettingsInput{Version: 1})

	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("admin: %v, want ErrForbidden", err)
	}
}

// CheckStore refuses a suspended store, passes an active one, and leaves a
// session without a store to the use cases' tenant_required (ADR 0022).
func TestService_CheckStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	active, owner := h.newStore(t, "ativa")
	paused, pausedOwner := h.newStore(t, "pausada")
	h.store.Suspend(paused)

	if err := h.svc.CheckStore(as(t, owner, active, shared.RoleOwner)); err != nil {
		t.Errorf("active store: %v", err)
	}
	if err := h.svc.CheckStore(as(t, pausedOwner, paused, shared.RoleOwner)); !errors.Is(err, tenancy.ErrTenantSuspended) {
		t.Errorf("suspended store: %v, want ErrTenantSuspended", err)
	}
	if err := h.svc.CheckStore(as(t, owner, shared.TenantID{}, "")); err != nil {
		t.Errorf("no store selected: %v, want nil", err)
	}
	if err := h.svc.CheckStore(context.Background()); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("no caller: %v", err)
	}
}

// A status that cannot be read fails the check, so the request fails closed.
func TestService_CheckStoreFailsClosed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	store, owner := h.newStore(t, "falha")
	h.store.Fail("TenantByID", errBoom)

	err := h.svc.CheckStore(as(t, owner, store, shared.RoleOwner))
	if !errors.Is(err, errBoom) || !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("CheckStore = %v, want the read's failure as ErrUnavailable (a 503)", err)
	}
}

// Suspended reaches the summaries identity reads through the lookup.
func TestLookup_MembershipsCarrySuspension(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	store, owner := h.newStore(t, "marcada")
	h.store.Suspend(store)

	ctx := appshared.WithScope(context.Background(), appshared.Scope{UserID: owner})
	var roles []apptenancy.StoreRole
	err := h.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		roles, err = h.lookup.MembershipsOf(ctx, owner)
		return err
	})
	if err != nil || len(roles) != 1 || !roles[0].Tenant.Suspended {
		t.Errorf("MembershipsOf = %+v, %v; want the store marked suspended", roles, err)
	}
}
