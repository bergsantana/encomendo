package order

import (
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Errors of the order use cases.
var (
	// ErrNotFound means the store has no order with that id. It matches
	// shared.ErrNotFound.
	ErrNotFound = fmt.Errorf("order not found: %w", shared.ErrNotFound)
	// ErrIdempotencyKeyRequired means a creating call came without an
	// Idempotency-Key. It matches shared.ErrInvalid; the API answers
	// idempotency_key_required.
	ErrIdempotencyKeyRequired = fmt.Errorf("idempotency key required: %w", shared.ErrInvalid)
)
