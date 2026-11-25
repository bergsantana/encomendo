package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

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
	root  *txRoot
}

// txRoot is the state one database transaction shares with its savepoints.
type txRoot struct {
	mu         sync.Mutex
	savepoints int
	// broken is set when undoing a failed nested call failed, so the nested
	// call's writes may still be in the transaction: it must not commit.
	broken error
}

func (r *txRoot) nextSavepoint() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.savepoints++
	return "sp_" + strconv.Itoa(r.savepoints)
}

func (r *txRoot) breakWith(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.broken == nil {
		r.broken = err
	}
}

func (r *txRoot) err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.broken
}

// WithinTx follows the appshared.TxManager contract. A nested call becomes a
// savepoint of the outer transaction, so its failure undoes only its own
// writes and events.
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
		return nested(ctx, outer, fn)
	}
	return m.begin(ctx, scope, fn)
}

// begin runs fn in a new transaction that starts by setting scope. fn's
// error comes back unchanged; a failure to begin, set the scope or commit is
// mapped like a repository error. A panic rolls back and propagates.
func (m *TxManager) begin(ctx context.Context, scope appshared.Scope, fn func(ctx context.Context) error) error {
	root := &txRoot{}
	var fnErr error
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(setScopeSQL, setting(scope.TenantID), setting(scope.UserID), setting(scope.ClientAccountID)).Error; err != nil {
			return fmt.Errorf("set rls scope: %w", err)
		}
		fnErr = fn(context.WithValue(ctx, txKey{}, &txConn{db: tx, scope: scope, root: root}))
		if fnErr != nil {
			return fnErr
		}
		return root.err()
	})
	if fnErr != nil {
		return fnErr
	}
	if err != nil {
		return fmt.Errorf("transaction: %w", mapError(err))
	}
	return nil
}

// nested runs fn under a savepoint of outer's transaction and rolls back to
// it when fn fails or panics. The savepoint statements run themselves, with
// their errors checked, because the GORM Postgres driver discards them; and
// they ignore the cancellation of ctx, because a canceled inner context must
// not stop the undo of the inner work. If the undo still fails, the whole
// transaction is refused at commit.
func nested(ctx context.Context, outer *txConn, fn func(ctx context.Context) error) (err error) {
	sp := outer.root.nextSavepoint()
	db := outer.db.WithContext(context.WithoutCancel(ctx))
	if err := db.Exec("SAVEPOINT " + sp).Error; err != nil {
		return fmt.Errorf("savepoint: %w", mapError(err))
	}
	done := false
	defer func() {
		if done {
			return // released with the transaction at commit
		}
		if rbErr := db.Exec("ROLLBACK TO SAVEPOINT " + sp).Error; rbErr != nil {
			rbErr = fmt.Errorf("roll back to savepoint: %w", mapError(rbErr))
			outer.root.breakWith(rbErr)
			err = errors.Join(err, rbErr)
		}
	}()
	err = fn(context.WithValue(ctx, txKey{}, &txConn{db: outer.db, scope: outer.scope, root: outer.root}))
	done = err == nil
	return err
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
