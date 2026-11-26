package system

import (
	"sync"
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/contract/sharedports"
)

func TestClock_PassesTheClockContract(t *testing.T) {
	t.Parallel()
	sharedports.RunClockContract(t, func(*testing.T) appshared.Clock { return NewClock() })
}

func TestIDs_PassTheIDGeneratorContract(t *testing.T) {
	t.Parallel()
	sharedports.RunIDGeneratorContract(t, func(*testing.T) appshared.IDGenerator { return NewIDs() })
}

// TestIDs_KeepTheirOrderWhenTheClockStandsStillOrStepsBack: the random part
// of a UUIDv7 alone would not order two ids of one millisecond.
func TestIDs_KeepTheirOrderWhenTheClockStandsStillOrStepsBack(t *testing.T) {
	t.Parallel()
	g := NewIDs()
	at := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return at }
	g.rand = func(b []byte) { // descending randomness: the worst case for ordering
		for i := range b {
			b[i] = 0xff - byte(i)
		}
	}
	prev := g.NewID()
	for i, step := range []time.Duration{0, 0, -time.Hour, time.Millisecond, 0} {
		at = at.Add(step)
		id := g.NewID()
		if id.String() <= prev.String() {
			t.Fatalf("id %d (%v) does not sort after %v", i, id, prev)
		}
		if id.Version() != 7 {
			t.Errorf("id %v has version %d, want 7", id, id.Version())
		}
		prev = id
	}
}

func TestSuccessor_RollsToTheNextMillisecondOnOverflow(t *testing.T) {
	t.Parallel()
	prev := shared.NewV7(1_000, [10]byte{0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	next := successor(prev, 1)
	if next.String() <= prev.String() || next.Version() != 7 {
		t.Errorf("successor of the last id of a millisecond = %v, want a later UUIDv7 than %v", next, prev)
	}
}

func TestIDs_AreUniqueAcrossGoroutines(t *testing.T) {
	t.Parallel()
	g := NewIDs()
	const workers, each = 8, 500
	var mu sync.Mutex
	seen := map[shared.ID]bool{}
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				id := g.NewID()
				mu.Lock()
				if seen[id] {
					t.Errorf("id %v issued twice", id)
				}
				seen[id] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != workers*each {
		t.Errorf("%d distinct ids, want %d", len(seen), workers*each)
	}
}
