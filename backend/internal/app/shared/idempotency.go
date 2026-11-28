package shared

import (
	"context"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// IdempotencyKey is a client's Idempotency-Key header with the request it
// came with (ADR 0030).
type IdempotencyKey struct {
	// Key is the header: 1 to 128 printable ASCII characters.
	Key string
	// RequestHash is the SHA-256 of the method, path and body, from the
	// HTTP adapter: the same key with another request is refused.
	RequestHash [32]byte
}

// IsZero reports whether no key was sent.
func (k IdempotencyKey) IsZero() bool { return k.Key == "" }

// ErrIdempotencyKeyReused means the key was already used, in this store,
// with another request. It matches shared.ErrConflict; the API answers
// 422 idempotency_key_reused.
var ErrIdempotencyKeyReused = fmt.Errorf("idempotency key reused: %w", shared.ErrConflict)

// ErrIdempotencyKeyInFlight means another call with the same key stored it
// first, while this one was running: the caller's transaction must roll
// back, and a retry then replays the first call's resource. It matches
// shared.ErrConflict; the API answers 409 conflict ("try again").
var ErrIdempotencyKeyInFlight = fmt.Errorf("idempotency key in flight: %w", shared.ErrConflict)

// Idempotency remembers which resource a key created, inside the write's
// own transaction, so a retry with the same key and request is answered
// with that resource as it is now, exactly once (ADR 0030). Keys are per
// store: the transaction's tenant.
type Idempotency interface {
	// Recall returns the id of the resource k created, with ok false when
	// the key is new or expired. It returns ErrIdempotencyKeyReused when the
	// key was used with another request hash, and ErrNoTransaction outside
	// TxManager.WithinTx.
	Recall(ctx context.Context, k IdempotencyKey) (resource string, ok bool, err error)
	// Remember stores that k created resource, for ttl. It must run in the
	// same WithinTx as the write, so a rolled back write forgets the key. It
	// returns ErrIdempotencyKeyInFlight when the key is already stored and
	// not expired (a concurrent first call won), and ErrNoTransaction
	// outside a transaction.
	Remember(ctx context.Context, k IdempotencyKey, resource string, ttl time.Duration) error
}
