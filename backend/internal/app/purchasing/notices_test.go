package purchasing_test

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestNotices_EachReason(t *testing.T) {
	w := newWorld(t)
	oCancelled, iCancelled := w.order(231, bought{name: "Banco de couro", qty: 1})
	oRemoved, iRemoved := w.order(232, bought{name: "Guidão", qty: 1})
	oChanged, iChanged := w.order(233, bought{name: "Pedal", qty: 2})
	_, iFine := w.order(234, bought{name: "Retrovisor", qty: 1})
	sent := w.send(w.owner, iCancelled[0], iRemoved[0], iChanged[0], iFine[0]).SupplierOrder
	if got, err := w.svc.Notices(w.ctx(w.owner)); err != nil || len(got) != 0 {
		t.Fatalf("nothing changed yet: %+v, %v", got, err)
	}

	w.items.Cancel(oCancelled)
	w.items.RemoveItem(oRemoved, iRemoved[0])
	w.items.SetQuantity(oChanged, iChanged[0], 3)

	got, err := w.svc.Notices(w.ctx(w.admin))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		order  shared.OrderID
		reason purchasing.NoticeReason
		name   string
		qty    int
		number string
	}{
		{oCancelled, purchasing.NoticeOrderCancelled, "Banco de couro", 1, "P-0231"},
		{oRemoved, purchasing.NoticeItemRemoved, "Guidão", 1, "P-0232"},
		{oChanged, purchasing.NoticeItemChanged, "Pedal", 2, "P-0233"},
	}
	if len(got) != len(want) {
		t.Fatalf("notices = %+v", got)
	}
	for i, n := range got {
		wn := want[i]
		if n.Order != wn.order || n.Reason != wn.reason || n.Name != wn.name || n.Quantity != wn.qty || n.OrderNumber != wn.number ||
			n.SupplierOrder != sent.ID || n.Number != 1 || n.SupplierName != w.supplier.Name || n.Line != sent.Lines[i].ID {
			t.Errorf("notice %d = %+v, want %+v", i, n, wn)
		}
		u, err := url.Parse(n.WhatsAppURL)
		if err != nil || u.Host != "wa.me" || !strings.Contains(u.Query().Get("text"), fmt.Sprintf("não vou mais precisar de %d× %s", wn.qty, wn.name)) {
			t.Errorf("notice %d link = %q", i, n.WhatsAppURL)
		}
	}
}

func TestResolveNotice_ChangedItemBackToBuy(t *testing.T) {
	w := newWorld(t)
	oChanged, iChanged := w.order(233, bought{name: "Pedal", qty: 2})
	oCancelled, iCancelled := w.order(231, bought{name: "Banco de couro", qty: 1})
	sent := w.send(w.owner, iChanged[0], iCancelled[0]).SupplierOrder
	w.items.SetQuantity(oChanged, iChanged[0], 3)
	w.items.Cancel(oCancelled)
	ctx := w.ctx(w.admin)

	// The changed item is bought again, as it is now.
	if err := w.svc.ResolveNotice(ctx, sent.Lines[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := w.itemStatus(oChanged, iChanged[0]); got != order.ItemToBuy {
		t.Fatalf("changed item = %s, want to_buy", got)
	}
	tb, err := w.svc.ToBuy(ctx)
	if err != nil || len(tb.Suppliers) != 1 || tb.Suppliers[0].Items[0].Quantity != 3 {
		t.Fatalf("to buy = %+v, %v", tb, err)
	}
	// A cancelled order's item is only cancelled on the line.
	if err := w.svc.ResolveNotice(ctx, sent.Lines[1].ID); err != nil {
		t.Fatal(err)
	}
	if got := w.itemStatus(oCancelled, iCancelled[0]); got != order.ItemSentToSupplier {
		t.Fatalf("cancelled order's item = %s, want it left alone", got)
	}
	v, err := w.svc.SupplierOrder(ctx, sent.ID)
	if err != nil || v.Lines[0].Status != purchasing.LineCancelled || v.Lines[1].Status != purchasing.LineCancelled {
		t.Fatalf("lines = %+v, %v", v.Lines, err)
	}
	if got, err := w.svc.Notices(ctx); err != nil || len(got) != 0 {
		t.Fatalf("notices after resolving = %+v, %v", got, err)
	}
	// Resolved already.
	if err := w.svc.ResolveNotice(ctx, sent.Lines[0].ID); !errors.Is(err, purchasing.ErrLineClosed) {
		t.Fatalf("again: %v, want ErrLineClosed", err)
	}
}

func TestResolveNotice_Refusals(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	sent := w.send(w.owner, items[0]).SupplierOrder
	ctx := w.ctx(w.owner)
	// Still needed: not a notice.
	if err := w.svc.ResolveNotice(ctx, sent.Lines[0].ID); !errors.Is(err, purchasing.ErrLineClosed) {
		t.Fatalf("a line still needed: %v", err)
	}
	if err := w.svc.ResolveNotice(ctx, shared.PurchaseLineID(w.ids.NewID())); !errors.Is(err, purchasing.ErrLineNotFound) {
		t.Fatalf("unknown line: %v", err)
	}
}

func TestOrderLines_ChatLinkOnlyForPurchaseWrite(t *testing.T) {
	w := newWorld(t)
	orderID, items := w.order(231, bought{name: "Banco de couro", qty: 1, choices: []purchasing.Choice{preto}}, bought{name: "Guidão", qty: 1})
	sent := w.send(w.owner, items[0]).SupplierOrder
	if _, err := w.svc.RecordPromisedDate(w.ctx(w.owner), apppurchasing.PromisedDateInput{ID: sent.ID, Version: 1, Date: w.today.AddDays(9)}); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		who  shared.Principal
		chat bool
	}{{w.owner, true}, {w.admin, true}, {w.staff, false}, {w.viewer, false}} {
		lines, err := w.svc.OrderLines(w.ctx(c.who), orderID)
		if err != nil || len(lines) != 2 {
			t.Fatalf("%s: lines = %+v, %v", c.who.Role(), lines, err)
		}
		sentLine, toBuy := lines[0], lines[1]
		if sentLine.Status != apppurchasing.OrderLineStatus(purchasing.LineSent) || sentLine.Number != 1 || sentLine.SupplierOrder != sent.ID ||
			sentLine.Line != sent.Lines[0].ID || sentLine.Promised != w.today.AddDays(9) || sentLine.SupplierName != w.supplier.Name {
			t.Errorf("%s: sent line = %+v", c.who.Role(), sentLine)
		}
		if toBuy.Status != apppurchasing.OrderLineToBuy || toBuy.OrderItem != items[1] || !toBuy.Line.IsZero() {
			t.Errorf("%s: item to buy = %+v", c.who.Role(), toBuy)
		}
		if (sentLine.ChatURL != "") != c.chat || (toBuy.ChatURL != "") != c.chat {
			t.Errorf("%s: chat links = %q, %q; want links: %v", c.who.Role(), sentLine.ChatURL, toBuy.ChatURL, c.chat)
		}
		if !c.chat {
			continue
		}
		text := func(raw string) string {
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			return u.Query().Get("text")
		}
		if got := text(sentLine.ChatURL); !strings.Contains(got, "Sobre o pedido de compra C-0001: 1× Banco de couro (Cor: Preto)") {
			t.Errorf("sent line text = %q", got)
		}
		if got := text(toBuy.ChatURL); !strings.Contains(got, "Sobre 1× Guidão: consegue me dizer o prazo de entrega?") {
			t.Errorf("to-buy text = %q", got)
		}
	}
	if _, err := w.svc.OrderLines(w.ctx(w.owner), shared.OrderID(w.ids.NewID())); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("unknown order: %v", err)
	}
}

