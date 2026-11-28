// Package schedule holds the store's calendar and the recalculation of an
// order's dates: which day it is in the store's time zone, and the
// per-order re-derivation that milestone 5 (promised dates) and a time zone
// change call.
//
// Rules it enforces:
//   - Dates count in the store's time zone, never the server's or UTC
//     (ADR 0030): Today is the store's day.
//   - Recalculate needs order:write and runs in one transaction scoped to
//     the caller's store, so RLS is the third line of defense.
//   - It never reads the system clock; the clock is a port.
//
// Not here yet: the worker's CheckAll, which checks every open order each
// day, lands in milestone 6 with the worker and is not stubbed (docs/plans/
// m4-code-design.md section 3.7).
package schedule
