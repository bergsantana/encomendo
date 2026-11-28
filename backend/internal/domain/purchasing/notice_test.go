package purchasing_test

import (
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestNoticeOf_EachReason(t *testing.T) {
	t.Parallel()
	black, big := choice("Cor", "Preto"), choice("Tamanho", "G")
	it := item("Banco", 2, black, big)
	same := purchasing.OrderSide{Quantity: 2, Choices: []purchasing.Choice{big, black}} // another order of the same set

	tests := []struct {
		name string
		side purchasing.OrderSide
		want purchasing.NoticeReason
		ok   bool
	}{
		{"nothing changed", same, "", false},
		{"order cancelled", purchasing.OrderSide{OrderCancelled: true, Quantity: 2, Choices: same.Choices}, purchasing.NoticeOrderCancelled, true},
		{"cancelled wins over removed", purchasing.OrderSide{OrderCancelled: true, ItemRemoved: true}, purchasing.NoticeOrderCancelled, true},
		{"item removed", purchasing.OrderSide{ItemRemoved: true}, purchasing.NoticeItemRemoved, true},
		{"quantity changed", purchasing.OrderSide{Quantity: 3, Choices: same.Choices}, purchasing.NoticeItemChanged, true},
		{"a choice swapped", purchasing.OrderSide{Quantity: 2, Choices: []purchasing.Choice{black, choice("Tamanho", "M")}}, purchasing.NoticeItemChanged, true},
		{"a choice dropped", purchasing.OrderSide{Quantity: 2, Choices: []purchasing.Choice{black}}, purchasing.NoticeItemChanged, true},
		{"a choice added", purchasing.OrderSide{Quantity: 2, Choices: append([]purchasing.Choice{choice("Extra", "Sim")}, same.Choices...)}, purchasing.NoticeItemChanged, true},
	}
	for _, tc := range tests {
		for _, status := range []string{"sent", "in_transit"} {
			o := sent(t, it)
			if status == "in_transit" {
				if _, err := o.MarkInTransit(day1); err != nil {
					t.Fatal(err)
				}
			}
			got, ok := purchasing.NoticeOf(o.Lines()[0], tc.side)
			if got != tc.want || ok != tc.ok {
				t.Errorf("%s (%s): NoticeOf = %q, %v; want %q, %v", tc.name, status, got, ok, tc.want, tc.ok)
			}
		}
	}
}

func TestNoticeOf_OnlyOpenLines(t *testing.T) {
	t.Parallel()
	cancelled := purchasing.OrderSide{OrderCancelled: true}
	o := sent(t, item("A", 1), item("B", 1), item("C", 1))
	ids := []shared.PurchaseLineID{o.Lines()[0].ID(), o.Lines()[1].ID(), o.Lines()[2].ID()}
	if _, err := o.Receive(ids[0], purchasing.CheckOK, "", date(t, "2026-12-05")); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Receive(ids[1], purchasing.CheckDefective, "x", date(t, "2026-12-05")); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Resolve(ids[2]); err != nil {
		t.Fatal(err)
	}
	for i, l := range o.Lines() {
		if got, ok := purchasing.NoticeOf(l, cancelled); ok {
			t.Errorf("closed line %d (%s) is a notice: %q", i, l.Status(), got)
		}
	}
}

func TestNoticeOf_ItemGoneFromTheLine(t *testing.T) {
	t.Parallel()
	// A removed order item is stored with a zero order item, so the line is
	// a notice even when the order side says nothing of it.
	r := sent(t, item("A", 1)).Record()
	r.Lines[0].Item.OrderItem = shared.OrderItemID{}
	o, err := purchasing.Restore(r)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := purchasing.NoticeOf(o.Lines()[0], purchasing.OrderSide{Quantity: 1}); !ok || got != purchasing.NoticeItemRemoved {
		t.Errorf("NoticeOf = %q, %v; want item_removed", got, ok)
	}
}
