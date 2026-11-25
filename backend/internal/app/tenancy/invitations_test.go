package tenancy_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

func (h *harness) inviteAs(ctx context.Context, t *testing.T, email string, role shared.Role) apptenancy.InvitationView {
	t.Helper()
	inv, err := h.svc.InviteMember(ctx, apptenancy.InviteMemberInput{Email: email, Role: string(role)})
	if err != nil {
		t.Fatalf("InviteMember(%s): %v", email, err)
	}
	return inv
}

func TestInviteMember_StoresOnlyTheHashAndEmailsTheLink(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)

	inv := h.inviteAs(tm.as(t, tm.admin, shared.RoleAdmin), t, " Novo@Ex.com ", shared.RoleStaff)

	if inv.Email != "Novo@Ex.com" || inv.Role != shared.RoleStaff || !inv.InvitedAt.Equal(h.clock.Now()) ||
		!inv.ExpiresAt.Equal(h.clock.Now().Add(7*24*time.Hour)) {
		t.Errorf("invitation = %+v", inv)
	}
	sent := h.mailer.Sent()
	if len(sent) != 1 {
		t.Fatalf("%d emails, want 1", len(sent))
	}
	e := sent[0]
	if e.To.String() != "Novo@Ex.com" || e.Store != "Loja altarpm" || e.InvitedBy != "Adm" || e.Role != shared.RoleStaff {
		t.Errorf("email = %+v", e)
	}
	// The link carries the store and the secret; only the hash is stored
	// (ADR 0011).
	tok, err := tenancy.ParseInvitationToken(e.Token.String())
	if err != nil || tok.TenantID() != tm.store {
		t.Fatalf("the emailed token: %v, store %v", err, tok.TenantID())
	}
	stored, _ := h.store.Invitation(inv.ID)
	if stored.Hash != tok.Hash() || stored.InvitedBy != tm.admin {
		t.Error("the stored invitation does not match the emailed link")
	}
	if ev, ok := h.events.Published()[len(h.events.Published())-1].(tenancy.MemberInvited); !ok || ev.InvitationID != inv.ID {
		t.Errorf("last event = %+v", h.events.Published()[len(h.events.Published())-1])
	}
}

func TestInviteMember_AgainReplacesThePendingInvitation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	ctx := tm.as(t, tm.owner, shared.RoleOwner)
	first := h.inviteAs(ctx, t, "novo@ex.com", shared.RoleStaff)
	h.clock.Advance(time.Hour)

	second := h.inviteAs(ctx, t, "NOVO@ex.com", shared.RoleViewer)

	if old, _ := h.store.Invitation(first.ID); old.RevokedAt.IsZero() {
		t.Error("the first invitation still works")
	}
	if second.ID == first.ID || second.Role != shared.RoleViewer || !second.ExpiresAt.Equal(h.clock.Now().Add(7*24*time.Hour)) {
		t.Errorf("second = %+v; want a new link, role and expiry", second)
	}
	page, _ := h.svc.Invitations(ctx, mustPage(t, "", 10))
	if len(page.Items) != 1 || page.Items[0].ID != second.ID {
		t.Errorf("pending = %+v, want only the second", page.Items)
	}
}

func TestInviteMember_ReplacesAnExpiredOneToo(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	ctx := tm.as(t, tm.owner, shared.RoleOwner)
	first := h.inviteAs(ctx, t, "novo@ex.com", shared.RoleStaff)
	h.clock.Advance(8 * 24 * time.Hour)

	// The one-open-invitation key counts expired ones (0004_tenancy).
	h.inviteAs(ctx, t, "novo@ex.com", shared.RoleStaff)

	if old, _ := h.store.Invitation(first.ID); old.RevokedAt.IsZero() {
		t.Error("the expired invitation was not closed")
	}
}

func TestInviteMember_Refuses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	admin := tm.as(t, tm.admin, shared.RoleAdmin)

	_, member := h.svc.InviteMember(admin, apptenancy.InviteMemberInput{Email: "STA@ex.com", Role: "viewer"})
	_, bad := h.svc.InviteMember(admin, apptenancy.InviteMemberInput{Email: "nope", Role: "owner"})
	_, staff := h.svc.InviteMember(tm.as(t, tm.staff, shared.RoleStaff), apptenancy.InviteMemberInput{Email: "x@ex.com", Role: "viewer"})

	if !errors.Is(member, tenancy.ErrAlreadyMember) {
		t.Errorf("a member's email: %v, want ErrAlreadyMember", member)
	}
	if got := fieldsOf(t, bad); !slices.Equal(got, []string{"email", "role"}) {
		t.Errorf("bad input: fields %v", got)
	}
	if !errors.Is(staff, shared.ErrForbidden) {
		t.Errorf("staff inviting: %v, want ErrForbidden", staff)
	}
	if len(h.mailer.Sent()) != 0 {
		t.Error("a refused invitation was emailed")
	}
}