func TestOrderLines_LineDateBeatsTheOrdersDate(t *testing.T) {
	w := newWorld(t)
	orderID, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	sent := w.send(w.owner, items[0]).SupplierOrder
	own := w.today.AddDays(5)
	if _, err := w.svc.RecordPromisedDate(w.ctx(w.owner), apppurchasing.PromisedDateInput{
		ID: sent.ID, Version: 1, Date: w.today.AddDays(9), Lines: []apppurchasing.LineDate{{Line: sent.Lines[0].ID, Date: own}},
	}); err != nil {
		t.Fatal(err)
	}
	lines, err := w.svc.OrderLines(w.ctx(w.staff), orderID)
	if err != nil || len(lines) != 1 || lines[0].Promised != own {
		t.Fatalf("lines = %+v, %v", lines, err)
	}
}

func TestOrderLines_UndoneAndInHouseItemsAndLatestLine(t *testing.T) {
	w := newWorld(t)
	orderID, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	sent := w.send(w.owner, items[0]).SupplierOrder
	if _, err := w.svc.Undo(w.ctx(w.owner), apppurchasing.VersionInput{ID: sent.ID, Version: 1}); err != nil {
		t.Fatal(err)
	}
	// Undone: back to buy, the cancelled line speaks for nothing.
	lines, err := w.svc.OrderLines(w.ctx(w.staff), orderID)
	if err != nil || len(lines) != 1 || lines[0].Status != apppurchasing.OrderLineToBuy || !lines[0].Line.IsZero() {
		t.Fatalf("after undo: %+v, %v", lines, err)
	}
	// Sent again then received defective: back to buy again; later, the
	// received one is the latest line.
	second := w.send(w.owner, items[0]).SupplierOrder
	if _, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{entry(second.Lines[0].ID, purchasing.CheckDefective, "Riscado")}}); err != nil {
		t.Fatal(err)
	}
	third := w.send(w.owner, items[0]).SupplierOrder
	if _, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{entry(third.Lines[0].ID, purchasing.CheckOK, "")}}); err != nil {
		t.Fatal(err)
	}
	lines, err = w.svc.OrderLines(w.ctx(w.staff), orderID)
	if err != nil || len(lines) != 1 || lines[0].Line != third.Lines[0].ID || lines[0].Status != apppurchasing.OrderLineStatus(purchasing.LineReceived) {
		t.Fatalf("after two receipts: %+v, %v", lines, err)
	}
	// An item that is not bought has nothing to say, and one past
	// "A comprar" with no line is left out.
	mixed := shared.OrderID(w.ids.NewID())
	w.items.AddOrder(apppurchasing.OrderFacts{Order: mixed, Number: "P-0500"}, true,
		fakeItem(w, "Cuba", false, order.ItemInProduction), fakeItem(w, "Cano", true, order.ItemSentToSupplier))
	lines, err = w.svc.OrderLines(w.ctx(w.staff), mixed)
	if err != nil || len(lines) != 0 {
		t.Fatalf("mixed order: %+v, %v", lines, err)
	}
}
