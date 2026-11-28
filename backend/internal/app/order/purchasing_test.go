package order_test

import (
	"errors"
	"testing"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
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
