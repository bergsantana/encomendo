package tenancyports

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// Harness is the tenancy repositories with what the suite needs to use
// them.
type Harness struct {
	Tenants     apptenancy.Tenants
	Members     apptenancy.Members
	Invitations apptenancy.Invitations
	Tx          appshared.TxManager
	NewID       func() shared.ID
	// NewUser returns a stored account that memberships and invitations
	// may point at.
	NewUser func(t *testing.T) shared.UserID
	// NewSlug returns a slug no store uses yet.
	NewSlug func(t *testing.T) string
	// Workflow is a published preset settings may point at.
	Workflow tenancy.WorkflowRef
	// Now is a recent time, at the store's precision (microseconds for
	// Postgres); every timestamp is built from it.
	Now time.Time
}

func (h Harness) in(t *testing.T, s appshared.Scope, fn func(ctx context.Context) error) {
	t.Helper()
	if err := h.try(s, fn); err != nil {
		t.Fatal(err)
	}
}

func (h Harness) try(s appshared.Scope, fn func(ctx context.Context) error) error {
	return h.Tx.WithinTx(appshared.WithScope(context.Background(), s), fn)
}

func storeScope(tenant shared.TenantID) appshared.Scope { return appshared.Scope{TenantID: tenant} }

// store creates a store named name, with settings and an owner, in its own
// scope, as CreateTenant does.
func (h Harness) store(t *testing.T, name string) (*tenancy.Tenant, shared.UserID) {
	t.Helper()
	n, err := tenancy.ParseStoreName(name)
	if err != nil {
		t.Fatal(err)
	}
	slug, err := tenancy.RestoreSlug(h.NewSlug(t))
	if err != nil {
		t.Fatal(err)
	}
	tn, err := tenancy.NewTenant(shared.TenantID(h.NewID()), n, slug, h.Now)
	if err != nil {
		t.Fatal(err)
	}
	owner := h.NewUser(t)
	h.in(t, storeScope(tn.ID()), func(ctx context.Context) error {
		if err := h.Tenants.InsertTenant(ctx, tn); err != nil {
			return err
		}
		settings, err := tenancy.DefaultSettings(tn.ID(), h.Workflow)
		if err != nil {
			return err
		}
		if err := h.Tenants.InsertSettings(ctx, settings); err != nil {
			return err
		}
		m, err := tenancy.NewOwnerMembership(tn.ID(), owner, h.Now)
		if err != nil {
			return err
		}
		return h.Members.InsertMembership(ctx, m)
	})
	return tn, owner
}

// member adds a new user to tenant with role, joined at joined.
func (h Harness) member(t *testing.T, tenant shared.TenantID, role shared.Role, joined time.Time) shared.UserID {
	t.Helper()
	user := h.NewUser(t)
	m, err := tenancy.RestoreMembership(tenancy.MembershipRecord{Tenant: tenant, User: user, Role: role, Version: 1, JoinedAt: joined})
	if err != nil {
		t.Fatal(err)
	}
	h.in(t, storeScope(tenant), func(ctx context.Context) error { return h.Members.InsertMembership(ctx, m) })
	return user
}

// RunRepositoryContract checks the tenancy repositories: what each scope
// sees and may write (RLS, 0007_rls_policies), the keys of 0004_tenancy,
// optimistic updates, paging, and the conditional invitation updates.
func RunRepositoryContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("a store and its settings read back in its own scope only", func(t *testing.T) { tenantsReadBack(t, newHarness(t)) })
	t.Run("slugs are unique across stores", func(t *testing.T) { tenantsUniqueSlug(t, newHarness(t)) })
	t.Run("rows cannot be written into another store", func(t *testing.T) { writesStayInScope(t, newHarness(t)) })
	t.Run("settings updates are optimistic", func(t *testing.T) { settingsUpdate(t, newHarness(t)) })
	t.Run("a team has one membership per person and one owner", func(t *testing.T) { membersKeys(t, newHarness(t)) })
	t.Run("the team pages oldest first", func(t *testing.T) { membersPage(t, newHarness(t)) })
	t.Run("roles change optimistically and members leave", func(t *testing.T) { membersChange(t, newHarness(t)) })
	t.Run("a user's stores are visible in their own scope", func(t *testing.T) { membershipsOfUser(t, newHarness(t)) })
	t.Run("invitations read back and keep one open per email", func(t *testing.T) { invitationsReadBack(t, newHarness(t)) })
	t.Run("pending invitations page newest first", func(t *testing.T) { invitationsPending(t, newHarness(t)) })
	t.Run("invitation updates are conditional", func(t *testing.T) { invitationsConditional(t, newHarness(t)) })
	t.Run("needs a transaction", func(t *testing.T) { needsTx(t, newHarness(t)) })
}

