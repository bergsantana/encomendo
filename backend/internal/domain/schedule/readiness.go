package schedule

import "github.com/bergsantana/encomendo/backend/internal/domain/shared"

// Timing is what an order item's readiness depends on.
type Timing struct {
	// LeadDays is the supplier's lead time for a bought item (to_order, and
	// stock until milestone 6), or the production days of an in-house item.
	LeadDays int
	// ExtraDays is the sum of the extra days of the options the client chose.
	ExtraDays int
	// Promised is the date the supplier promised, zero until milestone 5
	// records one.
	Promised shared.CivilDate
}

// ExpectedDate returns the day an item should be ready: the supplier's
// promised date when one is set, else release plus LeadDays, and then
// ExtraDays more. Negative day counts are treated as 0, because the catalog
// validates them to 0..365 and a negative value is a caller bug that must not
// move a date earlier than release. An unset release with no promised date
// gives an unset result: an order not yet released has no forecast counted
// from a release day.
//
// Product rule: a supplier's promised date replaces the lead time once it is
// known (docs/architecture.md, section 4.2, Deadline).
func ExpectedDate(t Timing, release shared.CivilDate) shared.CivilDate {
	ready := t.Promised
	if ready.IsZero() {
		ready = release.AddDays(nonNegative(t.LeadDays))
	}
	return ready.AddDays(nonNegative(t.ExtraDays))
}

func nonNegative(n int) int { return max(n, 0) }
