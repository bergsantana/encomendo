package purchasing_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	jpegBytes = append([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, make([]byte, 64)...)
	pngBytes  = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
)

// upload stages a photo as an app's signed PUT would and returns its key.
func (w *world) upload(data []byte) string {
	w.t.Helper()
	key := purchasing.PhotoKey(w.tenant, shared.PhotoID(w.ids.NewID()))
	w.photos.Upload(key, "application/octet-stream", data)
	return key
}

func (w *world) receive(who shared.Principal, in apppurchasing.ReceiptInput) (apppurchasing.ReceiptView, error) {
	if in.Idempotency.IsZero() {
		in.Idempotency = w.key()
	}
	return w.svc.Receive(w.ctx(who), in)
}

func entry(line shared.PurchaseLineID, check purchasing.Check, note string, photos ...string) apppurchasing.ReceiptEntryInput {
	return apppurchasing.ReceiptEntryInput{Line: line, Check: check, Note: note, PhotoKeys: photos}
}

func TestReceive_DefectBackToBuyAsReposicao(t *testing.T) {
	w := newWorld(t)
	orderID, items := w.order(231, bought{name: "Banco de couro", qty: 1, cost: 40_000})
	first := w.send(w.owner, items[0]).SupplierOrder
	photo := w.upload(jpegBytes)

	v, err := w.receive(w.staff, apppurchasing.ReceiptInput{
		Entries: []apppurchasing.ReceiptEntryInput{entry(first.Lines[0].ID, purchasing.CheckDefective, "  Couro rasgado  ", photo)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Entries) != 1 || v.Entries[0].Check != purchasing.CheckDefective || v.Entries[0].OrderNumber != "P-0231" || v.Entries[0].Name != "Banco de couro" {
		t.Fatalf("entries = %+v", v.Entries)
	}
	if len(v.BackToBuy) != 1 || v.BackToBuy[0].OrderItem != items[0] || len(v.CompletedOrders) != 0 {
		t.Fatalf("back to buy = %+v, completed = %+v", v.BackToBuy, v.CompletedOrders)
	}
	if got := w.itemStatus(orderID, items[0]); got != order.ItemToBuy {
		t.Fatalf("item = %s, want back in to_buy", got)
	}
	if _, _, ok := w.photos.Promoted(photo); !ok {
		t.Fatal("the photo was not promoted")
	}
	stored, _ := w.repo.Stored(w.tenant, first.ID)
	if l := stored.Lines[0]; l.Status != purchasing.LineDefective || l.CheckNote != "Couro rasgado" || l.ReceivedOn != w.today {
		t.Fatalf("stored line = %+v", l)
	}
	// Back in "A comprar", under the same supplier, as a replacement.
	tb, err := w.svc.ToBuy(w.ctx(w.admin))
	if err != nil || len(tb.Suppliers) != 1 || len(tb.Suppliers[0].Items) != 1 || !tb.Suppliers[0].Items[0].Replacement {
		t.Fatalf("to buy = %+v, %v", tb, err)
	}
	// It can be sent again, and received whole.
	second := w.send(w.owner, items[0]).SupplierOrder
	done, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{entry(second.Lines[0].ID, purchasing.CheckOK, "")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(done.BackToBuy) != 0 || len(done.CompletedOrders) != 1 || done.CompletedOrders[0].Order != orderID || done.CompletedOrders[0].Number != "P-0231" {
		t.Fatalf("done = %+v", done)
	}
	if got := w.itemStatus(orderID, items[0]); got != order.ItemReceived {
		t.Fatalf("item = %s, want received", got)
	}
}

func TestReceive_CounterLine(t *testing.T) {
	w := newWorld(t)
	orderID, items := w.order(231, bought{name: "Banco de couro", qty: 2, choices: []purchasing.Choice{preto}, cost: 40_000}, bought{name: "Guidão", qty: 1})
	sent := w.send(w.owner, items[1]).SupplierOrder

	v, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{
		{OrderItem: items[0], Check: purchasing.CheckOK},
		entry(sent.Lines[0].ID, purchasing.CheckOK, ""),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Entries) != 2 || v.Entries[0].OrderItem != items[0] || v.Entries[1].Line != sent.Lines[0].ID {
		t.Fatalf("entries = %+v", v.Entries)
	}
	if len(v.CompletedOrders) != 1 || v.CompletedOrders[0].Order != orderID {
		t.Fatalf("completed = %+v", v.CompletedOrders)
	}
	counters := w.repo.CounterLines()
	if len(counters) != 1 {
		t.Fatalf("counter lines = %+v", counters)
	}
	l := counters[0]
	if !l.SupplierOrder.IsZero() || l.Status != purchasing.LineReceived || l.Supplier != w.supplier.ID || l.Item.Quantity != 2 ||
		l.Item.Name != "Banco de couro" || l.ReceivedOn != w.today || l.Cost == nil || l.Cost.Cents() != 40_000 {
		t.Fatalf("counter line = %+v", l)
	}
	if got := w.itemStatus(orderID, items[0]); got != order.ItemReceived {
		t.Fatalf("item = %s, want received", got)
	}
	// The order's lines name it with no pedido de compra.
	lines, err := w.svc.OrderLines(w.ctx(w.staff), orderID)
	if err != nil || len(lines) != 2 {
		t.Fatalf("order lines = %+v, %v", lines, err)
	}
	if got := lines[0]; got.Status != apppurchasing.OrderLineStatus(purchasing.LineReceived) || got.Number != 0 || !got.SupplierOrder.IsZero() || got.SupplierName != w.supplier.Name {
		t.Fatalf("counter item = %+v", got)
	}
}

func TestReceive_CounterItemNoLongerToBuy(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	w.send(w.owner, items[0])
	_, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{{OrderItem: items[0], Check: purchasing.CheckOK}}})
	if !errors.Is(err, apppurchasing.ErrItemNotToBuy) {
		t.Fatalf("err = %v, want ErrItemNotToBuy", err)
	}
	if len(w.repo.CounterLines()) != 0 {
		t.Fatal("a counter line was written for an item sent to a supplier")
	}
}

func TestReceive_Refusals(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1}, bought{name: "Guidão", qty: 1})
	sent := w.send(w.owner, items[0]).SupplierOrder
	line := sent.Lines[0].ID
	ok := entry(line, purchasing.CheckOK, "")
	for _, c := range []struct {
		name    string
		entries []apppurchasing.ReceiptEntryInput
		want    error
	}{
		{"no entry", nil, purchasing.ErrEmptyReceipt},
		{"neither line nor item", []apppurchasing.ReceiptEntryInput{{Check: purchasing.CheckOK}}, shared.ErrInvalid},
		{"both line and item", []apppurchasing.ReceiptEntryInput{{Line: line, OrderItem: items[1], Check: purchasing.CheckOK}}, shared.ErrInvalid},
		{"line twice", []apppurchasing.ReceiptEntryInput{ok, ok}, purchasing.ErrLineTwice},
		{"counter item twice", []apppurchasing.ReceiptEntryInput{{OrderItem: items[1], Check: purchasing.CheckOK}, {OrderItem: items[1], Check: purchasing.CheckOK}}, purchasing.ErrLineTwice},
		{"defect without a note", []apppurchasing.ReceiptEntryInput{entry(line, purchasing.CheckDefective, " ")}, shared.ErrInvalid},
		{"unknown check", []apppurchasing.ReceiptEntryInput{entry(line, "meh", "")}, shared.ErrInvalid},
		{"unknown line", []apppurchasing.ReceiptEntryInput{entry(shared.PurchaseLineID(w.ids.NewID()), purchasing.CheckOK, "")}, purchasing.ErrLineNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: c.entries})
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if err.Error() == "" {
				t.Fatal("an error with no message")
			}
		})
	}
	if _, err := w.svc.Receive(w.ctx(w.staff), apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{ok}}); !errors.Is(err, apppurchasing.ErrIdempotencyKeyRequired) {
		t.Fatalf("no key: %v", err)
	}
	// A line received twice, in two receipts.
	if _, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{ok}}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{ok}}); !errors.Is(err, purchasing.ErrLineClosed) {
		t.Fatalf("second receipt: %v, want ErrLineClosed", err)
	}
	if w.repo.Receipts() != 1 {
		t.Fatalf("receipts = %d, want 1: refused calls store none", w.repo.Receipts())
	}
}

