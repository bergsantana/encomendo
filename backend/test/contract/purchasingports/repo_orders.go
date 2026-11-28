package purchasingports

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var supplierOrderCases = []repoCase{
	{"a supplier order reads back whole, with its message and lines in order", orderRoundTrip},
	{"a line's cost is copied from the order item, never taken from the caller", costIsCopied},
	{"an item of another store is refused and copies no cost", otherStoresItemCopiesNoCost},
	{"one open line per item, and nothing of the refused order is stored", oneOpenLinePerItem},
	{"a closed line frees its item for a new send", closedLineFreesItem},
	{"a taken id or number is a conflict", takenIdentities},
	{"an update stores what changes, under the version", updateStoresChanges},
	{"an undo is stored with its cancelled lines", undoIsStored},
	{"a stale version is a version conflict, an unknown order is not found", updateVersionAndNotFound},
	{"an update never adds or removes a line, nor changes the cost", updateKeepsLinesAndCost},
	{"the supplier order of a line, but not of a counter line", orderOfLine},
	{"the message reads back, an unknown order is not found", messageReads},
	{"supplier orders page newest sent first, ties by id, open only on request", ordersPaging},
	{"a cursor the repository did not issue is a validation error", badCursor},
}

func orderRoundTrip(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	first, second := h.item(t, w), h.item(t, w)
	so := h.newOrderWithChoices(t, w, first, second)
	h.in(t, w.s, func(ctx context.Context) error {
		n, err := h.Repo.NextNumber(ctx, w.s.Tenant)
		if err != nil {
			return err
		}
		if so, err = rebuildNumbered(so, n); err != nil {
			return err
		}
		return h.Repo.InsertSupplierOrder(ctx, so, sentText, sentURL)
	})
	want := so.Record()
	for i := range want.Lines {
		want.Lines[i].Cost = ptr(brl(t, 4000))
	}
	got := h.stored(t, w.s, so.ID())
	sameRecord(t, "stored supplier order", got, want)
	if got.Version != 1 || len(got.Lines) != 2 || got.Lines[0].ID != so.Lines()[0].ID() {
		t.Errorf("version %d, %d lines, first %s: want version 1 and the lines as picked", got.Version, len(got.Lines), got.Lines[0].ID)
	}
	var msg string
	h.in(t, w.s, func(ctx context.Context) error {
		m, err := h.Repo.Message(ctx, w.s.Tenant, so.ID())
		msg = m.Text + "|" + m.URL
		return err
	})
	if msg != sentText+"|"+sentURL {
		t.Errorf("message = %q", msg)
	}
}

// newOrderWithChoices is a supplier order of two lines, the first with
// two chosen options, numbered 1 (rebuildNumbered gives it its number).
func (h RepositoryHarness) newOrderWithChoices(t *testing.T, w world, items ...Bought) *purchasing.SupplierOrder {
	t.Helper()
	lines := make([]purchasing.LineSpec, len(items))
	for i, b := range items {
		lines[i] = purchasing.LineSpec{ID: shared.PurchaseLineID(h.NewID()), Item: b.item("Banco de couro", i+1)}
	}
	lines[0].Item.Choices = []purchasing.Choice{
		{Option: shared.OptionID(h.NewID()), ChoiceID: shared.ChoiceID(h.NewID()), Name: "Cor da costura", Label: "Vermelha"},
		{Option: shared.OptionID(h.NewID()), ChoiceID: shared.ChoiceID(h.NewID()), Name: "Tamanho", Label: "Grande"},
	}
	so, err := h.sendSpec(w, 1, lines)
	if err != nil {
		t.Fatal(err)
	}
	return so
}

