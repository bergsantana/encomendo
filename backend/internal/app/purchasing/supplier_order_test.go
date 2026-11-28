package purchasing_test

import (
	"context"
	"errors"
	"testing"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// sentOrder sends two items of two orders and returns what the tests need.
type sentOrder struct {
	view           apppurchasing.SupplierOrderView
	order1, order2 shared.OrderID
	items1, items2 []shared.OrderItemID
}

func sendTwo(w *world, who shared.Principal) sentOrder {
	w.t.Helper()
	o1, i1 := w.order(231, bought{name: "Banco de couro", qty: 1, cost: 40_000})
	o2, i2 := w.order(232, bought{name: "Guidão", qty: 1, cost: 15_000})
	v := w.send(who, i1[0], i2[0])
	return sentOrder{view: v.SupplierOrder, order1: o1, order2: o2, items1: i1, items2: i2}
}

func TestUndo_RefusedOnceReceived(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.owner)
	if _, err := w.svc.Receive(w.ctx(w.staff), apppurchasing.ReceiptInput{
		Entries:     []apppurchasing.ReceiptEntryInput{{Line: s.view.Lines[0].ID, Check: purchasing.CheckOK}},
		Idempotency: w.key(),
	}); err != nil {
		t.Fatal(err)
	}
	v, err := w.svc.SupplierOrder(w.ctx(w.owner), s.view.ID)
	if err != nil || v.CanUndo {
		t.Fatalf("after a line was received: can undo = %v, %v", v.CanUndo, err)
	}

	_, err = w.svc.Undo(w.ctx(w.owner), apppurchasing.VersionInput{ID: s.view.ID, Version: v.Version})
	if !errors.Is(err, purchasing.ErrSupplierOrderReceived) {
		t.Fatalf("err = %v, want ErrSupplierOrderReceived", err)
	}
	if got := w.itemStatus(s.order2, s.items2[0]); got != order.ItemSentToSupplier {
		t.Fatalf("the refused undo moved an item to %s", got)
	}
}

func TestUndo_CancelsLinesAndReturnsItemsToBuy(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.owner)
	// The second order was cancelled meanwhile: its item needs no move.
	w.items.Cancel(s.order2)

	v, err := w.svc.Undo(w.ctx(w.admin), apppurchasing.VersionInput{ID: s.view.ID, Version: s.view.Version})
	if err != nil {
		t.Fatal(err)
	}
	if v.State != purchasing.StateUndone || v.CanUndo || v.Total != 0 || v.Version != 2 {
		t.Fatalf("view = %+v", v.SupplierOrderSummary)
	}
	for _, l := range v.Lines {
		if l.Status != purchasing.LineCancelled {
			t.Fatalf("line status = %s", l.Status)
		}
	}
	if got := w.itemStatus(s.order1, s.items1[0]); got != order.ItemToBuy {
		t.Fatalf("item 1 = %s, want to_buy", got)
	}
	if got := w.itemStatus(s.order2, s.items2[0]); got != order.ItemSentToSupplier {
		t.Fatalf("item of the cancelled order = %s, want it left alone", got)
	}
	// Its number is never reused, and the item can be sent again.
	again := w.send(w.owner, s.items1[0])
	if again.SupplierOrder.Number != 2 {
		t.Fatalf("number = %v, want C-0002", again.SupplierOrder.Number)
	}
	if _, err := w.svc.Undo(w.ctx(w.owner), apppurchasing.VersionInput{ID: s.view.ID, Version: 2}); !errors.Is(err, purchasing.ErrUndone) {
		t.Fatalf("second undo: %v, want ErrUndone", err)
	}
}

func TestSupplierOrderWrites_VersionAndNotFound(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.owner)
	ctx := w.ctx(w.owner)
	stale := apppurchasing.VersionInput{ID: s.view.ID, Version: 7}
	missing := apppurchasing.VersionInput{ID: shared.SupplierOrderID(w.ids.NewID()), Version: 1}
	for name, call := range map[string]func(apppurchasing.VersionInput) error{
		"undo":       func(in apppurchasing.VersionInput) error { _, err := w.svc.Undo(ctx, in); return err },
		"in transit": func(in apppurchasing.VersionInput) error { _, err := w.svc.MarkInTransit(ctx, in); return err },
	} {
		if err := call(stale); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("%s with a stale version: %v", name, err)
		}
		if err := call(missing); !errors.Is(err, purchasing.ErrSupplierOrderNotFound) {
			t.Errorf("%s of an unknown order: %v", name, err)
		}
	}
	if _, err := w.svc.SupplierOrder(ctx, missing.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("get of an unknown order: %v", err)
	}
}

