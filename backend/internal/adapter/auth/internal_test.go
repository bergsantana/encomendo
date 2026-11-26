package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// errSource is a random source that has failed.
var errSource = errors.New("entropy source failed")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errSource }

// derivation is one call to the key derivation, as a test sees it.
type derivation struct {
	params Argon2Params
	salt   int
	key    uint32
}

// counting replaces h's derivation with one that records its calls and
// still computes the real key, so Verify keeps working.
func counting(h *Hasher) func() []derivation {
	var mu sync.Mutex
	var calls []derivation
	compute := h.derive
	h.derive = func(password, salt []byte, p Argon2Params, n uint32) []byte {
		mu.Lock()
		calls = append(calls, derivation{p, len(salt), n})
		mu.Unlock()
		return compute(password, salt, p, n)
	}
	return func() []derivation {
		mu.Lock()
		defer mu.Unlock()
		return append([]derivation(nil), calls...)
	}
}

func newTestHasher(t *testing.T, p Argon2Params, slots int) *Hasher {
	t.Helper()
	h, err := NewHasher(p, slots, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestHasher_VerifyOfTheEmptyHashDoesTheWorkOfARealOne: an unknown email
// verifies against the empty hash, and must cost one derivation at the
// current costs, as a wrong password does (ADR 0010).
func TestHasher_VerifyOfTheEmptyHashDoesTheWorkOfARealOne(t *testing.T) {
	t.Parallel()
	current := Argon2Params{Memory: 128, Iterations: 2, Parallelism: 2}
	older := Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1}
	hash, err := newTestHasher(t, older, 1).Hash(context.Background(), "segredo-forte")
	if err != nil {
		t.Fatal(err)
	}
	h := newTestHasher(t, current, 1)
	calls := counting(h)

	if ok, err := h.Verify(context.Background(), "", "segredo-forte"); ok || err != nil {
		t.Fatalf("Verify of the empty hash = %v, %v; want false", ok, err)
	}
	if ok, err := h.Verify(context.Background(), hash, "segredo-fraco"); ok || err != nil {
		t.Fatalf("Verify of a wrong password = %v, %v; want false", ok, err)
	}
	want := []derivation{{current, saltLength, keyLength}, {older, saltLength, keyLength}}
	if got := calls(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("derivations %+v; want %+v", got, want)
	}
}

// holding replaces h's derivation with one whose first call holds its
// memory until release is closed. It returns once that call holds it.
func holding(t *testing.T, h *Hasher, run func() error) (release func(), done <-chan error) {
	t.Helper()
	held, freed := make(chan struct{}), make(chan struct{})
	var once sync.Once
	compute := h.derive
	h.derive = func(password, salt []byte, p Argon2Params, n uint32) []byte {
		first := false
		once.Do(func() { first = true })
		if first {
			close(held)
			<-freed
		}
		return compute(password, salt, p, n)
	}
	out := make(chan error, 1)
	go func() { out <- run() }()
	<-held
	return func() { close(freed) }, out
}

// TestHasher_WaitsForAFreeSlotOnlyUntilTheContextEnds bounds the memory
// that hashing takes: with every slot busy, a call waits, and gives up when
// its request does instead of piling up 64 MiB allocations. Once the slot
// frees, the next call gets it.
func TestHasher_WaitsForAFreeSlotOnlyUntilTheContextEnds(t *testing.T) {
	t.Parallel()
	h := newTestHasher(t, Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1}, 1)
	release, done := holding(t, h, func() error {
		_, err := h.Hash(context.Background(), "segredo-forte")
		return err
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := h.Hash(ctx, "segredo-forte"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Hash with every slot busy: %v; want the deadline", err)
	}
	if ok, err := h.Verify(ctx, "", "segredo-forte"); ok || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Verify with every slot busy: %v, %v; want the deadline", ok, err)
	}
	release()
	if err := <-done; err != nil {
		t.Errorf("the call holding the slot: %v", err)
	}
	after, cancelAfter := context.WithTimeout(context.Background(), time.Minute)
	defer cancelAfter()
	if _, err := h.Hash(after, "segredo-forte"); err != nil {
		t.Errorf("Hash once the slot is free: %v; want the slot released", err)
	}
}

// TestHasher_GivesUpAfterItsLongestWait: with every slot busy for longer
// than maxWait, a call answers ErrBusy, a 503 with Retry-After, rather than
// queue for as long as its client waits (ADR 0019).
func TestHasher_GivesUpAfterItsLongestWait(t *testing.T) {
	t.Parallel()
	h, err := NewHasher(Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1}, 1, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	release, done := holding(t, h, func() error {
		_, err := h.Hash(context.Background(), "segredo-forte")
		return err
	})
	defer func() {
		release()
		if err := <-done; err != nil {
			t.Errorf("the call holding the memory: %v", err)
		}
	}()

	// The client waits far longer than maxWait; a hasher without the limit
	// would answer this deadline instead of ErrBusy.
	client, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := h.Hash(client, "segredo-forte"); !errors.Is(err, ErrBusy) || !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("Hash with every slot busy: %v; want ErrBusy, matching shared.ErrUnavailable", err)
	}
	if ok, err := h.Verify(client, "", "segredo-forte"); ok || !errors.Is(err, ErrBusy) {
		t.Errorf("Verify with every slot busy = %v, %v; want ErrBusy", ok, err)
	}
}

