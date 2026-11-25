package fake

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sync"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Secrets is a SecretGenerator whose secrets are predictable: the SHA-256 of
// a counter. Two generators give the same sequence, so a test can know a
// token before the use case draws it.
type Secrets struct {
	mu   sync.Mutex
	n    uint64
	fail error
}

// NewSecrets returns a generator at the start of its sequence.
func NewSecrets() *Secrets { return &Secrets{} }

// NewSecret returns the next secret, or the error set by Fail.
func (s *Secrets) NewSecret(context.Context) (shared.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return shared.Secret{}, s.fail
	}
	s.n++
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], s.n)
	return shared.Secret(sha256.Sum256(counter[:])), nil
}

// Fail makes every later NewSecret return err; nil makes it work again.
func (s *Secrets) Fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = err
}
