//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

var purchasingNow = time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)

func (e *orderEnv) principal() shared.Principal {
	e.t.Helper()
	p, err := shared.NewBackofficePrincipal(e.user, e.tenant, shared.RoleOwner)
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *orderEnv) byID(id shared.OrderID) order.Record {
	e.t.Helper()
	var r order.Record
	e.must(e.staff(), func(ctx context.Context) error {
		var err error
		r, err = e.repo.ByID(ctx, e.tenant, id)
		return err
	})
	return r
}

func (e *orderEnv) load(id shared.OrderID) *order.Order {
	e.t.Helper()
	o, err := order.Restore(e.byID(id), e.wf)
	if err != nil {
		e.t.Fatal(err)
	}
	return o
}

func (e *orderEnv) save(o *order.Order) {
	e.t.Helper()
	e.must(e.staff(), func(ctx context.Context) error { return e.repo.Update(ctx, o) })
}

// TestOrders_PurchasingColumnsRoundTrip: the supplier's promised date and the
// price paid live in the staff-only sourcing (0016), not on order_items, and
// the pedido de compra of a promised date rides in the history's detail.
func TestOrders_PurchasingColumnsRoundTrip(t *testing.T) {
	e := newOrderEnv(t)
	o := e.insert()
	item := o.Items()[0].ID()

	// Put the item where a supplier order would have left it.
	rec := e.byID(o.ID())
	rec.Lines[0].Items[0].Status = order.ItemSentToSupplier
	sent, err := order.Restore(rec, e.wf)
	if err != nil {
		t.Fatal(err)
	}
	e.save(sent)

	o = e.load(o.ID())
	day := testkit.DateOf(purchasingNow).AddDays(9)
	if err := o.RecordPromisedDate(item, day, "C-0012", testkit.DateOf(purchasingNow), purchasingNow, e.principal()); err != nil {
		t.Fatal(err)
	}
	if err := o.RecordPaidCost(item, testkit.BRL(t, 5500)); err != nil {
		t.Fatal(err)
	}
	e.save(o)

	got := e.load(o.ID()).Items()[0]
	if d, ok := got.Promised(); !ok || d != day {
		t.Errorf("promised date after a round trip = %v, %v, want %v", d, ok, day)
	}
	if c := got.Sourcing().PaidCost; c == nil || *c != testkit.BRL(t, 5500) {
		t.Errorf("paid cost after a round trip = %v, want R$ 55,00", c)
	}
	if c := got.Sourcing().Cost; c == nil || *c != testkit.BRL(t, 6000) {
		t.Errorf("order-time cost = %v, want R$ 60,00 untouched", c)
	}
	if n := e.count(`SELECT count(*) FROM order_item_sourcing WHERE order_item_id = $1 AND promised_date = $2 AND paid_cost_cents = 5500`,
		item.String(), day.String()); n != 1 {
		t.Errorf("sourcing rows holding the promised date and paid cost = %d, want 1 (they are staff-only)", n)
	}

	var history []order.HistoryEntry
	e.must(e.staff(), func(ctx context.Context) error {
		p, err := shared.NewPageRequest("", 50)
		if err != nil {
			return err
		}
		page, err := e.repo.History(ctx, e.tenant, o.ID(), p)
		for _, row := range page.Items {
			history = append(history, row.Entry)
		}
		return err
	})
	var found bool
	for _, h := range history {
		if h.Kind == order.HistoryPromised {
			found = h.Ref == "C-0012" && h.NewDate == day && h.Item == item
		}
	}
	if !found {
		t.Errorf("history has no promised_date entry with ref C-0012 and the date; entries = %+v", history)
	}

	// A later update that touches neither keeps both.
	again := e.load(o.ID())
	e.save(again)
	if d, ok := e.load(o.ID()).Items()[0].Promised(); !ok || d != day {
		t.Errorf("promised date after another update = %v, want %v kept", d, day)
	}
}

