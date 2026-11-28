package fake

import (
	"context"
	"sync"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Idempotency is an Idempotency in memory, per store as the scope of the
// fake TxManager's transaction says. A key remembered in a transaction that
// fails is forgotten, as in Postgres.
type Idempotency struct {
	mu    sync.Mutex
	clock appshared.Clock
	keys  map[idemKey]idemEntry
}

type idemKey struct {
	tenant shared.TenantID
	key    string
}

type idemEntry struct {
	hash     [32]byte
	resource string
	expires  time.Time
}

// NewIdempotency returns a store with no keys; clock decides expiry.
func NewIdempotency(clock appshared.Clock) *Idempotency {
	return &Idempotency{clock: clock, keys: map[idemKey]idemEntry{}}
}

// Recall follows the Idempotency contract.
func (f *Idempotency) Recall(ctx context.Context, k appshared.IdempotencyKey) (string, bool, error) {
	tx := txFrom(ctx)
	if tx == nil {
		return "", false, appshared.ErrNoTransaction
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.keys[idemKey{tx.scope.TenantID, k.Key}]
	if !ok || !f.clock.Now().Before(e.expires) {
		return "", false, nil
	}
	if e.hash != k.RequestHash {
		return "", false, appshared.ErrIdempotencyKeyReused
	}
	return e.resource, true, nil
}

// Remember follows the Idempotency contract.
func (f *Idempotency) Remember(ctx context.Context, k appshared.IdempotencyKey, resource string, ttl time.Duration) error {
	tx := txFrom(ctx)
	if tx == nil {
		return appshared.ErrNoTransaction
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	id := idemKey{tx.scope.TenantID, k.Key}
	if e, ok := f.keys[id]; ok && f.clock.Now().Before(e.expires) {
		return appshared.ErrIdempotencyKeyInFlight
	}
	KeepOnRollback(ctx, &f.mu, f.keys, id)
	f.keys[id] = idemEntry{hash: k.RequestHash, resource: resource, expires: f.clock.Now().Add(ttl)}
	return nil
}