func tenantsReadBack(t *testing.T, h Harness) {
	tn, _ := h.store(t, "Alta RPM")
	other, _ := h.store(t, "Outra")
	h.in(t, storeScope(tn.ID()), func(ctx context.Context) error {
		got, err := h.Tenants.TenantByID(ctx, tn.ID())
		if err != nil || got.Slug() != tn.Slug() || got.Name() != tn.Name() || got.Status() != tenancy.StatusActive || !got.CreatedAt().Equal(h.Now) {
			t.Errorf("TenantByID = %+v, %v", got, err)
		}
		s, err := h.Tenants.SettingsOf(ctx, tn.ID())
		if err != nil || s.Workflow() != h.Workflow || s.Editable().DepositPercent != 50 || s.Currency() != "BRL" || s.Version() != 1 {
			t.Errorf("SettingsOf = %+v, %v; want the defaults with the workflow's name", s, err)
		}
		if _, err := h.Tenants.TenantByID(ctx, other.ID()); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another store's row: %v, want ErrNotFound", err)
		}
		if _, err := h.Tenants.SettingsOf(ctx, other.ID()); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another store's settings: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func tenantsUniqueSlug(t *testing.T, h Harness) {
	tn, _ := h.store(t, "Alta")
	name, _ := tenancy.ParseStoreName("Copia")
	twin, err := tenancy.NewTenant(shared.TenantID(h.NewID()), name, tn.Slug(), h.Now)
	if err != nil {
		t.Fatal(err)
	}
	err = h.try(storeScope(twin.ID()), func(ctx context.Context) error { return h.Tenants.InsertTenant(ctx, twin) })
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second store with slug %q: %v, want ErrConflict", tn.Slug(), err)
	}
}

func writesStayInScope(t *testing.T, h Harness) {
	a, _ := h.store(t, "A")
	b, _ := h.store(t, "B")
	name, _ := tenancy.ParseStoreName("Intrusa")
	slug, _ := tenancy.RestoreSlug(h.NewSlug(t))
	intruder, err := tenancy.NewTenant(shared.TenantID(h.NewID()), name, slug, h.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.try(storeScope(a.ID()), func(ctx context.Context) error { return h.Tenants.InsertTenant(ctx, intruder) }); err == nil {
		t.Error("store A created another store's row")
	}
	m, _ := tenancy.RestoreMembership(tenancy.MembershipRecord{Tenant: b.ID(), User: h.NewUser(t), Role: shared.RoleAdmin, Version: 1, JoinedAt: h.Now})
	if err := h.try(storeScope(a.ID()), func(ctx context.Context) error { return h.Members.InsertMembership(ctx, m) }); err == nil {
		t.Error("store A added a member to store B")
	}
}

func settingsUpdate(t *testing.T, h Harness) {
	tn, _ := h.store(t, "Alta")
	h.in(t, storeScope(tn.ID()), func(ctx context.Context) error {
		s, err := h.Tenants.SettingsOf(ctx, tn.ID())
		if err != nil {
			return err
		}
		next := tenancy.Editable{ProductionEnabled: true, DepositPercent: 0, AssemblyDays: 0, SafetyMarginDays: 7, WhatsAppPhone: "5592991234567"}
		if err := s.Change(next, 1); err != nil {
			return err
		}
		if err := h.Tenants.UpdateSettings(ctx, s); err != nil {
			t.Errorf("UpdateSettings: %v", err)
		}
		got, err := h.Tenants.SettingsOf(ctx, tn.ID())
		if err != nil || got.Editable() != next || got.Version() != 2 {
			t.Errorf("after the update: %+v, %v; want the new values, zeros and false included, at version 2", got, err)
		}
		if err := h.Tenants.UpdateSettings(ctx, s); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("a stale update: %v, want ErrVersionConflict", err)
		}
		return nil
	})
}

