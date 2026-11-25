package tenancy

import (
	"context"
	"errors"
	"fmt"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// Lookup answers the identity context's questions about stores, and lets it
// use an invitation up. adapter/inproc implements identity's Memberships
// and Invitations ports on top of it (ADR 0011).
//
// It depends only on tenancy's own repositories and the event publisher,
// never on identity, so main.go builds it before either service and the
// two contexts never depend on each other's Service. Join writes, unlike
// the read-only queries ADR 0011 first described; it still needs nothing
// from identity, so no cycle appears. Every method runs inside the
// caller's transaction, in the scope the caller set: the user alone for
// MembershipsOf, the invitation's store for the others.
type Lookup struct {
	tenants     Tenants
	members     Members
	invitations Invitations
	events      appshared.EventPublisher
}

// NewLookup returns a Lookup over the repositories and events of d.
func NewLookup(d Deps) *Lookup {
	return &Lookup{tenants: d.Tenants, members: d.Members, invitations: d.Invitations, events: d.Events}
}

// TenantSummary names a store.
type TenantSummary struct {
	ID   shared.TenantID
	Name string
	Slug string
}

func summary(t *tenancy.Tenant) TenantSummary {
	return TenantSummary{ID: t.ID(), Name: t.Name().String(), Slug: t.Slug().String()}
}

// StoreRole is one store a user belongs to, with their role.
type StoreRole struct {
	Tenant TenantSummary
	Role   shared.Role
}

// MembershipsOf returns every store the user belongs to, sorted by name.
// Only a scope of the user alone shows them all (ADR 0010).
func (l *Lookup) MembershipsOf(ctx context.Context, user shared.UserID) ([]StoreRole, error) {
	ms, err := l.members.MembershipsOfUser(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("read memberships of %v: %w", user, err)
	}
	out := make([]StoreRole, 0, len(ms))
	for _, sm := range ms {
		out = append(out, StoreRole{Tenant: summary(sm.Tenant), Role: sm.Membership.Role()})
	}
	return out, nil
}

// InvitationDetails is a pending invitation as the accept screen shows it.
type InvitationDetails struct {
	ID        shared.InvitationID
	Tenant    TenantSummary
	Email     identity.Email
	Role      shared.Role
	ExpiresAt time.Time
}

// PendingInvitation returns the invitation of tenant whose link's secret
// has hash, if it can still be accepted at now. It returns
// tenancy.ErrInvitationNotPending, the same for every case, when it is
// unknown, expired, accepted or revoked, or when its sender no longer
// manages the team (ADR 0011).
func (l *Lookup) PendingInvitation(ctx context.Context, tenant shared.TenantID, hash shared.SecretHash, now time.Time) (InvitationDetails, error) {
	inv, err := l.pending(ctx, tenant, hash, now)
	if err != nil {
		return InvitationDetails{}, err
	}
	t, err := l.tenants.TenantByID(ctx, tenant)
	if err != nil {
		return InvitationDetails{}, fmt.Errorf("read store %v: %w", tenant, err)
	}
	return InvitationDetails{ID: inv.ID(), Tenant: summary(t), Email: inv.Email(), Role: inv.Role(), ExpiresAt: inv.ExpiresAt()}, nil
}

func (l *Lookup) pending(ctx context.Context, tenant shared.TenantID, hash shared.SecretHash, now time.Time) (*tenancy.Invitation, error) {
	inv, err := l.invitations.InvitationByHash(ctx, tenant, hash)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, tenancy.ErrInvitationNotPending
	}
	if err != nil {
		return nil, fmt.Errorf("find invitation: %w", err)
	}
	if !inv.PendingAt(now) {
		return nil, tenancy.ErrInvitationNotPending
	}
	sender, err := l.members.MembershipOf(ctx, tenant, inv.InvitedBy())
	if errors.Is(err, shared.ErrNotFound) {
		return nil, tenancy.ErrInvitationNotPending
	}
	if err != nil {
		return nil, fmt.Errorf("read the sender's membership: %w", err)
	}
	if !sender.ManagesTeam() {
		return nil, tenancy.ErrInvitationNotPending
	}
	return inv, nil
}

// JoinInput is a user using an invitation up.
type JoinInput struct {
	Tenant shared.TenantID
	Hash   shared.SecretHash
	User   shared.UserID
	Email  identity.Email // the user's; it must be the invited one
	At     time.Time
}

// Join uses the invitation up and makes the user a member with its role.
// It returns tenancy.ErrInvitationNotPending as PendingInvitation does, and
// also when j.Email is not the invited email; and tenancy.ErrAlreadyMember
// when the user is already in the team, leaving the invitation pending.
func (l *Lookup) Join(ctx context.Context, j JoinInput) error {
	inv, err := l.pending(ctx, j.Tenant, j.Hash, j.At)
	if err != nil {
		return err
	}
	if !inv.Email().Equal(j.Email) {
		return tenancy.ErrInvitationNotPending
	}
	_, err = l.members.MembershipOf(ctx, j.Tenant, j.User)
	if err == nil {
		return tenancy.ErrAlreadyMember
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return fmt.Errorf("read membership of %v: %w", j.User, err)
	}
	if err := inv.Accept(j.At); err != nil {
		return err
	}
	accepted, err := l.invitations.MarkInvitationAccepted(ctx, j.Tenant, inv.ID(), j.At)
	if err != nil {
		return fmt.Errorf("accept invitation: %w", err)
	}
	if !accepted {
		return tenancy.ErrInvitationNotPending
	}
	return l.addMember(ctx, inv, j)
}

func (l *Lookup) addMember(ctx context.Context, inv *tenancy.Invitation, j JoinInput) error {
	m, err := tenancy.NewMembershipFrom(inv, j.User, j.At)
	if err != nil {
		return err
	}
	if err := l.members.InsertMembership(ctx, m); err != nil {
		if errors.Is(err, shared.ErrConflict) {
			return tenancy.ErrAlreadyMember
		}
		return fmt.Errorf("add member: %w", err)
	}
	return l.events.Publish(ctx, tenancy.MemberJoined{UserID: j.User, Role: inv.Role(), InvitationID: inv.ID()})
}