func (h RepositoryHarness) sendSpec(w world, n purchasing.Number, lines []purchasing.LineSpec) (*purchasing.SupplierOrder, error) {
	by, err := shared.NewBackofficePrincipal(w.s.User, w.s.Tenant, shared.RoleOwner)
	if err != nil {
		return nil, err
	}
	return purchasing.Send(purchasing.SendSpec{
		Tenant: w.s.Tenant, ID: shared.SupplierOrderID(h.NewID()), Number: n, Supplier: w.supplier,
		SupplierName: "Couros Paulista", SupplierPhone: "5511987654321", Lines: lines, By: by, Now: epoch,
	})
}

// rebuildNumbered returns so with another number, keeping everything else.
func rebuildNumbered(so *purchasing.SupplierOrder, n purchasing.Number) (*purchasing.SupplierOrder, error) {
	r := so.Record()
	r.Number = n
	return purchasing.Restore(r)
}

func costIsCopied(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	costly := h.NewItem(t, w.s.Tenant, ptr(int64(12345)))
	free := h.NewItem(t, w.s.Tenant, nil)
	so := h.send(t, w.s, w.supplier, epoch, costly, free)
	got := h.stored(t, w.s, so.ID())
	if c := got.Lines[0].Cost; c == nil || *c != brl(t, 12345) {
		t.Errorf("cost of the first line = %v, want R$ 123,45 from its order item", c)
	}
	if got.Lines[1].Cost != nil {
		t.Errorf("cost of the second line = %v, want none: its order item has no cost", got.Lines[1].Cost)
	}
	if got.Lines[0].Paid != nil {
		t.Errorf("paid price of a new line = %v, want none", got.Lines[0].Paid)
	}
}

// Both stores have an order item; the first store names the second's item in
// its own supplier order. The line is refused (an invalid reference), and no
// cost of the other store reaches it.
func otherStoresItemCopiesNoCost(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	other := h.NewStore(t)
	foreign := h.NewItem(t, other.Tenant, ptr(int64(99900)))
	err := h.try(w.s.Scope(), func(ctx context.Context) error {
		n, err := h.Repo.NextNumber(ctx, w.s.Tenant)
		if err != nil {
			return err
		}
		return h.Repo.InsertSupplierOrder(ctx, h.newOrder(t, w.s, w.supplier, n, epoch, foreign), sentText, sentURL)
	})
	if !errors.Is(err, shared.ErrInvalid) {
		t.Fatalf("a line of another store's item: err = %v, want shared.ErrInvalid", err)
	}
	if lines := h.linesOf(t, w.s, foreign.Order); len(lines) != 0 {
		t.Errorf("lines of the other store's order seen from this store = %d, want 0", len(lines))
	}
}

func (h RepositoryHarness) linesOf(t *testing.T, s Store, order shared.OrderID) []apppurchasing.StoredLine {
	t.Helper()
	var out []apppurchasing.StoredLine
	h.in(t, s, func(ctx context.Context) error {
		var err error
		out, err = h.Repo.LinesOfOrder(ctx, s.Tenant, order)
		return err
	})
	return out
}

func oneOpenLinePerItem(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	b := h.item(t, w)
	first := h.send(t, w.s, w.supplier, epoch, b)
	var second *purchasing.SupplierOrder
	err := h.try(w.s.Scope(), func(ctx context.Context) error {
		n, err := h.Repo.NextNumber(ctx, w.s.Tenant)
		if err != nil {
			return err
		}
		second = h.newOrder(t, w.s, w.supplier, n, epoch.Add(time.Hour), b)
		return h.Repo.InsertSupplierOrder(ctx, second, sentText, sentURL)
	})
	if !errors.Is(err, apppurchasing.ErrOpenLine) || !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a second open line for an item: err = %v, want apppurchasing.ErrOpenLine (a conflict)", err)
	}
	h.in(t, w.s, func(ctx context.Context) error {
		if _, err := h.Repo.SupplierOrder(ctx, w.s.Tenant, second.ID()); !errors.Is(err, purchasing.ErrSupplierOrderNotFound) {
			t.Errorf("the refused order: err = %v, want not found", err)
		}
		lines, err := h.Repo.LinesOfOrder(ctx, w.s.Tenant, b.Order)
		if err != nil || len(lines) != 1 || lines[0].SupplierOrder != first.ID() {
			t.Errorf("lines of the order = %d, %v, want only the first send's", len(lines), err)
		}
		return nil
	})
}