// TestHasher_WeighsEachHashByItsMemory: the slots are memory, so two hashes
// at half the current costs run side by side, and a hash at the current
// costs waits for both.
func TestHasher_WeighsEachHashByItsMemory(t *testing.T) {
	t.Parallel()
	half, err := newTestHasher(t, Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1}, 1).Hash(context.Background(), "segredo-forte")
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewHasher(Argon2Params{Memory: 128, Iterations: 1, Parallelism: 1}, 1, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	release, done := holding(t, h, func() error {
		_, err := h.Verify(context.Background(), half, "segredo-forte")
		return err
	})
	defer func() {
		release()
		if err := <-done; err != nil {
			t.Errorf("the call holding the memory: %v", err)
		}
	}()

	if ok, err := h.Verify(context.Background(), half, "segredo-forte"); !ok || err != nil {
		t.Errorf("a second half-cost hash = %v, %v; want it run beside the first", ok, err)
	}
	if _, err := h.Hash(context.Background(), "segredo-forte"); !errors.Is(err, ErrBusy) {
		t.Errorf("a full-cost hash beside a half-cost one: %v; want ErrBusy", err)
	}
}

func TestHasher_DoesNoWorkForAnEndedRequest(t *testing.T) {
	t.Parallel()
	h := newTestHasher(t, Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1}, 1)
	calls := counting(h)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// With a slot free, a select alone would pick the slot half the time;
	// twenty tries leave a missing check one chance in a million.
	for range 20 {
		if _, err := h.Hash(ctx, "segredo-forte"); !errors.Is(err, context.Canceled) {
			t.Fatalf("Hash: %v; want context.Canceled", err)
		}
		if ok, err := h.Verify(ctx, "", "segredo-forte"); ok || !errors.Is(err, context.Canceled) {
			t.Fatalf("Verify = %v, %v; want context.Canceled", ok, err)
		}
	}
	if n := len(calls()); n != 0 {
		t.Errorf("%d derivations for an ended request; want none", n)
	}
}

func TestHasher_FailsWithoutASalt(t *testing.T) {
	t.Parallel()
	h := newTestHasher(t, Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1}, 1)
	h.rand = failingReader{}

	hash, err := h.Hash(context.Background(), "segredo-forte")
	if !errors.Is(err, errSource) || hash != identity.PasswordHash("") {
		t.Fatalf("Hash = %q, %v; want the source's error", hash, err)
	}
	if strings.Contains(err.Error(), "segredo-forte") {
		t.Errorf("the error %q carries the password", err)
	}
}

func TestSecrets_FailWhenTheSourceFails(t *testing.T) {
	t.Parallel()
	s := &Secrets{rand: failingReader{}}

	if got, err := s.NewSecret(context.Background()); !errors.Is(err, errSource) || got != [32]byte{} {
		t.Errorf("NewSecret = %v, %v; want the source's error and no secret", got, err)
	}
}

// FuzzDecode: a stored hash never panics the reader, and the reader
// accepts only the one spelling Hash writes for what it read.
func FuzzDecode(f *testing.F) {
	h, err := NewHasher(Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1}, 1, time.Minute)
	if err != nil {
		f.Fatal(err)
	}
	valid, err := h.Hash(context.Background(), "segredo-forte")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(string(valid))
	f.Add("$argon2id$v=19$m=64,t=1,p=1$$")
	f.Add("$argon2id$v=19$m=4294967295,t=1,p=1$c2FsdHNhbHQ$a2V5a2V5a2V5a2V5a2V5a2V5")

	f.Fuzz(func(t *testing.T, hash string) {
		p, salt, key, err := decode(hash)
		if err != nil {
			return
		}
		b64 := base64.RawStdEncoding
		canonical := fmt.Sprintf("$argon2id$v=19$%s$%s$%s", p, b64.EncodeToString(salt), b64.EncodeToString(key))
		if canonical != hash {
			t.Fatalf("decode accepted %q, which Hash would write as %q", hash, canonical)
		}
	})
}
