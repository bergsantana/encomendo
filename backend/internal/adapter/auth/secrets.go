package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Secrets draws the 32 random bytes of refresh tokens, invitation links and
// reset codes from crypto/rand (docs/architecture.md 5.1). It implements
// appshared.SecretGenerator.
type Secrets struct {
	rand io.Reader
}

// NewSecrets returns a generator over crypto/rand.
func NewSecrets() *Secrets { return &Secrets{rand: rand.Reader} }

// NewSecret returns a fresh secret. It fails only when the source does,
// with no weaker fallback.
func (s *Secrets) NewSecret(context.Context) (shared.Secret, error) {
	var out shared.Secret
	if _, err := io.ReadFull(s.rand, out[:]); err != nil {
		return shared.Secret{}, fmt.Errorf("draw a secret: %w", err)
	}
	return out, nil
}
