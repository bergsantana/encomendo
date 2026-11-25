package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/sync/semaphore"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrUnreadableHash means a stored password hash is not an argon2id PHC
// string within the bounds below, or asks for more memory than the hasher
// may take. It matches no sentinel, so a login against one is a 500, never
// a wrong password. Its text never carries the hash.
var ErrUnreadableHash = errors.New("auth: unreadable password hash")

// ErrBusy means every hashing slot stayed taken for the hasher's longest
// wait. It matches shared.ErrUnavailable: the API answers 503 with
// Retry-After, and the request may be tried again (ADR 0019).
var ErrBusy = fmt.Errorf("auth: password hashing is busy: %w", shared.ErrUnavailable)

// Argon2Params are the argon2id costs (RFC 9106). Every hash records the
// costs it was made with, so raising them later keeps old hashes working.
type Argon2Params struct {
	// Memory is in KiB.
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
}

// DefaultArgon2Params returns the costs main uses: the second recommended
// option of RFC 9106 (section 4), 64 MiB, 3 passes, 4 lanes (ADR 0019).
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{Memory: 64 * 1024, Iterations: 3, Parallelism: 4}
}

// DefaultMaxConcurrentHashes is the memory budget main uses, in hashes at
// the default costs: 256 MiB, however many logins arrive (ADR 0019).
const DefaultMaxConcurrentHashes = 4

// DefaultMaxHashWait is how long a call waits for its memory in main before
// it answers ErrBusy (ADR 0019).
const DefaultMaxHashWait = 3 * time.Second

// The bounds on costs, for NewHasher and for every stored hash. A stored
// hash asking for more is refused, not run, so a corrupt row cannot cost
// more memory than the hasher's budget or more than 16 passes.
const (
	maxMemory      = 1 << 20 // KiB, 1 GiB
	maxIterations  = 16
	maxParallelism = 16
	saltLength     = 16
	keyLength      = 32
	minSaltLength  = 8
	minKeyLength   = 16
	maxPartLength  = 64
)

// check returns an error for costs out of bounds. argon2 itself needs at
// least 8 KiB of memory per lane.
func (p Argon2Params) check() error {
	if p.Parallelism < 1 || p.Parallelism > maxParallelism || p.Iterations < 1 || p.Iterations > maxIterations ||
		p.Memory < 8*uint32(p.Parallelism) || p.Memory > maxMemory {
		return fmt.Errorf("argon2id costs %s out of bounds", p)
	}
	return nil
}

// String writes the costs as the PHC string does.
func (p Argon2Params) String() string {
	return fmt.Sprintf("m=%d,t=%d,p=%d", p.Memory, p.Iterations, p.Parallelism)
}

// Hasher hashes and checks passwords with argon2id, written as PHC strings
// ("$argon2id$v=19$m=…,t=…,p=…$salt$key"). It implements
// appidentity.PasswordHasher and is safe for concurrent use.
type Hasher struct {
	params  Argon2Params
	budget  int64 // KiB that derivations may hold at once
	memory  *semaphore.Weighted
	maxWait time.Duration
	rand    io.Reader
	derive  func(password, salt []byte, p Argon2Params, keyLen uint32) []byte
}

// NewHasher returns a hasher that hashes with p. Derivations hold at most
// maxConcurrent times p.Memory at once, each taking its own memory; a call
// waits for its share at most maxWait, then returns ErrBusy. It returns an
// error for costs out of bounds, fewer than one slot or no wait.
func NewHasher(p Argon2Params, maxConcurrent int, maxWait time.Duration) (*Hasher, error) {
	if err := p.check(); err != nil {
		return nil, err
	}
	if maxConcurrent < 1 || maxWait <= 0 {
		return nil, errors.New("auth: a hasher needs at least one slot and a wait")
	}
	budget := int64(maxConcurrent) * int64(p.Memory)
	return &Hasher{
		params: p, budget: budget, memory: semaphore.NewWeighted(budget), maxWait: maxWait,
		rand: rand.Reader, derive: idKey,
	}, nil
}

