package order_test

import (
	"context"
	"errors"
	"testing"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

func TestMovePurchased_PermissionPerMove(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, nil)
	ok := order.CheckOK
	receive := []apporder.PurchaseMoveInput{{Order: v.ID, Move: order.PurchaseMove{Item: v.Items[0].ID, To: order.ItemReceived, Check: &ok}}}
	send := []apporder.PurchaseMoveInput{{Order: v.ID, Move: order.PurchaseMove{Item: v.Items[0].ID, To: order.ItemSentToSupplier, Ref: "C-0001"}}}
	if err := h.svc.MovePurchased(h.viewer(t), receive); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("viewer receives: err = %v, want ErrForbidden", err)
	}
	if err := h.svc.MovePurchased(h.staff(t), send); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("staff sends: err = %v, want ErrForbidden", err)
	}
	if err := h.svc.MovePurchased(h.admin(t), send); err != nil {
		t.Fatalf("admin sends: %v", err)
	}
	if err := h.svc.MovePurchased(h.staff(t), receive); err != nil {
		t.Fatalf("staff receives: %v", err)
	}
	got, err := h.svc.Get(h.staff(t), v.ID)
	if err != nil || got.Items[0].Status != order.ItemReceived {
		t.Fatalf("item = %+v, %v; want received", got.Items, err)
	}
	if names := h.events.Names(); names[len(names)-1] != "order.item_moved" {
		t.Errorf("events = %v, want the move published", names)
	}
}

func TestMovePurchased_ARefusalLeavesEveryOrderUntouched(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.released(h.staff(t), t, nil)
	b := h.released(h.staff(t), t, nil)
	moves := []apporder.PurchaseMoveInput{
		{Order: a.ID, Move: order.PurchaseMove{Item: a.Items[0].ID, To: order.ItemSentToSupplier, Ref: "C-0001"}},
		{Order: b.ID, Move: order.PurchaseMove{Item: b.Items[0].ID, To: order.ItemInTransit, Ref: "C-0001"}}, // not sent yet
	}
	if err := h.svc.MovePurchased(h.admin(t), moves); !errors.Is(err, order.ErrInvalidItemMove) {
		t.Fatalf("err = %v, want ErrInvalidItemMove", err)
	}
	got, err := h.svc.Get(h.staff(t), a.ID)
	if err != nil || got.Items[0].Status != order.ItemToBuy || got.Version != a.Version {
		t.Errorf("order a = %s v%d, %v; want untouched at to_buy v%d", got.Items[0].Status, got.Version, err, a.Version)
	}
}

func TestMovePurchased_StaffReceivesADefect(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, nil)
	bad := order.CheckDefective
	send := []apporder.PurchaseMoveInput{{Order: v.ID, Move: order.PurchaseMove{Item: v.Items[0].ID, To: order.ItemSentToSupplier, Ref: "C-0001"}}}
	if err := h.svc.MovePurchased(h.admin(t), send); err != nil {
		t.Fatal(err)
	}
	// Keyed on the move asked for, not where it lands: a defect lands on
	// to_buy but is still a receipt, staff's to make.
	defect := []apporder.PurchaseMoveInput{{Order: v.ID, Move: order.PurchaseMove{Item: v.Items[0].ID, To: order.ItemReceived, Check: &bad, Note: "Rasgado"}}}
	if err := h.svc.MovePurchased(h.staff(t), defect); err != nil {
		t.Fatalf("staff defect: %v", err)
	}
	got, err := h.svc.Get(h.staff(t), v.ID)
	if err != nil || got.Items[0].Status != order.ItemToBuy {
		t.Fatalf("item = %+v, %v; want back to to_buy", got.Items, err)
	}
}

func TestMovePurchased_TwoMovesOnOneOrder(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, func(in *apporder.CreateInput) {
		in.Lines = append(in.Lines, in.Lines[0])
	})
	if len(v.Items) < 2 {
		t.Fatalf("order has %d items, want 2", len(v.Items))
	}
	moves := []apporder.PurchaseMoveInput{
		{Order: v.ID, Move: order.PurchaseMove{Item: v.Items[0].ID, To: order.ItemSentToSupplier, Ref: "C-0001"}},
		{Order: v.ID, Move: order.PurchaseMove{Item: v.Items[1].ID, To: order.ItemSentToSupplier, Ref: "C-0001"}},
	}
	if err := h.svc.MovePurchased(h.admin(t), moves); err != nil {
		t.Fatal(err)
	}
	got, err := h.svc.Get(h.staff(t), v.ID)
	if err != nil || got.Items[0].Status != order.ItemSentToSupplier || got.Items[1].Status != order.ItemSentToSupplier {
		t.Fatalf("items = %+v, %v", got.Items, err)
	}
	if got.Version != v.Version+1 {
		t.Errorf("version = %d, want one save for both moves (%d)", got.Version, v.Version+1)
	}
}