// itemPurchase sends the order's item to a supplier through the purchasing
// repository, in the same scope the order repository writes in.
func (e *orderEnv) sendItem(o *order.Order) (*purchasing.SupplierOrder, *postgres.Purchasing) {
	e.t.Helper()
	repo := postgres.NewPurchasing()
	supplier := e.seed.Supplier(e.tenant)
	it := o.Items()[0]
	var so *purchasing.SupplierOrder
	e.must(e.staff(), func(ctx context.Context) error {
		n, err := repo.NextNumber(ctx, e.tenant)
		if err != nil {
			return err
		}
		so, err = purchasing.Send(purchasing.SendSpec{
			Tenant: e.tenant, ID: shared.SupplierOrderID(pgtest.NewID()), Number: n, Supplier: supplier,
			SupplierName: "Couros Paulista", SupplierPhone: "5511987654321", By: e.principal(), Now: purchasingNow,
			Lines: []purchasing.LineSpec{{ID: shared.PurchaseLineID(pgtest.NewID()), Item: purchasing.Item{
				Order: o.ID(), OrderItem: it.ID(), CatalogItem: it.Snapshot().CatalogItem, Name: it.Snapshot().Name, Quantity: 1,
			}}},
		})
		if err != nil {
			return err
		}
		return repo.InsertSupplierOrder(ctx, so, "Olá", "https://wa.me/5511987654321?text=Ol%C3%A1")
	})
	return so, repo
}

func (e *orderEnv) openLineItem(line shared.PurchaseLineID) (item *string, status string) {
	e.t.Helper()
	err := e.seed.DB().QueryRowContext(e.t.Context(),
		`SELECT order_item_id::text, status FROM purchase_lines WHERE id = $1`, line.String()).Scan(&item, &status)
	if err != nil {
		e.t.Fatal(err)
	}
	return item, status
}

// changeLines is what app/order's changeLines builds for a line sent again
// as the same product with another quantity: the old line's id, and the old
// item's id with its snapshot and sourcing, kept (keepIDs). app/order never
// gives a kept item another line id: keepIDs takes the line id and the item
// ids from the same old line.
func changeLines(t *testing.T, o *order.Order, quantity int) []order.LineSpec {
	t.Helper()
	var specs []order.LineSpec
	for _, l := range o.Lines() {
		spec := order.LineSpec{ID: l.ID()}
		for _, it := range l.Items() {
			snap := it.Snapshot()
			snap.Quantity = quantity
			spec.Items = append(spec.Items, order.ItemSpec{ID: it.ID(), Snapshot: snap, Sourcing: it.Sourcing()})
			price, err := shared.NewMoney(snap.UnitPrice.Cents()*int64(quantity), snap.UnitPrice.Currency())
			if err != nil {
				t.Fatal(err)
			}
			spec.Price = price
		}
		specs = append(specs, spec)
	}
	return specs
}

// TestOrders_ChangeKeepsTheItemsPurchaseLine: changing an order's lines, as
// app/order does it, keeps a kept item's row, so the purchase line that
// holds the item stays open on it and the supplier's promised date and the
// price paid, kept in the item's sourcing, are not lost (security review of
// 5.5: a delete and insert of a kept item would null the line's item and
// cascade the sourcing away).
func TestOrders_ChangeKeepsTheItemsPurchaseLine(t *testing.T) {
	e := newOrderEnv(t)
	o := e.insert()
	so, _ := e.sendItem(o)
	line := so.Lines()[0].ID()
	item := o.Items()[0].ID()
	if _, err := e.seed.DB().ExecContext(t.Context(),
		`UPDATE order_item_sourcing SET promised_date = '2026-12-20', paid_cost_cents = 5500 WHERE order_item_id = $1`, item.String()); err != nil {
		t.Fatal(err)
	}

	changed := e.load(o.ID())
	today := testkit.DateOf(purchasingNow)
	if _, err := changed.Change(order.ChangeSpec{
		Lines: changeLines(t, changed, 2), Terms: changed.Terms(), By: e.principal(), Now: purchasingNow, Today: today,
	}); err != nil {
		t.Fatalf("change: %v", err)
	}
	e.save(changed)

	if got, status := e.openLineItem(line); got == nil || *got != item.String() || status != "sent" {
		t.Errorf("the purchase line after the change: item %v, status %q, want the same item and still sent", got, status)
	}
	kept := e.load(o.ID()).Items()[0]
	if kept.ID() != item || kept.Snapshot().Quantity != 2 {
		t.Errorf("item after the change = %s x%d, want %s x2", kept.ID(), kept.Snapshot().Quantity, item)
	}
	if d, ok := kept.Promised(); !ok || d != testkit.Date(t, "2026-12-20") {
		t.Errorf("promised date after the change = %v, %v, want 2026-12-20 kept", d, ok)
	}
	if c := kept.Sourcing().PaidCost; c == nil || c.Cents() != 5500 {
		t.Errorf("paid cost after the change = %v, want R$ 55,00 kept", c)
	}
}