func closedLineFreesItem(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	b := h.item(t, w)
	first := h.send(t, w.s, w.supplier, epoch, b)
	so := h.restore(t, w.s, first.ID())
	if _, err := so.Receive(so.Lines()[0].ID(), purchasing.CheckDefective, "Costura aberta", today()); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, so)
	h.send(t, w.s, w.supplier, epoch.Add(time.Hour), b) // a replacement is asked for the defective item
	if n := len(h.linesOf(t, w.s, b.Order)); n != 2 {
		t.Errorf("lines of the order = %d, want the defective one and the new one", n)
	}
}

func takenIdentities(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	first := h.send(t, w.s, w.supplier, epoch, h.item(t, w))
	fresh := func(b Bought, mutate func(*purchasing.Record)) *purchasing.SupplierOrder {
		so := h.newOrder(t, w.s, w.supplier, 90, epoch, b)
		r := so.Record()
		mutate(&r)
		out, err := purchasing.Restore(r)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	for name, mutate := range map[string]func(*purchasing.Record){
		"the id": func(r *purchasing.Record) {
			r.ID = first.ID()
			for i := range r.Lines {
				r.Lines[i].SupplierOrder = r.ID
			}
		},
		"the number":  func(r *purchasing.Record) { r.Number = first.Number() },
		"a line's id": func(r *purchasing.Record) { r.Lines[0].ID = first.Lines()[0].ID() },
	} {
		err := h.try(w.s.Scope(), func(ctx context.Context) error {
			return h.Repo.InsertSupplierOrder(ctx, fresh(h.item(t, w), mutate), sentText, sentURL)
		})
		if !errors.Is(err, shared.ErrConflict) {
			t.Errorf("a taken %s: err = %v, want shared.ErrConflict", name, err)
		}
	}
}

func updateStoresChanges(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	a, b := h.item(t, w), h.item(t, w)
	sent := h.send(t, w.s, w.supplier, epoch, a, b)
	so := h.restore(t, w.s, sent.ID())
	ids := []shared.PurchaseLineID{so.Lines()[0].ID(), so.Lines()[1].ID()}
	later := epoch.Add(2 * time.Hour)
	if _, err := so.MarkInTransit(later); err != nil {
		t.Fatal(err)
	}
	if _, err := so.RecordPromisedDate(today().AddDays(10), map[shared.PurchaseLineID]shared.CivilDate{ids[1]: today().AddDays(12)}, today()); err != nil {
		t.Fatal(err)
	}
	if _, err := so.RecordPaidPrices(map[shared.PurchaseLineID]shared.Money{ids[0]: brl(t, 3900)}); err != nil {
		t.Fatal(err)
	}
	if _, err := so.Receive(ids[0], purchasing.CheckOK, "", today().AddDays(1)); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, so)

	got := h.stored(t, w.s, sent.ID())
	want := so.Record()
	want.Version = 2
	want.Lines[0].Cost, want.Lines[1].Cost = ptr(brl(t, 4000)), ptr(brl(t, 4000))
	sameRecord(t, "updated supplier order", got, want)
	if !got.InTransitAt.Equal(later) || got.Lines[0].Status != purchasing.LineReceived || got.Lines[1].Promised != today().AddDays(12) ||
		got.Lines[0].Paid == nil || *got.Lines[0].Paid != brl(t, 3900) {
		t.Errorf("update lost something: %+v", got)
	}
}

