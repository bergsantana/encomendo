package tenancy_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

func mustEmail(t *testing.T, s string) identity.Email {
	t.Helper()
	e, err := identity.ParseEmail(s)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func mustPage(t *testing.T, cursor string, limit int) shared.PageRequest {
	t.Helper()
	p, err := shared.NewPageRequest(cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// team is a store with an owner, an admin, a staff member and a viewer.
type team struct {
	store                       shared.TenantID
	owner, admin, staff, viewer shared.UserID
}

func (h *harness) team(t *testing.T) team {
	t.Helper()
	store, owner := h.newStore(t, "altarpm")
	tm := team{store: store, owner: owner}
	tm.admin = h.user(t, "Adm", "adm@ex.com")
	tm.staff = h.user(t, "Sta", "sta@ex.com")
	tm.viewer = h.user(t, "Vie", "vie@ex.com")
	h.join(t, store, tm.admin, shared.RoleAdmin)
	h.join(t, store, tm.staff, shared.RoleStaff)
	h.join(t, store, tm.viewer, shared.RoleViewer)
	return tm
}

func (tm team) as(t *testing.T, user shared.UserID, role shared.Role) context.Context {
	t.Helper()
	return as(t, user, tm.store, role)
}

func TestMembers_ListsTheTeamOldestFirst(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	ctx := tm.as(t, tm.admin, shared.RoleAdmin)

	first, err := h.svc.Members(ctx, mustPage(t, "", 3))
	if err != nil {
		t.Fatal(err)
	}
	rest, err := h.svc.Members(ctx, mustPage(t, first.NextCursor, 3))
	if err != nil {
		t.Fatal(err)
	}

	var got []shared.UserID
	for _, m := range append(first.Items, rest.Items...) {
		got = append(got, m.UserID)
	}
	if want := []shared.UserID{tm.owner, tm.admin, tm.staff, tm.viewer}; !slices.Equal(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
	if !first.HasMore() || rest.HasMore() {
		t.Errorf("pages: first has more %v, rest has more %v", first.HasMore(), rest.HasMore())
	}
	if m := first.Items[1]; m.Name != "Adm" || m.Email != "adm@ex.com" || m.Role != shared.RoleAdmin || m.JoinedAt.IsZero() {
		t.Errorf("member = %+v", m)
	}
	if _, err := h.svc.Members(tm.as(t, tm.staff, shared.RoleStaff), mustPage(t, "", 10)); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("staff listing the team: %v, want ErrForbidden", err)
	}
}

func TestMembers_ReportsAMemberWithoutAnAccount(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	h.join(t, tm.store, shared.UserID(h.ids.NewID()), shared.RoleStaff) // not in the directory

	if _, err := h.svc.Members(tm.as(t, tm.owner, shared.RoleOwner), mustPage(t, "", 10)); err == nil || errors.Is(err, shared.ErrNotFound) {
		t.Errorf("err = %v, want a server error", err)
	}
}

func TestChangeRole(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)

	m, err := h.svc.ChangeRole(tm.as(t, tm.admin, shared.RoleAdmin), apptenancy.ChangeRoleInput{UserID: tm.staff, Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}

	if m.Role != shared.RoleAdmin || m.UserID != tm.staff || m.Name != "Sta" {
		t.Errorf("member = %+v", m)
	}
	if r, _ := h.store.Member(tm.store, tm.staff); r.Role != shared.RoleAdmin || r.Version != 2 {
		t.Errorf("stored = %+v", r)
	}
	ev, ok := h.events.Published()[len(h.events.Published())-1].(tenancy.MemberRoleChanged)
	if !ok || ev.From != shared.RoleStaff || ev.To != shared.RoleAdmin || ev.By != tm.admin {
		t.Errorf("last event = %+v", h.events.Published()[len(h.events.Published())-1])
	}
}

func TestChangeRole_Refuses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	admin := tm.as(t, tm.admin, shared.RoleAdmin)

	_, owner := h.svc.ChangeRole(admin, apptenancy.ChangeRoleInput{UserID: tm.owner, Role: "viewer"})
	_, toOwner := h.svc.ChangeRole(admin, apptenancy.ChangeRoleInput{UserID: tm.staff, Role: "owner"})
	_, unknown := h.svc.ChangeRole(admin, apptenancy.ChangeRoleInput{UserID: shared.UserID(h.ids.NewID()), Role: "staff"})
	_, byStaff := h.svc.ChangeRole(tm.as(t, tm.staff, shared.RoleStaff), apptenancy.ChangeRoleInput{UserID: tm.viewer, Role: "staff"})

	if !errors.Is(owner, tenancy.ErrOwnerFixed) {
		t.Errorf("the owner's role: %v, want ErrOwnerFixed", owner)
	}
	if got := fieldsOf(t, toOwner); !slices.Equal(got, []string{"role"}) {
		t.Errorf("making an owner: fields %v", got)
	}
	if !errors.Is(unknown, shared.ErrNotFound) {
		t.Errorf("not a member: %v, want ErrNotFound", unknown)
	}
	if !errors.Is(byStaff, shared.ErrForbidden) {
		t.Errorf("staff changing roles: %v, want ErrForbidden", byStaff)
	}
}

// TestTeamManagement_ReadsTheCallersRoleAgain is ADR 0011: a demoted or
// removed admin loses team management at once, not when their 15-minute
// token expires.
func TestTeamManagement_ReadsTheCallersRoleAgain(t *testing.T) {
	t.Parallel()

	calls := map[string]func(h *harness, tm team, ctx context.Context) error{
		"change role": func(h *harness, tm team, ctx context.Context) error {
			_, err := h.svc.ChangeRole(ctx, apptenancy.ChangeRoleInput{UserID: tm.viewer, Role: "staff"})
			return err
		},
		"remove member": func(h *harness, tm team, ctx context.Context) error {
			return h.svc.RemoveMember(ctx, tm.viewer)
		},
		"invite": func(h *harness, _ team, ctx context.Context) error {
			_, err := h.svc.InviteMember(ctx, apptenancy.InviteMemberInput{Email: "novo@ex.com", Role: "staff"})
			return err
		},
		"revoke invitation": func(h *harness, _ team, ctx context.Context) error {
			return h.svc.RevokeInvitation(ctx, shared.InvitationID(h.ids.NewID()))
		},
	}
	for name, call := range calls {
		t.Run(name+", demoted", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			tm := h.team(t)
			stale := tm.as(t, tm.admin, shared.RoleAdmin) // the token still says admin
			if _, err := h.svc.ChangeRole(tm.as(t, tm.owner, shared.RoleOwner), apptenancy.ChangeRoleInput{UserID: tm.admin, Role: "staff"}); err != nil {
				t.Fatal(err)
			}
			if err := call(h, tm, stale); !errors.Is(err, shared.ErrForbidden) {
				t.Errorf("err = %v, want ErrForbidden", err)
			}
		})
		t.Run(name+", removed", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			tm := h.team(t)
			stale := tm.as(t, tm.admin, shared.RoleAdmin)
			if err := h.svc.RemoveMember(tm.as(t, tm.owner, shared.RoleOwner), tm.admin); err != nil {
				t.Fatal(err)
			}
			if err := call(h, tm, stale); !errors.Is(err, shared.ErrForbidden) {
				t.Errorf("err = %v, want ErrForbidden", err)
			}
		})
	}
}