func TestReceive_TwoLinesOfOneSupplierOrderSaveItOnce(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.owner)
	_, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{
		entry(s.view.Lines[0].ID, purchasing.CheckOK, ""), entry(s.view.Lines[1].ID, purchasing.CheckDefective, "Riscado"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := w.repo.Stored(w.tenant, s.view.ID)
	if stored.Version != 2 {
		t.Fatalf("version = %d, want 2: one update", stored.Version)
	}
	if v, _ := w.svc.SupplierOrder(w.ctx(w.owner), s.view.ID); v.State != purchasing.StateReceived || v.Received != 2 {
		t.Fatalf("state = %s, received = %d", v.State, v.Received)
	}
}

func TestReceive_PhotoChecks(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	sent := w.send(w.owner, items[0]).SupplierOrder
	line := sent.Lines[0].ID
	foreign := purchasing.PhotoKey(shared.TenantID(w.ids.NewID()), shared.PhotoID(w.ids.NewID()))
	missing := purchasing.PhotoKey(w.tenant, shared.PhotoID(w.ids.NewID()))
	notImage := w.upload([]byte("<html>not a photo, only text of some length</html>"))
	empty := w.upload(nil)
	big := w.upload(make([]byte, catalog.MaxPhotoBytes+1))
	good := w.upload(pngBytes)
	many := func(n int) []string {
		keys := make([]string, n)
		for i := range keys {
			keys[i] = w.upload(jpegBytes)
		}
		return keys
	}

	for _, c := range []struct {
		name     string
		in       apppurchasing.ReceiptInput
		want     error
		storage  bool // whether storage was touched
		deletion string
	}{
		{"another store's key", apppurchasing.ReceiptInput{InvoicePhotoKeys: []string{foreign}}, purchasing.ErrPhotoKey, false, ""},
		{"not a key at all", apppurchasing.ReceiptInput{InvoicePhotoKeys: []string{"../x"}}, purchasing.ErrPhotoKey, false, ""},
		{"three photos on an entry", apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{entry(line, purchasing.CheckOK, "", many(3)...)}}, purchasing.ErrPhotoLimit, false, ""},
		{"five invoice photos", apppurchasing.ReceiptInput{InvoicePhotoKeys: many(5)}, purchasing.ErrPhotoLimit, false, ""},
		{"same photo twice", apppurchasing.ReceiptInput{InvoicePhotoKeys: []string{good, good}}, shared.ErrInvalid, false, ""},
		{"never uploaded", apppurchasing.ReceiptInput{InvoicePhotoKeys: []string{missing}}, catalog.ErrPhotoNotUploaded, true, ""},
		{"not an image", apppurchasing.ReceiptInput{InvoicePhotoKeys: []string{notImage}}, catalog.ErrPhotoInvalid, true, notImage},
		{"empty upload", apppurchasing.ReceiptInput{InvoicePhotoKeys: []string{empty}}, catalog.ErrPhotoInvalid, true, empty},
		{"over 10 MB", apppurchasing.ReceiptInput{InvoicePhotoKeys: []string{big}}, catalog.ErrPhotoInvalid, true, big},
	} {
		t.Run(c.name, func(t *testing.T) {
			before := len(w.photos.Calls())
			if len(c.in.Entries) == 0 {
				c.in.Entries = []apppurchasing.ReceiptEntryInput{entry(line, purchasing.CheckOK, "")}
			}
			_, err := w.receive(w.staff, c.in)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if touched := len(w.photos.Calls()) > before; touched != c.storage {
				t.Fatalf("storage touched = %v, want %v (%v)", touched, c.storage, w.photos.Calls()[before:])
			}
			if c.deletion != "" && w.photos.Uploaded(c.deletion) {
				t.Fatal("the refused upload was not deleted")
			}
			if l, _ := w.repo.Stored(w.tenant, sent.ID); l.Lines[0].Status != purchasing.LineSent {
				t.Fatalf("a refused receipt changed the line to %s", l.Lines[0].Status)
			}
		})
	}
	// Good photos on the entry and the invoice are promoted, as sniffed.
	in := apppurchasing.ReceiptInput{
		Entries: []apppurchasing.ReceiptEntryInput{entry(line, purchasing.CheckOK, "", good)}, InvoicePhotoKeys: []string{w.upload(jpegBytes)},
	}
	if _, err := w.receive(w.staff, in); err != nil {
		t.Fatal(err)
	}
	if _, ct, ok := w.photos.Promoted(good); !ok || ct != "image/png" {
		t.Fatalf("photo = %q, %v; want a promoted PNG", ct, ok)
	}
	if _, ct, ok := w.photos.Promoted(in.InvoicePhotoKeys[0]); !ok || ct != "image/jpeg" {
		t.Fatalf("invoice photo = %q, %v; want a promoted JPEG", ct, ok)
	}
}

