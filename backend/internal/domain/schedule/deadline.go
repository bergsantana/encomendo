package schedule

import "github.com/bergsantana/encomendo/backend/internal/domain/shared"

// Delivery returns the promise date of an order: the latest of the items'
// expected dates and the release day, plus the assembly days, plus the safety
// margin ("Margem de segurança"). An order with nothing to wait for is ready
// assembly + margin days after release, and the result is never before
// release. Negative assembly or margin count as 0; callers validate them
// first, as for Days.
//
// A workflow without an assembling phase has no assembly days: the caller
// passes 0 (docs/architecture.md, section 4.2).
//
// Product rule: delivery = latest item ready date + assembly days + safety
// margin (docs/product/design-document.md, Deadline formula). The Kit
// Scrambler CB 500X, released on 2026-12-01 with a 15 day seat, 3 assembly
// days and a margin of 2, is promised for 2026-12-21.
func Delivery(expected []shared.CivilDate, release shared.CivilDate, assembly, margin int) shared.CivilDate {
	latest := shared.Latest(shared.Latest(expected...), release)
	return latest.AddDays(nonNegative(assembly) + nonNegative(margin))
}

// ArrivalLimit returns "Precisa chegar até": the promise minus the assembly
// days, the last day an item can arrive and still be assembled in time.
//
// Product rule: arrival limit = delivery date - assembly days
// (docs/architecture.md, section 4.2, Deadline).
func ArrivalLimit(promise shared.CivilDate, assembly int) shared.CivilDate {
	return promise.AddDays(-nonNegative(assembly))
}