func TestRemoveMember(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	admin := tm.as(t, tm.admin, shared.RoleAdmin)

	if err := h.svc.RemoveMember(admin, tm.staff); err != nil {
		t.Fatal(err)
	}

	if _, ok := h.store.Member(tm.store, tm.staff); ok {
		t.Error("the member is still there")
	}
	if ev, ok := h.events.Published()[len(h.events.Published())-1].(tenancy.MemberRemoved); !ok || ev.UserID != tm.staff || ev.By != tm.admin {
		t.Errorf("last event = %+v", h.events.Published()[len(h.events.Published())-1])
	}
	if err := h.svc.RemoveMember(admin, tm.owner); !errors.Is(err, tenancy.ErrOwnerFixed) {
		t.Errorf("removing the owner: %v, want ErrOwnerFixed", err)
	}
	if err := h.svc.RemoveMember(admin, tm.staff); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("removing twice: %v, want ErrNotFound", err)
	}
}

// TestLosingTeamManagement_RevokesSentInvitations is ADR 0011: nobody keeps
// a way back in through an invitation sent before they lost access.
func TestLosingTeamManagement_RevokesSentInvitations(t *testing.T) {
	t.Parallel()

	for name, lose := range map[string]func(t *testing.T, h *harness, tm team) error{
		"demoted": func(t *testing.T, h *harness, tm team) error {
			_, err := h.svc.ChangeRole(tm.as(t, tm.owner, shared.RoleOwner), apptenancy.ChangeRoleInput{UserID: tm.admin, Role: "viewer"})
			return err
		},
		"removed": func(t *testing.T, h *harness, tm team) error {
			return h.svc.RemoveMember(tm.as(t, tm.owner, shared.RoleOwner), tm.admin)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			tm := h.team(t)
			sent, err := h.svc.InviteMember(tm.as(t, tm.admin, shared.RoleAdmin), apptenancy.InviteMemberInput{Email: "novo@ex.com", Role: "staff"})
			if err != nil {
				t.Fatal(err)
			}
			byOwner, err := h.svc.InviteMember(tm.as(t, tm.owner, shared.RoleOwner), apptenancy.InviteMemberInput{Email: "outro@ex.com", Role: "staff"})
			if err != nil {
				t.Fatal(err)
			}

			if err := lose(t, h, tm); err != nil {
				t.Fatal(err)
			}

			if r, _ := h.store.Invitation(sent.ID); r.RevokedAt.IsZero() {
				t.Error("the admin's invitation is still open")
			}
			if r, _ := h.store.Invitation(byOwner.ID); !r.RevokedAt.IsZero() {
				t.Error("the owner's invitation was revoked too")
			}
			if !slices.Contains(h.events.Names(), "tenancy.invitation_revoked") {
				t.Errorf("events = %v", h.events.Names())
			}
		})
	}
}