func TestReceive_PhotoStorageOutageIsNotARefusal(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	sent := w.send(w.owner, items[0]).SupplierOrder
	key := w.upload(jpegBytes)
	w.photos.Fail("Stat", shared.ErrUnavailable)
	_, err := w.receive(w.staff, apppurchasing.ReceiptInput{
		Entries: []apppurchasing.ReceiptEntryInput{entry(sent.Lines[0].ID, purchasing.CheckOK, "", key)},
	})
	if !errors.Is(err, shared.ErrUnavailable) || errors.Is(err, shared.ErrInvalid) {
		t.Fatalf("err = %v, want an outage", err)
	}
}

func TestReceive_PhotoChecksRunBeforeTheTransaction(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	sent := w.send(w.owner, items[0]).SupplierOrder
	inside := map[string][]bool{}
	for _, method := range []string{"PresignPut", "Stat", "Head", "DeleteUpload", "Promote"} {
		w.photos.Before(method, func() { inside[method] = append(inside[method], w.tx.Inside()) })
	}
	bad := w.upload([]byte("not a photo, just some text that is long enough"))
	_, _ = w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{entry(sent.Lines[0].ID, purchasing.CheckOK, "", bad)}})

	good := w.upload(jpegBytes)
	if _, err := w.receive(w.staff, apppurchasing.ReceiptInput{
		Entries: []apppurchasing.ReceiptEntryInput{entry(sent.Lines[0].ID, purchasing.CheckOK, "", good)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.RequestPhotoUpload(w.ctx(w.staff), "image/png"); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"PresignPut", "Stat", "Head", "DeleteUpload"} {
		if len(inside[method]) == 0 {
			t.Fatalf("%s was never called", method)
		}
		for _, in := range inside[method] {
			if in {
				t.Errorf("%s ran inside a transaction", method)
			}
		}
	}
	// Promoting is the one storage call inside it, and the last thing it does.
	if p := inside["Promote"]; len(p) != 1 || !p[0] {
		t.Fatalf("Promote inside a transaction = %v, want [true]", p)
	}
}

func TestReceive_PromoteFailureRollsBackAndReportsOrphans(t *testing.T) {
	w := newWorld(t)
	orderID, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	sent := w.send(w.owner, items[0]).SupplierOrder
	first, second := w.upload(jpegBytes), w.upload(jpegBytes)
	in := apppurchasing.ReceiptInput{
		Entries:     []apppurchasing.ReceiptEntryInput{entry(sent.Lines[0].ID, purchasing.CheckOK, "", first, second)},
		Idempotency: w.key(),
	}
	calls := 0
	w.photos.Before("Promote", func() {
		calls++
		if calls == 2 {
			w.photos.Fail("Promote", errors.New("bucket unreachable"))
		}
	})

	if _, err := w.svc.Receive(w.ctx(w.staff), in); err == nil {
		t.Fatal("receive succeeded with a failing promote")
	}
	if w.repo.Receipts() != 0 {
		t.Fatal("a receipt was stored")
	}
	if l, _ := w.repo.Stored(w.tenant, sent.ID); l.Lines[0].Status != purchasing.LineSent {
		t.Fatalf("line = %s, want it still sent", l.Lines[0].Status)
	}
	if got := w.itemStatus(orderID, items[0]); got != order.ItemSentToSupplier {
		t.Fatalf("item = %s, want it unmoved", got)
	}
	// Both copies may exist with no row: both are reported.
	if got := w.photos.Orphans(); len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("orphans = %v", got)
	}
	// The key was forgotten with the rollback, so the retry works.
	w.photos.Fail("Promote", nil)
	if _, err := w.svc.Receive(w.ctx(w.staff), in); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestReceive_UploadReplacedBeforePromoteIsNotAnOrphan(t *testing.T) {
	w := newWorld(t)
	_, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	sent := w.send(w.owner, items[0]).SupplierOrder
	key := w.upload(jpegBytes)
	w.photos.Before("Promote", func() { w.photos.Upload(key, "image/jpeg", pngBytes) })

	_, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{entry(sent.Lines[0].ID, purchasing.CheckOK, "", key)}})
	if !errors.Is(err, catalog.ErrPhotoNotUploaded) {
		t.Fatalf("err = %v, want ErrPhotoNotUploaded", err)
	}
	if got := w.photos.Orphans(); len(got) != 0 {
		t.Fatalf("orphans = %v, want none: nothing was copied", got)
	}
}

