package shared

import (
	"errors"
	"testing"
)

var (
	someUser   = UserID(NewV7(1, [10]byte{1}))
	someTenant = TenantID(NewV7(2, [10]byte{2}))
	someClient = ClientAccountID(NewV7(3, [10]byte{3}))
)

func TestParseRole(t *testing.T) {
	t.Parallel()

	for _, r := range []Role{RoleOwner, RoleAdmin, RoleStaff, RoleViewer} {
		got, err := ParseRole(string(r))
		if err != nil || got != r || !got.Valid() {
			t.Errorf("ParseRole(%q) = %q, %v", r, got, err)
		}
	}
	for _, s := range []string{"", "client", "Owner", "root"} {
		if _, err := ParseRole(s); !errors.Is(err, ErrInvalidRole) || !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseRole(%q) error = %v, want ErrInvalidRole", s, err)
		}
	}
}

func TestNewBackofficePrincipal_WithTenant(t *testing.T) {
	t.Parallel()

	p, err := NewBackofficePrincipal(someUser, someTenant, RoleStaff)
	if err != nil {
		t.Fatal(err)
	}
	if p.Audience() != AudienceBackoffice || p.UserID() != someUser || p.TenantID() != someTenant ||
		p.Role() != RoleStaff || !p.HasTenant() || !p.ClientAccountID().IsZero() || p.IsZero() {
		t.Errorf("principal = %+v", p)
	}
}

func TestNewBackofficePrincipal_WithoutTenant(t *testing.T) {
	t.Parallel()

	// Right after sign-up, or when a user of several stores has not picked
	// one yet, the session has no tenant and therefore no role (ADR 0010).
	p, err := NewBackofficePrincipal(someUser, TenantID{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.HasTenant() || p.Role() != "" || p.UserID() != someUser {
		t.Errorf("principal = %+v", p)
	}
}

func TestNewBackofficePrincipal_RejectsInconsistentInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		user   UserID
		tenant TenantID
		role   Role
	}{
		{"no user", UserID{}, someTenant, RoleOwner},
		{"tenant without role", someUser, someTenant, ""},
		{"role without tenant", someUser, TenantID{}, RoleAdmin},
		{"unknown role", someUser, someTenant, Role("client")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewBackofficePrincipal(tt.user, tt.tenant, tt.role); !errors.Is(err, ErrInvalidPrincipal) {
				t.Errorf("error = %v, want ErrInvalidPrincipal", err)
			}
		})
	}
}

func TestNewClientPrincipal(t *testing.T) {
	t.Parallel()

	p, err := NewClientPrincipal(someClient)
	if err != nil {
		t.Fatal(err)
	}
	// A client account is global: it never carries a tenant or a role
	// (docs/architecture.md section 5.1).
	if p.Audience() != AudienceClient || p.ClientAccountID() != someClient ||
		p.HasTenant() || p.Role() != "" || !p.UserID().IsZero() {
		t.Errorf("principal = %+v", p)
	}
	if _, err := NewClientPrincipal(ClientAccountID{}); !errors.Is(err, ErrInvalidPrincipal) {
		t.Errorf("zero account error = %v, want ErrInvalidPrincipal", err)
	}
}

func TestPrincipal_ZeroValue(t *testing.T) {
	t.Parallel()

	var p Principal
	if !p.IsZero() || p.HasTenant() || p.Audience() != "" {
		t.Errorf("zero principal = %+v", p)
	}
}
