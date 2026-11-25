package authz

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	user    = shared.UserID(shared.NewV7(1, [10]byte{1}))
	tenant  = shared.TenantID(shared.NewV7(2, [10]byte{2}))
	account = shared.ClientAccountID(shared.NewV7(3, [10]byte{3}))
)

func staff(t *testing.T, role shared.Role) shared.Principal {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(user, tenant, role)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func tenantless(t *testing.T) shared.Principal {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(user, shared.TenantID{}, "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func client(t *testing.T) shared.Principal {
	t.Helper()
	p, err := shared.NewClientPrincipal(account)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRequire_GrantsWhatTheRoleAllows(t *testing.T) {
	t.Parallel()

	ctx := WithPrincipal(context.Background(), staff(t, shared.RoleAdmin))
	got, err := Require(ctx, identity.PermMembersManage)
	if err != nil {
		t.Fatalf("Require error = %v", err)
	}
	if got.UserID() != user || got.TenantID() != tenant {
		t.Errorf("Require returned %+v, want the caller", got)
	}
}

func TestRequire_Denies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     func(t *testing.T) context.Context
		perm    identity.Permission
		wantErr error
	}{
		{
			name:    "no principal",
			ctx:     func(*testing.T) context.Context { return context.Background() },
			perm:    identity.PermOrderRead,
			wantErr: shared.ErrUnauthenticated,
		},
		{
			name: "role lacks the permission",
			ctx: func(t *testing.T) context.Context {
				return WithPrincipal(context.Background(), staff(t, shared.RoleStaff))
			},
			perm:    identity.PermSettingsWrite,
			wantErr: shared.ErrForbidden,
		},
		{
			name:    "staff user without a tenant",
			ctx:     func(t *testing.T) context.Context { return WithPrincipal(context.Background(), tenantless(t)) },
			perm:    identity.PermOrderRead,
			wantErr: ErrTenantRequired,
		},
		{
			// Client accounts never use admin use cases (docs/architecture.md section 5.1).
			name:    "client account",
			ctx:     func(t *testing.T) context.Context { return WithPrincipal(context.Background(), client(t)) },
			perm:    identity.PermOrderRead,
			wantErr: shared.ErrForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := Require(tt.ctx(t), tt.perm)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Require error = %v, want %v", err, tt.wantErr)
			}
			if !p.IsZero() {
				t.Errorf("Require returned %+v on error, want the zero principal", p)
			}
		})
	}
}

func TestErrTenantRequired_IsForbidden(t *testing.T) {
	t.Parallel()

	if !errors.Is(ErrTenantRequired, shared.ErrForbidden) {
		t.Error("ErrTenantRequired should match shared.ErrForbidden")
	}
}

func TestRequireUser(t *testing.T) {
	t.Parallel()

	for _, p := range []shared.Principal{tenantless(t), staff(t, shared.RoleViewer)} {
		got, err := RequireUser(WithPrincipal(context.Background(), p))
		if err != nil || got != p {
			t.Errorf("RequireUser(%+v) = %+v, %v", p, got, err)
		}
	}
	if _, err := RequireUser(context.Background()); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("no principal error = %v, want ErrUnauthenticated", err)
	}
	if _, err := RequireUser(WithPrincipal(context.Background(), client(t))); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("client error = %v, want ErrForbidden", err)
	}
}

func TestWithPrincipal_SetsTheStaffScope(t *testing.T) {
	t.Parallel()

	// A stale scope must not survive: the staff principal decides the tenant.
	stale := appshared.WithScope(context.Background(), appshared.Scope{TenantID: shared.TenantID(shared.NewV7(9, [10]byte{9}))})
	ctx := WithPrincipal(stale, staff(t, shared.RoleOwner))

	want := appshared.Scope{TenantID: tenant, UserID: user}
	if got := appshared.ScopeFrom(ctx); got != want {
		t.Errorf("scope = %+v, want %+v", got, want)
	}
	if got, ok := PrincipalFrom(ctx); !ok || got.UserID() != user {
		t.Errorf("PrincipalFrom = %+v, %v", got, ok)
	}

	noTenant := WithPrincipal(context.Background(), tenantless(t))
	if got := appshared.ScopeFrom(noTenant); got != (appshared.Scope{UserID: user}) {
		t.Errorf("tenant-less scope = %+v, want only the user", got)
	}
}

func TestWithPrincipal_ClientKeepsTheStoreTenant(t *testing.T) {
	t.Parallel()

	// A logged-in client on a store page: the tenant comes from the
	// subdomain, the account from the token.
	store := appshared.WithScope(context.Background(), appshared.Scope{TenantID: tenant})
	ctx := WithPrincipal(store, client(t))

	want := appshared.Scope{TenantID: tenant, ClientAccountID: account}
	if got := appshared.ScopeFrom(ctx); got != want {
		t.Errorf("scope = %+v, want %+v", got, want)
	}
}

func TestWithPrincipal_IgnoresTheZeroPrincipal(t *testing.T) {
	t.Parallel()

	ctx := WithPrincipal(context.Background(), shared.Principal{})
	if _, ok := PrincipalFrom(ctx); ok {
		t.Error("PrincipalFrom reports a principal after WithPrincipal(zero)")
	}
	if got := appshared.ScopeFrom(ctx); got != (appshared.Scope{}) {
		t.Errorf("scope = %+v, want zero", got)
	}
}
