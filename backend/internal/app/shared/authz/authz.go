package authz

import (
	"context"
	"fmt"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Authorization errors. Both match shared.ErrForbidden.
var (
	// ErrTenantRequired means a staff user called a tenant use case before
	// picking a store: right after sign-up, or with several stores and none
	// chosen (ADR 0010). The API reports it as tenant_required so the apps
	// can open the store picker.
	ErrTenantRequired = fmt.Errorf("authz: no store selected: %w", shared.ErrForbidden)
	// ErrScopeMismatch means the RLS scope in the context is not the one the
	// staff caller's token gives, so RLS and authorization would judge
	// different tenants. It happens when something replaced the scope after
	// WithPrincipal, such as a store page middleware running on a staff
	// request (ADR 0009).
	ErrScopeMismatch = fmt.Errorf("authz: request scope does not match the caller: %w", shared.ErrForbidden)
)

type principalKey struct{}

// WithPrincipal returns a copy of ctx that carries p and the Scope that
// matches it, so RLS never sees more than the caller may:
//   - a staff principal replaces the scope with its own tenant and user;
//   - a client principal on a store page (the scope already has the store's
//     tenant, set by the store page middleware, which runs first) leaves the
//     scope alone: the account never joins a tenant scope;
//   - a client principal anywhere else gets a scope of its account only.
//
// The zero principal leaves ctx unchanged. Store page routes must attach
// client principals only; a staff token there would replace the store's
// tenant with its own.
func WithPrincipal(ctx context.Context, p shared.Principal) context.Context {
	if p.IsZero() {
		return ctx
	}
	ctx = context.WithValue(ctx, principalKey{}, p)
	if p.Audience() != shared.AudienceClient {
		return appshared.WithScope(ctx, staffScope(p))
	}
	if !appshared.ScopeFrom(ctx).TenantID.IsZero() {
		return ctx
	}
	return appshared.WithScope(ctx, appshared.Scope{ClientAccountID: p.ClientAccountID()})
}

func staffScope(p shared.Principal) appshared.Scope {
	return appshared.Scope{TenantID: p.TenantID(), UserID: p.UserID()}
}

// PrincipalFrom returns the caller in ctx and whether there is one.
func PrincipalFrom(ctx context.Context) (shared.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(shared.Principal)
	return p, ok
}

// Require returns the caller when it is a staff user acting in a tenant
// whose role grants perm. Every tenant use case calls it first. It returns
// shared.ErrUnauthenticated without a caller, ErrTenantRequired when no store
// is selected, ErrScopeMismatch when the RLS scope is not the caller's, and
// an error matching shared.ErrForbidden for a client account or a role
// without perm.
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
// tenant, whose RLS scope is still the one WithPrincipal gave it. Use cases
// that work across stores call it, such as creating a store. It returns
// shared.ErrUnauthenticated without a caller, ErrScopeMismatch when the
// scope was replaced, and an error matching shared.ErrForbidden for a
// client account.
func RequireUser(ctx context.Context) (shared.Principal, error) {
	p, ok := PrincipalFrom(ctx)
	if !ok {
		return shared.Principal{}, shared.ErrUnauthenticated
	}
	if p.Audience() != shared.AudienceBackoffice {
		return shared.Principal{}, fmt.Errorf("authz: %s token on a staff use case: %w", p.Audience(), shared.ErrForbidden)
	}
	if appshared.ScopeFrom(ctx) != staffScope(p) {
		return shared.Principal{}, ErrScopeMismatch
	}
	return p, nil
}
