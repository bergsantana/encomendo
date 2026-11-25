package sharedports

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var errFromFn = errors.New("fn failed")

var (
	txTenant  = shared.TenantID(shared.NewV7(1_700_000_000_000, [10]byte{1}))
	txUser    = shared.UserID(shared.NewV7(1_700_000_000_000, [10]byte{2}))
	txAccount = shared.ClientAccountID(shared.NewV7(1_700_000_000_000, [10]byte{3}))
)

// RunTxManagerContract checks the part of the TxManager contract every
// implementation shares: fn runs once with the caller's scope, its error
// comes back unchanged, a canceled context or a mixed scope never starts a
// transaction, and a nested call keeps the outer scope. Rollback, savepoints
// and the RLS settings are checked against Postgres in test/integration and,
// for events, by RunEventPublisherContract.
func RunTxManagerContract(t *testing.T, newTx func(t *testing.T) appshared.TxManager) {
	t.Run("runs fn once and commits", func(t *testing.T) { txRunsOnce(t, newTx(t)) })
	t.Run("returns fn's error unchanged", func(t *testing.T) { txReturnsFnError(t, newTx(t)) })
	t.Run("fn sees the caller's scope", func(t *testing.T) { txKeepsScope(t, newTx(t)) })
	t.Run("canceled context does not call fn", func(t *testing.T) { txCanceled(t, newTx(t)) })
	t.Run("refuses a client account mixed with a tenant", func(t *testing.T) { txMixedScope(t, newTx(t)) })
	t.Run("nested call joins the outer one", func(t *testing.T) { txNested(t, newTx(t)) })
	t.Run("nested call cannot change the scope", func(t *testing.T) { txNestedScope(t, newTx(t)) })
}

func txRunsOnce(t *testing.T, tx appshared.TxManager) {
	calls := 0
	err := tx.WithinTx(context.Background(), func(context.Context) error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Errorf("WithinTx = %v with %d calls, want nil and 1", err, calls)
	}
}

func txReturnsFnError(t *testing.T, tx appshared.TxManager) {
	err := tx.WithinTx(context.Background(), func(context.Context) error {
		return errFromFn
	})
	if !errors.Is(err, errFromFn) {
		t.Errorf("WithinTx error = %v, want fn's error", err)
	}
}

func txKeepsScope(t *testing.T, tx appshared.TxManager) {
	want := appshared.Scope{TenantID: txTenant, UserID: txUser}
	var got appshared.Scope
	err := tx.WithinTx(appshared.WithScope(context.Background(), want), func(ctx context.Context) error {
		got = appshared.ScopeFrom(ctx)
		return nil
	})
	if err != nil || got != want {
		t.Errorf("scope inside fn = %+v (err %v), want %+v", got, err, want)
	}
}

func txCanceled(t *testing.T, tx appshared.TxManager) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := tx.WithinTx(ctx, func(context.Context) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) || called {
		t.Errorf("WithinTx on a canceled ctx = %v, called %v; want context.Canceled and no call", err, called)
	}
}

func txMixedScope(t *testing.T, tx appshared.TxManager) {
	ctx := appshared.WithScope(context.Background(), appshared.Scope{TenantID: txTenant, ClientAccountID: txAccount})
	called := false
	err := tx.WithinTx(ctx, func(context.Context) error {
		called = true
		return nil
	})
	if !errors.Is(err, appshared.ErrInvalidScope) || called {
		t.Errorf("WithinTx with tenant + client = %v, called %v; want ErrInvalidScope and no call", err, called)
	}
}

func txNested(t *testing.T, tx appshared.TxManager) {
	inner := 0
	err := tx.WithinTx(context.Background(), func(ctx context.Context) error {
		return tx.WithinTx(ctx, func(context.Context) error {
			inner++
			return errFromFn
		})
	})
	if !errors.Is(err, errFromFn) || inner != 1 {
		t.Errorf("nested WithinTx = %v with %d inner calls, want fn's error and 1", err, inner)
	}
}

func txNestedScope(t *testing.T, tx appshared.TxManager) {
	outer := appshared.WithScope(context.Background(), appshared.Scope{TenantID: txTenant, UserID: txUser})
	var innerErr error
	called := false
	err := tx.WithinTx(outer, func(ctx context.Context) error {
		other := appshared.WithScope(ctx, appshared.Scope{UserID: txUser})
		innerErr = tx.WithinTx(other, func(context.Context) error {
			called = true
			return nil
		})
		return nil
	})
	if err != nil {
		t.Fatalf("outer WithinTx error = %v", err)
	}
	if !errors.Is(innerErr, appshared.ErrInvalidScope) || called {
		t.Errorf("nested WithinTx with another scope = %v, called %v; want ErrInvalidScope and no call", innerErr, called)
	}
}
