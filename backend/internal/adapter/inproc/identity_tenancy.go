package inproc

import (
	"context"
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// IdentityMemberships implements appidentity.Memberships over tenancy's
// Lookup, so identity learns which stores a user belongs to without
// importing tenancy.
type IdentityMemberships struct{ lookup *apptenancy.Lookup }

// NewIdentityMemberships returns identity's memberships port.
func NewIdentityMemberships(l *apptenancy.Lookup) *IdentityMemberships {
	return &IdentityMemberships{lookup: l}
}

// MembershipsOf returns the user's memberships sorted by store name, then
// id; none is an empty list.
func (m *IdentityMemberships) MembershipsOf(ctx context.Context, user shared.UserID) ([]appidentity.Membership, error) {
	roles, err := m.lookup.MembershipsOf(ctx, user)
	if err != nil {
		return nil, err
	}
	out := make([]appidentity.Membership, 0, len(roles))
	for _, r := range roles {
		out = append(out, appidentity.Membership{Tenant: tenantSummary(r.Tenant), Role: r.Role})
	}
	return out, nil
}

// IdentityInvitations implements appidentity.Invitations over tenancy's
// Lookup.
type IdentityInvitations struct{ lookup *apptenancy.Lookup }

// NewIdentityInvitations returns identity's invitations port.
func NewIdentityInvitations(l *apptenancy.Lookup) *IdentityInvitations {
	return &IdentityInvitations{lookup: l}
}

// PendingInvitation returns the invitation whose secret has hash, if it is
// pending at now.
func (i *IdentityInvitations) PendingInvitation(ctx context.Context, tenant shared.TenantID, hash shared.SecretHash, now time.Time) (appidentity.PendingInvitation, error) {
	d, err := i.lookup.PendingInvitation(ctx, tenant, hash, now)
	if err != nil {
		return appidentity.PendingInvitation{}, err
	}
	return appidentity.PendingInvitation{
		ID: d.ID, Tenant: tenantSummary(d.Tenant), Email: d.Email, Role: d.Role, ExpiresAt: d.ExpiresAt,
	}, nil
}

// JoinByInvitation uses the invitation up and makes the user a member.
func (i *IdentityInvitations) JoinByInvitation(ctx context.Context, j appidentity.Join) error {
	return i.lookup.Join(ctx, apptenancy.JoinInput{Tenant: j.Tenant, Hash: j.Hash, User: j.User, Email: j.Email, At: j.At})
}

func tenantSummary(t apptenancy.TenantSummary) appidentity.TenantSummary {
	return appidentity.TenantSummary{ID: t.ID, Name: t.Name, Slug: t.Slug}
}

// TenancyUsers implements apptenancy.UserDirectory over identity's
// Directory.
type TenancyUsers struct{ directory *appidentity.Directory }

// NewTenancyUsers returns tenancy's user directory port.
func NewTenancyUsers(d *appidentity.Directory) *TenancyUsers { return &TenancyUsers{directory: d} }

// UserByEmail returns the account with this email, or an error matching
// shared.ErrNotFound.
func (u *TenancyUsers) UserByEmail(ctx context.Context, email identity.Email) (apptenancy.UserSummary, error) {
	v, err := u.directory.UserByEmail(ctx, email)
	if err != nil {
		return apptenancy.UserSummary{}, err
	}
	return apptenancy.UserSummary{ID: v.ID, Name: v.Name, Email: v.Email}, nil
}

// UsersByID returns the accounts that exist among ids.
func (u *TenancyUsers) UsersByID(ctx context.Context, ids []shared.UserID) ([]apptenancy.UserSummary, error) {
	views, err := u.directory.UsersByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]apptenancy.UserSummary, 0, len(views))
	for _, v := range views {
		out = append(out, apptenancy.UserSummary{ID: v.ID, Name: v.Name, Email: v.Email})
	}
	return out, nil
}