func TestMovePurchased_VersionConflictFailsTheCall(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, nil)
	h.repo.ConcurrentWriteBeforeNextUpdate()
	send := []apporder.PurchaseMoveInput{{Order: v.ID, Move: order.PurchaseMove{Item: v.Items[0].ID, To: order.ItemSentToSupplier, Ref: "C-0001"}}}
	if err := h.svc.MovePurchased(h.admin(t), send); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("err = %v, want ErrVersionConflict", err)
	}
	got, err := h.svc.Get(h.staff(t), v.ID)
	if err != nil || got.Items[0].Status != order.ItemToBuy {
		t.Errorf("item = %+v, %v; want untouched", got.Items, err)
	}
}

// toBuyItems returns the ids of the items in "A comprar" for ctx's caller.
func (h *harness) toBuyItems(ctx context.Context, t *testing.T) []apporder.BoughtItemView {
	t.Helper()
	v, err := h.svc.ToBuy(ctx)
	if err != nil {
		t.Fatalf("ToBuy: %v", err)
	}
	return v.Items
}

func TestToBuy_AfterDepositNotBefore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, func(in *apporder.CreateInput) { in.SendToCustomer = true }).Order
	if got := h.toBuyItems(h.admin(t), t); len(got) != 0 {
		t.Fatalf("before the deposit: %d items to buy, want none", len(got))
	}
	h.pay(h.staff(t), t, v, order.PaymentDeposit, v.DepositDue.Cents())
	got := h.toBuyItems(h.admin(t), t)
	if len(got) != 1 || got[0].Item != v.Items[0].ID || got[0].Order != v.ID || got[0].Name != "Banco de couro" ||
		got[0].SupplierName != "Couros Paulista" || got[0].Quantity != 1 || got[0].ArrivalLimit.IsZero() || len(got[0].Options) != 0 {
		t.Errorf("after the deposit = %+v, want the bought item with its supplier and arrival limit", got)
	}
	if _, err := h.svc.ToBuy(h.viewer(t)); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("viewer: err = %v, want ErrForbidden", err)
	}
	if got := h.toBuyItems(h.staff(t), t); len(got) != 1 {
		t.Errorf("staff (who receive at the counter) see %d items, want 1", len(got))
	}
}

func TestToBuy_BeforeDepositSettingPastDraft(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.facts.SetPurchaseBeforeDeposit(true)
	draft := h.create(h.staff(t), t, nil).Order
	if got := h.toBuyItems(h.admin(t), t); len(got) != 0 {
		t.Fatalf("a draft shows %d items to buy, want none", len(got))
	}
	if _, err := h.svc.Advance(h.staff(t), apporder.AdvanceInput{Order: draft.ID, To: "awaiting_deposit", Version: draft.Version}); err != nil {
		t.Fatal(err)
	}
	if got := h.toBuyItems(h.admin(t), t); len(got) != 1 {
		t.Errorf("past draft with the setting on: %d items, want 1", len(got))
	}
	h.facts.SetPurchaseBeforeDeposit(false)
	if got := h.toBuyItems(h.admin(t), t); len(got) != 0 {
		t.Errorf("setting off again, no deposit: %d items, want none", len(got))
	}
}

func TestToBuy_DepositPaidAgainShowsOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, nil)
	v, err := h.svc.VoidPayment(h.owner(t), apporder.VoidPaymentInput{Order: v.ID, Payment: v.Payments[0].ID, Reason: "Pix devolvido", Version: v.Version})
	if err != nil {
		t.Fatal(err)
	}
	h.pay(h.staff(t), t, v, order.PaymentDeposit, v.DepositDue.Cents())
	if got := h.toBuyItems(h.admin(t), t); len(got) != 1 {
		t.Errorf("after a void and a new deposit: %d items, want the item once", len(got))
	}
}

func TestToBuy_LeavesOutCancelledRemovedInHouse(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	kit := h.kit(t)
	withMade := h.released(h.staff(t), t, func(in *apporder.CreateInput) { in.Lines = []apporder.LineRequest{{Kit: kit}} })
	cancelled := h.released(h.staff(t), t, nil)
	if _, err := h.svc.Cancel(h.admin(t), apporder.CancelInput{Order: cancelled.ID, Reason: "Desistiu", Version: cancelled.Version}); err != nil {
		t.Fatal(err)
	}
	changed := h.released(h.staff(t), t, nil)
	if _, err := h.svc.Change(h.admin(t), apporder.ChangeInput{Order: changed.ID, Version: changed.Version, CustomerApproved: true,
		Lines: []apporder.LineRequest{{Kit: kit}}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}}); err != nil {
		t.Fatal(err)
	}
	got := h.toBuyItems(h.admin(t), t)
	byOrder := map[shared.OrderID][]string{}
	for _, it := range got {
		byOrder[it.Order] = append(byOrder[it.Order], it.Name)
	}
	if n := len(byOrder[cancelled.ID]); n != 0 {
		t.Errorf("a cancelled order shows %d items", n)
	}
	for _, id := range []shared.OrderID{withMade.ID, changed.ID} {
		if names := byOrder[id]; len(names) != 1 || names[0] != "Banco de couro" {
			t.Errorf("order %s shows %v, want only its bought item (no in-house Guidão, no removed loose item)", id, names)
		}
	}
	for _, it := range got {
		if it.Order == changed.ID && it.Item == changed.Items[0].ID {
			t.Errorf("the loose item removed by the change is still to buy")
		}
	}
}

