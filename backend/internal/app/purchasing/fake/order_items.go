// Package fake holds hand-written fakes of app/purchasing's driven ports,
// for use case tests. Each passes its port's contract suite
// (test/contract/purchasingports).
package fake

import (
	"context"
	"fmt"
	"slices"
	"sync"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// OrderItems is an in-memory apppurchasing.OrderItems. It keeps orders as
// purchasing sees them and applies the bought lifecycle the order context
// enforces (ADR 0031). It checks no permission: that is the order
// context's, tested there.
type OrderItems struct {
	mu        sync.Mutex
	orders    map[shared.OrderID]*fakeOrder
	order     []shared.OrderID
	today     shared.CivilDate
	fail      error
	paid      map[shared.OrderItemID]shared.Money
	moves     int
	truncated bool
}

type fakeOrder struct {
	facts       apppurchasing.OrderFacts
	purchasable bool
	catalog     map[shared.OrderItemID]shared.ItemID
	costs       map[shared.OrderItemID]shared.Money
}

// NewOrderItems returns an empty fake whose day is today.
func NewOrderItems(today shared.CivilDate) *OrderItems {
	return &OrderItems{orders: map[shared.OrderID]*fakeOrder{}, today: today, paid: map[shared.OrderItemID]shared.Money{}}
}

// Item is one item a test adds to an order.
type Item struct {
	Facts       apppurchasing.OrderItemFacts
	CatalogItem shared.ItemID
	Cost        *shared.Money
}

// AddOrder stores an order with items; purchasable says whether its bought
// items to buy are in "A comprar" (released, or past draft with the
// setting on).
func (f *OrderItems) AddOrder(facts apppurchasing.OrderFacts, purchasable bool, items ...Item) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := &fakeOrder{facts: facts, purchasable: purchasable, catalog: map[shared.OrderItemID]shared.ItemID{}, costs: map[shared.OrderItemID]shared.Money{}}
	o.facts.Items = nil
	for _, it := range items {
		o.facts.Items = append(o.facts.Items, it.Facts)
		o.catalog[it.Facts.Item] = it.CatalogItem
		if it.Cost != nil {
			o.costs[it.Facts.Item] = *it.Cost
		}
	}
	f.orders[facts.Order] = o
	f.order = append(f.order, facts.Order)
}

// Cancel cancels an order.
func (f *OrderItems) Cancel(id shared.OrderID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if o := f.orders[id]; o != nil {
		o.facts.Cancelled, o.facts.Closed = true, true
	}
}

// RemoveItem removes an item from its order, as a change would.
func (f *OrderItems) RemoveItem(order shared.OrderID, item shared.OrderItemID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if o := f.orders[order]; o != nil {
		o.facts.Items = slices.DeleteFunc(o.facts.Items, func(it apppurchasing.OrderItemFacts) bool { return it.Item == item })
	}
}

// SetQuantity changes an item's quantity, as a change would.
func (f *OrderItems) SetQuantity(order shared.OrderID, item shared.OrderItemID, qty int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if o := f.orders[order]; o != nil {
		for i := range o.facts.Items {
			if o.facts.Items[i].Item == item {
				o.facts.Items[i].Quantity = qty
			}
		}
	}
}

// SetTruncated makes ToBuy say the store has more orders than it read.
func (f *OrderItems) SetTruncated(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.truncated = on
}

// Fail makes every later call return err; nil heals it.
func (f *OrderItems) Fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = err
}

// PaidCost returns the price paid recorded for item.
func (f *OrderItems) PaidCost(item shared.OrderItemID) (shared.Money, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.paid[item]
	return m, ok
}

// Moves returns how many moves were applied.
func (f *OrderItems) Moves() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.moves
}

