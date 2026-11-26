package identity_test

import (
	"context"
	"errors"
	"testing"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// The suspended-store rules of ADR 0022: a session never selects a
// suspended store, which still shows in the store picker.

func suspendedIn(st appidentity.TenantSummary) appidentity.TenantSummary {
	st.Suspended = true
	return st
}

func TestLogin_NeverSelectsASuspendedStore(t *testing.T) {
	t.Parallel()

	t.Run("the only store", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		user := h.signUp(t, "ana@ex.com", "segredo-forte").Session.User.ID
		st := suspendedIn(h.store1("pausada"))
		h.members.Add(user, appidentity.Membership{Tenant: st, Role: shared.RoleOwner})

		res, err := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "ana@ex.com", Password: "segredo-forte"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Session.HasTenant() || h.lastClaims(t).Principal.HasTenant() {
			t.Errorf("selected %+v; a suspended store must not be selected", res.Session.Tenant)
		}
		if len(res.Session.Memberships) != 1 || !res.Session.Memberships[0].Tenant.Suspended {
			t.Errorf("memberships = %+v; the picker must still show the suspended store", res.Session.Memberships)
		}
	})
	t.Run("a hint for it", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		user := h.signUp(t, "ana@ex.com", "segredo-forte").Session.User.ID
		paused, open := suspendedIn(h.store1("pausada")), h.store1("aberta")
		h.members.Add(user, appidentity.Membership{Tenant: paused, Role: shared.RoleOwner})
		h.members.Add(user, appidentity.Membership{Tenant: open, Role: shared.RoleOwner})

		res, err := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "ana@ex.com", Password: "segredo-forte", TenantID: paused.ID})
		if err != nil {
			t.Fatal(err)
		}
		if res.Session.HasTenant() {
			t.Errorf("selected %+v from a hint for a suspended store", res.Session.Tenant)
		}
	})
}

func TestRefresh_DropsAStoreSuspendedMeanwhile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, st := h.session(t)
	h.members.Add(login.Session.User.ID, appidentity.Membership{Tenant: suspendedIn(st), Role: shared.RoleStaff})

	res, err := h.refresh(login.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if res.Session.HasTenant() || h.lastClaims(t).Principal.HasTenant() {
		t.Errorf("the refreshed session kept the suspended store %+v", res.Session.Tenant)
	}
	if len(res.Session.Memberships) != 1 || !res.Session.Memberships[0].Tenant.Suspended {
		t.Errorf("memberships = %+v; the suspended store must stay in the list", res.Session.Memberships)
	}
}

// Switching into a suspended store is refused before anything rotates, so
// the session stays as it was (ADR 0013).
func TestSwitchTenant_RefusesASuspendedStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, _ := h.session(t)
	paused := suspendedIn(h.store1("pausada"))
	h.members.Add(login.Session.User.ID, appidentity.Membership{Tenant: paused, Role: shared.RoleOwner})

	_, err := h.svc.SwitchTenant(context.Background(), appidentity.SwitchTenantInput{RefreshToken: login.RefreshToken, TenantID: paused.ID})
	if !errors.Is(err, tenancy.ErrTenantSuspended) {
		t.Fatalf("switch = %v, want ErrTenantSuspended", err)
	}
	if tok := h.storedToken(t, login.RefreshToken); !tok.UsedAt.IsZero() {
		t.Error("a refused switch used the refresh token")
	}
}

func TestInvitation_RefusesASuspendedStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token, st := h.invite(t, "novo@ex.com", shared.RoleStaff)
	// The invitation's store is suspended after it was sent.
	h.invites.Suspend(st.ID)

	if _, err := h.svc.PreviewInvitation(context.Background(), token); !errors.Is(err, tenancy.ErrTenantSuspended) {
		t.Errorf("preview = %v, want ErrTenantSuspended", err)
	}
	if _, err := h.accept(token, "Bia", "segredo-forte"); !errors.Is(err, tenancy.ErrTenantSuspended) {
		t.Errorf("accept = %v, want ErrTenantSuspended", err)
	}
	if joins := h.invites.Joins(); len(joins) != 0 {
		t.Errorf("joined %v a suspended store", joins)
	}
}
