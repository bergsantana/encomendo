//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/test/contract/sharedports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

func TestIdempotency_Contract(t *testing.T) {
	db := openApp(t)
	seeder := env.Seeder(t)
	sharedports.RunIdempotencyContract(t, func(t *testing.T) sharedports.IdempotencyHarness {
		return sharedports.IdempotencyHarness{
			Tx: postgres.NewTxManager(db), Keys: postgres.NewIdempotency(),
			Store: seeder.Tenant(seeder.User()), Other: seeder.Tenant(seeder.User()),
			Resource: pgtest.NewID().String(),
		}
	})
}

// Two first calls with one key race: one stores the key, the other waits on
// the primary key and gets ErrIdempotencyKeyInFlight, so one resource wins.
func TestIdempotency_ConcurrentFirstCallsLeaveOneWinner(t *testing.T) {
	db := openApp(t)
	seeder := env.Seeder(t)
	tx := postgres.NewTxManager(db)
	store := seeder.Tenant(seeder.User())
	ctx := appshared.WithScope(context.Background(), appshared.Scope{TenantID: store})
	keys := postgres.NewIdempotency()
	k := appshared.IdempotencyKey{Key: "corrida", RequestHash: [32]byte{7}}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = tx.WithinTx(ctx, func(ctx context.Context) error {
				return keys.Remember(ctx, k, pgtest.NewID().String(), time.Hour)
			})
		}()
	}
	wg.Wait()
	won, lost := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case errors.Is(err, appshared.ErrIdempotencyKeyInFlight):
			lost++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if won != 1 || lost != 1 {
		t.Errorf("won %d lost %d, want one of each", won, lost)
	}
}

// An expired key is forgotten by Recall and replaced by Remember.
func TestIdempotency_ExpiredKeysAreReplaced(t *testing.T) {
	db := openApp(t)
	seeder := env.Seeder(t)
	tx := postgres.NewTxManager(db)
	store := seeder.Tenant(seeder.User())
	ctx := appshared.WithScope(context.Background(), appshared.Scope{TenantID: store})
	keys := postgres.NewIdempotency()
	k := appshared.IdempotencyKey{Key: "velha", RequestHash: [32]byte{1}}
	if err := tx.WithinTx(ctx, func(ctx context.Context) error { return keys.Remember(ctx, k, pgtest.NewID().String(), time.Hour) }); err != nil {
		t.Fatal(err)
	}
	if _, err := seeder.DB().ExecContext(t.Context(), `UPDATE idempotency_keys SET created_at = now() - interval '2 days',
		expires_at = now() - interval '1 day' WHERE tenant_id = $1 AND key = 'velha'`, store.String()); err != nil {
		t.Fatal(err)
	}
	other := appshared.IdempotencyKey{Key: "velha", RequestHash: [32]byte{2}}
	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, ok, err := keys.Recall(ctx, other); err != nil || ok {
			t.Errorf("recall of an expired key: ok %v err %v, want a new key", ok, err)
		}
		return keys.Remember(ctx, other, pgtest.NewID().String(), time.Hour)
	})
	if err != nil {
		t.Errorf("replacing an expired key: %v", err)
	}
}
