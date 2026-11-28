package purchasingports

import (
	"context"
	"errors"
	"testing"
	"time"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var receiptCases = []repoCase{
	{"a receipt reads back with its lines in the order of its entries", receiptRoundTrip},
	{"a line already in a receipt is closed, and the whole receipt is refused", lineInTwoReceipts},
	{"a taken receipt id or photo is a conflict", takenReceiptIdentities},
	{"a line that was not received, or is unknown, is refused", receiptOfOpenLine},
	{"an unknown receipt is not found", receiptNotFound},
}

func (h RepositoryHarness) photo(w world) purchasing.Photo {
	id := shared.PhotoID(h.NewID())
	return purchasing.Photo{ID: id, Key: purchasing.PhotoKey(w.s.Tenant, id), ContentType: "image/jpeg", Size: 2048}
}

func (h RepositoryHarness) entry(l shared.PurchaseLineID, check purchasing.Check, note string, photos ...purchasing.Photo) purchasing.ReceiptEntry {
	return purchasing.ReceiptEntry{Line: l, Check: check, Note: note, Photos: photos}
}

func (h RepositoryHarness) receipt(t *testing.T, w world, id shared.ReceiptID, invoice []purchasing.Photo, entries ...purchasing.ReceiptEntry) *purchasing.Receipt {
	t.Helper()
	r, err := purchasing.NewReceipt(purchasing.ReceiptSpec{
		Tenant: w.s.Tenant, ID: id, By: h.principal(t, w.s), Now: epoch.Add(3 * time.Hour), Entries: entries, Invoice: invoice,
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (h RepositoryHarness) insertReceipt(w world, r *purchasing.Receipt) error {
	return h.try(w.s.Scope(), func(ctx context.Context) error { return h.Repo.InsertReceipt(ctx, r) })
}

// received makes a line received at the counter and returns its id.
func (h RepositoryHarness) received(t *testing.T, w world) shared.PurchaseLineID {
	t.Helper()
	return h.counterLine(t, w, h.item(t, w), purchasing.CheckOK, "").ID()
}

func receiptRoundTrip(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	counter := h.received(t, w)
	bad := h.counterLine(t, w, h.item(t, w), purchasing.CheckDefective, "Riscado").ID()
	sent := h.send(t, w.s, w.supplier, epoch, h.item(t, w))
	so := h.restore(t, w.s, sent.ID())
	if _, err := so.Receive(so.Lines()[0].ID(), purchasing.CheckOK, "", today()); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, so)
	onOrder := so.Lines()[0].ID()

	id := shared.ReceiptID(h.NewID())
	r := h.receipt(t, w, id, []purchasing.Photo{h.photo(w)},
		h.entry(bad, purchasing.CheckDefective, "Riscado", h.photo(w), h.photo(w)),
		h.entry(onOrder, purchasing.CheckOK, ""),
		h.entry(counter, purchasing.CheckOK, "", h.photo(w)))
	if err := h.insertReceipt(w, r); err != nil {
		t.Fatal(err)
	}

	var got apppurchasing.StoredReceipt
	h.in(t, w.s, func(ctx context.Context) error {
		var err error
		got, err = h.Repo.Receipt(ctx, w.s.Tenant, id)
		return err
	})
	if got.ID != id || !got.At.Equal(r.At()) {
		t.Errorf("receipt %s at %v, want %s at %v", got.ID, got.At, id, r.At())
	}
	if len(got.Lines) != 3 || got.Lines[0].ID != bad || got.Lines[1].ID != onOrder || got.Lines[2].ID != counter {
		t.Fatalf("receipt lines = %v, want the entries' order: defective, on order, counter", got.Lines)
	}
	if got.Lines[0].Status != purchasing.LineDefective || got.Lines[0].CheckNote != "Riscado" ||
		got.Lines[1].Status != purchasing.LineReceived || got.Lines[1].SupplierOrder != sent.ID() {
		t.Errorf("receipt lines read back as %+v", got.Lines)
	}
	if got.Lines[1].Cost == nil || *got.Lines[1].Cost != brl(t, 4000) {
		t.Errorf("cost of a received line = %v, want the order-time cost", got.Lines[1].Cost)
	}
}

func lineInTwoReceipts(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	closed, fresh := h.received(t, w), h.received(t, w)
	first := shared.ReceiptID(h.NewID())
	if err := h.insertReceipt(w, h.receipt(t, w, first, nil, h.entry(closed, purchasing.CheckOK, ""))); err != nil {
		t.Fatal(err)
	}
	second := shared.ReceiptID(h.NewID())
	err := h.insertReceipt(w, h.receipt(t, w, second, nil,
		h.entry(fresh, purchasing.CheckOK, ""), h.entry(closed, purchasing.CheckOK, "")))
	if !errors.Is(err, purchasing.ErrLineClosed) || !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a line in a second receipt: err = %v, want purchasing.ErrLineClosed", err)
	}
	err = h.try(w.s.Scope(), func(ctx context.Context) error {
		_, err := h.Repo.Receipt(ctx, w.s.Tenant, second)
		return err
	})
	if !errors.Is(err, apppurchasing.ErrReceiptNotFound) {
		t.Errorf("the refused receipt: err = %v, want nothing stored", err)
	}
	// The line the refused receipt would have checked is free for another.
	third := shared.ReceiptID(h.NewID())
	if err := h.insertReceipt(w, h.receipt(t, w, third, nil, h.entry(fresh, purchasing.CheckOK, ""))); err != nil {
		t.Errorf("a receipt of the line the refused one named: %v", err)
	}
}

func takenReceiptIdentities(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	photo := h.photo(w)
	id := shared.ReceiptID(h.NewID())
	if err := h.insertReceipt(w, h.receipt(t, w, id, []purchasing.Photo{photo}, h.entry(h.received(t, w), purchasing.CheckOK, ""))); err != nil {
		t.Fatal(err)
	}
	err := h.insertReceipt(w, h.receipt(t, w, id, nil, h.entry(h.received(t, w), purchasing.CheckOK, "")))
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a taken receipt id: err = %v, want shared.ErrConflict", err)
	}
	err = h.insertReceipt(w, h.receipt(t, w, shared.ReceiptID(h.NewID()), []purchasing.Photo{photo},
		h.entry(h.received(t, w), purchasing.CheckOK, "")))
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a taken photo: err = %v, want shared.ErrConflict", err)
	}
}

func receiptOfOpenLine(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	sent := h.send(t, w.s, w.supplier, epoch, h.item(t, w))
	for name, line := range map[string]shared.PurchaseLineID{
		"an open line": sent.Lines()[0].ID(), "an unknown line": shared.PurchaseLineID(h.NewID()),
	} {
		err := h.insertReceipt(w, h.receipt(t, w, shared.ReceiptID(h.NewID()), nil, h.entry(line, purchasing.CheckOK, "")))
		if !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("a receipt of %s: err = %v, want shared.ErrInvalid", name, err)
		}
	}
}

func receiptNotFound(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	err := h.try(w.s.Scope(), func(ctx context.Context) error {
		_, err := h.Repo.Receipt(ctx, w.s.Tenant, shared.ReceiptID(h.NewID()))
		return err
	})
	if !errors.Is(err, apppurchasing.ErrReceiptNotFound) || !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an unknown receipt: err = %v, want ErrReceiptNotFound", err)
	}
}