func TestChangeRole_WithinTheSameRoleChangesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	before := len(h.events.Names())

	m, err := h.svc.ChangeRole(tm.as(t, tm.owner, shared.RoleOwner), apptenancy.ChangeRoleInput{UserID: tm.staff, Role: "staff"})

	if err != nil || m.Role != shared.RoleStaff {
		t.Fatalf("ChangeRole = %+v, %v", m, err)
	}
	if r, _ := h.store.Member(tm.store, tm.staff); r.Version != 1 || len(h.events.Names()) != before {
		t.Error("a no-op change wrote or published")
	}
}

func TestMembership_FailuresStayServerErrors(t *testing.T) {
	t.Parallel()

	for _, method := range []string{"MembershipOf", "UpdateRole", "DeleteMembership", "RevokeInvitationsBy"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			tm := h.team(t)
			h.store.Fail(method, errBoom)
			ctx := tm.as(t, tm.owner, shared.RoleOwner)
			_, changeErr := h.svc.ChangeRole(ctx, apptenancy.ChangeRoleInput{UserID: tm.admin, Role: "viewer"})
			removeErr := h.svc.RemoveMember(ctx, tm.staff)
			if !errors.Is(changeErr, errBoom) && !errors.Is(removeErr, errBoom) {
				t.Errorf("change role: %v; remove: %v; want the failure", changeErr, removeErr)
			}
		})
	}
	t.Run("directory", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		tm := h.team(t)
		h.users.Fail(errBoom)
		if _, err := h.svc.Members(tm.as(t, tm.owner, shared.RoleOwner), mustPage(t, "", 10)); !errors.Is(err, errBoom) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestMemberCalls_RunInTheCallersStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := h.team(t)
	before := len(h.tx.Scopes())

	if _, err := h.svc.Members(tm.as(t, tm.owner, shared.RoleOwner), mustPage(t, "", 10)); err != nil {
		t.Fatal(err)
	}

	if got := h.tx.Scopes()[before:]; !slices.Equal(got, []appshared.Scope{{TenantID: tm.store, UserID: tm.owner}}) {
		t.Errorf("scopes = %+v", got)
	}
}
