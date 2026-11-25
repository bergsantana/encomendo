package shared

import (
	"context"
	"errors"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrNoTransaction means a port that must run inside TxManager.WithinTx,
// such as EventPublisher, was called outside one. It is a wiring bug.
var ErrNoTransaction = errors.New("no transaction in context")

// TxManager runs a unit of work in one database transaction.
type TxManager interface {
	// WithinTx calls fn once, inside one transaction, with a context that
	// carries the transaction and the Scope of ctx. Postgres reads the scope
	// for row level security: app.tenant_id, app.user_id and
	// app.client_account_id are set transaction-local (ADR 0002). With no
	// scope set, tenant tables show no rows, so a forgotten scope fails
	// closed.
	//
	// It commits when fn returns nil and rolls back otherwise, returning
	// fn's error unchanged so callers can use errors.Is. A canceled ctx
	// returns its error without calling fn. A WithinTx inside another joins
	// the outer transaction.
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Scope is whose rows a transaction may see. A staff request carries its
// tenant and user; a store page request carries the tenant resolved from the
// subdomain; a client area request carries the client account. A use case
// may set a narrower or new scope, as tenant creation does for the new
// tenant's id.
type Scope struct {
	TenantID        shared.TenantID
	UserID          shared.UserID
	ClientAccountID shared.ClientAccountID
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
