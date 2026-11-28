package order

import "github.com/bergsantana/encomendo/backend/internal/domain/shared"

// Item is one item of an order: its snapshot, where it stands and when it
// is expected.
type Item struct {
	id        shared.OrderItemID
	line      shared.OrderLineID
	snapshot  ItemSnapshot
	sourcing  Sourcing
	status    ItemStatus
	check     *Check
	expected  shared.CivilDate
	arrivedOn shared.CivilDate
	promised  shared.CivilDate // the supplier's promised date; staff-only
}

// ID returns the item's id.
func (i Item) ID() shared.OrderItemID { return i.id }

// Line returns the id of the line the item belongs to.
func (i Item) Line() shared.OrderLineID { return i.line }

// Snapshot returns what the client may see of the item at order time.
func (i Item) Snapshot() ItemSnapshot { return cloneSnapshot(i.snapshot) }

// Sourcing returns the staff-only part of the snapshot: supplier and cost.
func (i Item) Sourcing() Sourcing { return cloneSourcing(i.sourcing) }

// Status returns where the item stands.
func (i Item) Status() ItemStatus { return i.status }

// Check returns the last check recorded on the item, and false before it
// was ever received or made.
func (i Item) Check() (Check, bool) {
	if i.check == nil {
		return "", false
	}
	return *i.check, true
}

// Expected returns the date the item should be ready (the forecast).
func (i Item) Expected() shared.CivilDate { return i.expected }

// ArrivedOn returns the day the item was received or made with a good
// check, and false while it has not arrived.
func (i Item) ArrivedOn() (shared.CivilDate, bool) {
	return i.arrivedOn, !i.arrivedOn.IsZero()
}

// Promised returns the date the supplier promised for a bought item, and
// false while none is recorded. It is staff data: it lives in the
// staff-only sourcing table and no client view carries it (ADR 0031).
func (i Item) Promised() (shared.CivilDate, bool) { return i.promised, !i.promised.IsZero() }

// Arrived reports whether the item is ready to be assembled: received or
// made and checked ok, or installed.
func (i Item) Arrived() bool { return arrived(i.status) }

// NextMoves returns the statuses MoveItem accepts for the item now: for a
// bought item only installed, once received, since purchasing makes every
// other move (ADR 0031).
func (i Item) NextMoves() []ItemStatus {
	if isBought(i.snapshot.SourcingMode) {
		if i.status == ItemReceived {
			return []ItemStatus{ItemInstalled}
		}
		return nil
	}
	return NextItemMoves(i.snapshot.SourcingMode, i.status)
}

// keepState returns i with the status, check, arrival and expected date of
// prev when prev is the same item (same id and sourcing mode) before a
// change. The expected date is kept only while the item's timing (lead days
// and option days) is the same, so a redone item keeps its later date and
// its alert (review of PR B).
func (i Item) keepState(prev Item) Item {
	if prev.id != i.id || prev.snapshot.SourcingMode != i.snapshot.SourcingMode {
		return i
	}
	i.status, i.check, i.arrivedOn, i.promised = prev.status, clonePtr(prev.check), prev.arrivedOn, prev.promised
	if timing(prev) == timing(i) {
		i.expected = prev.expected
	}
	return i
}
