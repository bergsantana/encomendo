package order

import (
	"slices"
	"strings"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ChangeSpec is a change to an order's lines and terms. Items that survive
// keep their ids, which the app layer reuses.
type ChangeSpec struct {
	Lines            []LineSpec
	Terms            Terms
	CustomerApproved bool // the seller confirms the customer agreed; needed once paid
	By               shared.Principal
	Now              time.Time
	Today            shared.CivilDate
}

// ChangeResult is what a change did, for the summary the customer gets.
type ChangeResult struct {
	Paid                   bool // the change needed the customer's approval
	OldTotal, NewTotal     shared.Money
	OldPromise, NewPromise shared.CivilDate
	RefundDue              shared.Money
	Added, Removed         []shared.OrderItemID
}

// Change replaces the order's lines and terms, then reprices and redates
// it (ADR 0030):
//
//   - before any payment, release or move past awaiting_deposit, the change
//     is free (which roles may set the discount, deposit percent or days is
//     the app layer's);
//   - after one, it needs CustomerApproved, else ErrApprovalRequired;
//   - from assembling on (ready, in a workflow without assembling) it is
//     ErrNotEditable, and after handover or cancel ErrOrderClosed.
//
// Items kept by id keep their status, check, arrival and, while their
// timing is the same, their expected date; new ones start at their mode's
// initial status. The domain takes each item's snapshot and sourcing from
// the spec, so the app layer must pass a kept item's existing ones (its
// price, options and any backup switch), not the catalog's current ones. Before the release, the promise follows the
// forecast unless it was changed by hand; after it, the promise moves only
// to a later forecast. A total below what was paid shows as RefundDue. It
// returns a *shared.ValidationError for lines or terms that break a rule.
func (o *Order) Change(s ChangeSpec) (ChangeResult, error) {
	if s.Today.IsZero() {
		return ChangeResult{}, errMissingToday
	}
	if o.Closed() {
		return ChangeResult{}, ErrOrderClosed
	}
	if o.pastEditing() {
		return ChangeResult{}, ErrNotEditable
	}
	paid := o.paid()
	if paid && !s.CustomerApproved {
		return ChangeResult{}, ErrApprovalRequired
	}
	if s.Terms.Freight.Currency() != o.currency() {
		return ChangeResult{}, fieldError("freightCents", msgCurrency)
	}
	before := o.snapshotForChange()
	lines, err := buildLines(s.Lines, o.currency(), before.items)
	if err != nil {
		return ChangeResult{}, err
	}
	next := *o
	next.lines, next.terms = lines, cloneTerms(s.Terms)
	if err := next.terms.check(next.Subtotal()); err != nil {
		return ChangeResult{}, err
	}
	if _, err := next.Total(); err != nil {
		return ChangeResult{}, err
	}
	o.lines, o.terms = next.lines, next.terms
	o.redateAfterChange(s.Today)
	o.repromise(s.Today)
	res := o.changeResult(before, paid)
	o.addHistory(HistoryEntry{
		Kind: HistoryChanged, By: s.By.UserID(), At: s.Now, CustomerApproved: s.CustomerApproved,
		OldDate: before.promise, NewDate: o.promise, Amount: res.NewTotal,
	})
	return res, nil
}

// redateAfterChange dates the items after a change. Before the release
// nothing was bought, so every item that has not arrived is forecast from
// today. After it, items kept with the same timing keep their date, and new
// or retimed items count from today: they can only be bought from now.
func (o *Order) redateAfterChange(today shared.CivilDate) {
	if o.releaseDate.IsZero() {
		o.redate(today)
		return
	}
	for li := range o.lines {
		for ii := range o.lines[li].items {
			it := &o.lines[li].items[ii]
			if it.expected.IsZero() {
				it.expected = schedule.ExpectedDate(timing(*it), shared.Latest(o.releaseDate, today))
			}
		}
	}
}

// repromise applies the promise rule after a change.
func (o *Order) repromise(today shared.CivilDate) {
	f := o.forecast(today)
	switch {
	case o.releaseDate.IsZero() && !o.promiseByHand:
		o.promise = f
	case f.After(o.promise):
		o.promise = f
	}
}

type beforeChange struct {
	items   map[shared.OrderItemID]Item
	total   shared.Money
	promise shared.CivilDate
}

func (o *Order) snapshotForChange() beforeChange {
	items := map[shared.OrderItemID]Item{}
	for _, i := range o.Items() {
		items[i.id] = i
	}
	return beforeChange{items: items, total: o.total(), promise: o.promise}
}

func (o *Order) changeResult(b beforeChange, paid bool) ChangeResult {
	res := ChangeResult{
		Paid: paid, OldTotal: b.total, NewTotal: o.total(),
		OldPromise: b.promise, NewPromise: o.promise, RefundDue: o.RefundDue(),
	}
	now := map[shared.OrderItemID]bool{}
	for _, i := range o.Items() {
		now[i.id] = true
		if _, ok := b.items[i.id]; !ok {
			res.Added = append(res.Added, i.id)
		}
	}
	for _, i := range sortedItems(b.items) {
		if !now[i] {
			res.Removed = append(res.Removed, i)
		}
	}
	return res
}

// sortedItems returns the ids of m in a fixed order, so results and events
// do not depend on map iteration.
func sortedItems(m map[shared.OrderItemID]Item) []shared.OrderItemID {
	out := make([]shared.OrderItemID, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	slices.SortFunc(out, func(a, b shared.OrderItemID) int { return strings.Compare(a.String(), b.String()) })
	return out
}
