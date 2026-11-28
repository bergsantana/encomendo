package purchasingports

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var lineCases = []repoCase{
	{"OpenLines come oldest supplier order first, then as picked, open lines only", openLinesOrder},
	{"OpenLines carry the supplier order's facts and its promised date", openLinesCarryFacts},
	{"LinesOfOrder come in the order written, every status, counter lines included", linesOfOrderWritten},
	{"a line whose item was removed stays, with no order item", linesSurviveRemovedItem},
	{"DefectiveItems marks only the items with a defective line", defectiveItems},
	{"a counter line is stored whole, with its cost, and a taken id is a conflict", counterLineStored},
}

func (h RepositoryHarness) openLines(t *testing.T, s Store) []apppurchasing.StoredLine {
	t.Helper()
	var out []apppurchasing.StoredLine
	h.in(t, s, func(ctx context.Context) error {
		var err error
		out, err = h.Repo.OpenLines(ctx, s.Tenant)
		return err
	})
	return out
}

func lineIDs(ls []apppurchasing.StoredLine) []shared.PurchaseLineID {
	out := make([]shared.PurchaseLineID, len(ls))
	for i, l := range ls {
		out[i] = l.ID
	}
	return out
}

func (h RepositoryHarness) counterLine(t *testing.T, w world, b Bought, check purchasing.Check, note string) purchasing.Line {
	t.Helper()
	l, err := purchasing.CounterLine(purchasing.CounterSpec{
		ID: shared.PurchaseLineID(h.NewID()), Supplier: w.supplier, Item: b.item("Banco de couro", 1),
		Check: check, Note: note, Today: today(),
	})
	if err != nil {
		t.Fatal(err)
	}
	h.in(t, w.s, func(ctx context.Context) error { return h.Repo.InsertCounterLine(ctx, w.s.Tenant, l) })
	return l
}

func openLinesOrder(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	a, b, c, d, e := h.item(t, w), h.item(t, w), h.item(t, w), h.item(t, w), h.item(t, w)
	later := h.send(t, w.s, w.supplier, epoch.Add(time.Hour), c, d)
	earlier := h.send(t, w.s, w.supplier, epoch, a, b, e)
	h.counterLine(t, w, h.item(t, w), purchasing.CheckOK, "") // never open

	so := h.restore(t, w.s, earlier.ID())
	if _, err := so.Receive(so.Lines()[2].ID(), purchasing.CheckOK, "", today()); err != nil { // e arrived
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, so)
	moving := h.restore(t, w.s, later.ID())
	if _, err := moving.MarkInTransit(epoch.Add(2 * time.Hour)); err != nil { // c and d in transit
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, moving)

	want := []shared.PurchaseLineID{
		earlier.Lines()[0].ID(), earlier.Lines()[1].ID(), later.Lines()[0].ID(), later.Lines()[1].ID(),
	}
	if got := lineIDs(h.openLines(t, w.s)); !slices.Equal(got, want) {
		t.Errorf("OpenLines\n got: %v\nwant: %v", got, want)
	}
}

func openLinesCarryFacts(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	own, follows := h.item(t, w), h.item(t, w)
	sent := h.send(t, w.s, w.supplier, epoch, own, follows)
	so := h.restore(t, w.s, sent.ID())
	ownDate := today().AddDays(9)
	if _, err := so.RecordPromisedDate(today().AddDays(7), map[shared.PurchaseLineID]shared.CivilDate{so.Lines()[0].ID(): ownDate}, today()); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, so)

	lines := h.openLines(t, w.s)
	if len(lines) != 2 {
		t.Fatalf("open lines = %d, want 2", len(lines))
	}
	for i, l := range lines {
		if l.Number != sent.Number() || l.SupplierName != "Couros Paulista" || l.SupplierPhone != "5511987654321" ||
			!l.SentAt.Equal(epoch) || l.OrderPromised != today().AddDays(7) {
			t.Errorf("line %d facts = %v %q %q %v %v", i, l.Number, l.SupplierName, l.SupplierPhone, l.SentAt, l.OrderPromised)
		}
	}
	if got := lines[0].EffectivePromised(); got != ownDate {
		t.Errorf("effective date of the line with its own = %v, want %v", got, ownDate)
	}
	if got := lines[1].EffectivePromised(); got != today().AddDays(7) {
		t.Errorf("effective date of the line that follows the order = %v, want the order's", got)
	}
	if c := lines[0].Cost; c == nil || *c != brl(t, 4000) {
		t.Errorf("cost of an open line = %v, want the order-time cost", c)
	}
}

