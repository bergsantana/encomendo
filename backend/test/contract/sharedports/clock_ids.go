package sharedports

import (
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

// RunClockContract checks that a Clock reports a real time in UTC. It does
// not check order: the wall clock may step back (see Clock.Now).
func RunClockContract(t *testing.T, newClock func(t *testing.T) appshared.Clock) {
	now := newClock(t).Now()
	if now.IsZero() {
		t.Error("Now() is the zero time")
	}
	if now.Location() != time.UTC {
		t.Errorf("Now() location = %v, want UTC", now.Location())
	}
}

// RunIDGeneratorContract checks that an IDGenerator issues unique UUIDv7s
// in non-decreasing order.
func RunIDGeneratorContract(t *testing.T, newGenerator func(t *testing.T) appshared.IDGenerator) {
	g := newGenerator(t)
	const n = 1000
	seen := make(map[string]bool, n)
	prev := ""
	for range n {
		id := g.NewID()
		s := id.String()
		if id.IsZero() || id.Version() != 7 {
			t.Fatalf("NewID() = %s (version %d), want a non-nil UUIDv7", s, id.Version())
		}
		if seen[s] {
			t.Fatalf("NewID() repeated %s", s)
		}
		if s < prev {
			t.Fatalf("NewID() went backwards: %s after %s", s, prev)
		}
		seen[s], prev = true, s
	}
}