// ToBuy implements apppurchasing.OrderItems. Costs are given to every
// caller: the fake knows no principal, so use case tests check that views
// for non-owners leave them out.
func (f *OrderItems) ToBuy(context.Context) (apppurchasing.ToBuyItems, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return apppurchasing.ToBuyItems{}, f.fail
	}
	out := apppurchasing.ToBuyItems{Truncated: f.truncated}
	// Earliest delivery first, as the port promises; the arrival limit
	// stands for the delivery date here (both move together).
	ids := slices.Clone(f.order)
	slices.SortStableFunc(ids, func(a, b shared.OrderID) int {
		return f.orders[b].facts.ArrivalLimit.DaysUntil(f.orders[a].facts.ArrivalLimit)
	})
	for _, id := range ids {
		o := f.orders[id]
		if !o.purchasable || o.facts.Closed {
			continue
		}
		for _, it := range o.facts.Items {
			if !it.Bought || it.Status != order.ItemToBuy {
				continue
			}
			v := apppurchasing.ToBuyItem{
				Item: purchasing.Item{Order: id, OrderItem: it.Item, CatalogItem: o.catalog[it.Item], Name: it.Name,
					Quantity: it.Quantity, Choices: slices.Clone(it.Choices)},
				Number: o.facts.Number, Supplier: it.Supplier, SupplierName: it.SupplierName, ArrivalLimit: o.facts.ArrivalLimit,
			}
			if c, ok := o.costs[it.Item]; ok {
				v.Cost = &c
			}
			out.Items = append(out.Items, v)
		}
	}
	return out, nil
}

// Orders implements apppurchasing.OrderItems.
func (f *OrderItems) Orders(_ context.Context, ids []shared.OrderID) ([]apppurchasing.OrderFacts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return nil, f.fail
	}
	var out []apppurchasing.OrderFacts
	seen := map[shared.OrderID]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		o := f.orders[id]
		if o == nil {
			return nil, fmt.Errorf("fake: order %s: %w", id, shared.ErrNotFound)
		}
		facts := o.facts
		facts.Items = slices.Clone(o.facts.Items)
		out = append(out, facts)
	}
	return out, nil
}

// Move implements apppurchasing.OrderItems: every move is checked before
// any is applied, so a refusal moves nothing.
func (f *OrderItems) Move(ctx context.Context, moves []apppurchasing.ItemMove) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	for _, m := range moves {
		if _, err := f.item(m.Order, m.Item, true); err != nil {
			return err
		}
	}
	staged := map[shared.OrderItemID]order.ItemStatus{}
	for _, m := range moves {
		it, _ := f.item(m.Order, m.Item, true)
		from := it.Status
		if s, ok := staged[m.Item]; ok {
			from = s
		}
		if err := checkMove(it.Bought, from, m); err != nil {
			return err
		}
		staged[m.Item] = m.To
	}
	for _, m := range moves {
		f.keep(ctx, m.Order)
		it, _ := f.item(m.Order, m.Item, true)
		apply(it, m, f.today)
		f.moves++
	}
	return nil
}

// RecordPromisedDates implements apppurchasing.OrderItems.
func (f *OrderItems) RecordPromisedDates(ctx context.Context, dates []apppurchasing.ItemPromise) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	for _, d := range dates {
		it, err := f.item(d.Order, d.Item, true)
		if err != nil {
			return err
		}
		if !it.Bought || (it.Status != order.ItemSentToSupplier && it.Status != order.ItemInTransit) {
			return order.ErrInvalidItemMove
		}
		if d.Date.Before(f.today) {
			return order.ErrDateInPast
		}
	}
	for _, d := range dates {
		f.keep(ctx, d.Order)
		it, _ := f.item(d.Order, d.Item, true)
		it.Promised, it.Expected = d.Date, d.Date
		it.Alerts = alertsOf(it.Expected, f.orders[d.Order].facts.ArrivalLimit, f.today)
	}
	return nil
}

