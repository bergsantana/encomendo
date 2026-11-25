package identityports

import (
	"context"
	"errors"
	"testing"
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// TenancyHarness is identity's two ports into tenancy, with ways to set up
// what they read. adapter/inproc builds them on tenancy's Lookup; the fakes
// on in-memory lists.
type TenancyHarness struct {
	Memberships appidentity.Memberships
	Invitations appidentity.Invitations
	Tx          appshared.TxManager
	// NewStore creates a store named name, owned by someone else, and
	// returns its summary.
	NewStore func(t *testing.T, name string) appidentity.TenantSummary
	// NewUser creates an account with email and returns its id.
	NewUser func(t *testing.T, email string) shared.UserID
	// AddMember makes user a member of store with role.
	AddMember func(t *testing.T, store shared.TenantID, user shared.UserID, role shared.Role)
	// Invite creates a pending invitation of email to store, sent by its
	// owner and expiring at expires, and returns the hash of its link's
	// secret.
	Invite func(t *testing.T, store shared.TenantID, email string, role shared.Role, expires time.Time) shared.SecretHash
	// Now is the time the invitations are judged at.
	Now time.Time
}

func scoped(t *testing.T, tx appshared.TxManager, s appshared.Scope, fn func(ctx context.Context) error) {
	t.Helper()
	if err := tx.WithinTx(appshared.WithScope(context.Background(), s), fn); err != nil {
		t.Fatal(err)
	}
}

// RunMembershipsContract checks that a user's memberships are all visible
// in a scope of that user alone, sorted by store name, and that a store's
// scope or another user's shows no more than RLS allows (ADR 0010).
func RunMembershipsContract(t *testing.T, newHarness func(t *testing.T) TenancyHarness) {
	h := newHarness(t)
	user := h.NewUser(t, "membro@ex.com")
	beta, alfa, gama := h.NewStore(t, "Beta"), h.NewStore(t, "Alfa"), h.NewStore(t, "Gama")
	h.AddMember(t, beta.ID, user, shared.RoleStaff)
	h.AddMember(t, alfa.ID, user, shared.RoleAdmin)

	scoped(t, h.Tx, appshared.Scope{UserID: user}, func(ctx context.Context) error {
		ms, err := h.Memberships.MembershipsOf(ctx, user)
		want := []appidentity.Membership{{Tenant: alfa, Role: shared.RoleAdmin}, {Tenant: beta, Role: shared.RoleStaff}}
		if err != nil || len(ms) != 2 || ms[0] != want[0] || ms[1] != want[1] {
			t.Errorf("in the user's scope: %+v, %v; want %+v", ms, err, want)
		}
		return nil
	})
	scoped(t, h.Tx, appshared.Scope{TenantID: beta.ID, UserID: user}, func(ctx context.Context) error {
		ms, err := h.Memberships.MembershipsOf(ctx, user)
		if err != nil || len(ms) != 1 || ms[0].Tenant.ID != beta.ID {
			t.Errorf("in Beta's scope: %+v, %v; want Beta only", ms, err)
		}
		return nil
	})
	stranger := h.NewUser(t, "estranho@ex.com")
	scoped(t, h.Tx, appshared.Scope{UserID: stranger}, func(ctx context.Context) error {
		ms, err := h.Memberships.MembershipsOf(ctx, user)
		if err != nil || ms == nil || len(ms) != 0 {
			t.Errorf("in another user's scope: %+v, %v; want an empty list", ms, err)
		}
		mine, err := h.Memberships.MembershipsOf(ctx, stranger)
		if err != nil || mine == nil || len(mine) != 0 {
			t.Errorf("a user with no store: %+v, %v; want an empty list", mine, err)
		}
		return nil
	})
	_ = gama // a store the user is not in
}

// RunInvitationsContract checks that a pending invitation is found and used
// up only in its own store's scope, once, by the invited email, and that a
// member is refused without using it up (ADR 0011).
func RunInvitationsContract(t *testing.T, newHarness func(t *testing.T) TenancyHarness) {
	t.Run("found and joined in its store's scope", func(t *testing.T) { invitationsJoin(t, newHarness(t)) })
	t.Run("invisible from another store's scope", func(t *testing.T) { invitationsOtherScope(t, newHarness(t)) })
	t.Run("expired is not pending", func(t *testing.T) { invitationsExpired(t, newHarness(t)) })
	t.Run("another email or a member is refused", func(t *testing.T) { invitationsRefused(t, newHarness(t)) })
}

func invitationsJoin(t *testing.T, h TenancyHarness) {
	store := h.NewStore(t, "Alta")
	hash := h.Invite(t, store.ID, "novo@ex.com", shared.RoleViewer, h.Now.Add(time.Hour))
	user := h.NewUser(t, "novo@ex.com")
	email, _ := identity.ParseEmail("NOVO@ex.com")
	scoped(t, h.Tx, appshared.Scope{TenantID: store.ID}, func(ctx context.Context) error {
		p, err := h.Invitations.PendingInvitation(ctx, store.ID, hash, h.Now)
		if err != nil || p.Tenant != store || p.Role != shared.RoleViewer || p.Email.Key() != "novo@ex.com" || p.ID.IsZero() {
			t.Errorf("PendingInvitation = %+v, %v", p, err)
		}
		return h.Invitations.JoinByInvitation(ctx, appidentity.Join{Tenant: store.ID, Hash: hash, User: user, Email: email, At: h.Now})
	})
	scoped(t, h.Tx, appshared.Scope{TenantID: store.ID}, func(ctx context.Context) error {
		if _, err := h.Invitations.PendingInvitation(ctx, store.ID, hash, h.Now); !errors.Is(err, tenancy.ErrInvitationNotPending) {
			t.Errorf("after joining: %v, want ErrInvitationNotPending", err)
		}
		return nil
	})
	scoped(t, h.Tx, appshared.Scope{UserID: user}, func(ctx context.Context) error {
		ms, err := h.Memberships.MembershipsOf(ctx, user)
		if err != nil || len(ms) != 1 || ms[0].Tenant.ID != store.ID || ms[0].Role != shared.RoleViewer {
			t.Errorf("memberships after joining = %+v, %v", ms, err)
		}
		return nil
	})
}

func invitationsOtherScope(t *testing.T, h TenancyHarness) {
	store, other := h.NewStore(t, "Alta"), h.NewStore(t, "Beta")
	hash := h.Invite(t, store.ID, "novo@ex.com", shared.RoleStaff, h.Now.Add(time.Hour))
	scoped(t, h.Tx, appshared.Scope{TenantID: other.ID}, func(ctx context.Context) error {
		if _, err := h.Invitations.PendingInvitation(ctx, store.ID, hash, h.Now); !errors.Is(err, tenancy.ErrInvitationNotPending) {
			t.Errorf("from another store's scope: %v, want ErrInvitationNotPending", err)
		}
		return nil
	})
}

func invitationsExpired(t *testing.T, h TenancyHarness) {
	store := h.NewStore(t, "Alta")
	hash := h.Invite(t, store.ID, "novo@ex.com", shared.RoleStaff, h.Now.Add(time.Minute))
	scoped(t, h.Tx, appshared.Scope{TenantID: store.ID}, func(ctx context.Context) error {
		if _, err := h.Invitations.PendingInvitation(ctx, store.ID, hash, h.Now.Add(time.Minute)); !errors.Is(err, tenancy.ErrInvitationNotPending) {
			t.Errorf("at expiry: %v, want ErrInvitationNotPending", err)
		}
		return nil
	})
}

func invitationsRefused(t *testing.T, h TenancyHarness) {
	store := h.NewStore(t, "Alta")
	hash := h.Invite(t, store.ID, "novo@ex.com", shared.RoleStaff, h.Now.Add(time.Hour))
	member := h.NewUser(t, "membro@ex.com")
	h.AddMember(t, store.ID, member, shared.RoleViewer)
	wrong, _ := identity.ParseEmail("outro@ex.com")
	invited, _ := identity.ParseEmail("novo@ex.com")
	scoped(t, h.Tx, appshared.Scope{TenantID: store.ID}, func(ctx context.Context) error {
		err := h.Invitations.JoinByInvitation(ctx, appidentity.Join{Tenant: store.ID, Hash: hash, User: h.NewUser(t, "outro@ex.com"), Email: wrong, At: h.Now})
		if !errors.Is(err, tenancy.ErrInvitationNotPending) {
			t.Errorf("another email: %v, want ErrInvitationNotPending", err)
		}
		err = h.Invitations.JoinByInvitation(ctx, appidentity.Join{Tenant: store.ID, Hash: hash, User: member, Email: invited, At: h.Now})
		if !errors.Is(err, tenancy.ErrAlreadyMember) {
			t.Errorf("a member: %v, want ErrAlreadyMember", err)
		}
		if _, err := h.Invitations.PendingInvitation(ctx, store.ID, hash, h.Now); err != nil {
			t.Errorf("the refused joins used the invitation up: %v", err)
		}
		return nil
	})
}
