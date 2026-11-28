package order

import (
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Release lets the order go ahead without the deposit, on behalf of by: the
// release date becomes today and every date counts from it, as a paid
// deposit would. Only the owner may (order:override), which the app layer
// checks; the domain records who did it. It returns ErrOrderClosed after
// handover or cancel, ErrInvalidTransition past awaiting_deposit,
// ErrAlreadyReleased when the deposit is paid or the owner already released
// it, and shared.ErrInvalid without a user or today. A deposit that was
// voided can be replaced by a release: the release date becomes today.
func (o *Order) Release(by shared.Principal, today shared.CivilDate, now time.Time) error {
	if by.UserID().IsZero() || today.IsZero() {
		return fmt.Errorf("order: release needs who and today: %w", shared.ErrInvalid)
	}
	if o.Closed() {
		return ErrOrderClosed
	}
	if o.Phase().Rank() > workflow.PhaseAwaitingDeposit.Rank() {
		return ErrInvalidTransition
	}
	if !o.releasedBy.IsZero() || o.hasDeposit() {
		return refuse(ErrAlreadyReleased, "release", msgAlreadyReleased)
	}
	o.releasedBy = by.UserID()
	o.release(today)
	o.addHistory(HistoryEntry{Kind: HistoryReleased, By: by.UserID(), At: now})
	return nil
}

// hasDeposit reports whether deposit payments reach the deposit due, and
// the deposit due is above zero (a 0% deposit releases nothing by itself).
func (o *Order) hasDeposit() bool { return !o.DepositDue().IsZero() && o.DepositPaid() }

// Advance moves the order to status to of its own workflow, on behalf of
// by. The move must be one workflow.CanMove allows, and the guard of every
// phase it enters must hold (docs/architecture.md section 4.2):
//
//   - leaving awaiting_deposit: the deposit is paid or the owner released
//     the order, else GuardDepositUnpaid. An order whose deposit due is zero
//     is released on the day it leaves;
//   - entering assembling, or ready in a workflow without assembling: every
//     item arrived and was checked, else GuardItemsNotReady;
//   - entering handed_over: handover says how (ErrHandoverRequired) and
//     nothing is left to pay, else GuardBalanceUnpaid.
//
// An order with nothing left to source passes sourcing at once, because its
// guards already hold. Cancelling goes through Cancel: moving to cancelled
// here is ErrInvalidTransition. Guards fail as a *GuardError.
func (o *Order) Advance(to workflow.StatusKey, handover *Handover, by shared.Principal, today shared.CivilDate, now time.Time) error {
	if today.IsZero() {
		return errMissingToday
	}
	if to == workflow.Cancelled.Key {
		return ErrInvalidTransition
	}
	from := o.status
	if err := o.workflow.CanMove(from.Key, to); err != nil {
		return err
	}
	entered := o.workflow.PhasesEntered(from.Key, to)
	if err := o.checkGuards(from.Phase, entered, handover); err != nil {
		return err
	}
	target, _ := o.workflow.Status(to)
	if leavesAwaitingDeposit(from.Phase, entered) && o.releaseDate.IsZero() {
		o.release(today)
	}
	o.status = target
	if target.Phase == workflow.PhaseHandedOver {
		o.handedOverAs, o.handedOverAt = *handover, now
	}
	o.addHistory(HistoryEntry{Kind: HistoryStatus, By: by.UserID(), At: now, From: from, To: target, Handover: o.handedOverAs})
	return nil
}

func (o *Order) checkGuards(from workflow.Phase, entered []workflow.Phase, handover *Handover) error {
	if leavesAwaitingDeposit(from, entered) && !o.DepositPaid() && o.releasedBy.IsZero() {
		return &GuardError{Guard: GuardDepositUnpaid}
	}
	if o.entersAssembly(entered) && !o.allArrived() {
		return &GuardError{Guard: GuardItemsNotReady}
	}
	if !contains(entered, workflow.PhaseHandedOver) {
		return nil
	}
	if handover == nil || !handover.Valid() {
		return refuse(ErrHandoverRequired, "handover", msgHandedOverAs)
	}
	if !o.BalanceDue().IsZero() {
		return &GuardError{Guard: GuardBalanceUnpaid}
	}
	return nil
}

// leavesAwaitingDeposit reports whether a move from phase from that enters
// entered goes past awaiting_deposit.
func leavesAwaitingDeposit(from workflow.Phase, entered []workflow.Phase) bool {
	if from.Rank() > workflow.PhaseAwaitingDeposit.Rank() {
		return false
	}
	for _, p := range entered {
		if p.Rank() > workflow.PhaseAwaitingDeposit.Rank() {
			return true
		}
	}
	return false
}

// entersAssembly reports whether entered includes the phase where every
// item must have arrived: assembling, or ready when the workflow has no
// assembling.
func (o *Order) entersAssembly(entered []workflow.Phase) bool {
	if contains(entered, workflow.PhaseAssembling) {
		return true
	}
	return !o.workflow.HasPhase(workflow.PhaseAssembling) && contains(entered, workflow.PhaseReady)
}

func (o *Order) allArrived() bool {
	for _, i := range o.Items() {
		if !i.Arrived() {
			return false
		}
	}
	return true
}

func contains(ps []workflow.Phase, p workflow.Phase) bool {
	for _, q := range ps {
		if q == p {
			return true
		}
	}
	return false
}

// MoveItem moves one item by hand, on behalf of by. In-house items follow
// their lifecycle until milestone 6's production board. A bought item may
// only go from received to installed by hand: purchasing moves it the rest
// of the way (MovePurchased), so the order and its purchase lines never
// disagree (ADR 0031). It returns ErrOrderClosed, ErrItemNotFound,
// ErrItemMovedByPurchasing for any other move of a bought item, and
// ErrInvalidItemMove for a move the lifecycle lacks or a wrong check.
func (o *Order) MoveItem(id shared.OrderItemID, to ItemStatus, check *Check, today shared.CivilDate, now time.Time, by shared.Principal) error {
	it, err := o.movable(id, today)
	if err != nil {
		return err
	}
	if isBought(it.snapshot.SourcingMode) && to != ItemInstalled {
		return ErrItemMovedByPurchasing
	}
	if err := CanMoveItem(it.snapshot.SourcingMode, it.status, to, check); err != nil {
		return err
	}
	o.applyMove(it, to, check, today, HistoryEntry{By: by.UserID(), At: now})
	return nil
}

// movable returns the item id of an open order, for a move made today.
func (o *Order) movable(id shared.OrderItemID, today shared.CivilDate) (*Item, error) {
	if today.IsZero() {
		return nil, errMissingToday
	}
	if o.Closed() {
		return nil, ErrOrderClosed
	}
	it := o.item(id)
	if it == nil {
		return nil, ErrItemNotFound
	}
	return it, nil
}

// applyMove moves it to to with check, already allowed, and records entry
// (who, when and any ref or note) as an item move. A defect lands the item
// back where it is bought or made again; landing on to_buy drops the
// supplier's promised date, since a new purchase gets a new one. An item
// that leaves its arrival, or lands somewhere other than asked, is
// expected again from today.
func (o *Order) applyMove(it *Item, to ItemStatus, check *Check, today shared.CivilDate, entry HistoryEntry) {
	mode := it.snapshot.SourcingMode
	from := it.status
	it.status = landing(mode, to, check)
	if check != nil {
		c := *check
		it.check = &c
	}
	if it.status == ItemToBuy {
		it.promised = shared.CivilDate{}
	}
	switch {
	case arrived(it.status) && it.arrivedOn.IsZero():
		it.arrivedOn = today
	case it.status != to || it.status == ItemToBuy:
		it.arrivedOn = shared.CivilDate{}
		it.expected = schedule.ExpectedDate(timing(*it), today)
	}
	entry.Kind, entry.Item, entry.ItemFrom, entry.ItemTo = HistoryItem, it.id, from, it.status
	if check != nil {
		entry.Check = *check
	}
	o.addHistory(entry)
}

// UseBackupSupplier switches a bought item still to be bought to the backup
// supplier in its snapshot ("Usar o fornecedor reserva"), on behalf of by:
// its supplier, lead days and cost become the backup's, and it is expected
// again from today. The promise does not move. It returns ErrOrderClosed,
// ErrItemNotFound, and ErrNoBackupSupplier for an in-house item, an item
// past to_buy or one with no backup.
func (o *Order) UseBackupSupplier(id shared.OrderItemID, today shared.CivilDate, now time.Time, by shared.Principal) error {
	if today.IsZero() {
		return errMissingToday
	}
	if o.Closed() {
		return ErrOrderClosed
	}
	it := o.item(id)
	if it == nil {
		return ErrItemNotFound
	}
	b := it.sourcing.Backup
	if it.snapshot.SourcingMode == catalog.ModeInHouse || it.status != ItemToBuy || b == nil {
		return ErrNoBackupSupplier
	}
	it.sourcing = Sourcing{Supplier: b.Supplier, SupplierName: b.Name, Cost: clonePtr(b.Cost)}
	it.snapshot.LeadDays = b.LeadDays
	it.expected = schedule.ExpectedDate(timing(*it), today)
	o.addHistory(HistoryEntry{Kind: HistoryBackupSupplier, By: by.UserID(), At: now, Item: id})
	return nil
}

// item returns a pointer to the item with id, or nil.
func (o *Order) item(id shared.OrderItemID) *Item {
	for li := range o.lines {
		for ii := range o.lines[li].items {
			if o.lines[li].items[ii].id == id {
				return &o.lines[li].items[ii]
			}
		}
	}
	return nil
}