func TestReceive_ReplayAnswersSameReceipt(t *testing.T) {
	w := newWorld(t)
	orderID, items := w.order(231, bought{name: "Banco de couro", qty: 1})
	sent := w.send(w.owner, items[0]).SupplierOrder
	photo := w.upload(jpegBytes)
	in := apppurchasing.ReceiptInput{
		Entries:     []apppurchasing.ReceiptEntryInput{entry(sent.Lines[0].ID, purchasing.CheckOK, "", photo)},
		Idempotency: w.key(),
	}
	first, err := w.svc.Receive(w.ctx(w.staff), in)
	if err != nil {
		t.Fatal(err)
	}
	// The lines are closed now, and the staged upload is gone.
	if err := w.photos.DeleteUpload(context.Background(), photo); err != nil {
		t.Fatal(err)
	}
	calls := len(w.photos.Calls())

	again, err := w.svc.Receive(w.ctx(w.staff), in)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if again.ID != first.ID || len(again.Entries) != 1 || again.Entries[0].Line != first.Entries[0].Line || !again.ReceivedAt.Equal(first.ReceivedAt) {
		t.Fatalf("replay = %+v, first = %+v", again, first)
	}
	if len(again.CompletedOrders) != 1 || again.CompletedOrders[0].Order != orderID {
		t.Fatalf("replay completed = %+v", again.CompletedOrders)
	}
	if got := len(w.photos.Calls()); got != calls {
		t.Fatalf("the replay touched storage: %v", w.photos.Calls()[calls:])
	}
	if w.repo.Receipts() != 1 {
		t.Fatalf("receipts = %d", w.repo.Receipts())
	}
	// The same key with another request is refused; so is a key that made a
	// supplier order, not a receipt.
	other := in
	other.Idempotency.RequestHash = [32]byte{42}
	if _, err := w.svc.Receive(w.ctx(w.staff), other); !errors.Is(err, appshared.ErrIdempotencyKeyReused) {
		t.Fatalf("other request: %v", err)
	}
	_, items2 := w.order(232, bought{name: "Guidão", qty: 1})
	k := w.key()
	if _, err := w.svc.Send(w.ctx(w.owner), apppurchasing.SendInput{
		PickInput: apppurchasing.PickInput{Supplier: w.supplier.ID, OrderItems: items2}, Idempotency: k,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = w.receive(w.staff, apppurchasing.ReceiptInput{Entries: in.Entries, Idempotency: k})
	if !errors.Is(err, appshared.ErrIdempotencyKeyReused) {
		t.Fatalf("a send's key: %v", err)
	}
}

func TestReceive_LineOfCancelledOrderIsReceivedNotMoved(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.owner)
	w.items.Cancel(s.order1)
	// The second order's item was removed from it.
	w.items.RemoveItem(s.order2, s.items2[0])
	w.repo.ForgetItem(s.items2[0])
	o3, i3 := w.order(233, bought{name: "Pedal", qty: 1})
	third := w.send(w.owner, i3[0]).SupplierOrder

	v, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{
		entry(s.view.Lines[0].ID, purchasing.CheckDefective, "Quebrado"),
		entry(s.view.Lines[1].ID, purchasing.CheckOK, ""),
		entry(third.Lines[0].ID, purchasing.CheckOK, ""),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Entries) != 3 || len(v.BackToBuy) != 0 || len(v.CompletedOrders) != 1 || v.CompletedOrders[0].Order != o3 {
		t.Fatalf("receipt = %+v", v)
	}
	stored, _ := w.repo.Stored(w.tenant, s.view.ID)
	if stored.Lines[0].Status != purchasing.LineDefective || stored.Lines[1].Status != purchasing.LineReceived {
		t.Fatalf("lines = %+v", stored.Lines)
	}
	if got := w.itemStatus(s.order1, s.items1[0]); got != order.ItemSentToSupplier {
		t.Fatalf("item of the cancelled order = %s, want it left alone", got)
	}
	if got := w.itemStatus(o3, i3[0]); got != order.ItemReceived {
		t.Fatalf("item of the open order = %s, want received", got)
	}
}

func TestReceive_CompletedOrders(t *testing.T) {
	w := newWorld(t)
	orderID, items := w.order(231, bought{name: "Banco de couro", qty: 1}, bought{name: "Guidão", qty: 1})
	sent := w.send(w.owner, items...).SupplierOrder

	one, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{entry(sent.Lines[0].ID, purchasing.CheckOK, "")}})
	if err != nil || len(one.CompletedOrders) != 0 {
		t.Fatalf("after one of two: %+v, %v", one.CompletedOrders, err)
	}
	both, err := w.receive(w.staff, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{entry(sent.Lines[1].ID, purchasing.CheckOK, "")}})
	if err != nil || len(both.CompletedOrders) != 1 || both.CompletedOrders[0].Order != orderID {
		t.Fatalf("after both: %+v, %v", both.CompletedOrders, err)
	}
}