func TestInviteMember_AnAccountInAnotherStoreCanBeInvited(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	_, otherOwner := h.newStore(t, "beta") // beta@ex.com owns another store

	inv, err := h.svc.InviteMember(tm.as(t, tm.owner, shared.RoleOwner), apptenancy.InviteMemberInput{Email: "beta@ex.com", Role: "admin"})

	if err != nil || inv.Role != shared.RoleAdmin {
		t.Errorf("inviting %v of another store: %+v, %v", otherOwner, inv, err)
	}
}

func TestInviteMember_EmailFailureKeepsTheInvitation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	ctx := tm.as(t, tm.owner, shared.RoleOwner)
	h.mailer.Fail(errBoom)

	_, err := h.svc.InviteMember(ctx, apptenancy.InviteMemberInput{Email: "novo@ex.com", Role: "staff"})

	if !errors.Is(err, apptenancy.ErrEmailUnavailable) {
		t.Fatalf("err = %v, want ErrEmailUnavailable", err)
	}
	page, _ := h.svc.Invitations(ctx, mustPage(t, "", 10))
	if len(page.Items) != 1 {
		t.Errorf("%d pending, want the saved invitation (ADR 0011)", len(page.Items))
	}
}

func TestInviteMember_FailuresStayServerErrors(t *testing.T) {
	t.Parallel()

	for name, breakIt := range map[string]func(h *harness){
		"secrets":   func(h *harness) { h.secrets.Fail(errBoom) },
		"directory": func(h *harness) { h.users.Fail(errBoom) },
		"open":      func(h *harness) { h.store.Fail("OpenInvitationByEmail", errBoom) },
		"insert":    func(h *harness) { h.store.Fail("InsertInvitation", errBoom) },
		"store":     func(h *harness) { h.store.Fail("TenantByID", errBoom) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			tm := h.team(t)
			breakIt(h)
			_, err := h.svc.InviteMember(tm.as(t, tm.owner, shared.RoleOwner), apptenancy.InviteMemberInput{Email: "novo@ex.com", Role: "staff"})
			if !errors.Is(err, errBoom) || errors.Is(err, apptenancy.ErrEmailUnavailable) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestInvitations_PendingOnlyNewestFirst(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	ctx := tm.as(t, tm.owner, shared.RoleOwner)
	old := h.inviteAs(ctx, t, "velho@ex.com", shared.RoleStaff)
	h.clock.Advance(6 * 24 * time.Hour)
	revoked := h.inviteAs(ctx, t, "revogado@ex.com", shared.RoleStaff)
	if err := h.svc.RevokeInvitation(ctx, revoked.ID); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Hour)
	a := h.inviteAs(ctx, t, "a@ex.com", shared.RoleStaff)
	h.clock.Advance(time.Minute)
	b := h.inviteAs(ctx, t, "b@ex.com", shared.RoleViewer)
	h.clock.Advance(23 * time.Hour) // the oldest expires

	page, err := h.svc.Invitations(ctx, mustPage(t, "", 10))
	if err != nil {
		t.Fatal(err)
	}

	var got []shared.InvitationID
	for _, inv := range page.Items {
		got = append(got, inv.ID)
	}
	if want := []shared.InvitationID{b.ID, a.ID}; !slices.Equal(got, want) {
		t.Errorf("pending = %v, want %v (not %v, expired, nor %v, revoked)", got, want, old.ID, revoked.ID)
	}
	if _, err := h.svc.Invitations(tm.as(t, tm.staff, shared.RoleStaff), mustPage(t, "", 10)); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("staff listing invitations: %v", err)
	}
}

func TestRevokeInvitation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	ctx := tm.as(t, tm.admin, shared.RoleAdmin)
	inv := h.inviteAs(ctx, t, "novo@ex.com", shared.RoleStaff)

	if err := h.svc.RevokeInvitation(ctx, inv.ID); err != nil {
		t.Fatal(err)
	}

	if r, _ := h.store.Invitation(inv.ID); r.RevokedAt.IsZero() {
		t.Error("not revoked")
	}
	if ev, ok := h.events.Published()[len(h.events.Published())-1].(tenancy.InvitationRevoked); !ok || ev.InvitationID != inv.ID || ev.By != tm.admin {
		t.Errorf("last event = %+v", h.events.Published()[len(h.events.Published())-1])
	}
	if err := h.svc.RevokeInvitation(ctx, inv.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("revoking twice: %v, want ErrNotFound", err)
	}
	if err := h.svc.RevokeInvitation(ctx, shared.InvitationID(h.ids.NewID())); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("unknown: %v, want ErrNotFound", err)
	}
	expired := h.inviteAs(ctx, t, "tarde@ex.com", shared.RoleStaff)
	h.clock.Advance(tenancy.InvitationLifetime)
	if err := h.svc.RevokeInvitation(ctx, expired.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("expired: %v, want ErrNotFound", err)
	}
}