func idKey(password, salt []byte, p Argon2Params, keyLen uint32) []byte {
	return argon2.IDKey(password, salt, p.Iterations, p.Memory, p.Parallelism, keyLen)
}

// Hash returns a hash of password with a fresh 16-byte salt and a 32-byte
// key. It fails when the salt cannot be drawn, with ErrBusy, or with ctx's
// error when ctx ends first; no error carries the password.
func (h *Hasher) Hash(ctx context.Context, password string) (identity.PasswordHash, error) {
	salt := make([]byte, saltLength)
	if _, err := io.ReadFull(h.rand, salt); err != nil {
		return "", fmt.Errorf("draw a password salt: %w", err)
	}
	key, err := h.run(ctx, password, salt, h.params, keyLength)
	if err != nil {
		return "", err
	}
	b64 := base64.RawStdEncoding
	return identity.PasswordHash(fmt.Sprintf("$argon2id$v=%d$%s$%s$%s",
		argon2.Version, h.params, b64.EncodeToString(salt), b64.EncodeToString(key))), nil
}

// Verify reports whether password matches hash, comparing in constant
// time, at the costs written in hash. For the empty hash it derives a key
// at the current costs and reports false, so an unknown email takes as
// long as a wrong password (ADR 0010). It returns ErrUnreadableHash for a
// hash it cannot read or that asks for more than the whole budget, ErrBusy,
// or ctx's error when ctx ends first.
func (h *Hasher) Verify(ctx context.Context, hash identity.PasswordHash, password string) (bool, error) {
	if hash == "" {
		_, err := h.run(ctx, password, make([]byte, saltLength), h.params, keyLength)
		return false, err
	}
	p, salt, want, err := decode(string(hash))
	if err != nil {
		return false, err
	}
	if int64(p.Memory) > h.budget {
		return false, ErrUnreadableHash
	}
	got, err := h.run(ctx, password, salt, p, uint32(len(want))) //nolint:gosec // decode bounds len(want) to 64
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// run derives a key once its memory is free. A context that has ended gets
// no work, one that ends while waiting gives up its place, and after
// maxWait the call gives up with ErrBusy rather than queue without end.
func (h *Hasher) run(ctx context.Context, password string, salt []byte, p Argon2Params, keyLen uint32) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("hash a password: %w", err)
	}
	wait, cancel := context.WithTimeout(ctx, h.maxWait)
	defer cancel()
	if err := h.memory.Acquire(wait, int64(p.Memory)); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("wait to hash a password: %w", ctx.Err())
		}
		return nil, ErrBusy
	}
	defer h.memory.Release(int64(p.Memory))
	return h.derive([]byte(password), salt, p, keyLen), nil
}

// decode reads a PHC string, refusing every spelling but the one Hash
// writes for the same costs, and costs, salts and keys out of bounds.
func decode(hash string) (Argon2Params, []byte, []byte, error) {
	f := strings.Split(hash, "$") // "", argon2id, v=19, costs, salt, key
	if len(f) != 6 || f[0] != "" || f[1] != "argon2id" || f[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return Argon2Params{}, nil, nil, ErrUnreadableHash
	}
	var p Argon2Params
	if _, err := fmt.Sscanf(f[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil ||
		p.String() != f[3] || p.check() != nil {
		return Argon2Params{}, nil, nil, ErrUnreadableHash
	}
	salt, ok := decodePart(f[4], minSaltLength)
	if !ok {
		return Argon2Params{}, nil, nil, ErrUnreadableHash
	}
	key, ok := decodePart(f[5], minKeyLength)
	if !ok {
		return Argon2Params{}, nil, nil, ErrUnreadableHash
	}
	return p, salt, key, nil
}

// decodePart reads a salt or key of min to maxPartLength bytes. It compares
// the text with the bytes written back, because the base64 decoder skips
// line breaks even in strict mode.
func decodePart(text string, minLength int) ([]byte, bool) {
	b64 := base64.RawStdEncoding.Strict()
	b, err := b64.DecodeString(text)
	if err != nil || len(b) < minLength || len(b) > maxPartLength || b64.EncodeToString(b) != text {
		return nil, false
	}
	return b, true
}