func undoIsStored(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	sent := h.send(t, w.s, w.supplier, epoch, h.item(t, w), h.item(t, w))
	later := epoch.Add(2 * time.Hour)
	undo := h.restore(t, w.s, sent.ID())
	if _, err := undo.Undo(h.principal(t, w.s), later); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, undo)
	back := h.stored(t, w.s, sent.ID())
	if back.UndoneBy != w.s.User || !back.UndoneAt.Equal(later) || back.Lines[1].Status != purchasing.LineCancelled ||
		back.Lines[1].CancelReason != purchasing.CancelUndone || back.Version != 2 {
		t.Errorf("an undone order read back as %+v", back)
	}
	if open := h.openLines(t, w.s); len(open) != 0 {
		t.Errorf("open lines after an undo = %d, want 0", len(open))
	}
}

func updateVersionAndNotFound(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	sent := h.send(t, w.s, w.supplier, epoch, h.item(t, w))
	stale := h.restore(t, w.s, sent.ID())
	fresh := h.restore(t, w.s, sent.ID())
	if _, err := fresh.MarkInTransit(epoch); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, fresh)
	if _, err := stale.MarkInTransit(epoch); err != nil {
		t.Fatal(err)
	}
	err := h.update(w.s, stale)
	if !errors.Is(err, shared.ErrVersionConflict) || !errors.Is(err, shared.ErrConflict) {
		t.Errorf("update with a stale version: err = %v, want shared.ErrVersionConflict", err)
	}
	if got := h.stored(t, w.s, sent.ID()); got.Version != 2 {
		t.Errorf("version after a refused update = %d, want 2", got.Version)
	}
	ghost := h.newOrder(t, w.s, w.supplier, 77, epoch, h.item(t, w))
	if err := h.update(w.s, ghost); !errors.Is(err, purchasing.ErrSupplierOrderNotFound) {
		t.Errorf("update of an unknown order: err = %v, want not found", err)
	}
}

func updateKeepsLinesAndCost(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	sent := h.send(t, w.s, w.supplier, epoch, h.item(t, w))
	r := h.stored(t, w.s, sent.ID())
	extra := r.Lines[0]
	extra.ID = shared.PurchaseLineID(h.NewID())
	more := r
	more.Lines = append(slices.Clone(r.Lines), extra)
	so, err := purchasing.Restore(more)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.update(w.s, so); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("update that adds a line: err = %v, want shared.ErrInvalid", err)
	}

	// A cost a caller sets on a stored line is not written.
	forged := r
	forged.Lines = slices.Clone(r.Lines)
	forged.Lines[0].Cost = ptr(brl(t, 1))
	so, err = purchasing.Restore(forged)
	if err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, so)
	if c := h.stored(t, w.s, sent.ID()).Lines[0].Cost; c == nil || *c != brl(t, 4000) {
		t.Errorf("cost after an update that carried another = %v, want the order-time R$ 40,00", c)
	}
}

func orderOfLine(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	sent := h.send(t, w.s, w.supplier, epoch, h.item(t, w), h.item(t, w))
	counter := h.counterLine(t, w, h.item(t, w), purchasing.CheckOK, "")
	h.in(t, w.s, func(ctx context.Context) error {
		got, err := h.Repo.SupplierOrderOfLine(ctx, w.s.Tenant, sent.Lines()[1].ID())
		if err != nil || got.ID != sent.ID() || len(got.Lines) != 2 {
			t.Errorf("SupplierOrderOfLine = %v, %v, want the sent order with its two lines", got.ID, err)
		}
		return nil
	})
	for name, id := range map[string]shared.PurchaseLineID{
		"an unknown line": shared.PurchaseLineID(h.NewID()), "a counter line": counter.ID(),
	} {
		err := h.try(w.s.Scope(), func(ctx context.Context) error {
			_, err := h.Repo.SupplierOrderOfLine(ctx, w.s.Tenant, id)
			return err
		})
		if !errors.Is(err, purchasing.ErrLineNotFound) {
			t.Errorf("SupplierOrderOfLine of %s: err = %v, want ErrLineNotFound", name, err)
		}
	}
}