func TestRevokeInvitation_AnotherStoresInvitationIsNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	other, otherOwner := h.newStore(t, "beta")
	theirs := h.inviteAs(as(t, otherOwner, other, shared.RoleOwner), t, "novo@ex.com", shared.RoleStaff)

	err := h.svc.RevokeInvitation(tm.as(t, tm.owner, shared.RoleOwner), theirs.ID)

	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if r, _ := h.store.Invitation(theirs.ID); !r.RevokedAt.IsZero() {
		t.Error("one store revoked another's invitation")
	}
}

// ---- Lookup, for identity through adapter/inproc ----

func storeCtx(tenant shared.TenantID) context.Context {
	return appshared.WithScope(context.Background(), appshared.Scope{TenantID: tenant})
}

// sentLink returns the token emailed with the last invitation.
func (h *harness) sentLink(t *testing.T) tenancy.InvitationToken {
	t.Helper()
	sent := h.mailer.Sent()
	if len(sent) == 0 {
		t.Fatal("no invitation emailed")
	}
	return sent[len(sent)-1].Token
}

func TestLookup_MembershipsOf(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	beta, owner := h.newStore(t, "beta")
	alfa, err := h.svc.CreateTenant(as(t, owner, shared.TenantID{}, ""), apptenancy.CreateTenantInput{Name: "Alfa", Slug: "alfa"})
	if err != nil {
		t.Fatal(err)
	}

	var got []apptenancy.StoreRole
	err = h.tx.WithinTx(appshared.WithScope(context.Background(), appshared.Scope{UserID: owner}), func(ctx context.Context) error {
		got, err = h.lookup.MembershipsOf(ctx, owner)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 || got[0].Tenant.ID != alfa.ID || got[1].Tenant.ID != beta || got[0].Role != shared.RoleOwner {
		t.Errorf("memberships = %+v; want Alfa then Loja beta, both as owner", got)
	}
	if got[0].Tenant.Name != "Alfa" || got[0].Tenant.Slug != "alfa" {
		t.Errorf("summary = %+v", got[0].Tenant)
	}
}

func TestLookup_PendingInvitationAndJoin(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	inv := h.inviteAs(tm.as(t, tm.admin, shared.RoleAdmin), t, "novo@ex.com", shared.RoleViewer)
	link := h.sentLink(t)
	newcomer := h.user(t, "Novo", "novo@ex.com")
	now := h.clock.Now()

	err := h.tx.WithinTx(storeCtx(tm.store), func(ctx context.Context) error {
		p, err := h.lookup.PendingInvitation(ctx, tm.store, link.Hash(), now)
		if err != nil {
			return err
		}
		if p.ID != inv.ID || p.Tenant.ID != tm.store || p.Tenant.Name != "Loja altarpm" || p.Email.String() != "novo@ex.com" || p.Role != shared.RoleViewer {
			t.Errorf("pending = %+v", p)
		}
		return h.lookup.Join(ctx, apptenancy.JoinInput{Tenant: tm.store, Hash: link.Hash(), User: newcomer, Email: mustEmail(t, "NOVO@ex.com"), At: now})
	})
	if err != nil {
		t.Fatal(err)
	}

	if m, ok := h.store.Member(tm.store, newcomer); !ok || m.Role != shared.RoleViewer {
		t.Errorf("membership = %+v, %v", m, ok)
	}
	if r, _ := h.store.Invitation(inv.ID); r.AcceptedAt.IsZero() {
		t.Error("the invitation was not used up")
	}
	rec := h.events.Records()[len(h.events.Records())-1]
	if ev, ok := rec.Event.(tenancy.MemberJoined); !ok || ev.UserID != newcomer || rec.TenantID != tm.store {
		t.Errorf("last event = %+v", rec)
	}
}

func TestLookup_RefusesAlike(t *testing.T) {
	t.Parallel()

	cases := map[string]func(t *testing.T, h *harness, tm team) (shared.TenantID, shared.SecretHash, context.Context){
		"unknown hash": func(_ *testing.T, _ *harness, tm team) (shared.TenantID, shared.SecretHash, context.Context) {
			return tm.store, shared.Secret{1}.Hash(), storeCtx(tm.store)
		},
		"another store's scope": func(t *testing.T, h *harness, tm team) (shared.TenantID, shared.SecretHash, context.Context) {
			other, _ := h.newStore(t, "beta")
			return tm.store, h.sentLink(t).Hash(), storeCtx(other)
		},
		"expired": func(t *testing.T, h *harness, tm team) (shared.TenantID, shared.SecretHash, context.Context) {
			h.clock.Advance(tenancy.InvitationLifetime)
			return tm.store, h.sentLink(t).Hash(), storeCtx(tm.store)
		},
		"revoked": func(t *testing.T, h *harness, tm team) (shared.TenantID, shared.SecretHash, context.Context) {
			page, _ := h.svc.Invitations(tm.as(t, tm.owner, shared.RoleOwner), mustPage(t, "", 10))
			if err := h.svc.RevokeInvitation(tm.as(t, tm.owner, shared.RoleOwner), page.Items[0].ID); err != nil {
				t.Fatal(err)
			}
			return tm.store, h.sentLink(t).Hash(), storeCtx(tm.store)
		},
		"the sender can no longer manage the team": func(t *testing.T, h *harness, tm team) (shared.TenantID, shared.SecretHash, context.Context) {
			// Not through ChangeRole, which would revoke the invitation:
			// the accept-time check must hold on its own (ADR 0011).
			demoteDirectly(t, h, tm.store, tm.admin)
			return tm.store, h.sentLink(t).Hash(), storeCtx(tm.store)
		},
	}
	for name, setUp := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			tm := h.team(t)
			h.inviteAs(tm.as(t, tm.admin, shared.RoleAdmin), t, "novo@ex.com", shared.RoleStaff)
			newcomer := h.user(t, "Novo", "novo@ex.com")
			tenant, hash, ctx := setUp(t, h, tm)
			now := h.clock.Now()
			_ = h.tx.WithinTx(ctx, func(ctx context.Context) error {
				if _, err := h.lookup.PendingInvitation(ctx, tenant, hash, now); !errors.Is(err, tenancy.ErrInvitationNotPending) {
					t.Errorf("PendingInvitation: %v, want ErrInvitationNotPending", err)
				}
				j := apptenancy.JoinInput{Tenant: tenant, Hash: hash, User: newcomer, Email: mustEmail(t, "novo@ex.com"), At: now}
				if err := h.lookup.Join(ctx, j); !errors.Is(err, tenancy.ErrInvitationNotPending) {
					t.Errorf("Join: %v, want ErrInvitationNotPending", err)
				}
				return nil
			})
			if _, ok := h.store.Member(tm.store, newcomer); ok {
				t.Error("joined anyway")
			}
		})
	}
}

