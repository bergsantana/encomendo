// Package system is the adapter for the machine the process runs on: the
// wall clock and the random source behind primary keys. Everything else
// takes time and ids through the appshared ports, so tests fix both.
package system

import (
	"bytes"
	"crypto/rand"
	"sync"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Clock implements appshared.Clock over the system clock.
type Clock struct{}

// NewClock returns the system clock.
func NewClock() Clock { return Clock{} }

// Now returns the current time in UTC.
func (Clock) Now() time.Time { return time.Now().UTC() }

// IDs implements appshared.IDGenerator: UUIDv7 over crypto/rand. It is safe
// for concurrent use.
type IDs struct {
	mu   sync.Mutex
	last shared.ID
	now  func() time.Time
	rand func([]byte)
}

// NewIDs returns a generator on the system clock.
func NewIDs() *IDs {
	return &IDs{now: time.Now, rand: func(b []byte) { _, _ = rand.Read(b) }} // crypto/rand.Read never fails (Go 1.24+)
}

// NewID returns a UUIDv7 that sorts after every id this generator issued
// before, even within one millisecond and when the clock steps back: the
// ids of one process keep their order, which the keyset pages rely on.
// Ids are not secrets: after a step back they advance by small random
// amounts, so a neighbour of one id is easy to guess. Nothing may use an id
// as a capability; secrets come from appshared.SecretGenerator.
func (g *IDs) NewID() shared.ID {
	g.mu.Lock()
	defer g.mu.Unlock()
	millis := g.now().UnixMilli()
	var random [10]byte
	g.rand(random[:])
	id := shared.NewV7(millis, random)
	if bytes.Compare(id[:], g.last[:]) <= 0 {
		var step [3]byte
		g.rand(step[:])
		id = successor(g.last, 1+uint32(step[0])<<16|uint32(step[1])<<8|uint32(step[2]))
	}
	g.last = id
	return id
}

// successor is an id after prev with the same version and variant: prev plus
// step (at least 1) in its low 56 bits. When those overflow, which 2^56 ids
// in one millisecond would take, the next millisecond starts over.
func successor(prev shared.ID, step uint32) shared.ID {
	next := prev
	carry := uint64(step)
	for i := len(next) - 1; i >= 9 && carry > 0; i-- {
		sum := uint64(next[i]) + carry
		next[i] = byte(sum & 0xff)
		carry = sum >> 8
	}
	if carry == 0 {
		return next
	}
	var zero [10]byte
	ms := int64(prev[0])<<40 | int64(prev[1])<<32 | int64(prev[2])<<24 | int64(prev[3])<<16 | int64(prev[4])<<8 | int64(prev[5])
	return shared.NewV7(ms+1, zero)
}