func TestMarkInTransit_MovesSentItems(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.owner)
	w.items.Cancel(s.order2)

	v, err := w.svc.MarkInTransit(w.ctx(w.admin), apppurchasing.VersionInput{ID: s.view.ID, Version: s.view.Version})
	if err != nil {
		t.Fatal(err)
	}
	if v.State != purchasing.StateInTransit || v.Lines[0].Status != purchasing.LineInTransit {
		t.Fatalf("view = %+v", v)
	}
	if got := w.itemStatus(s.order1, s.items1[0]); got != order.ItemInTransit {
		t.Fatalf("item = %s, want in_transit", got)
	}
	if got := w.itemStatus(s.order2, s.items2[0]); got != order.ItemSentToSupplier {
		t.Fatalf("item of the cancelled order = %s, want it left alone", got)
	}
	if _, err := w.svc.MarkInTransit(w.ctx(w.admin), apppurchasing.VersionInput{ID: s.view.ID, Version: v.Version}); !errors.Is(err, purchasing.ErrNothingOpen) {
		t.Fatalf("second mark: %v, want ErrNothingOpen", err)
	}
}

func TestRecordPromisedDate_RedatesAndRaisesAtRisk(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.owner)
	late := w.today.AddDays(20) // after "Precisa chegar até" (today + 17)
	soon := w.today.AddDays(10)

	v, err := w.svc.RecordPromisedDate(w.ctx(w.admin), apppurchasing.PromisedDateInput{
		ID: s.view.ID, Version: s.view.Version, Date: late,
		Lines: []apppurchasing.LineDate{{Line: s.view.Lines[1].ID, Date: soon}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.Promised != late || v.Version != 2 {
		t.Fatalf("view = %+v", v.SupplierOrderSummary)
	}
	first, second := v.Lines[0], v.Lines[1]
	if first.Promised != late || first.Expected != late || len(first.Alerts) != 1 || first.Alerts[0] != schedule.AlertAtRisk {
		t.Fatalf("late line = %+v", first)
	}
	if second.Promised != soon || second.Expected != soon || len(second.Alerts) != 0 {
		t.Fatalf("line with its own date = %+v", second)
	}
	// The order context has the forecast; the promise to the client never moved.
	f := w.facts(s.order1)
	if it, _ := f.Item(s.items1[0]); it.Promised != late || it.Expected != late || len(it.Alerts) != 1 {
		t.Fatalf("order item = %+v", it)
	}
	if f.ArrivalLimit != w.today.AddDays(17) {
		t.Fatalf("arrival limit moved to %v", f.ArrivalLimit)
	}
}

func TestRecordPromisedDate_Refusals(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.owner)
	line := s.view.Lines[0].ID
	for _, c := range []struct {
		name string
		in   apppurchasing.PromisedDateInput
		want error
	}{
		{"past day", apppurchasing.PromisedDateInput{Date: w.today.AddDays(-1)}, purchasing.ErrDateInPast},
		{"unknown line", apppurchasing.PromisedDateInput{Date: w.today, Lines: []apppurchasing.LineDate{{Line: shared.PurchaseLineID(w.ids.NewID()), Date: w.today}}}, purchasing.ErrLineNotFound},
		{"line twice", apppurchasing.PromisedDateInput{Date: w.today, Lines: []apppurchasing.LineDate{{Line: line, Date: w.today}, {Line: line, Date: w.today}}}, shared.ErrInvalid},
		{"no date", apppurchasing.PromisedDateInput{}, shared.ErrInvalid},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.in.ID, c.in.Version = s.view.ID, s.view.Version
			if _, err := w.svc.RecordPromisedDate(w.ctx(w.owner), c.in); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
	// An order cancelled meanwhile keeps its line's date but is not redated.
	w.items.Cancel(s.order1)
	v, err := w.svc.RecordPromisedDate(w.ctx(w.owner), apppurchasing.PromisedDateInput{ID: s.view.ID, Version: s.view.Version, Date: w.today.AddDays(3)})
	if err != nil {
		t.Fatal(err)
	}
	if v.Lines[0].Promised != w.today.AddDays(3) || v.Lines[0].Notice != purchasing.NoticeOrderCancelled {
		t.Fatalf("line of the cancelled order = %+v", v.Lines[0])
	}
	if it, _ := w.facts(s.order1).Item(s.items1[0]); !it.Promised.IsZero() {
		t.Fatalf("the cancelled order's item was redated: %+v", it)
	}
}

func TestRecordPaidPrices_OwnerOnly(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.admin)
	in := apppurchasing.PaidPricesInput{
		ID: s.view.ID, Version: s.view.Version,
		Prices: []apppurchasing.LinePrice{{Line: s.view.Lines[0].ID, Cents: 38_000}},
	}
	for _, who := range []shared.Principal{w.admin, w.staff, w.viewer} {
		if _, err := w.svc.RecordPaidPrices(w.ctx(who), in); !errors.Is(err, shared.ErrForbidden) {
			t.Fatalf("%s: err = %v, want ErrForbidden", who.Role(), err)
		}
	}
	if _, ok := w.items.PaidCost(s.items1[0]); ok {
		t.Fatal("a refused call recorded a price")
	}

	v, err := w.svc.RecordPaidPrices(w.ctx(w.owner), in)
	if err != nil {
		t.Fatal(err)
	}
	if p := v.Lines[0].Paid; p == nil || p.Cents() != 38_000 || p.Currency() != "BRL" {
		t.Fatalf("paid = %v", p)
	}
	if c := v.Lines[0].Cost; c == nil || c.Cents() != 40_000 {
		t.Fatalf("cost = %v, want the order-time cost kept", c)
	}
	if got, ok := w.items.PaidCost(s.items1[0]); !ok || got.Cents() != 38_000 {
		t.Fatalf("the order item's paid cost = %v, %v", got, ok)
	}
}

func TestRecordPaidPrices_Refusals(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.owner)
	line := s.view.Lines[0].ID
	for _, c := range []struct {
		name   string
		prices []apppurchasing.LinePrice
		want   error
	}{
		{"negative", []apppurchasing.LinePrice{{Line: line, Cents: -1}}, shared.ErrInvalid},
		{"unknown line", []apppurchasing.LinePrice{{Line: shared.PurchaseLineID(w.ids.NewID()), Cents: 1}}, purchasing.ErrLineNotFound},
		{"line twice", []apppurchasing.LinePrice{{Line: line, Cents: 1}, {Line: line, Cents: 2}}, shared.ErrInvalid},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := w.svc.RecordPaidPrices(w.ctx(w.owner), apppurchasing.PaidPricesInput{ID: s.view.ID, Version: s.view.Version, Prices: c.prices})
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
	// An item removed from its order keeps its line's price, and the order
	// is not asked to record it.
	w.items.RemoveItem(s.order1, s.items1[0])
	v, err := w.svc.RecordPaidPrices(w.ctx(w.owner), apppurchasing.PaidPricesInput{
		ID: s.view.ID, Version: s.view.Version, Prices: []apppurchasing.LinePrice{{Line: line, Cents: 100}},
	})
	if err != nil || v.Lines[0].Paid == nil {
		t.Fatalf("price of a removed item: %+v, %v", v.Lines[0], err)
	}
	if _, ok := w.items.PaidCost(s.items1[0]); ok {
		t.Fatal("the order recorded a price for a removed item")
	}
}

func TestViews_NoCostForNonOwner(t *testing.T) {
	w := newWorld(t)
	_, i1 := w.order(231, bought{name: "Banco de couro", qty: 1, cost: 40_000})
	_, i2 := w.order(232, bought{name: "Guidão", qty: 1, cost: 15_000})

	// "A comprar" before it is sent.
	for _, c := range []struct {
		who  shared.Principal
		cost bool
	}{{w.owner, true}, {w.admin, false}} {
		tb, err := w.svc.ToBuy(w.ctx(c.who))
		if err != nil {
			t.Fatal(err)
		}
		for _, sup := range tb.Suppliers {
			for _, it := range sup.Items {
				if (it.Cost != nil) != c.cost {
					t.Fatalf("%s sees to-buy cost %v", c.who.Role(), it.Cost)
				}
			}
		}
	}
	// A supplier order, read and changed, by the owner and by an admin.
	sent := w.send(w.owner, i1[0], i2[0])
	if sent.SupplierOrder.Lines[0].Cost == nil {
		t.Fatal("the owner does not see the cost of the order they sent")
	}
	if _, err := w.svc.RecordPaidPrices(w.ctx(w.owner), apppurchasing.PaidPricesInput{
		ID: sent.SupplierOrder.ID, Version: 1, Prices: []apppurchasing.LinePrice{{Line: sent.SupplierOrder.Lines[0].ID, Cents: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	read, err := w.svc.SupplierOrder(w.ctx(w.admin), sent.SupplierOrder.ID)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := w.svc.MarkInTransit(w.ctx(w.admin), apppurchasing.VersionInput{ID: sent.SupplierOrder.ID, Version: read.Version})
	if err != nil {
		t.Fatal(err)
	}
	for name, lines := range map[string][]apppurchasing.PurchaseLineView{"read": read.Lines, "change": moved.Lines} {
		for _, l := range lines {
			if l.Cost != nil || l.Paid != nil {
				t.Fatalf("admin's %s carries cost %v, paid %v", name, l.Cost, l.Paid)
			}
		}
	}
	owners, err := w.svc.SupplierOrder(w.ctx(w.owner), sent.SupplierOrder.ID)
	if err != nil || owners.Lines[0].Cost == nil || owners.Lines[0].Paid == nil {
		t.Fatalf("owner's view lost its costs: %+v, %v", owners.Lines[0], err)
	}
}

// removedItemFact guards the fake: an item gone from the order is gone from
// its facts.
func TestSupplierOrder_ViewOfLineWithRemovedItem(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.owner)
	w.items.RemoveItem(s.order1, s.items1[0])
	w.repo.ForgetItem(s.items1[0])

	v, err := w.svc.SupplierOrder(w.ctx(w.owner), s.view.ID)
	if err != nil {
		t.Fatal(err)
	}
	l := v.Lines[0]
	if !l.OrderItem.IsZero() || l.Notice != purchasing.NoticeItemRemoved || !l.Expected.IsZero() || !l.ArrivalLimit.IsZero() || l.Name != "Banco de couro" {
		t.Fatalf("line = %+v", l)
	}
	// Moves skip the removed item; the other still moves.
	if _, err := w.svc.MarkInTransit(w.ctx(w.owner), apppurchasing.VersionInput{ID: v.ID, Version: v.Version}); err != nil {
		t.Fatal(err)
	}
	if got := w.itemStatus(s.order2, s.items2[0]); got != order.ItemInTransit {
		t.Fatalf("item = %s", got)
	}
}

func TestSupplierOrders_PagesNewestFirst(t *testing.T) {
	w := newWorld(t)
	var sent []apppurchasing.SupplierOrderView
	for i := range 3 {
		_, items := w.order(240+i, bought{name: "Pedal", qty: 1})
		sent = append(sent, w.send(w.owner, items[0]).SupplierOrder)
		w.clock.Advance(1_000_000_000)
	}
	// The oldest is received, so it is not open.
	if _, err := w.svc.Receive(w.ctx(w.owner), apppurchasing.ReceiptInput{
		Entries: []apppurchasing.ReceiptEntryInput{{Line: sent[0].Lines[0].ID, Check: purchasing.CheckOK}}, Idempotency: w.key(),
	}); err != nil {
		t.Fatal(err)
	}
	ctx := w.ctx(w.admin)
	all := page(ctx, t, w, false, "", 2)
	if len(all.Items) != 2 || all.Items[0].ID != sent[2].ID || all.Items[1].ID != sent[1].ID || !all.HasMore() {
		t.Fatalf("first page = %+v", all)
	}
	rest := page(ctx, t, w, false, all.NextCursor, 2)
	if len(rest.Items) != 1 || rest.Items[0].ID != sent[0].ID || rest.Items[0].State != purchasing.StateReceived || rest.HasMore() {
		t.Fatalf("second page = %+v", rest)
	}
	open := page(ctx, t, w, true, "", 10)
	if len(open.Items) != 2 {
		t.Fatalf("open = %+v", open.Items)
	}
	if _, err := shared.NewPageRequest("not-a-cursor", 2); err != nil {
		t.Fatal(err)
	}
	bad, _ := shared.NewPageRequest("not-a-cursor", 2)
	if _, err := w.svc.SupplierOrders(ctx, false, bad); !errors.Is(err, shared.ErrInvalid) {
		t.Fatalf("bad cursor: %v", err)
	}
}

func page(ctx context.Context, t *testing.T, w *world, open bool, cursor string, limit int) shared.Page[apppurchasing.SupplierOrderSummary] {
	t.Helper()
	p, err := shared.NewPageRequest(cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	got, err := w.svc.SupplierOrders(ctx, open, p)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
