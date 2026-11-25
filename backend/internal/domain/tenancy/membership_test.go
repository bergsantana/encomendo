package tenancy

import (
	"errors"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestParseInvitableRole(t *testing.T) {
	t.Parallel()

	for _, ok := range []shared.Role{shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer} {
		if r, err := ParseInvitableRole(string(ok)); err != nil || r != ok {
			t.Errorf("ParseInvitableRole(%q) = %q, %v", ok, r, err)
		}
	}
	// A store has one owner: no one becomes owner by invitation or role
	// change (ADR 0011).
	for _, bad := range []string{"owner", "", "Admin", "client"} {
		_, err := ParseInvitableRole(bad)
		var ve *shared.ValidationError
		if !errors.As(err, &ve) || ve.Fields[0].Field != "role" {
			t.Errorf("ParseInvitableRole(%q) = %v, want a ValidationError on role", bad, err)
		}
	}
}

func TestNewOwnerMembership(t *testing.T) {
	t.Parallel()

	m, err := NewOwnerMembership(tenantID, ownerID, t0)
	if err != nil {
		t.Fatal(err)
	}
	if m.TenantID() != tenantID || m.UserID() != ownerID || m.Role() != shared.RoleOwner {
		t.Errorf("owner membership = %v %v %q", m.TenantID(), m.UserID(), m.Role())
	}
	if !m.JoinedAt().Equal(t0) || m.Version() != 1 {
		t.Errorf("joined %v, version %d", m.JoinedAt(), m.Version())
	}
	if _, err := NewOwnerMembership(shared.TenantID{}, ownerID, t0); !errors.Is(err, ErrInvalidMembership) {
		t.Errorf("zero tenant: %v", err)
	}
	if _, err := NewOwnerMembership(tenantID, shared.UserID{}, t0); !errors.Is(err, ErrInvalidMembership) {
		t.Errorf("zero user: %v", err)
	}
}

func TestNewMembershipFrom_TakesTheInvitedRole(t *testing.T) {
	t.Parallel()

	inv, err := NewInvitation(spec(t), t0)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewMembershipFrom(inv, staffID, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if m.TenantID() != tenantID || m.UserID() != staffID || m.Role() != shared.RoleStaff || !m.JoinedAt().Equal(t0.Add(time.Hour)) {
		t.Errorf("membership = %v %v %q %v", m.TenantID(), m.UserID(), m.Role(), m.JoinedAt())
	}
	if _, err := NewMembershipFrom(inv, shared.UserID{}, t0); !errors.Is(err, ErrInvalidMembership) {
		t.Errorf("zero user: %v", err)
	}
}

func TestRestoreMembership(t *testing.T) {
	t.Parallel()

	m, err := RestoreMembership(MembershipRecord{Tenant: tenantID, User: staffID, Role: shared.RoleStaff, Version: 2, JoinedAt: t0})
	if err != nil || m.Role() != shared.RoleStaff || m.Version() != 2 {
		t.Fatalf("RestoreMembership = %+v, %v", m, err)
	}
	for name, rec := range map[string]MembershipRecord{
		"bad role":     {Tenant: tenantID, User: staffID, Role: "client", Version: 1},
		"zero version": {Tenant: tenantID, User: staffID, Role: shared.RoleStaff},
		"zero user":    {Tenant: tenantID, Role: shared.RoleStaff, Version: 1},
	} {
		if _, err := RestoreMembership(rec); !errors.Is(err, ErrInvalidMembership) {
			t.Errorf("%s: %v, want ErrInvalidMembership", name, err)
		}
	}
}

func member(t *testing.T, role shared.Role) *Membership {
	t.Helper()
	m, err := RestoreMembership(MembershipRecord{Tenant: tenantID, User: staffID, Role: role, Version: 1, JoinedAt: t0})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMembership_ChangeRole(t *testing.T) {
	t.Parallel()

	m := member(t, shared.RoleStaff)
	if err := m.ChangeRole(shared.RoleAdmin); err != nil || m.Role() != shared.RoleAdmin {
		t.Errorf("staff → admin: %q, %v", m.Role(), err)
	}
	if err := m.ChangeRole(shared.RoleAdmin); err != nil {
		t.Errorf("the same role again: %v", err)
	}

	// Nobody becomes owner by a role change.
	err := m.ChangeRole(shared.RoleOwner)
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || m.Role() != shared.RoleAdmin {
		t.Errorf("→ owner: %v, role now %q; want a ValidationError and no change", err, m.Role())
	}

	// The owner's role never changes (ADR 0011).
	owner := member(t, shared.RoleOwner)
	if err := owner.ChangeRole(shared.RoleViewer); !errors.Is(err, ErrOwnerFixed) || owner.Role() != shared.RoleOwner {
		t.Errorf("owner → viewer: %v, role now %q; want ErrOwnerFixed", err, owner.Role())
	}
}

func TestMembership_CheckRemovable(t *testing.T) {
	t.Parallel()

	for _, role := range []shared.Role{shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer} {
		if err := member(t, role).CheckRemovable(); err != nil {
			t.Errorf("%s: %v", role, err)
		}
	}
	if err := member(t, shared.RoleOwner).CheckRemovable(); !errors.Is(err, ErrOwnerFixed) {
		t.Errorf("owner: %v, want ErrOwnerFixed", err)
	}
}

func TestMembership_ManagesTeam(t *testing.T) {
	t.Parallel()

	for role, want := range map[shared.Role]bool{
		shared.RoleOwner: true, shared.RoleAdmin: true, shared.RoleStaff: false, shared.RoleViewer: false,
	} {
		if got := member(t, role).ManagesTeam(); got != want {
			t.Errorf("%s ManagesTeam = %v, want %v", role, got, want)
		}
		if got := member(t, role).Allows(identity.PermMembersManage); got != want {
			t.Errorf("%s Allows(members:manage) = %v, want %v", role, got, want)
		}
	}
}

func TestErrOwnerFixed_IsForbidden(t *testing.T) {
	t.Parallel()

	// The contract answers 403 forbidden when the target is the owner.
	if !errors.Is(ErrOwnerFixed, shared.ErrForbidden) {
		t.Error("ErrOwnerFixed does not match ErrForbidden")
	}
}