// alertsOf is the alert the forecast raises against the order's "Precisa
// chegar até": at risk when it comes after it, late when that day is
// already past too. It stands in for the schedule's rules, which the order
// context owns.
func alertsOf(expected, arrivalLimit, today shared.CivilDate) []schedule.Alert {
	switch {
	case !expected.After(arrivalLimit):
		return nil
	case arrivalLimit.Before(today):
		return []schedule.Alert{schedule.AlertLate}
	default:
		return []schedule.Alert{schedule.AlertAtRisk}
	}
}

// RecordPaidCosts implements apppurchasing.OrderItems.
func (f *OrderItems) RecordPaidCosts(ctx context.Context, costs []apppurchasing.ItemCost) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	for _, c := range costs {
		it, err := f.item(c.Order, c.Item, false)
		if err != nil {
			return err
		}
		if !it.Bought {
			return order.ErrInvalidItemMove
		}
		if c.Cost.Currency() != "BRL" || c.Cost.IsNegative() {
			return fmt.Errorf("fake: paid cost %s: %w", c.Cost, shared.ErrInvalid) // the fake's orders are in BRL
		}
	}
	for _, c := range costs {
		sharedfake.KeepOnRollback(ctx, &f.mu, f.paid, c.Item)
		f.paid[c.Item] = c.Cost
	}
	return nil
}

// keep makes a failed transaction undo what this one does to an order's
// items, as the order context's own repository would. The caller holds the
// lock; the undo takes it.
func (f *OrderItems) keep(ctx context.Context, id shared.OrderID) {
	o := f.orders[id]
	if o == nil {
		return
	}
	before := slices.Clone(o.facts.Items)
	sharedfake.OnRollback(ctx, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		o.facts.Items = before
	})
}

// item returns a pointer to an item of an order. With open, a closed order
// is order.ErrOrderClosed.
func (f *OrderItems) item(id shared.OrderID, item shared.OrderItemID, open bool) (*apppurchasing.OrderItemFacts, error) {
	o := f.orders[id]
	if o == nil {
		return nil, fmt.Errorf("fake: order %s: %w", id, shared.ErrNotFound)
	}
	if open && o.facts.Closed {
		return nil, order.ErrOrderClosed
	}
	for i := range o.facts.Items {
		if o.facts.Items[i].Item == item {
			return &o.facts.Items[i], nil
		}
	}
	return nil, order.ErrItemNotFound
}

// next is the bought lifecycle as purchasing drives it (ADR 0031).
var next = map[order.ItemStatus][]order.ItemStatus{
	order.ItemToBuy:          {order.ItemSentToSupplier, order.ItemReceived},
	order.ItemSentToSupplier: {order.ItemInTransit, order.ItemReceived, order.ItemToBuy},
	order.ItemInTransit:      {order.ItemReceived, order.ItemToBuy},
}

func checkMove(bought bool, from order.ItemStatus, m apppurchasing.ItemMove) error {
	if !bought || !slices.Contains(next[from], m.To) {
		return order.ErrInvalidItemMove
	}
	if (m.To == order.ItemReceived) != (m.Check != nil) {
		return order.ErrInvalidItemMove
	}
	if m.Check != nil && !m.Check.Valid() {
		return order.ErrInvalidItemMove
	}
	return nil
}

func apply(it *apppurchasing.OrderItemFacts, m apppurchasing.ItemMove, today shared.CivilDate) {
	it.Status, it.Arrived = m.To, false
	if m.To == order.ItemReceived && *m.Check == purchasing.CheckDefective {
		it.Status = order.ItemToBuy
	}
	if it.Status == order.ItemReceived {
		it.Arrived = true
	}
	if it.Status == order.ItemToBuy {
		it.Promised, it.Expected, it.Alerts = shared.CivilDate{}, today, nil
	}
}

// Cost returns the order-time unit cost of an item, as the order's
// sourcing holds it; the fake Repository copies it into new lines, as the
// Postgres one does in SQL.
func (f *OrderItems) Cost(item shared.OrderItemID) (shared.Money, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range f.orders {
		if c, ok := o.costs[item]; ok {
			return c, true
		}
	}
	return shared.Money{}, false
}
