package sharedports

import (
	"context"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// RunSecretGeneratorContract checks that a SecretGenerator never repeats a
// secret and never returns the zero one. Randomness itself is the real
// adapter's own test (crypto/rand).
func RunSecretGeneratorContract(t *testing.T, newGenerator func(t *testing.T) appshared.SecretGenerator) {
	g := newGenerator(t)
	const n = 1000
	seen := make(map[shared.Secret]bool, n)
	for range n {
		s, err := g.NewSecret(context.Background())
		if err != nil {
			t.Fatalf("NewSecret: %v", err)
		}
		if s == (shared.Secret{}) {
			t.Fatal("NewSecret returned the zero secret")
		}
		if seen[s] {
			t.Fatalf("NewSecret repeated %s", s)
		}
		seen[s] = true
	}
}
