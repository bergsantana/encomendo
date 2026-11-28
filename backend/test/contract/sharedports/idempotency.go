package sharedports

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// IdempotencyHarness is an Idempotency with the TxManager it writes
// through, and two stores. For Postgres both stores must exist.
type IdempotencyHarness struct {
	Tx           appshared.TxManager
	Keys         appshared.Idempotency
	Store, Other shared.TenantID
	// Resource is an id the adapter can store for a created resource.
	Resource string
}

// RunIdempotencyContract checks the Idempotency promise: a key remembered
// in a committed transaction is recalled with its resource, the same key
// with another request is refused, a rolled back write forgets it, keys are
// per store, and both calls need a transaction.
func RunIdempotencyContract(t *testing.T, newHarness func(t *testing.T) IdempotencyHarness) {
	t.Run("a new key is not recalled", func(t *testing.T) { idemNew(t, newHarness(t)) })
	t.Run("a remembered key is recalled with its resource", func(t *testing.T) { idemRecall(t, newHarness(t)) })
	t.Run("the same key with another request is refused", func(t *testing.T) { idemReused(t, newHarness(t)) })
	t.Run("a rolled back write forgets the key", func(t *testing.T) { idemRollback(t, newHarness(t)) })
	t.Run("keys are per store", func(t *testing.T) { idemPerStore(t, newHarness(t)) })
	t.Run("a key stored by another call is in flight", func(t *testing.T) { idemInFlight(t, newHarness(t)) })
	t.Run("needs a transaction", func(t *testing.T) { idemNeedsTx(t, newHarness(t)) })
}

func idemKey(name string, body byte) appshared.IdempotencyKey {
	return appshared.IdempotencyKey{Key: name, RequestHash: [32]byte{body}}
}

func inStore(tenant shared.TenantID) context.Context {
	return appshared.WithScope(context.Background(), appshared.Scope{TenantID: tenant})
}

func recall(t *testing.T, h IdempotencyHarness, tenant shared.TenantID, k appshared.IdempotencyKey) (string, bool, error) {
	t.Helper()
	var res string
	var ok bool
	err := h.Tx.WithinTx(inStore(tenant), func(ctx context.Context) error {
		var err error
		res, ok, err = h.Keys.Recall(ctx, k)
		return err
	})
	return res, ok, err
}

func remember(t *testing.T, h IdempotencyHarness, tenant shared.TenantID, k appshared.IdempotencyKey) {
	t.Helper()
	err := h.Tx.WithinTx(inStore(tenant), func(ctx context.Context) error {
		return h.Keys.Remember(ctx, k, h.Resource, time.Hour)
	})
	if err != nil {
		t.Fatalf("remember %s: %v", k.Key, err)
	}
}

func idemNew(t *testing.T, h IdempotencyHarness) {
	if _, ok, err := recall(t, h, h.Store, idemKey("novo", 1)); err != nil || ok {
		t.Errorf("new key: ok %v err %v, want not recalled", ok, err)
	}
}

func idemRecall(t *testing.T, h IdempotencyHarness) {
	k := idemKey("pedido-1", 1)
	remember(t, h, h.Store, k)
	res, ok, err := recall(t, h, h.Store, k)
	if err != nil || !ok || res != h.Resource {
		t.Errorf("recall = %q %v %v, want %q", res, ok, err, h.Resource)
	}
}

func idemReused(t *testing.T, h IdempotencyHarness) {
	remember(t, h, h.Store, idemKey("pedido-2", 1))
	if _, _, err := recall(t, h, h.Store, idemKey("pedido-2", 2)); !errors.Is(err, appshared.ErrIdempotencyKeyReused) {
		t.Errorf("same key, other request: err = %v, want ErrIdempotencyKeyReused", err)
	}
}

func idemRollback(t *testing.T, h IdempotencyHarness) {
	k := idemKey("pedido-3", 1)
	boom := errors.New("write failed")
	err := h.Tx.WithinTx(inStore(h.Store), func(ctx context.Context) error {
		if err := h.Keys.Remember(ctx, k, h.Resource, time.Hour); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the write's error", err)
	}
	if _, ok, err := recall(t, h, h.Store, k); err != nil || ok {
		t.Errorf("after a rollback: ok %v err %v, want the key forgotten", ok, err)
	}
}

func idemPerStore(t *testing.T, h IdempotencyHarness) {
	k := idemKey("pedido-4", 1)
	remember(t, h, h.Store, k)
	if _, ok, err := recall(t, h, h.Other, idemKey("pedido-4", 2)); err != nil || ok {
		t.Errorf("another store's key: ok %v err %v, want a new key", ok, err)
	}
}

func idemInFlight(t *testing.T, h IdempotencyHarness) {
	k := idemKey("pedido-5", 1)
	remember(t, h, h.Store, k)
	err := h.Tx.WithinTx(inStore(h.Store), func(ctx context.Context) error {
		return h.Keys.Remember(ctx, k, h.Resource, time.Hour)
	})
	if !errors.Is(err, appshared.ErrIdempotencyKeyInFlight) {
		t.Errorf("second Remember of a stored key: err = %v, want ErrIdempotencyKeyInFlight", err)
	}
}

func idemNeedsTx(t *testing.T, h IdempotencyHarness) {
	ctx := inStore(h.Store)
	if _, _, err := h.Keys.Recall(ctx, idemKey("fora", 1)); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("Recall outside a transaction: err = %v, want ErrNoTransaction", err)
	}
	if err := h.Keys.Remember(ctx, idemKey("fora", 1), h.Resource, time.Hour); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("Remember outside a transaction: err = %v, want ErrNoTransaction", err)
	}
}
