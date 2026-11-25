package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

// setScopeSQL sets the three RLS settings transaction-local, in one round
// trip. Every transaction sets all three, the unset ones to the empty
// string, so no transaction runs with a value it did not set; the 0005
// helpers read the empty string as no scope (ADR 0009).
const setScopeSQL = `SELECT set_config('app.tenant_id', ?, true),
	set_config('app.user_id', ?, true),
	set_config('app.client_account_id', ?, true)`

// TxManager implements appshared.TxManager with GORM transactions on the
// encomendo_app pool. It is what makes RLS see the caller: every transaction
// starts by setting the scope of its context.
type TxManager struct {
	db *gorm.DB
}

// NewTxManager returns a TxManager over db.
func NewTxManager(db *DB) *TxManager {
	return &TxManager{db: db.gorm}
}

type txKey struct{}

// txConn is the transaction a context carries, with the scope it was opened
// with.
type txConn struct {
	db    *gorm.DB
	scope appshared.Scope
}

// WithinTx follows the appshared.TxManager contract. A nested call becomes a
// savepoint of the outer transaction (GORM's nested Transaction), so its
// failure undoes only its own writes and events.
func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	scope := appshared.ScopeFrom(ctx)
	if err := scope.Validate(); err != nil {
		return err
	}
	if outer, ok := ctx.Value(txKey{}).(*txConn); ok {
		if outer.scope != scope {
			return appshared.ErrInvalidScope
		}
		return run(ctx, outer.db.WithContext(ctx), scope, false, fn)
	}
	return run(ctx, m.db.WithContext(ctx), scope, true, fn)
}

// run calls fn inside a transaction on db, or a savepoint when db is already
// in one. fn's error comes back unchanged; a failure to begin, set the scope
// or commit is mapped like a repository error.
func run(ctx context.Context, db *gorm.DB, scope appshared.Scope, setScope bool, fn func(ctx context.Context) error) error {
	var fnErr error
	err := db.Transaction(func(tx *gorm.DB) error {
		if setScope {
			if err := tx.Exec(setScopeSQL, setting(scope.TenantID), setting(scope.UserID), setting(scope.ClientAccountID)).Error; err != nil {
				return fmt.Errorf("set rls scope: %w", err)
			}
		}
		fnErr = fn(context.WithValue(ctx, txKey{}, &txConn{db: tx, scope: scope}))
		return fnErr
	})
	if fnErr != nil {
		return fnErr
	}
	if err != nil {
		return fmt.Errorf("transaction: %w", mapError(err))
	}
	return nil
}

// conn returns the transaction in ctx, bound to ctx. Repositories run only
// inside TxManager.WithinTx: outside it no scope is set and RLS would show
// nothing, so they fail loudly with appshared.ErrNoTransaction instead.
func conn(ctx context.Context) (*gorm.DB, error) {
	c, ok := ctx.Value(txKey{}).(*txConn)
	if !ok {
		return nil, appshared.ErrNoTransaction
	}
	return c.db.WithContext(ctx), nil
}

type scopeID interface {
	IsZero() bool
	String() string
}

// setting is the value of an RLS setting for id: the empty string when
// unset.
func setting(id scopeID) string {
	if id.IsZero() {
		return ""
	}
	return id.String()
}
