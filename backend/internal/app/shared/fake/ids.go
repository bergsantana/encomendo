package fake

import (
	"encoding/binary"
	"sync"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// IDs is an IDGenerator whose ids are predictable: UUIDv7s stamped with the
// clock's time and a counter in place of the random bits. Two generators
// with the same clock produce the same sequence.
type IDs struct {
	mu     sync.Mutex
	clock  appshared.Clock
	n      uint64
	lastMS int64
}

// NewIDs returns a generator that reads the time from clock.
func NewIDs(clock appshared.Clock) *IDs {
	return &IDs{clock: clock}
}

// NewID returns the next id. Ids keep their order even when the test moves
// the clock backwards: the time part never goes below the last one issued,
// as the IDGenerator contract requires of the real generator too.
func (g *IDs) NewID() shared.ID {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	g.lastMS = max(g.lastMS, g.clock.Now().UnixMilli())
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], g.n)
	// The counter fills the last seven random bytes, which NewV7 copies
	// unchanged, so a later id always sorts after an earlier one within a
	// millisecond.
	var random [10]byte
	copy(random[3:], counter[1:])
	return shared.NewV7(g.lastMS, random)
}
