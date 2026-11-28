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

func TestMovePurchased_JoinsOuterTxAndRollsBack(t *testing.T) {
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
