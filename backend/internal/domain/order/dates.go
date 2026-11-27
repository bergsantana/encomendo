package order

import (
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// assemblyForDeadline returns the assembly days the deadline counts: the
// order's own, or 0 when its workflow has no assembling phase
// (docs/architecture.md section 4.2).
func (o *Order) assemblyForDeadline() int {
	if !o.workflow.HasPhase(workflow.PhaseAssembling) {
		return 0
	}
	return o.terms.AssemblyDays
}

// AssemblyDaysCounted returns the assembly days the order's deadline
// counts: 0 when its workflow has no assembling phase.
func (o *Order) AssemblyDaysCounted() int { return o.assemblyForDeadline() }

// base returns the day dates count from: the release date, or today before
// the order is released (ADR 0030).
func (o *Order) base(today shared.CivilDate) shared.CivilDate {
	if !o.releaseDate.IsZero() {
		return o.releaseDate
	}
	return today
}

// timing returns what an item's readiness depends on. Until milestone 5
// records supplier promised dates, Promised stays unset; until milestone 6,
// stock items count their supplier's lead time (ADR 0030).
func timing(i Item) schedule.Timing {
	return schedule.Timing{LeadDays: i.snapshot.LeadDays, ExtraDays: i.snapshot.extraDays()}
}

// redate recomputes every item's expected date from the base day. Items
// that already arrived keep theirs, since their date is history.
func (o *Order) redate(today shared.CivilDate) {
	b := o.base(today)
	for li := range o.lines {
		for ii := range o.lines[li].items {
			it := &o.lines[li].items[ii]
			if it.Arrived() && !it.expected.IsZero() {
				continue
			}
			it.expected = schedule.ExpectedDate(timing(*it), b)
		}
	}
}

// forecast returns the delivery date the items' expected dates give:
// the latest of them (never before the base day) + assembly + margin.
func (o *Order) forecast(today shared.CivilDate) shared.CivilDate {
	items := o.Items()
	expected := make([]shared.CivilDate, 0, len(items))
	for _, i := range items {
		expected = append(expected, i.expected)
	}
	return schedule.Delivery(expected, o.base(today), o.assemblyForDeadline(), o.terms.SafetyMarginDays)
}

// Forecast returns the delivery date the items' current expected dates
// give, for today. It differs from Promise when an item is late or the
// promise was changed by hand.
func (o *Order) Forecast(today shared.CivilDate) shared.CivilDate { return o.forecast(today) }

// release fixes the release date and counts every date from it. The
// promise follows unless someone changed it by hand (ADR 0030).
func (o *Order) release(today shared.CivilDate) {
	o.releaseDate = today
	o.redate(today)
	if !o.promiseByHand {
		o.promise = o.forecast(today)
	}
}

// ArrivalLimit returns "Precisa chegar até": the promise minus the assembly
// days the deadline counts.
func (o *Order) ArrivalLimit() shared.CivilDate {
	return schedule.ArrivalLimit(o.promise, o.assemblyForDeadline())
}

// Alerts returns each item's alerts for today: at_risk ("Pode atrasar")
// when it is expected after the arrival limit, late ("Atrasada") when its
// expected date passed and it has not arrived. Items with none are left
// out, and a closed order has none. Alerts are staff data: no client view
// carries them (docs/product/design-document.md, System rules).
func (o *Order) Alerts(today shared.CivilDate) map[shared.OrderItemID][]schedule.Alert {
	out := map[shared.OrderItemID][]schedule.Alert{}
	if o.Closed() {
		return out
	}
	limit := o.ArrivalLimit()
	for _, i := range o.Items() {
		if a := schedule.Alerts(schedule.ItemState{Expected: i.expected, Arrived: i.Arrived()}, limit, today); len(a) > 0 {
			out[i.id] = a
		}
	}
	return out
}

// ChangeDeliveryDate moves the promise to date, on behalf of by, and
// returns the old promise. It records the change; the app layer returns a
// wa.me link to warn the client, and nothing is sent automatically
// (docs/architecture.md section 4.2). A deposit or release no longer resets
// the promise afterwards. It returns ErrOrderClosed after handover or
// cancel, ErrDateInPast for a date before today, and a ValidationError for
// a reason over 500 characters.
func (o *Order) ChangeDeliveryDate(date shared.CivilDate, reason string, by shared.Principal, today shared.CivilDate, now time.Time) (shared.CivilDate, error) {
	if o.Closed() {
		return shared.CivilDate{}, ErrOrderClosed
	}
	if date.IsZero() || date.Before(today) {
		return shared.CivilDate{}, refuse(ErrDateInPast, "date", msgDateFromToday)
	}
	reason, err := optionalReason(reason)
	if err != nil {
		return shared.CivilDate{}, err
	}
	old := o.promise
	o.promise, o.promiseByHand = date, true
	o.addHistory(HistoryEntry{Kind: HistoryDate, By: by.UserID(), At: now, OldDate: old, NewDate: date, Reason: reason})
	return old, nil
}