func TestToReceive_GroupsOpenLinesOldestFirstThenCounterItems(t *testing.T) {
	w := newWorld(t)
	_, i1 := w.order(231, bought{name: "Banco de couro", qty: 1})
	_, i2 := w.order(232, bought{name: "Guidão", qty: 2, choices: []purchasing.Choice{preto}})
	_, i3 := w.order(233, bought{name: "Pedal", qty: 1})
	first := w.send(w.owner, i1[0]).SupplierOrder
	w.clock.Advance(60_000_000_000)
	second := w.send(w.owner, i2[0]).SupplierOrder
	w.items.SetTruncated(true)

	v, err := w.svc.ToReceive(w.ctx(w.staff))
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Groups) != 2 || v.Groups[0].SupplierOrder != first.ID || v.Groups[1].SupplierOrder != second.ID || !v.Truncated {
		t.Fatalf("groups = %+v", v.Groups)
	}
	g := v.Groups[1]
	if g.Number != 2 || g.SupplierName != w.supplier.Name || len(g.Lines) != 1 || g.Lines[0].Line != second.Lines[0].ID ||
		g.Lines[0].OrderNumber != "P-0232" || g.Lines[0].Quantity != 2 || len(g.Lines[0].Choices) != 1 || g.Lines[0].Choices[0].Label != "Preto" {
		t.Fatalf("group = %+v", g)
	}
	if len(v.ToBuy) != 1 || v.ToBuy[0].OrderItem != i3[0] || !v.ToBuy[0].Line.IsZero() || v.ToBuy[0].OrderNumber != "P-0233" {
		t.Fatalf("to buy = %+v", v.ToBuy)
	}
}

func TestRequestPhotoUpload(t *testing.T) {
	w := newWorld(t)
	up, err := w.svc.RequestPhotoUpload(w.ctx(w.staff), "image/webp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := purchasing.ParsePhotoKey(w.tenant, up.Key); err != nil {
		t.Fatalf("key %q: %v", up.Key, err)
	}
	if up.Method != "PUT" || !strings.HasSuffix(up.URL, up.Key) || up.Headers["Content-Type"] != "image/webp" || !up.ExpiresAt.After(w.clock.Now()) {
		t.Fatalf("upload = %+v", up)
	}
	if _, err := w.svc.RequestPhotoUpload(w.ctx(w.staff), "application/pdf"); !errors.Is(err, shared.ErrInvalid) {
		t.Fatalf("pdf: %v", err)
	}
	w.photos.Fail("PresignPut", shared.ErrUnavailable)
	if _, err := w.svc.RequestPhotoUpload(w.ctx(w.staff), "image/png"); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("outage: %v", err)
	}
}