// TestOrders_ChangeThatRemovesTheItemKeepsItsLine: an item the change drops
// is gone, and its purchase line stays, open, with no order item, so Compras
// can say "Avisar o fornecedor".
func TestOrders_ChangeThatRemovesTheItemKeepsItsLine(t *testing.T) {
	e := newOrderEnv(t)
	o := e.insert()
	so, _ := e.sendItem(o)
	line := so.Lines()[0].ID()

	fresh := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 5, Cost: ptr(int64(7000))}).Spec(t)
	changed := e.load(o.ID())
	if _, err := changed.Change(order.ChangeSpec{
		Lines: fresh.Lines, Terms: changed.Terms(), By: e.principal(), Now: purchasingNow, Today: testkit.DateOf(purchasingNow),
	}); err != nil {
		t.Fatalf("change: %v", err)
	}
	e.save(changed)

	if got, status := e.openLineItem(line); got != nil || status != "sent" {
		t.Errorf("the purchase line after its item was dropped: item %v, status %q, want no item and still sent", got, status)
	}
	if n := e.count(`SELECT count(*) FROM order_items WHERE id = $1`, o.Items()[0].ID().String()); n != 0 {
		t.Errorf("the dropped item still has %d rows", n)
	}
}

// A store page scope writes no order: the repository refuses before the
// delete that would reach a purchase line past RLS.
func TestOrders_StorePageScopeCannotUpdateAnOrderWithPurchaseLines(t *testing.T) {
	e := newOrderEnv(t)
	o := e.insert()
	e.sendItem(o)
	changed := e.load(o.ID())
	if _, err := changed.Change(order.ChangeSpec{
		Lines: changeLines(t, changed, 3), Terms: changed.Terms(), By: e.principal(), Now: purchasingNow, Today: testkit.DateOf(purchasingNow),
	}); err != nil {
		t.Fatal(err)
	}
	err := e.run(appshared.Scope{TenantID: e.tenant}, func(ctx context.Context) error { return e.repo.Update(ctx, changed) })
	if err == nil || errors.Is(err, shared.ErrConflict) {
		t.Errorf("update from a store page scope: err = %v, want a refusal", err)
	}
}

// send is one "Enviar pedido": the order's item moves to sent_to_supplier
// and the pedido de compra is stored, in one transaction of one TxManager,
// as the use case does it. after runs last, inside the transaction.
func (e *orderEnv) send(o *order.Order, stale *order.Order, after func() error) (shared.SupplierOrderID, error) {
	e.t.Helper()
	repo := postgres.NewPurchasing()
	supplier := e.seed.Supplier(e.tenant)
	it := o.Items()[0]
	id := shared.SupplierOrderID(pgtest.NewID())
	err := e.run(e.staff(), func(ctx context.Context) error {
		n, err := repo.NextNumber(ctx, e.tenant)
		if err != nil {
			return err
		}
		so, err := purchasing.Send(purchasing.SendSpec{
			Tenant: e.tenant, ID: id, Number: n, Supplier: supplier, SupplierName: "Couros Paulista",
			SupplierPhone: "5511987654321", By: e.principal(), Now: purchasingNow,
			Lines: []purchasing.LineSpec{{ID: shared.PurchaseLineID(pgtest.NewID()), Item: purchasing.Item{
				Order: o.ID(), OrderItem: it.ID(), CatalogItem: it.Snapshot().CatalogItem, Name: it.Snapshot().Name, Quantity: 1,
			}}},
		})
		if err != nil {
			return err
		}
		if err := repo.InsertSupplierOrder(ctx, so, "Olá", "https://wa.me/5511987654321?text=Ol%C3%A1"); err != nil {
			return err
		}
		target := o
		if stale != nil {
			target = stale
		}
		move := order.PurchaseMove{Item: it.ID(), To: order.ItemSentToSupplier, Ref: n.String()}
		if err := target.MovePurchased(move, testkit.DateOf(purchasingNow), purchasingNow, e.principal()); err != nil {
			return err
		}
		if err := e.repo.Update(ctx, target); err != nil {
			return err
		}
		if after != nil {
			return after()
		}
		return nil
	})
	return id, err
}

