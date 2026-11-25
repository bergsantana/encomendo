package shared

import (
	"context"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// SecretGenerator draws the random part of opaque tokens: refresh tokens,
// invitation links and password reset codes (and order tracking tokens in
// milestone 4). The domain decides how a secret is encoded and hashed
// (shared.Secret); only drawing it is a port, so tests can fix it.
type SecretGenerator interface {
	// NewSecret returns 32 bytes from a cryptographically secure source.
	// It returns an error only when that source fails, and the caller then
	// fails the request; there is no weaker fallback.
	NewSecret(ctx context.Context) (shared.Secret, error)
}
