package fake

import (
	"context"
	"slices"
	"sync"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

// TxManager is a TxManager without a database. It records the scope of
// every call, so a use case test can check that the right tenant was set,
// and it runs the work staged by other fakes (such as Events) only when the
// transaction commits. Nested calls behave like savepoints.
type TxManager struct {
	mu     sync.Mutex
	scopes []appshared.Scope
}

// NewTxManager returns a TxManager with no calls recorded.
func NewTxManager() *TxManager {
	return &TxManager{}
}

type txKey struct{}

// txState is one transaction or savepoint: the scope it started with (what
// RLS would see) and the work other fakes stage until it commits.
type txState struct {
	scope    appshared.Scope
	mu       sync.Mutex
	onCommit []func()
}

func (s *txState) stage(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCommit = append(s.onCommit, fn)
}

// adopt moves a released savepoint's work into s.
func (s *txState) adopt(child *txState) {
	child.mu.Lock()
	work := child.onCommit
	child.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCommit = append(s.onCommit, work...)
}

func txFrom(ctx context.Context) *txState {
	s, _ := ctx.Value(txKey{}).(*txState) // nil means no transaction
	return s
}

// WithinTx follows the TxManager contract: fn runs once with the caller's
// scope and its error is returned unchanged; a canceled ctx or an invalid
// scope is refused; a nested call must keep the outer scope and drops its
// own staged work when it fails.
func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	scope := appshared.ScopeFrom(ctx)
	if err := scope.Validate(); err != nil {
		return err
	}
	outer := txFrom(ctx)
	if outer != nil && scope != outer.scope {
		return appshared.ErrInvalidScope
	}
	m.record(scope)
	state := &txState{scope: scope}
	if err := fn(context.WithValue(ctx, txKey{}, state)); err != nil {
		return err
	}
	if outer != nil {
		outer.adopt(state)
		return nil
	}
	for _, commit := range state.onCommit {
		commit()
	}
	return nil
}

func (m *TxManager) record(s appshared.Scope) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scopes = append(m.scopes, s)
}

// Scopes returns the scope of every accepted WithinTx call so far, nested
// calls included, oldest first.
func (m *TxManager) Scopes() []appshared.Scope {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.scopes)
}