func membersKeys(t *testing.T, h Harness) {
	tn, owner := h.store(t, "Alta")
	user := h.member(t, tn.ID(), shared.RoleStaff, h.Now)
	again, _ := tenancy.RestoreMembership(tenancy.MembershipRecord{Tenant: tn.ID(), User: user, Role: shared.RoleViewer, Version: 1, JoinedAt: h.Now})
	if err := h.try(storeScope(tn.ID()), func(ctx context.Context) error { return h.Members.InsertMembership(ctx, again) }); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second membership: %v, want ErrConflict", err)
	}
	second, _ := tenancy.NewOwnerMembership(tn.ID(), h.NewUser(t), h.Now)
	if err := h.try(storeScope(tn.ID()), func(ctx context.Context) error { return h.Members.InsertMembership(ctx, second) }); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second owner: %v, want ErrConflict", err)
	}
	h.in(t, storeScope(tn.ID()), func(ctx context.Context) error {
		m, err := h.Members.MembershipOf(ctx, tn.ID(), owner)
		if err != nil || m.Role() != shared.RoleOwner || m.Version() != 1 || !m.JoinedAt().Equal(h.Now) {
			t.Errorf("MembershipOf(owner) = %+v, %v", m, err)
		}
		if _, err := h.Members.MembershipOf(ctx, tn.ID(), h.NewUser(t)); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("not a member: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func membersPage(t *testing.T, h Harness) {
	tn, owner := h.store(t, "Alta")
	a := h.member(t, tn.ID(), shared.RoleAdmin, h.Now.Add(time.Minute))
	b := h.member(t, tn.ID(), shared.RoleViewer, h.Now.Add(2*time.Minute))
	page := func(cursor string) shared.PageRequest {
		p, err := shared.NewPageRequest(cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	h.in(t, storeScope(tn.ID()), func(ctx context.Context) error {
		first, err := h.Members.Memberships(ctx, tn.ID(), page(""))
		if err != nil || len(first.Items) != 2 || first.Items[0].UserID() != owner || first.Items[1].UserID() != a || !first.HasMore() {
			t.Fatalf("first page = %+v, %v; want owner then admin, with more", first, err)
		}
		rest, err := h.Members.Memberships(ctx, tn.ID(), page(first.NextCursor))
		if err != nil || len(rest.Items) != 1 || rest.Items[0].UserID() != b || rest.HasMore() {
			t.Errorf("second page = %+v, %v; want the viewer, last", rest, err)
		}
		if _, err := h.Members.Memberships(ctx, tn.ID(), page("not-a-cursor")); !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("a cursor it did not issue: %v, want ErrInvalid", err)
		}
		return nil
	})
}

func membersChange(t *testing.T, h Harness) {
	tn, _ := h.store(t, "Alta")
	user := h.member(t, tn.ID(), shared.RoleStaff, h.Now)
	h.in(t, storeScope(tn.ID()), func(ctx context.Context) error {
		m, err := h.Members.MembershipOf(ctx, tn.ID(), user)
		if err != nil {
			return err
		}
		stale := *m
		if err := m.ChangeRole(shared.RoleAdmin); err != nil {
			return err
		}
		if err := h.Members.UpdateRole(ctx, m); err != nil {
			t.Errorf("UpdateRole: %v", err)
		}
		if got, _ := h.Members.MembershipOf(ctx, tn.ID(), user); got == nil || got.Role() != shared.RoleAdmin || got.Version() != 2 {
			t.Errorf("after the change: %+v", got)
		}
		if err := h.Members.UpdateRole(ctx, &stale); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("a stale change: %v, want ErrVersionConflict", err)
		}
		if err := h.Members.DeleteMembership(ctx, tn.ID(), user); err != nil {
			t.Errorf("DeleteMembership: %v", err)
		}
		if err := h.Members.DeleteMembership(ctx, tn.ID(), user); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("deleting twice: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func membershipsOfUser(t *testing.T, h Harness) {
	beta, _ := h.store(t, "Beta")
	alfa, _ := h.store(t, "Alfa")
	h.store(t, "Gama")
	user := h.member(t, beta.ID(), shared.RoleStaff, h.Now)
	h.in(t, storeScope(alfa.ID()), func(ctx context.Context) error {
		m, _ := tenancy.RestoreMembership(tenancy.MembershipRecord{Tenant: alfa.ID(), User: user, Role: shared.RoleViewer, Version: 1, JoinedAt: h.Now})
		return h.Members.InsertMembership(ctx, m)
	})
	check := func(s appshared.Scope, want ...*tenancy.Tenant) {
		t.Helper()
		h.in(t, s, func(ctx context.Context) error {
			got, err := h.Members.MembershipsOfUser(ctx, user)
			if err != nil || got == nil || len(got) != len(want) {
				t.Errorf("scope %+v: %d memberships, %v; want %d", s, len(got), err, len(want))
				return nil
			}
			for i, w := range want {
				if got[i].Tenant.ID() != w.ID() || got[i].Tenant.Name() != w.Name() || got[i].Membership.UserID() != user {
					t.Errorf("scope %+v, #%d = %v, want %v", s, i, got[i].Tenant.Name(), w.Name())
				}
			}
			return nil
		})
	}
	check(appshared.Scope{UserID: user}, alfa, beta)
	check(appshared.Scope{TenantID: beta.ID(), UserID: user}, beta)
	check(appshared.Scope{UserID: h.NewUser(t)})
}

func (h Harness) invitation(t *testing.T, tenant shared.TenantID, sender shared.UserID, email string, created time.Time) *tenancy.Invitation {
	t.Helper()
	addr, err := identity.ParseEmail(email)
	if err != nil {
		t.Fatal(err)
	}
	var secret shared.Secret
	id := h.NewID()
	copy(secret[:], id[:])
	inv, err := tenancy.NewInvitation(tenancy.InvitationSpec{
		ID: shared.InvitationID(h.NewID()), Tenant: tenant, Email: addr, Role: shared.RoleStaff, Hash: secret.Hash(), InvitedBy: sender,
	}, created)
	if err != nil {
		t.Fatal(err)
	}
	h.in(t, storeScope(tenant), func(ctx context.Context) error { return h.Invitations.InsertInvitation(ctx, inv) })
	return inv
}

func invitationsReadBack(t *testing.T, h Harness) {
	tn, owner := h.store(t, "Alta")
	other, _ := h.store(t, "Outra")
	inv := h.invitation(t, tn.ID(), owner, "novo@ex.com", h.Now.Add(-8*24*time.Hour)) // expired, still open
	shouting, _ := identity.ParseEmail("NOVO@EX.COM")
	h.in(t, storeScope(tn.ID()), func(ctx context.Context) error {
		for name, get := range map[string]func() (*tenancy.Invitation, error){
			"by id":   func() (*tenancy.Invitation, error) { return h.Invitations.InvitationByID(ctx, tn.ID(), inv.ID()) },
			"by hash": func() (*tenancy.Invitation, error) { return h.Invitations.InvitationByHash(ctx, tn.ID(), inv.Hash()) },
			"by email": func() (*tenancy.Invitation, error) {
				return h.Invitations.OpenInvitationByEmail(ctx, tn.ID(), shouting)
			},
		} {
			got, err := get()
			if err != nil || got.ID() != inv.ID() || got.InvitedBy() != owner || !got.ExpiresAt().Equal(inv.ExpiresAt()) {
				t.Errorf("%s = %+v, %v", name, got, err)
			}
		}
		return nil
	})
	twin, _ := identity.ParseEmail("Novo@Ex.com")
	dup, _ := tenancy.NewInvitation(tenancy.InvitationSpec{
		ID: shared.InvitationID(h.NewID()), Tenant: tn.ID(), Email: twin, Role: shared.RoleViewer, Hash: shared.Secret{3}.Hash(), InvitedBy: owner,
	}, h.Now)
	if err := h.try(storeScope(tn.ID()), func(ctx context.Context) error { return h.Invitations.InsertInvitation(ctx, dup) }); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second open invitation for the email: %v, want ErrConflict", err)
	}
	h.in(t, storeScope(other.ID()), func(ctx context.Context) error {
		if _, err := h.Invitations.InvitationByID(ctx, tn.ID(), inv.ID()); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("from another store: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func invitationsPending(t *testing.T, h Harness) {
	tn, owner := h.store(t, "Alta")
	h.invitation(t, tn.ID(), owner, "velho@ex.com", h.Now.Add(-8*24*time.Hour)) // expired
	revoked := h.invitation(t, tn.ID(), owner, "revogado@ex.com", h.Now.Add(-3*time.Hour))
	a := h.invitation(t, tn.ID(), owner, "a@ex.com", h.Now.Add(-2*time.Hour))
	b := h.invitation(t, tn.ID(), owner, "b@ex.com", h.Now.Add(-time.Hour))
	h.in(t, storeScope(tn.ID()), func(ctx context.Context) error {
		if _, err := h.Invitations.MarkInvitationRevoked(ctx, tn.ID(), revoked.ID(), h.Now); err != nil {
			return err
		}
		p1, _ := shared.NewPageRequest("", 1)
		first, err := h.Invitations.PendingInvitations(ctx, tn.ID(), h.Now, p1)
		if err != nil || len(first.Items) != 1 || first.Items[0].ID() != b.ID() || !first.HasMore() {
			t.Fatalf("first page = %+v, %v; want b, with more", first, err)
		}
		p2, _ := shared.NewPageRequest(first.NextCursor, 10)
		rest, err := h.Invitations.PendingInvitations(ctx, tn.ID(), h.Now, p2)
		var ids []shared.InvitationID
		for _, inv := range rest.Items {
			ids = append(ids, inv.ID())
		}
		if err != nil || !slices.Equal(ids, []shared.InvitationID{a.ID()}) || rest.HasMore() {
			t.Errorf("second page = %v, %v; want only a", ids, err)
		}
		return nil
	})
}

func invitationsConditional(t *testing.T, h Harness) {
	tn, owner := h.store(t, "Alta")
	admin := h.member(t, tn.ID(), shared.RoleAdmin, h.Now)
	fresh := h.invitation(t, tn.ID(), owner, "a@ex.com", h.Now)
	expired := h.invitation(t, tn.ID(), owner, "b@ex.com", h.Now.Add(-8*24*time.Hour))
	byAdmin1 := h.invitation(t, tn.ID(), admin, "c@ex.com", h.Now)
	byAdmin2 := h.invitation(t, tn.ID(), admin, "d@ex.com", h.Now.Add(-8*24*time.Hour))
	at := h.Now.Add(time.Minute)
	h.in(t, storeScope(tn.ID()), func(ctx context.Context) error {
		if ok, err := h.Invitations.MarkInvitationAccepted(ctx, tn.ID(), expired.ID(), at); err != nil || ok {
			t.Errorf("accepting an expired invitation = %v, %v; want false", ok, err)
		}
		for i, want := range []bool{true, false} {
			if ok, err := h.Invitations.MarkInvitationAccepted(ctx, tn.ID(), fresh.ID(), at); err != nil || ok != want {
				t.Errorf("accept #%d = %v, %v; want %v", i+1, ok, err, want)
			}
		}
		if ok, err := h.Invitations.MarkInvitationRevoked(ctx, tn.ID(), fresh.ID(), at); err != nil || ok {
			t.Errorf("revoking an accepted invitation = %v, %v; want false", ok, err)
		}
		ids, err := h.Invitations.RevokeInvitationsBy(ctx, tn.ID(), admin, at)
		want := []shared.InvitationID{byAdmin1.ID(), byAdmin2.ID()}
		slices.SortFunc(ids, func(a, b shared.InvitationID) int { return compare(a.String(), b.String()) })
		slices.SortFunc(want, func(a, b shared.InvitationID) int { return compare(a.String(), b.String()) })
		if err != nil || !slices.Equal(ids, want) {
			t.Errorf("RevokeInvitationsBy = %v, %v; want both of the admin's, expired included", ids, err)
		}
		if still, _ := h.Invitations.InvitationByID(ctx, tn.ID(), expired.ID()); still == nil || !still.RevokedAt().IsZero() {
			t.Error("the owner's invitation was revoked too")
		}
		return nil
	})
}

func compare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func needsTx(t *testing.T, h Harness) {
	if _, err := h.Tenants.TenantByID(context.Background(), shared.TenantID(h.NewID())); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("outside a transaction: %v, want ErrNoTransaction", err)
	}
}