func TestToBuy_BackupSwitchShowsBackupSupplier(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, nil)
	if _, err := h.svc.UseBackupSupplier(h.admin(t), apporder.ItemVersionInput{Order: v.ID, Item: v.Items[0].ID, Version: v.Version}); err != nil {
		t.Fatal(err)
	}
	if got := h.toBuyItems(h.admin(t), t); len(got) != 1 || got[0].SupplierName != "Couros Rio" {
		t.Errorf("after the switch = %+v, want the backup supplier", got)
	}
}

func TestToBuy_CostOnlyForOwner(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.released(h.staff(t), t, nil)
	if got := h.toBuyItems(h.owner(t), t); len(got) != 1 || got[0].Cost == nil || got[0].Cost.Cents() != 40_000 {
		t.Errorf("owner = %+v, want the order-time cost", got)
	}
	for name, ctx := range map[string]context.Context{"admin": h.admin(t), "staff": h.staff(t)} {
		if got := h.toBuyItems(ctx, t); len(got) != 1 || got[0].Cost != nil {
			t.Errorf("%s = %+v, want no cost", name, got)
		}
	}
}

func TestRecordPromisedDates_RedatesKeepsPromise(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, nil)
	sent := []apporder.PurchaseMoveInput{{Order: v.ID, Move: order.PurchaseMove{Item: v.Items[0].ID, To: order.ItemSentToSupplier, Ref: "C-0001"}}}
	if err := h.svc.MovePurchased(h.admin(t), sent); err != nil {
		t.Fatal(err)
	}
	day := testkit.Date(t, "2026-12-30")
	in := []apporder.PromisedDateInput{{Order: v.ID, Item: v.Items[0].ID, Date: day, Ref: "C-0001"}}
	if err := h.svc.RecordPromisedDates(h.staff(t), in); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("staff: err = %v, want ErrForbidden", err)
	}
	if err := h.svc.RecordPromisedDates(h.admin(t), in); err != nil {
		t.Fatal(err)
	}
	got, err := h.svc.PurchaseOrders(h.staff(t), []shared.OrderID{v.ID})
	if err != nil || len(got) != 1 {
		t.Fatalf("PurchaseOrders = %+v, %v", got, err)
	}
	it := got[0].Items[0]
	if it.Promised != day || it.Expected != day || len(it.Alerts) == 0 {
		t.Errorf("item = %+v, want promised and expected on %s, with an alert past the limit", it, day)
	}
	after, err := h.svc.Get(h.staff(t), v.ID)
	if err != nil || after.DeliveryDate != v.DeliveryDate {
		t.Errorf("delivery date = %s, want %s kept (%v)", after.DeliveryDate, v.DeliveryDate, err)
	}
}

func TestRecordPaidCosts_OwnerOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, nil)
	in := []apporder.PaidCostInput{{Order: v.ID, Item: v.Items[0].ID, Cost: brl(t, 41_000)}}
	if err := h.svc.RecordPaidCosts(h.admin(t), in); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("admin: err = %v, want ErrForbidden", err)
	}
	if err := h.svc.RecordPaidCosts(h.owner(t), in); err != nil {
		t.Fatal(err)
	}
	stored, _ := h.repo.Stored(h.store, v.ID)
	if c := stored.Lines[0].Items[0].Sourcing.PaidCost; c == nil || c.Cents() != 41_000 {
		t.Errorf("stored paid cost = %v, want R$ 410,00", c)
	}
}

func TestPurchaseOrders_RemovedItemAbsent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	kit := h.kit(t)
	v := h.released(h.staff(t), t, nil)
	if _, err := h.svc.Change(h.admin(t), apporder.ChangeInput{Order: v.ID, Version: v.Version, CustomerApproved: true,
		Lines: []apporder.LineRequest{{Kit: kit}}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}}); err != nil {
		t.Fatal(err)
	}
	got, err := h.svc.PurchaseOrders(h.viewer(t), []shared.OrderID{v.ID, v.ID})
	if err != nil || len(got) != 1 {
		t.Fatalf("PurchaseOrders = %d orders, %v; want one", len(got), err)
	}
	for _, it := range got[0].Items {
		if it.Item == v.Items[0].ID {
			t.Errorf("the removed item is still on the order")
		}
	}
	bought := 0
	for _, it := range got[0].Items {
		if it.Bought {
			bought++
		}
	}
	if bought != 1 || got[0].Cancelled || got[0].Closed {
		t.Errorf("order = %+v, want one bought item, open", got[0])
	}
}