func (e *orderEnv) supplierOrders() int {
	return e.count(`SELECT count(*) FROM supplier_orders WHERE tenant_id = $1`, e.tenant.String())
}

func (e *orderEnv) itemStatus(o *order.Order) order.ItemStatus {
	return e.load(o.ID()).Items()[0].Status()
}

// TestSend_CommitsTheLinesAndTheOrderMoveTogetherOrNeither: the supplier
// order, its lines, its number, and the order's item move and history are
// one unit of work, as the send use case runs them with the real
// repositories on one TxManager.
func TestSend_CommitsTheLinesAndTheOrderMoveTogetherOrNeither(t *testing.T) {
	boom := errors.New("the send failed after its writes")
	t.Run("both commit", func(t *testing.T) {
		e := newOrderEnv(t)
		o := e.insert()
		id, err := e.send(e.load(o.ID()), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := e.itemStatus(o); got != order.ItemSentToSupplier {
			t.Errorf("item status = %s, want sent_to_supplier", got)
		}
		if n := e.count(`SELECT count(*) FROM purchase_lines WHERE supplier_order_id = $1 AND status = 'sent'`, id.String()); n != 1 {
			t.Errorf("sent lines = %d, want 1", n)
		}
		if n := e.count(`SELECT count(*) FROM order_status_history WHERE order_id = $1 AND detail ->> 'ref' = 'C-0001'`, o.ID().String()); n == 0 {
			t.Errorf("no history entry carries the pedido de compra C-0001")
		}
	})
	t.Run("a failure after both writes undoes both", func(t *testing.T) {
		e := newOrderEnv(t)
		o := e.insert()
		if _, err := e.send(e.load(o.ID()), nil, func() error { return boom }); !errors.Is(err, boom) {
			t.Fatalf("send: err = %v, want the failure", err)
		}
		e.assertNothingSent(o)
	})
	t.Run("a lost update of the order undoes the lines", func(t *testing.T) {
		e := newOrderEnv(t)
		o := e.insert()
		stale := e.load(o.ID())
		e.save(e.load(o.ID())) // moves the order's version on
		_, err := e.send(e.load(o.ID()), stale, nil)
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("send on a stale order: err = %v, want a version conflict", err)
		}
		e.assertNothingSent(o)
	})
}

func (e *orderEnv) assertNothingSent(o *order.Order) {
	e.t.Helper()
	if n := e.supplierOrders(); n != 0 {
		e.t.Errorf("supplier orders after a failed send = %d, want 0", n)
	}
	if n := e.count(`SELECT count(*) FROM purchase_lines WHERE tenant_id = $1`, e.tenant.String()); n != 0 {
		e.t.Errorf("purchase lines after a failed send = %d, want 0", n)
	}
	if got := e.itemStatus(o); got == order.ItemSentToSupplier {
		e.t.Errorf("item status after a failed send = %s, want it unmoved", got)
	}
	if n := e.count(`SELECT count(*) FROM order_status_history WHERE order_id = $1 AND detail ->> 'ref' IS NOT NULL`, o.ID().String()); n != 0 {
		e.t.Errorf("history entries with a pedido de compra after a failed send = %d, want 0", n)
	}
	// The number goes back with the rollback.
	repo := postgres.NewPurchasing()
	e.must(e.staff(), func(ctx context.Context) error {
		if n, err := repo.PeekNumber(ctx, e.tenant); err != nil || n != 1 {
			e.t.Errorf("next supplier order number after a failed send = %v, %v, want 1", n, err)
		}
		return nil
	})
}
