package order

import (
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// isBought reports whether items of mode m are bought from a supplier:
// to_order, and stock until milestone 6 adds reservation (ADR 0030, ADR
// 0031). Purchasing moves them; in-house items are made.
func isBought(m catalog.SourcingMode) bool { return m != catalog.ModeInHouse }

// Bought reports whether the item is bought from a supplier, so purchasing
// moves it (ADR 0031).
func (i Item) Bought() bool { return isBought(i.snapshot.SourcingMode) }

// PurchaseMove is a move purchasing makes on a bought item: to
// sent_to_supplier (sent), in_transit, received with its check (received
// whole; a defect lands on to_buy), or back to to_buy (an undone send, or a
// resolved notice of an item that changed).
type PurchaseMove struct {
	Item  shared.OrderItemID
	To    ItemStatus
	Check *Check
	// Note is a defect's note, kept as the history entry's reason.
	Note string
	// Ref is the pedido de compra ("C-0012"); empty for an item received
	// at the counter, bought with no pedido de compra.
	Ref string
}

// MovePurchased applies m on behalf of by (ADR 0031). It returns
// ErrOrderClosed, ErrItemNotFound, and ErrInvalidItemMove for an in-house
// item, a move to installed (that one is MoveItem's), a move the bought
// lifecycle lacks or a wrong check. Going back to to_buy is allowed from
// sent_to_supplier and in_transit. Landing on to_buy drops the supplier's
// promised date and expects the item again from today.
func (o *Order) MovePurchased(m PurchaseMove, today shared.CivilDate, now time.Time, by shared.Principal) error {
	it, err := o.movable(m.Item, today)
	if err != nil {
		return err
	}
	mode := it.snapshot.SourcingMode
	if !isBought(mode) || m.To == ItemInstalled {
		return ErrInvalidItemMove
	}
	if !undo(it.status, m.To, m.Check) {
		if err := CanMoveItem(mode, it.status, m.To, m.Check); err != nil {
			return err
		}
	}
	o.applyMove(it, m.To, m.Check, today, HistoryEntry{By: by.UserID(), At: now, Ref: m.Ref, Reason: m.Note})
	return nil
}

// undo reports whether a move from from back to to_buy, with no check,
// undoes a purchase that was sent: the bought lifecycle itself only steps
// back one place, but an undone pedido de compra may already be in transit.
func undo(from, to ItemStatus, check *Check) bool {
	return to == ItemToBuy && check == nil && (from == ItemSentToSupplier || from == ItemInTransit)
}

// RecordPromisedDate records the date the supplier promised for a bought
// item that was sent (sent_to_supplier or in_transit), on behalf of by, and
// redates it: it is now expected on that date plus its options' extra days,
// and alerts follow. The promise to the client never moves (ADR 0030); it
// moves only by ChangeDeliveryDate. ref is the pedido de compra. It returns
// ErrOrderClosed, ErrItemNotFound, ErrInvalidItemMove for any other item,
// and ErrDateInPast (field "date") for a day before today.
func (o *Order) RecordPromisedDate(item shared.OrderItemID, date shared.CivilDate, ref string, today shared.CivilDate, now time.Time, by shared.Principal) error {
	it, err := o.movable(item, today)
	if err != nil {
		return err
	}
	if !isBought(it.snapshot.SourcingMode) || (it.status != ItemSentToSupplier && it.status != ItemInTransit) {
		return ErrInvalidItemMove
	}
	if date.IsZero() || date.Before(today) {
		return refuse(ErrDateInPast, "date", msgDateFromToday)
	}
	it.promised = date
	it.expected = schedule.ExpectedDate(timing(*it), o.base(today))
	o.addHistory(HistoryEntry{Kind: HistoryPromised, By: by.UserID(), At: now, Item: item, NewDate: date, Ref: ref})
	return nil
}

// RecordPaidCost records the unit price actually paid for a bought item.
// It records no history: history is read by every staff role, and costs by
// the owner only (identity.SeesCosts, ADR 0025). It works on a closed order
// too, since margins are read after handover. It returns ErrItemNotFound,
// ErrInvalidItemMove for an in-house item, and an error matching
// shared.ErrInvalid for an amount in another currency than the order's.
func (o *Order) RecordPaidCost(item shared.OrderItemID, cost shared.Money) error {
	it := o.item(item)
	if it == nil {
		return ErrItemNotFound
	}
	if !isBought(it.snapshot.SourcingMode) {
		return ErrInvalidItemMove
	}
	if cost.Currency() != o.terms.Freight.Currency() {
		return fmt.Errorf("paid cost of item %s in %s, order in %s: %w", item, cost.Currency(), o.terms.Freight.Currency(), shared.ErrInvalid)
	}
	c := cost
	it.sourcing.PaidCost = &c
	return nil
}

// Redate recomputes the forecast of every item that has not arrived, from
// the release date (or today before it), taking each supplier's promised
// date where one is recorded. The promise never moves.
func (o *Order) Redate(today shared.CivilDate) { o.redate(today) }

// Purchasable reports whether the order's bought items still to buy belong
// in Compras, "A comprar" (ADR 0031): the order is open, and it was
// released (its deposit paid, or the owner released it) or, when the store
// buys before the deposit ("Liberar compras antes do sinal"), it has left
// its draft.
//
// Product rule: a purchase only goes out after the deposit is paid, unless
// the owner changed this (docs/product/design-document.md, System rules).
func (o *Order) Purchasable(beforeDeposit bool) bool {
	if o.Closed() {
		return false
	}
	return !o.releaseDate.IsZero() || (beforeDeposit && o.Phase() != workflow.PhaseDraft)
}
