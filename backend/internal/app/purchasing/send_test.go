package purchasing_test

import (
	"errors"
	"strings"
	"testing"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestSend_WritesLinesMovesItemsRecordsLink(t *testing.T) {
	w := newWorld(t)
	o1, items1 := w.order(231, bought{name: "Banco de couro", qty: 1, choices: []purchasing.Choice{preto}}, bought{name: "Guidão", qty: 1})
	o2, items2 := w.order(232, bought{name: "Banco de couro", qty: 2, choices: []purchasing.Choice{preto}})

	v := w.send(w.admin, items1[0], items1[1], items2[0])

	so := v.SupplierOrder
	if so.Number != 1 || so.State != purchasing.StateSent || so.Total != 3 || so.Received != 0 {
		t.Fatalf("supplier order = %+v", so.SupplierOrderSummary)
	}
	if so.SentBy.ID != w.admin.UserID() || so.SentBy.Name != "Seu Jorge" || so.Version != 1 || !so.CanUndo {
		t.Fatalf("supplier order = %+v", so)
	}
	if len(so.Lines) != 3 || so.Lines[0].Status != purchasing.LineSent || so.Lines[0].OrderNumber != "P-0231" {
		t.Fatalf("lines = %+v", so.Lines)
	}
	for _, c := range []struct {
		order shared.OrderID
		item  shared.OrderItemID
	}{{o1, items1[0]}, {o1, items1[1]}, {o2, items2[0]}} {
		if got := w.itemStatus(c.order, c.item); got != order.ItemSentToSupplier {
			t.Fatalf("item status = %s, want sent_to_supplier", got)
		}
	}
	wantText := "Olá, Couros Paulista! Aqui é da Alta RPM.\nPedido de compra C-0001:\n• 3× Banco de couro (Cor: Preto)\n• 1× Guidão\nPode me confirmar a data de entrega?"
	if v.Text != wantText {
		t.Fatalf("text = %q, want %q", v.Text, wantText)
	}
	if !strings.HasPrefix(v.WhatsAppURL, "https://wa.me/"+supplierPhone+"?text=") || so.WhatsAppURL != "https://wa.me/"+supplierPhone {
		t.Fatalf("urls = %q, %q", v.WhatsAppURL, so.WhatsAppURL)
	}
	stored, ok := w.repo.Stored(w.tenant, so.ID)
	if !ok || stored.Number != 1 || len(stored.Lines) != 3 {
		t.Fatalf("stored = %+v, %v", stored, ok)
	}
	msg := storedMessage(t, w, so.ID)
	if msg.Text != v.Text || msg.URL != v.WhatsAppURL {
		t.Fatalf("stored message = %+v", msg)
	}
	// "A comprar" is empty now, and the next number is 2.
	tb, err := w.svc.ToBuy(w.ctx(w.owner))
	if err != nil || len(tb.Suppliers) != 0 {
		t.Fatalf("to buy = %+v, %v", tb, err)
	}
	if n, err := w.peek(w.owner); err != nil || n != 2 {
		t.Fatalf("next number = %v, %v; want 2", n, err)
	}
}

func TestSend_Refusals(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	otherSupplier := apppurchasing.SupplierContact{ID: shared.SupplierID(w.ids.NewID()), Name: "Outro", Phone: supplierPhone}
	w.suppliers.Add(otherSupplier)
	_, foreign := w.orderOf(otherSupplier, 232, bought{name: "Pedal", qty: 1})
	pick := func(sup shared.SupplierID, ids ...shared.OrderItemID) apppurchasing.SendInput {
		return apppurchasing.SendInput{PickInput: apppurchasing.PickInput{Supplier: sup, OrderItems: ids}, Idempotency: w.key()}
	}
	for _, c := range []struct {
		name string
		in   apppurchasing.SendInput
		want error
	}{
		{"no key", apppurchasing.SendInput{PickInput: pick(w.supplier.ID, items[0]).PickInput}, apppurchasing.ErrIdempotencyKeyRequired},
		{"empty pick", pick(w.supplier.ID), purchasing.ErrEmptyPick},
		{"item of another supplier", pick(w.supplier.ID, items[0], foreign[0]), apppurchasing.ErrItemNotToBuy},
		{"unknown item", pick(w.supplier.ID, shared.OrderItemID(w.ids.NewID())), apppurchasing.ErrItemNotToBuy},
		{"item twice", pick(w.supplier.ID, items[0], items[0]), purchasing.ErrItemPickedTwice},
		{"unknown supplier", pick(shared.SupplierID(w.ids.NewID()), items[0]), apppurchasing.ErrItemNotToBuy},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := w.svc.Send(w.ctx(w.owner), c.in); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestSend_ItemSentByAnotherIsNotToBuy(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	w.send(w.owner, items[0])

	_, err := w.svc.Send(w.ctx(w.admin), apppurchasing.SendInput{
		PickInput: apppurchasing.PickInput{Supplier: w.supplier.ID, OrderItems: items}, Idempotency: w.key(),
	})
	if !errors.Is(err, apppurchasing.ErrItemNotToBuy) || !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("err = %v, want ErrItemNotToBuy (a conflict)", err)
	}
}

// A race the read does not see: the repository's unique open-line index
// refuses the second insert, and the app says the item is not to buy.
func TestSend_RaceOnTheOpenLineIsNotToBuy(t *testing.T) {
	w := newWorld(t)
	orderID, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	// Another request holds an open line for the item, but the order
	// context still shows it to buy (its commit is not visible yet).
	w.sendThenRewind(orderID, items[0])

	_, err := w.svc.Send(w.ctx(w.admin), apppurchasing.SendInput{
		PickInput: apppurchasing.PickInput{Supplier: w.supplier.ID, OrderItems: items}, Idempotency: w.key(),
	})
	if !errors.Is(err, apppurchasing.ErrItemNotToBuy) {
		t.Fatalf("err = %v, want ErrItemNotToBuy", err)
	}
	// Nothing of the refused send stayed: the number was given back.
	n, err := w.peek(w.owner)
	if err != nil || n != 2 {
		t.Fatalf("next number = %v, %v; want C-0002", n, err)
	}
}

func TestSend_ReplayAnswersSameSupplierOrder(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	in := apppurchasing.SendInput{PickInput: apppurchasing.PickInput{Supplier: w.supplier.ID, OrderItems: items}, Idempotency: w.key()}

	first, err := w.svc.Send(w.ctx(w.owner), in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := w.svc.Send(w.ctx(w.owner), in)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if again.SupplierOrder.ID != first.SupplierOrder.ID || again.Text != first.Text || again.WhatsAppURL != first.WhatsAppURL {
		t.Fatalf("replay = %+v, first = %+v", again, first)
	}
	if n, _ := w.peek(w.owner); n != 2 {
		t.Fatalf("next number = %v, want 2: the replay took none", n)
	}
	// The same key for another request is refused.
	other := in
	other.Idempotency.RequestHash = [32]byte{99}
	if _, err := w.svc.Send(w.ctx(w.owner), other); !errors.Is(err, appshared.ErrIdempotencyKeyReused) {
		t.Fatalf("err = %v, want ErrIdempotencyKeyReused", err)
	}
}

func TestSend_ReplayOfAKeyThatMadeSomethingElse(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	k := w.key()
	// A key a receipt remembered, or one whose resource is gone.
	for _, resource := range []string{"not-an-id", w.ids.NewID().String()} {
		remember(t, w, k, resource)
		_, err := w.svc.Send(w.ctx(w.owner), apppurchasing.SendInput{
			PickInput: apppurchasing.PickInput{Supplier: w.supplier.ID, OrderItems: items}, Idempotency: k,
		})
		if !errors.Is(err, appshared.ErrIdempotencyKeyReused) {
			t.Fatalf("resource %q: err = %v, want ErrIdempotencyKeyReused", resource, err)
		}
		k = w.key()
	}
}

func TestSend_AdminSendKeepsOwnerCost(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1, cost: 40_000})

	sent := w.send(w.admin, items[0])
	if got := sent.SupplierOrder.Lines[0].Cost; got != nil {
		t.Fatalf("the admin's answer carries a cost: %v", got)
	}
	// The owner finds the cost the admin could not see.
	v, err := w.svc.SupplierOrder(w.ctx(w.owner), sent.SupplierOrder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c := v.Lines[0].Cost; c == nil || c.Cents() != 40_000 {
		t.Fatalf("owner's cost = %v, want 40000", c)
	}
}

func TestSend_MessageHasNoClientData(t *testing.T) {
	w := newWorld(t)
	orderID, items := w.order(231, bought{name: "Banco de couro", qty: 2, choices: []purchasing.Choice{preto}})
	other, items2 := w.order(232, bought{name: "Banco de couro", qty: 1, choices: []purchasing.Choice{preto}})
	customer := w.customers[orderID]
	numbers := []string{w.facts(orderID).Number, w.facts(other).Number, "P-0", orderID.String(), other.String()}
	forbidden := append(customer.fields(), numbers...)

	preview, err := w.svc.Preview(w.ctx(w.owner), apppurchasing.PickInput{Supplier: w.supplier.ID, OrderItems: []shared.OrderItemID{items[0], items2[0]}})
	if err != nil {
		t.Fatal(err)
	}
	sent := w.send(w.owner, items[0], items2[0])
	// An order is cancelled and one of its items changes: both flag a notice.
	w.items.Cancel(orderID)
	notices, err := w.svc.Notices(w.ctx(w.owner))
	if err != nil || len(notices) != 1 {
		t.Fatalf("notices = %+v, %v", notices, err)
	}
	// The Falar com o fornecedor text, before and after sending.
	_, loose := w.order(233, bought{name: "Pedal", qty: 1})
	lines, err := w.svc.OrderLines(w.ctx(w.owner), w.itemOrder(loose[0]))
	if err != nil || len(lines) != 1 {
		t.Fatalf("order lines = %+v, %v", lines, err)
	}
	talkAfter, err := w.svc.OrderLines(w.ctx(w.owner), other)
	if err != nil || len(talkAfter) != 1 {
		t.Fatalf("order lines after send = %+v, %v", talkAfter, err)
	}
	texts := map[string]string{
		"preview": preview, "sent text": sent.Text, "sent link": sent.WhatsAppURL, "stored text": storedMessage(t, w, sent.SupplierOrder.ID).Text,
		"stored url": storedMessage(t, w, sent.SupplierOrder.ID).URL, "notice": notices[0].WhatsAppURL,
		"falar before": lines[0].ChatURL, "falar after": talkAfter[0].ChatURL,
	}
	for what, text := range texts {
		if text == "" {
			t.Fatalf("%s is empty", what)
		}
		for _, bad := range forbidden {
			if strings.Contains(text, bad) {
				t.Errorf("%s carries %q: %s", what, bad, text)
			}
		}
	}
}
