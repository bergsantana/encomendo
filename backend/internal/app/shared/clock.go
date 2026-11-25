package shared

import "time"

// Clock tells the time. Use cases take time from it and pass it to the
// domain as an argument, so tests run on a fixed clock and the domain never
// calls time.Now (backend/CLAUDE.md, Layers).
type Clock interface {
	// Now returns the current wall-clock time in UTC. It may step back when
	// the system clock is corrected, so never use it to order events; ids
	// from IDGenerator keep their order.
	Now() time.Time
}
