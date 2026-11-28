// Package schedule holds the store's calendar and the recalculation of an
// order's dates: which day it is in the store's time zone, and the
// per-order re-derivation, which counts a supplier's promised date where
// purchasing recorded one (ADR 0031). Nothing calls it when the store's time
// zone changes: dates already computed stay as they were
// (docs/plans/m4-code-design.md section 8).
//
// Rules it enforces:
//   - Dates count in the store's time zone, never the server's or UTC
//     (ADR 0030): Today is the store's day.
//   - Recalculate needs order:write and runs in one transaction scoped to
//     the caller's store, so RLS is the third line of defense. It saves the
//     order only when an expected date moved.
//   - It never reads the system clock; the clock is a port.
//
// Not here yet: the worker's CheckAll, which checks every open order each
// day, lands in milestone 6 with the worker and is not stubbed (docs/plans/
// m4-code-design.md section 3.7).
package schedule
