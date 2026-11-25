package fake

import (
	"context"
	"slices"
	"sync"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

// TxManager is a TxManager without a database. It records the scope of
// every call, so a use case test can check that the right tenant was set,
// and it runs the work staged by other fakes (such as Events) only when fn
// succeeds.
type TxManager struct {
	mu     sync.Mutex
	scopes []appshared.Scope
}

// NewTxManager returns a TxManager with no calls recorded.
func NewTxManager() *TxManager {
	return &TxManager{}
}

type txKey struct{}

// txState is the work other fakes stage until the outer transaction
// commits.
type txState struct {
	mu       sync.Mutex
	onCommit []func()
}

func (s *txState) stage(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCommit = append(s.onCommit, fn)
}

func txFrom(ctx context.Context) *txState {
	s, _ := ctx.Value(txKey{}).(*txState) // nil means no transaction
	return s
}

// WithinTx follows the TxManager contract: fn runs once with the caller's
// scope, its error is returned unchanged, a canceled ctx is refused, and a
// nested call joins the outer transaction.
func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.record(appshared.ScopeFrom(ctx))
	if txFrom(ctx) != nil {
		return fn(ctx)
	}
	state := &txState{}
	if err := fn(context.WithValue(ctx, txKey{}, state)); err != nil {
		return err
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

// Scopes returns the scope of every WithinTx call so far, nested calls
// included, oldest first.
func (m *TxManager) Scopes() []appshared.Scope {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.scopes)
}