// demoteDirectly changes a role in the store without the use case, so the
// member's sent invitations stay open.
func demoteDirectly(t *testing.T, h *harness, store shared.TenantID, user shared.UserID) {
	t.Helper()
	err := h.tx.WithinTx(storeCtx(store), func(ctx context.Context) error {
		m, err := h.store.MembershipOf(ctx, store, user)
		if err != nil {
			return err
		}
		if err := m.ChangeRole(shared.RoleViewer); err != nil {
			return err
		}
		return h.store.UpdateRole(ctx, m)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLookup_Join_RefusesAnotherEmailAndMembers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	h.inviteAs(tm.as(t, tm.admin, shared.RoleAdmin), t, "novo@ex.com", shared.RoleStaff)
	hash := h.sentLink(t).Hash()
	now := h.clock.Now()

	err := h.tx.WithinTx(storeCtx(tm.store), func(ctx context.Context) error {
		other := apptenancy.JoinInput{Tenant: tm.store, Hash: hash, User: shared.UserID(h.ids.NewID()), Email: mustEmail(t, "outro@ex.com"), At: now}
		if err := h.lookup.Join(ctx, other); !errors.Is(err, tenancy.ErrInvitationNotPending) {
			t.Errorf("another email: %v, want ErrInvitationNotPending", err)
		}
		member := apptenancy.JoinInput{Tenant: tm.store, Hash: hash, User: tm.staff, Email: mustEmail(t, "novo@ex.com"), At: now}
		if err := h.lookup.Join(ctx, member); !errors.Is(err, tenancy.ErrAlreadyMember) {
			t.Errorf("already a member: %v, want ErrAlreadyMember", err)
		}
		_, err := h.lookup.PendingInvitation(ctx, tm.store, hash, now)
		return err
	})
	if err != nil {
		t.Errorf("the invitation did not stay pending: %v", err)
	}
}
