package shared

import (
	"context"
	"errors"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Errors for misuse of the transaction ports. They mean the calling code is
// wrong, not the input, so they match none of the domain sentinels and reach
// the API as a logged 500 (ADR 0009).
var (
	// ErrNoTransaction means a port that must run inside TxManager.WithinTx,
	// such as EventPublisher, was called outside one.
	ErrNoTransaction = errors.New("no transaction in context")
	// ErrInvalidScope means a Scope mixes a client account with a tenant or
	// a user, or a nested WithinTx asked for a scope other than the outer
	// one.
	ErrInvalidScope = errors.New("invalid transaction scope")
)

// TxManager runs a unit of work in one database transaction.
type TxManager interface {
	// WithinTx calls fn once, inside one transaction, with a context that
	// carries the transaction and the Scope of ctx. It commits when fn
	// returns nil and rolls back otherwise, returning fn's error unchanged so
	// callers can use errors.Is. A failed commit returns its error, mapped
	// like a repository error (a serialization failure matches
	// shared.ErrConflict). A canceled ctx returns its error without calling
	// fn. A scope that fails Scope.Validate returns ErrInvalidScope without
	// calling fn.
	//
	// RLS reads the scope. The Postgres adapter sets app.tenant_id,
	// app.user_id and app.client_account_id transaction-local on every
	// transaction, to '' when unset, and policies read them through helpers
	// that turn '' into NULL, so the zero scope sees no tenant rows even on a
	// pooled connection that served another tenant before (ADR 0009).
	//
	// A WithinTx inside another joins the outer transaction through a
	// savepoint: when the inner fn fails, its writes and events are undone
	// and its error comes back to the outer fn, which decides whether the
	// whole transaction fails. The inner call keeps the outer scope; asking
	// for a different one returns ErrInvalidScope.
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Scope is whose rows a transaction may see:
//   - a staff request: its tenant (if a store is selected) and its user;
//   - a store page request: the tenant resolved from the subdomain;
//   - a client area request: the client account, and nothing else.
//
// A client account never shares a scope with a tenant or a user. Postgres
// ORs permissive policies, so that mix would read the whole store plus the
// client's orders at every other store (docs/architecture.md section 5.5,
// item 7).
//
// Only these set a scope themselves instead of taking it from the
// principal (ADR 0009): the store page middleware; tenant creation (the new
// tenant); login, refresh and tenant switch (the user they authenticated);
// invitation preview (the tenant inside the token); invitation acceptance
// (first the tenant inside the token, then, in a second transaction, the
// user it created or checked).
type Scope struct {
	TenantID        shared.TenantID
	UserID          shared.UserID
	ClientAccountID shared.ClientAccountID
}

// Validate returns ErrInvalidScope when s has a client account together with
// a tenant or a user.
func (s Scope) Validate() error {
	if !s.ClientAccountID.IsZero() && (!s.TenantID.IsZero() || !s.UserID.IsZero()) {
		return ErrInvalidScope
	}
	return nil
}

type scopeKey struct{}

// WithScope returns a copy of ctx that carries s, replacing any scope it had.
func WithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, s)
}

// ScopeFrom returns the scope in ctx, or the zero Scope (no rows of any
// tenant) when none was set.
func ScopeFrom(ctx context.Context) Scope {
	s, _ := ctx.Value(scopeKey{}).(Scope) // absent means the zero scope, by design
	return s
}
