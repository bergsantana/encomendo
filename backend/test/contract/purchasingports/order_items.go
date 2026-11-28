// Package purchasingports holds the contract suites of app/purchasing's
// driven ports. RunOrderItemsContract runs the same cases against the fake
// (fakes_test.go) and against the inproc adapter over app/order
// (inproc_test.go), so the fake that use case tests rely on behaves as the
// order context does (backend/CLAUDE.md, Testing).
package purchasingports

import (
	"context"
	"errors"
	"testing"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// OrderItemsHarness builds the world a case runs in.
type OrderItemsHarness struct {
	Items apppurchasing.OrderItems
	// Ctx is a caller who may do everything (the owner).
	Ctx context.Context
	// Today is the store's day.
	Today shared.CivilDate
	// Released makes an order whose one bought item is still to buy and
	// purchasable (its deposit paid), and returns its ids.
	Released func(t *testing.T) (shared.OrderID, shared.OrderItemID)
	// Draft makes an order not yet purchasable, with one bought item.
	Draft func(t *testing.T) (shared.OrderID, shared.OrderItemID)
	// Cancel cancels an order.
	Cancel func(t *testing.T, id shared.OrderID)
	// As returns a caller of the store with role. Nil for a fake, which
	// knows no principal: permissions are the order context's.
	As func(t *testing.T, role shared.Role) context.Context
}

type orderItemsCase struct {
	name string
	run  func(*testing.T, OrderItemsHarness)
}

// RunOrderItemsContract checks apppurchasing.OrderItems.
func RunOrderItemsContract(t *testing.T, newHarness func(t *testing.T) OrderItemsHarness) {
	t.Helper()
	for _, c := range []orderItemsCase{
		{"a released order's bought item is to buy, a draft's is not", toBuyReleasedNotDraft},
		{"a send takes the item out of A comprar", sendLeavesToBuy},
		{"a received item arrives; a defect goes back to A comprar", receiveAndDefect},
		{"an undone send is to buy again, without its promised date", undoDropsPromise},
		{"a promised date redates the item", promisedDateRedates},
		{"a cancelled order refuses moves and says it is cancelled", cancelledRefuses},
		{"a refused move moves nothing", refusedMoveMovesNothing},
		{"Orders answers each order once, and an unknown one is not found", ordersOnceAndNotFound},
		{"staff receive at the counter but never send; only the owner records a price paid", rolesThroughThePort},
		{"a price paid is recorded for a bought item, also on a closed order, in the order's currency", paidCosts},
	} {
		t.Run(c.name, func(t *testing.T) { c.run(t, newHarness(t)) })
	}
}

func toBuy(t *testing.T, h OrderItemsHarness) map[shared.OrderItemID]apppurchasing.ToBuyItem {
	t.Helper()
	v, err := h.Items.ToBuy(h.Ctx)
	if err != nil {
		t.Fatalf("ToBuy: %v", err)
	}
	out := map[shared.OrderItemID]apppurchasing.ToBuyItem{}
	for _, it := range v.Items {
		out[it.Item.OrderItem] = it
	}
	return out
}

func itemOf(t *testing.T, h OrderItemsHarness, o shared.OrderID, id shared.OrderItemID) apppurchasing.OrderItemFacts {
	t.Helper()
	facts, err := h.Items.Orders(h.Ctx, []shared.OrderID{o})
	if err != nil || len(facts) != 1 {
		t.Fatalf("Orders = %v, %v", facts, err)
	}
	it, ok := facts[0].Item(id)
	if !ok {
		t.Fatalf("order %s has no item %s", o, id)
	}
	return it
}

func move(t *testing.T, h OrderItemsHarness, m ...apppurchasing.ItemMove) {
	t.Helper()
	if err := h.Items.Move(h.Ctx, m); err != nil {
		t.Fatalf("Move: %v", err)
	}
}

func check(c purchasing.Check) *purchasing.Check { return &c }

func toBuyReleasedNotDraft(t *testing.T, h OrderItemsHarness) {
	o, item := h.Released(t)
	d, draftItem := h.Draft(t)
	got := toBuy(t, h)
	it, ok := got[item]
	if !ok || it.Item.Order != o || it.Item.Name == "" || it.Item.Quantity < 1 || it.Supplier.IsZero() ||
		it.SupplierName == "" || it.Number == "" || it.ArrivalLimit.IsZero() {
		t.Errorf("released item = %+v (found %v), want it with its order, supplier and arrival limit", it, ok)
	}
	if _, ok := got[draftItem]; ok {
		t.Errorf("the draft %s's item is to buy", d)
	}
	f := itemOf(t, h, o, item)
	if !f.Bought || f.Status != order.ItemToBuy || f.Arrived {
		t.Errorf("item facts = %+v, want bought, to buy", f)
	}
}

func sendLeavesToBuy(t *testing.T, h OrderItemsHarness) {
	o, item := h.Released(t)
	move(t, h, apppurchasing.ItemMove{Order: o, Item: item, To: order.ItemSentToSupplier, Ref: "C-0001"})
	if _, ok := toBuy(t, h)[item]; ok {
		t.Error("a sent item is still to buy")
	}
	if f := itemOf(t, h, o, item); f.Status != order.ItemSentToSupplier {
		t.Errorf("status = %s, want sent_to_supplier", f.Status)
	}
	move(t, h, apppurchasing.ItemMove{Order: o, Item: item, To: order.ItemInTransit, Ref: "C-0001"})
	if f := itemOf(t, h, o, item); f.Status != order.ItemInTransit {
		t.Errorf("status = %s, want in_transit", f.Status)
	}
}

func receiveAndDefect(t *testing.T, h OrderItemsHarness) {
	o, item := h.Released(t)
	move(t, h, apppurchasing.ItemMove{Order: o, Item: item, To: order.ItemSentToSupplier, Ref: "C-0001"})
	move(t, h, apppurchasing.ItemMove{Order: o, Item: item, To: order.ItemReceived, Check: check(purchasing.CheckDefective), Note: "Rasgado", Ref: "C-0001"})
	if f := itemOf(t, h, o, item); f.Status != order.ItemToBuy || f.Arrived {
		t.Errorf("after a defect = %+v, want to_buy, not arrived", f)
	}
	if _, ok := toBuy(t, h)[item]; !ok {
		t.Error("a defective item is not back in A comprar")
	}
	move(t, h, apppurchasing.ItemMove{Order: o, Item: item, To: order.ItemReceived, Check: check(purchasing.CheckOK)})
	if f := itemOf(t, h, o, item); f.Status != order.ItemReceived || !f.Arrived {
		t.Errorf("received at the counter = %+v, want received and arrived", f)
	}
}

func undoDropsPromise(t *testing.T, h OrderItemsHarness) {
	o, item := h.Released(t)
	move(t, h, apppurchasing.ItemMove{Order: o, Item: item, To: order.ItemSentToSupplier, Ref: "C-0001"})
	if err := h.Items.RecordPromisedDates(h.Ctx, []apppurchasing.ItemPromise{{Order: o, Item: item, Date: h.Today.AddDays(9), Ref: "C-0001"}}); err != nil {
		t.Fatal(err)
	}
	move(t, h, apppurchasing.ItemMove{Order: o, Item: item, To: order.ItemToBuy, Ref: "C-0001"})
	f := itemOf(t, h, o, item)
	if f.Status != order.ItemToBuy || !f.Promised.IsZero() {
		t.Errorf("after undo = %+v, want to_buy with no promised date", f)
	}
	if _, ok := toBuy(t, h)[item]; !ok {
		t.Error("an undone item is not back in A comprar")
	}
}

func promisedDateRedates(t *testing.T, h OrderItemsHarness) {
	o, item := h.Released(t)
	day := h.Today.AddDays(9)
	if err := h.Items.RecordPromisedDates(h.Ctx, []apppurchasing.ItemPromise{{Order: o, Item: item, Date: day}}); !errors.Is(err, order.ErrInvalidItemMove) {
		t.Errorf("a promised date for an item not sent: err = %v, want ErrInvalidItemMove", err)
	}
	move(t, h, apppurchasing.ItemMove{Order: o, Item: item, To: order.ItemSentToSupplier, Ref: "C-0001"})
	if err := h.Items.RecordPromisedDates(h.Ctx, []apppurchasing.ItemPromise{{Order: o, Item: item, Date: h.Today.AddDays(-1)}}); !errors.Is(err, order.ErrDateInPast) {
		t.Errorf("a past day: err = %v, want ErrDateInPast", err)
	}
	if err := h.Items.RecordPromisedDates(h.Ctx, []apppurchasing.ItemPromise{{Order: o, Item: item, Date: day, Ref: "C-0001"}}); err != nil {
		t.Fatal(err)
	}
	if f := itemOf(t, h, o, item); f.Promised != day || f.Expected != day {
		t.Errorf("after the promised date = %+v, want promised and expected on %s", f, day)
	}
}

func cancelledRefuses(t *testing.T, h OrderItemsHarness) {
	o, item := h.Released(t)
	h.Cancel(t, o)
	if err := h.Items.Move(h.Ctx, []apppurchasing.ItemMove{{Order: o, Item: item, To: order.ItemSentToSupplier}}); !errors.Is(err, order.ErrOrderClosed) {
		t.Errorf("move on a cancelled order: err = %v, want ErrOrderClosed", err)
	}
	facts, err := h.Items.Orders(h.Ctx, []shared.OrderID{o})
	if err != nil || len(facts) != 1 || !facts[0].Cancelled || !facts[0].Closed {
		t.Errorf("Orders = %+v, %v; want it cancelled and closed", facts, err)
	}
	if _, ok := toBuy(t, h)[item]; ok {
		t.Error("a cancelled order's item is to buy")
	}
}

func refusedMoveMovesNothing(t *testing.T, h OrderItemsHarness) {
	a, itemA := h.Released(t)
	b, itemB := h.Released(t)
	err := h.Items.Move(h.Ctx, []apppurchasing.ItemMove{
		{Order: a, Item: itemA, To: order.ItemSentToSupplier, Ref: "C-0001"},
		{Order: b, Item: itemB, To: order.ItemInTransit, Ref: "C-0001"}, // not sent yet
	})
	if !errors.Is(err, order.ErrInvalidItemMove) {
		t.Fatalf("err = %v, want ErrInvalidItemMove", err)
	}
	if f := itemOf(t, h, a, itemA); f.Status != order.ItemToBuy {
		t.Errorf("the first move stayed: %s, want to_buy", f.Status)
	}
	if err := h.Items.Move(h.Ctx, []apppurchasing.ItemMove{{Order: a, Item: itemB, To: order.ItemSentToSupplier}}); !errors.Is(err, order.ErrItemNotFound) {
		t.Errorf("an item of another order: err = %v, want ErrItemNotFound", err)
	}
}

func ordersOnceAndNotFound(t *testing.T, h OrderItemsHarness) {
	o, _ := h.Released(t)
	facts, err := h.Items.Orders(h.Ctx, []shared.OrderID{o, o})
	if err != nil || len(facts) != 1 || facts[0].Order != o || facts[0].Number == "" || facts[0].Cancelled {
		t.Errorf("Orders twice = %+v, %v; want the order once", facts, err)
	}
	unknown, err := shared.ParseID("01a2ffff-ffff-7fff-bfff-ffffffffffff") // no harness made it
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Items.Orders(h.Ctx, []shared.OrderID{shared.OrderID(unknown)}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an unknown order: err = %v, want ErrNotFound", err)
	}
}

// rolesThroughThePort runs purchasing's calls as staff and as an admin
// through the order context: staff see "A comprar" without costs and
// receive at the counter, but may not send; an admin may not record a price
// paid (security review of 5.6–5.8).
func rolesThroughThePort(t *testing.T, h OrderItemsHarness) {
	if h.As == nil {
		t.Skip("the fake knows no principal; permissions are the order context's")
	}
	o, item := h.Released(t)
	staff, admin := h.As(t, shared.RoleStaff), h.As(t, shared.RoleAdmin)
	v, err := h.Items.ToBuy(staff)
	if err != nil {
		t.Fatalf("staff ToBuy: %v", err)
	}
	for _, it := range v.Items {
		if it.Cost != nil {
			t.Errorf("staff see a cost on %s", it.Item.Name)
		}
	}
	if err := h.Items.Move(staff, []apppurchasing.ItemMove{{Order: o, Item: item, To: order.ItemSentToSupplier, Ref: "C-0001"}}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("staff send: err = %v, want ErrForbidden", err)
	}
	cost, err := shared.NewMoney(41_000, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Items.RecordPaidCosts(admin, []apppurchasing.ItemCost{{Order: o, Item: item, Cost: cost}}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("admin price paid: err = %v, want ErrForbidden", err)
	}
	if err := h.Items.Move(staff, []apppurchasing.ItemMove{{Order: o, Item: item, To: order.ItemReceived, Check: check(purchasing.CheckOK)}}); err != nil {
		t.Errorf("staff receive at the counter: %v", err)
	}
}

func paidCosts(t *testing.T, h OrderItemsHarness) {
	o, item := h.Released(t)
	brl, err := shared.NewMoney(41_000, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	usd, err := shared.NewMoney(100, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Items.RecordPaidCosts(h.Ctx, []apppurchasing.ItemCost{{Order: o, Item: item, Cost: usd}}); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("another currency: err = %v, want ErrInvalid", err)
	}
	if err := h.Items.RecordPaidCosts(h.Ctx, []apppurchasing.ItemCost{{Order: o, Item: item, Cost: brl}}); err != nil {
		t.Errorf("a price paid: %v", err)
	}
	h.Cancel(t, o)
	if err := h.Items.RecordPaidCosts(h.Ctx, []apppurchasing.ItemCost{{Order: o, Item: item, Cost: brl}}); err != nil {
		t.Errorf("a price paid on a closed order (margins are read after it): %v", err)
	}
}