func messageReads(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	sent := h.send(t, w.s, w.supplier, epoch, h.item(t, w))
	err := h.try(w.s.Scope(), func(ctx context.Context) error {
		_, err := h.Repo.Message(ctx, w.s.Tenant, shared.SupplierOrderID(h.NewID()))
		return err
	})
	if !errors.Is(err, purchasing.ErrSupplierOrderNotFound) {
		t.Errorf("Message of an unknown order: err = %v, want not found", err)
	}
	h.in(t, w.s, func(ctx context.Context) error {
		m, err := h.Repo.Message(ctx, w.s.Tenant, sent.ID())
		if err != nil || m.Text != sentText || m.URL != sentURL {
			t.Errorf("Message = %+v, %v", m, err)
		}
		return nil
	})
}

func ordersPaging(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	var all []*purchasing.SupplierOrder
	for i := range 3 { // 0 and 1 tie on the sent time
		at := epoch.Add(time.Duration(i/2) * time.Hour)
		all = append(all, h.send(t, w.s, w.supplier, at, h.item(t, w)))
	}
	closed := h.restore(t, w.s, all[2].ID())
	if _, err := closed.Receive(closed.Lines()[0].ID(), purchasing.CheckOK, "", today()); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, w.s, closed)

	newestFirst := slices.Clone(all)
	slices.SortFunc(newestFirst, func(a, b *purchasing.SupplierOrder) int {
		if c := b.SentAt().Compare(a.SentAt()); c != 0 {
			return c
		}
		return strings.Compare(b.ID().String(), a.ID().String())
	})
	want := make([]shared.SupplierOrderID, len(newestFirst))
	for i, so := range newestFirst {
		want[i] = so.ID()
	}
	if got := h.pagesOfTwo(t, w); !slices.Equal(got, want) {
		t.Errorf("supplier orders newest first, in pages of 2\n got: %v\nwant: %v", got, want)
	}

	var open shared.Page[purchasing.Record]
	h.in(t, w.s, func(ctx context.Context) (err error) {
		open, err = h.Repo.SupplierOrders(ctx, w.s.Tenant, true, shared.PageRequest{})
		return err
	})
	for _, r := range open.Items {
		if r.ID == all[2].ID() {
			t.Errorf("open supplier orders include the one with every line received")
		}
	}
	if len(open.Items) != 2 {
		t.Errorf("open supplier orders = %d, want 2", len(open.Items))
	}
}

// pagesOfTwo reads every supplier order of the world's store, two a page,
// and returns their ids in the order they came.
func (h RepositoryHarness) pagesOfTwo(t *testing.T, w world) []shared.SupplierOrderID {
	t.Helper()
	var got []shared.SupplierOrderID
	cursor := ""
	for range 5 {
		var page shared.Page[purchasing.Record]
		h.in(t, w.s, func(ctx context.Context) error {
			p, err := shared.NewPageRequest(cursor, 2)
			if err != nil {
				return err
			}
			page, err = h.Repo.SupplierOrders(ctx, w.s.Tenant, false, p)
			return err
		})
		for _, r := range page.Items {
			got = append(got, r.ID)
		}
		if cursor = page.NextCursor; cursor == "" {
			break
		}
		if len(page.Items) != 2 {
			t.Errorf("a page with a next cursor holds %d orders, want 2", len(page.Items))
		}
	}
	return got
}

func badCursor(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	err := h.try(w.s.Scope(), func(ctx context.Context) error {
		p, err := shared.NewPageRequest("not-a-cursor", 10)
		if err != nil {
			return err
		}
		_, err = h.Repo.SupplierOrders(ctx, w.s.Tenant, false, p)
		return err
	})
	var v *shared.ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("a cursor nobody issued: err = %v, want a *shared.ValidationError", err)
	}
	if len(v.Fields) != 1 || v.Fields[0].Field != "cursor" {
		t.Errorf("validation fields = %+v, want one on cursor", v.Fields)
	}
}