func linesOfOrderWritten(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	b := h.item(t, w)
	other := h.item(t, w)
	h.send(t, w.s, w.supplier, epoch, other) // another order's line, never listed
	sent := h.send(t, w.s, w.supplier, epoch, b)
	undo := h.restore(t, w.s, sent.ID())
	if _, err := undo.Undo(h.principal(t, w.s), epoch.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, undo)
	counter := h.counterLine(t, w, b, purchasing.CheckDefective, "Veio rasgado")
	resent := h.send(t, w.s, w.supplier, epoch.Add(2*time.Hour), b)

	want := []shared.PurchaseLineID{sent.Lines()[0].ID(), counter.ID(), resent.Lines()[0].ID()}
	got := h.linesOf(t, w.s, b.Order)
	if !slices.Equal(lineIDs(got), want) {
		t.Fatalf("LinesOfOrder\n got: %v\nwant: %v", lineIDs(got), want)
	}
	if got[0].Status != purchasing.LineCancelled || got[1].Status != purchasing.LineDefective || got[2].Status != purchasing.LineSent {
		t.Errorf("statuses = %s, %s, %s", got[0].Status, got[1].Status, got[2].Status)
	}
	if !got[1].SupplierOrder.IsZero() || got[1].Number != 0 || got[1].SupplierName != "" || !got[1].SentAt.IsZero() {
		t.Errorf("a counter line has no supplier order facts: %+v", got[1])
	}
	if n := len(h.linesOf(t, w.s, h.NewItem(t, w.s.Tenant, nil).Order)); n != 0 {
		t.Errorf("lines of an order with none = %d", n)
	}
}

func linesSurviveRemovedItem(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	b := h.item(t, w)
	sent := h.send(t, w.s, w.supplier, epoch, b)
	h.RemoveItem(t, w.s.Tenant, b)

	got := h.linesOf(t, w.s, b.Order)
	if len(got) != 1 {
		t.Fatalf("lines of the order after its item was removed = %d, want the line kept", len(got))
	}
	want := sent.Lines()[0].Record()
	want.Item.OrderItem = shared.OrderItemID{}
	want.Cost = ptr(brl(t, 4000))
	sameRecord(t, "line after its item was removed", got[0].LineRecord, want)
	if open := h.openLines(t, w.s); len(open) != 1 || !open[0].Item.OrderItem.IsZero() {
		t.Errorf("OpenLines after the removal = %+v, want the line, still open, with no order item", open)
	}
	if rec := h.stored(t, w.s, sent.ID()); !rec.Lines[0].Item.OrderItem.IsZero() {
		t.Errorf("the supplier order's line still names the removed item")
	}
	// The record still restores and the line can still be received.
	so := h.restore(t, w.s, sent.ID())
	if _, err := so.Receive(so.Lines()[0].ID(), purchasing.CheckOK, "", today()); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, so)
}

func defectiveItems(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	sentBad, counterBad, fine, unsent := h.item(t, w), h.item(t, w), h.item(t, w), h.item(t, w)
	sent := h.send(t, w.s, w.supplier, epoch, sentBad, fine)
	so := h.restore(t, w.s, sent.ID())
	if _, err := so.Receive(so.Lines()[0].ID(), purchasing.CheckDefective, "Quebrado", today()); err != nil {
		t.Fatal(err)
	}
	if _, err := so.Receive(so.Lines()[1].ID(), purchasing.CheckOK, "", today()); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, so)
	h.counterLine(t, w, counterBad, purchasing.CheckDefective, "Riscado")

	ask := []shared.OrderItemID{sentBad.Item, counterBad.Item, fine.Item, unsent.Item, shared.OrderItemID(h.NewID())}
	var got map[shared.OrderItemID]bool
	h.in(t, w.s, func(ctx context.Context) error {
		var err error
		got, err = h.Repo.DefectiveItems(ctx, w.s.Tenant, ask)
		return err
	})
	if len(got) != 2 || !got[sentBad.Item] || !got[counterBad.Item] {
		t.Errorf("DefectiveItems = %v, want only the two defective items", got)
	}
	h.in(t, w.s, func(ctx context.Context) error {
		none, err := h.Repo.DefectiveItems(ctx, w.s.Tenant, nil)
		if err != nil || len(none) != 0 {
			t.Errorf("DefectiveItems of no item = %v, %v", none, err)
		}
		return nil
	})
}

func counterLineStored(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	b := h.item(t, w)
	l := h.counterLine(t, w, b, purchasing.CheckDefective, "Veio sem a tampa")
	got := h.linesOf(t, w.s, b.Order)
	if len(got) != 1 {
		t.Fatalf("lines = %d, want the counter line", len(got))
	}
	want := l.Record()
	want.Cost = ptr(brl(t, 4000))
	sameRecord(t, "counter line", got[0].LineRecord, want)
	if got[0].ReceivedOn != today() || got[0].CheckNote != "Veio sem a tampa" {
		t.Errorf("counter line received on %v with note %q", got[0].ReceivedOn, got[0].CheckNote)
	}
	again := l.Record()
	err := h.try(w.s.Scope(), func(ctx context.Context) error {
		dup, err := purchasing.RestoreLine(again)
		if err != nil {
			return err
		}
		return h.Repo.InsertCounterLine(ctx, w.s.Tenant, dup)
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a counter line with a taken id: err = %v, want shared.ErrConflict", err)
	}
}
