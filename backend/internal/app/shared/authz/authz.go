package authz

import (
	"context"
	"fmt"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrTenantRequired means a staff user called a tenant use case before
// picking a store: right after sign-up, or with several stores and none
// chosen (ADR 0010). It matches shared.ErrForbidden; the API reports it as
// tenant_required so the apps can open the tenant picker.
var ErrTenantRequired = fmt.Errorf("authz: no store selected: %w", shared.ErrForbidden)

type principalKey struct{}

// WithPrincipal returns a copy of ctx that carries p and the Scope that
// matches it, so RLS can never see more than the caller may. A staff
// principal replaces the scope with its own tenant and user. A client
// principal keeps the scope's tenant (the store page it is on) and adds its
// account. The zero principal leaves ctx unchanged.
func WithPrincipal(ctx context.Context, p shared.Principal) context.Context {
	if p.IsZero() {
		return ctx
	}
	ctx = context.WithValue(ctx, principalKey{}, p)
	if p.Audience() == shared.AudienceClient {
		s := appshared.ScopeFrom(ctx)
		s.UserID = shared.UserID{}
		s.ClientAccountID = p.ClientAccountID()
		return appshared.WithScope(ctx, s)
	}
	return appshared.WithScope(ctx, appshared.Scope{TenantID: p.TenantID(), UserID: p.UserID()})
}

// PrincipalFrom returns the caller in ctx and whether there is one.
func PrincipalFrom(ctx context.Context) (shared.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(shared.Principal)
	return p, ok
}

// Require returns the caller when it is a staff user acting in a tenant
// whose role grants perm. Every tenant use case calls it first. It returns
// shared.ErrUnauthenticated without a caller, ErrTenantRequired when no store
// is selected, and an error matching shared.ErrForbidden for a client
// account or a role without perm.
func Require(ctx context.Context, perm identity.Permission) (shared.Principal, error) {
	p, err := RequireUser(ctx)
	if err != nil {
		return shared.Principal{}, err
	}
	if !p.HasTenant() {
		return shared.Principal{}, ErrTenantRequired
	}
	if !identity.Allows(p.Role(), perm) {
		return shared.Principal{}, fmt.Errorf("authz: role %s lacks %s: %w", p.Role(), perm, shared.ErrForbidden)
	}
	return p, nil
}

// RequireUser returns the caller when it is a staff user, with or without a
// tenant. Use cases that work across stores call it, such as creating a
// store. It returns shared.ErrUnauthenticated without a caller and an error
// matching shared.ErrForbidden for a client account.
func RequireUser(ctx context.Context) (shared.Principal, error) {
	p, ok := PrincipalFrom(ctx)
	if !ok {
		return shared.Principal{}, shared.ErrUnauthenticated
	}
	if p.Audience() != shared.AudienceBackoffice {
		return shared.Principal{}, fmt.Errorf("authz: %s token on a staff use case: %w", p.Audience(), shared.ErrForbidden)
	}
	return p, nil
}
