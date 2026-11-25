package tenancy

import (
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ParseInvitableRole returns the role named s when it is admin, staff or
// viewer: the roles an invitation or a role change may give. It returns a
// ValidationError on the field "role" otherwise, owner included: a store has
// one owner (ADR 0011).
func ParseInvitableRole(s string) (shared.Role, error) {
	r := shared.Role(s)
	if !invitable(r) {
		return "", shared.NewValidationError(shared.FieldError{Field: "role", Message: "Escolha admin, staff ou viewer."})
	}
	return r, nil
}

func invitable(r shared.Role) bool {
	return r == shared.RoleAdmin || r == shared.RoleStaff || r == shared.RoleViewer
}

// Membership is one person's place in one store's team ("Equipe"): their
// role there.
type Membership struct {
	tenant   shared.TenantID
	user     shared.UserID
	role     shared.Role
	version  int
	joinedAt time.Time
}

// NewOwnerMembership returns the membership of a store's creator. It returns
// ErrInvalidMembership for a zero tenant or user.
func NewOwnerMembership(tenant shared.TenantID, user shared.UserID, now time.Time) (*Membership, error) {
	return newMembership(tenant, user, shared.RoleOwner, now)
}

// NewMembershipFrom returns the membership an accepted invitation gives
// user, with the invitation's role. It returns ErrInvalidMembership for a
// zero user.
func NewMembershipFrom(inv *Invitation, user shared.UserID, now time.Time) (*Membership, error) {
	return newMembership(inv.TenantID(), user, inv.Role(), now)
}

func newMembership(tenant shared.TenantID, user shared.UserID, role shared.Role, now time.Time) (*Membership, error) {
	if tenant.IsZero() || user.IsZero() || !role.Valid() {
		return nil, fmt.Errorf("new membership of %v in %v: %w", user, tenant, ErrInvalidMembership)
	}
	return &Membership{tenant: tenant, user: user, role: role, version: 1, joinedAt: now}, nil
}

// MembershipRecord is a membership as a repository stores it.
type MembershipRecord struct {
	Tenant   shared.TenantID
	User     shared.UserID
	Role     shared.Role
	Version  int
	JoinedAt time.Time
}

// RestoreMembership rebuilds a stored membership. It returns
// ErrInvalidMembership when the record breaks a rule a stored membership
// always keeps.
func RestoreMembership(r MembershipRecord) (*Membership, error) {
	if r.Version < 1 {
		return nil, fmt.Errorf("restore membership of %v: %w", r.User, ErrInvalidMembership)
	}
	m, err := newMembership(r.Tenant, r.User, r.Role, r.JoinedAt)
	if err != nil {
		return nil, err
	}
	m.version = r.Version
	return m, nil
}

// TenantID returns the store.
func (m *Membership) TenantID() shared.TenantID { return m.tenant }

// UserID returns the member.
func (m *Membership) UserID() shared.UserID { return m.user }

// Role returns the member's role in the store.
func (m *Membership) Role() shared.Role { return m.role }

// Version returns the version the membership was read or created at.
func (m *Membership) Version() int { return m.version }

// JoinedAt returns when the member joined.
func (m *Membership) JoinedAt() time.Time { return m.joinedAt }

// Allows reports whether the member's role grants perm.
func (m *Membership) Allows(perm identity.Permission) bool { return identity.Allows(m.role, perm) }

// ManagesTeam reports whether the member may invite, change roles and remove
// members. Team management checks this on the membership read inside its
// own transaction, never on the token's role (ADR 0011).
func (m *Membership) ManagesTeam() bool { return m.Allows(identity.PermMembersManage) }

// ChangeRole gives the member role to. It returns ErrOwnerFixed when the
// member is the owner and a ValidationError on "role" when to is owner or
// not a role; either way nothing changes. The caller revokes the pending
// invitations of a member who no longer manages the team (ADR 0011).
func (m *Membership) ChangeRole(to shared.Role) error {
	if m.role == shared.RoleOwner {
		return ErrOwnerFixed
	}
	if _, err := ParseInvitableRole(string(to)); err != nil {
		return err
	}
	m.role = to
	return nil
}

// CheckRemovable returns ErrOwnerFixed for the owner, who never leaves their
// store, and nil for anyone else.
func (m *Membership) CheckRemovable() error {
	if m.role == shared.RoleOwner {
		return ErrOwnerFixed
	}
	return nil
}
