package purchasing_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

func TestSend_RefusesEmptyAndTwice(t *testing.T) {
	t.Parallel()
	a := item("Banco", 1)
	twice := a
	twice.Quantity = 2
	tests := []struct {
		name  string
		edit  func(*purchasing.SendSpec)
		want  error
		field string
	}{
		{"no lines", func(s *purchasing.SendSpec) { s.Lines = nil }, purchasing.ErrEmptyPick, "lines"},
		{"order item twice", func(s *purchasing.SendSpec) {
			s.Lines = []purchasing.LineSpec{spec(a), spec(twice)}
		}, purchasing.ErrItemPickedTwice, "lines[1].orderItemId"},
		{"quantity 0", func(s *purchasing.SendSpec) { s.Lines[0].Item.Quantity = 0 }, shared.ErrInvalid, "lines[0].quantity"},
		{"quantity 100", func(s *purchasing.SendSpec) { s.Lines[0].Item.Quantity = 100 }, shared.ErrInvalid, "lines[0].quantity"},
		{"blank name", func(s *purchasing.SendSpec) { s.Lines[0].Item.Name = "  " }, shared.ErrInvalid, "lines[0].name"},
		{"blank supplier name", func(s *purchasing.SendSpec) { s.SupplierName = "" }, shared.ErrInvalid, "supplierName"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := sendOf(t, item("Banco", 1)).spec
			tc.edit(&s)
			got, err := purchasing.Send(s)
			if !errors.Is(err, tc.want) || got != nil {
				t.Fatalf("Send = %v, %v; want %v", got, err, tc.want)
			}
			if f := fieldOf(t, err); f != tc.field {
				t.Errorf("field = %q, want %q", f, tc.field)
			}
		})
	}
}

func TestSend_MissingArgumentsAreBugs(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]func(*purchasing.SendSpec){
		"tenant":     func(s *purchasing.SendSpec) { s.Tenant = shared.TenantID{} },
		"id":         func(s *purchasing.SendSpec) { s.ID = shared.SupplierOrderID{} },
		"number":     func(s *purchasing.SendSpec) { s.Number = 0 },
		"supplier":   func(s *purchasing.SendSpec) { s.Supplier = shared.SupplierID{} },
		"by":         func(s *purchasing.SendSpec) { s.By = shared.Principal{} },
		"now":        func(s *purchasing.SendSpec) { s.Now = time.Time{} },
		"line id":    func(s *purchasing.SendSpec) { s.Lines[0].ID = shared.PurchaseLineID{} },
		"order item": func(s *purchasing.SendSpec) { s.Lines[0].Item.OrderItem = shared.OrderItemID{} },
		"order":      func(s *purchasing.SendSpec) { s.Lines[0].Item.Order = shared.OrderID{} },
		"catalog":    func(s *purchasing.SendSpec) { s.Lines[0].Item.CatalogItem = shared.ItemID{} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sendOf(t, item("Banco", 1)).spec
			edit(&s)
			_, err := purchasing.Send(s)
			if err == nil || errors.Is(err, shared.ErrInvalid) {
				t.Errorf("Send = %v, want a non-invalid error (a bug in the caller)", err)
			}
		})
	}
}

func TestSend_BuildsSentLines(t *testing.T) {
	t.Parallel()
	f := sendOf(t, item("Banco", 2, choice("Cor", "Preto")), item("Pedal", 1))
	o, err := purchasing.Send(f.spec)
	if err != nil {
		t.Fatal(err)
	}
	if o.Number().String() != "C-0012" || o.Tenant() != f.tenant || o.SupplierName() != "Moto Peças Ltda" ||
		o.SentBy() != f.by.UserID() || !o.SentAt().Equal(day1) || o.Version() != 1 || o.State() != purchasing.StateSent {
		t.Errorf("order = %+v", o.Record())
	}
	lines := o.Lines()
	if len(lines) != 2 {
		t.Fatalf("%d lines, want 2", len(lines))
	}
	for i, l := range lines {
		if l.Status() != purchasing.LineSent || l.SupplierOrder() != o.ID() || l.Supplier() != o.Supplier() ||
			l.ID() != f.spec.Lines[i].ID || l.Paid() != nil {
			t.Errorf("line %d = %+v", i, l.Record())
		}
	}
	// A sent line has no cost of its own: the repository copies it from the
	// order's sourcing, whoever sends (security review of 5.1).
	if lines[0].Cost() != nil || lines[1].Cost() != nil {
		t.Errorf("costs = %v, %v, want none from the domain", lines[0].Cost(), lines[1].Cost())
	}
	// Getters hand out copies: changing them leaves the order alone.
	got := lines[0].Item()
	got.Choices[0].Label = "changed"
	if o.Lines()[0].Item().Choices[0].Label != "Preto" {
		t.Error("Item() shares its choices with the line")
	}
	if err := o.CheckVersion(1); err != nil || !errors.Is(o.CheckVersion(2), shared.ErrVersionConflict) {
		t.Errorf("CheckVersion: %v", err)
	}
	if _, ok := o.Line(lines[1].ID()); !ok {
		t.Error("Line(id) did not find the line")
	}
	if _, ok := o.Line(lineID()); ok {
		t.Error("Line(unknown) found a line")
	}
}

func TestSupplierOrder_State(t *testing.T) {
	t.Parallel()
	a, b := item("A", 1), item("B", 1)
	o := sent(t, a, b)
	if o.State() != purchasing.StateSent {
		t.Fatalf("State = %s, want sent", o.State())
	}
	items, err := o.MarkInTransit(day1)
	if err != nil || len(items) != 2 || o.State() != purchasing.StateInTransit || !o.InTransitAt().Equal(day1) {
		t.Fatalf("MarkInTransit = %v, %v, state %s", items, err, o.State())
	}
	if _, err := o.MarkInTransit(day1); !errors.Is(err, purchasing.ErrNothingOpen) {
		t.Errorf("second MarkInTransit error = %v, want ErrNothingOpen", err)
	}
	for _, l := range o.Lines() {
		if _, err := o.Receive(l.ID(), purchasing.CheckOK, "", date(t, "2026-12-10")); err != nil {
			t.Fatal(err)
		}
	}
	if o.State() != purchasing.StateReceived {
		t.Errorf("State = %s, want received", o.State())
	}
}

func TestMarkInTransit_SkipsRemovedItemsAndRefusesUndone(t *testing.T) {
	t.Parallel()
	removed := item("A", 1)
	removed.OrderItem = shared.OrderItemID{}
	// A line of an item removed from its order is stored with a zero order item.
	r := sent(t, item("B", 1)).Record()
	r.Lines[0].Item = removed
	o, err := purchasing.Restore(r)
	if err != nil {
		t.Fatal(err)
	}
	if items, err := o.MarkInTransit(day1); err != nil || len(items) != 0 {
		t.Errorf("MarkInTransit = %v, %v; want no item to move, no error", items, err)
	}
	u := sent(t, item("C", 1))
	if _, err := u.Undo(testkit.Staff(t, u.Tenant(), shared.RoleOwner), day1); err != nil {
		t.Fatal(err)
	}
	if _, err := u.MarkInTransit(day1); !errors.Is(err, purchasing.ErrUndone) {
		t.Errorf("MarkInTransit on undone = %v, want ErrUndone", err)
	}
}

func TestUndo_RefusedOnceReceived(t *testing.T) {
	t.Parallel()
	a, b := item("A", 1), item("B", 1)
	o := sent(t, a, b)
	owner := testkit.Staff(t, o.Tenant(), shared.RoleOwner)
	first := o.Lines()[0].ID()
	if _, err := o.Receive(first, purchasing.CheckOK, "", date(t, "2026-12-10")); err != nil {
		t.Fatal(err)
	}
	if o.CanUndo() {
		t.Error("CanUndo = true after a line was received")
	}
	if items, err := o.Undo(owner, day1); !errors.Is(err, purchasing.ErrSupplierOrderReceived) || items != nil {
		t.Fatalf("Undo = %v, %v; want ErrSupplierOrderReceived", items, err)
	}
	if o.State() == purchasing.StateUndone || o.Lines()[1].Status() != purchasing.LineSent {
		t.Error("a refused Undo changed the order")
	}

	// A defect counts as received too.
	d := sent(t, item("D", 1))
	if _, err := d.Receive(d.Lines()[0].ID(), purchasing.CheckDefective, "trincado", date(t, "2026-12-10")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Undo(owner, day1); !errors.Is(err, purchasing.ErrSupplierOrderReceived) {
		t.Errorf("Undo after a defect = %v, want ErrSupplierOrderReceived", err)
	}
}

func TestUndo_CancelsOpenLines(t *testing.T) {
	t.Parallel()
	a, b := item("A", 1), item("B", 1)
	o := sent(t, a, b)
	owner := testkit.Staff(t, o.Tenant(), shared.RoleOwner)
	// A resolved notice stays resolved when the order is undone later.
	if _, err := o.Resolve(o.Lines()[1].ID()); err != nil {
		t.Fatal(err)
	}
	if !o.CanUndo() {
		t.Fatal("CanUndo = false with only cancelled and sent lines")
	}
	items, err := o.Undo(owner, day1)
	if err != nil || !slices.Equal(items, []shared.OrderItemID{a.OrderItem}) {
		t.Fatalf("Undo = %v, %v; want only the open line's item", items, err)
	}
	lines := o.Lines()
	if lines[0].CancelReason() != purchasing.CancelUndone || lines[1].CancelReason() != purchasing.CancelResolved ||
		o.State() != purchasing.StateUndone || o.UndoneBy() != owner.UserID() || !o.UndoneAt().Equal(day1) || o.CanUndo() {
		t.Errorf("after Undo: %+v", o.Record())
	}
	if _, err := o.Undo(owner, day1); !errors.Is(err, purchasing.ErrUndone) {
		t.Errorf("second Undo = %v, want ErrUndone", err)
	}
}

func TestRecordPromisedDate_OverridesAndPastDays(t *testing.T) {
	t.Parallel()
	today := date(t, "2026-12-01")
	type overrides = map[shared.PurchaseLineID]shared.CivilDate
	tests := []struct {
		name      string
		date      shared.CivilDate
		overrides func(ids []shared.PurchaseLineID) overrides
		want      error
		field     string
	}{
		{"date in the past", date(t, "2026-11-30"), nil, purchasing.ErrDateInPast, "date"},
		{"zero date", shared.CivilDate{}, nil, shared.ErrInvalid, "date"},
		{"override in the past", date(t, "2026-12-10"), func(ids []shared.PurchaseLineID) overrides {
			return overrides{ids[1]: date(t, "2026-11-01")}
		}, purchasing.ErrDateInPast, "lines[1].date"},
		{"override zero", date(t, "2026-12-10"), func(ids []shared.PurchaseLineID) overrides {
			return overrides{ids[2]: {}}
		}, shared.ErrInvalid, "lines[2].date"},
		{"override of a stranger", date(t, "2026-12-10"), func([]shared.PurchaseLineID) overrides {
			return overrides{lineID(): date(t, "2026-12-11")}
		}, purchasing.ErrLineNotFound, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := sent(t, item("A", 1), item("B", 1), item("C", 1))
			var ov overrides
			if tc.overrides != nil {
				ov = tc.overrides([]shared.PurchaseLineID{o.Lines()[0].ID(), o.Lines()[1].ID(), o.Lines()[2].ID()})
			}
			got, err := o.RecordPromisedDate(tc.date, ov, today)
			if !errors.Is(err, tc.want) || got != nil {
				t.Fatalf("RecordPromisedDate = %v, %v; want %v", got, err, tc.want)
			}
			if tc.field != "" {
				if f := fieldOf(t, err); f != tc.field {
					t.Errorf("field = %q, want %q", f, tc.field)
				}
			}
			if !o.Promised().IsZero() {
				t.Error("a refused call changed the promised date")
			}
		})
	}

	t.Run("sets the date and the overrides", func(t *testing.T) {
		t.Parallel()
		a, b, c := item("A", 1), item("B", 1), item("C", 1)
		o := sent(t, a, b, c)
		ids := []shared.PurchaseLineID{o.Lines()[0].ID(), o.Lines()[1].ID(), o.Lines()[2].ID()}
		got, err := o.RecordPromisedDate(date(t, "2026-12-10"), overrides{ids[1]: date(t, "2026-12-20")}, today)
		if err != nil {
			t.Fatal(err)
		}
		want := []purchasing.ItemDate{
			{Order: a.Order, Item: a.OrderItem, Date: date(t, "2026-12-10")},
			{Order: b.Order, Item: b.OrderItem, Date: date(t, "2026-12-20")},
			{Order: c.Order, Item: c.OrderItem, Date: date(t, "2026-12-10")},
		}
		if !slices.Equal(got, want) || o.Promised() != date(t, "2026-12-10") {
			t.Errorf("RecordPromisedDate = %v, want %v", got, want)
		}
		if o.PromisedFor(o.Lines()[1]) != date(t, "2026-12-20") || o.PromisedFor(o.Lines()[0]) != date(t, "2026-12-10") {
			t.Error("PromisedFor does not follow the override, then the order's date")
		}
		// A second call is the full set: the override is gone.
		if _, err := o.RecordPromisedDate(date(t, "2026-12-12"), nil, today); err != nil {
			t.Fatal(err)
		}
		if !o.Lines()[1].Promised().IsZero() || o.PromisedFor(o.Lines()[1]) != date(t, "2026-12-12") {
			t.Error("an override survived a call that did not repeat it")
		}
	})

	t.Run("closed lines are not redated", func(t *testing.T) {
		t.Parallel()
		a, b := item("A", 1), item("B", 1)
		o := sent(t, a, b)
		closed := o.Lines()[0].ID()
		if _, err := o.Receive(closed, purchasing.CheckOK, "", today); err != nil {
			t.Fatal(err)
		}
		_, err := o.RecordPromisedDate(date(t, "2026-12-10"), overrides{closed: date(t, "2026-12-11")}, today)
		if !errors.Is(err, purchasing.ErrLineClosed) {
			t.Errorf("override on a closed line = %v, want ErrLineClosed", err)
		}
		got, err := o.RecordPromisedDate(date(t, "2026-12-10"), nil, today)
		if err != nil || len(got) != 1 || got[0].Item != b.OrderItem {
			t.Errorf("RecordPromisedDate = %v, %v; want the open line only", got, err)
		}
	})

	t.Run("a removed item is left out", func(t *testing.T) {
		t.Parallel()
		r := sent(t, item("A", 1)).Record()
		r.Lines[0].Item.OrderItem = shared.OrderItemID{}
		o, err := purchasing.Restore(r)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := o.RecordPromisedDate(date(t, "2026-12-10"), nil, today); err != nil || len(got) != 0 {
			t.Errorf("RecordPromisedDate = %v, %v; want nothing to redate", got, err)
		}
	})

	t.Run("today is required", func(t *testing.T) {
		t.Parallel()
		o := sent(t, item("A", 1))
		if _, err := o.RecordPromisedDate(date(t, "2026-12-10"), nil, shared.CivilDate{}); err == nil || errors.Is(err, shared.ErrInvalid) {
			t.Errorf("error = %v, want a caller bug", err)
		}
	})
}

func TestRecordPaidPrices(t *testing.T) {
	t.Parallel()
	a, b := item("A", 1), item("B", 1)
	o := sent(t, a, b)
	ids := []shared.PurchaseLineID{o.Lines()[0].ID(), o.Lines()[1].ID()}

	for name, tc := range map[string]struct {
		prices map[shared.PurchaseLineID]shared.Money
		want   error
		field  string
	}{
		"a stranger": {map[shared.PurchaseLineID]shared.Money{lineID(): brl(t, 1)}, purchasing.ErrLineNotFound, ""},
		"negative":   {map[shared.PurchaseLineID]shared.Money{ids[1]: brl(t, -5)}, shared.ErrInvalid, "lines[1].paid"},
		"too large":  {map[shared.PurchaseLineID]shared.Money{ids[0]: brl(t, 100_000_000_001)}, shared.ErrInvalid, "lines[0].paid"},
	} {
		got, err := o.RecordPaidPrices(tc.prices)
		if !errors.Is(err, tc.want) || got != nil {
			t.Fatalf("%s: RecordPaidPrices = %v, %v; want %v", name, got, err, tc.want)
		}
		if tc.field != "" && fieldOf(t, err) != tc.field {
			t.Errorf("%s: field = %q, want %q", name, fieldOf(t, err), tc.field)
		}
	}
	if o.Lines()[0].Paid() != nil || o.Lines()[1].Paid() != nil {
		t.Error("a refused call recorded a price")
	}

	got, err := o.RecordPaidPrices(map[shared.PurchaseLineID]shared.Money{ids[1]: brl(t, 900), ids[0]: brl(t, 1_250)})
	want := []purchasing.ItemPrice{
		{Order: a.Order, Item: a.OrderItem, Price: brl(t, 1_250)},
		{Order: b.Order, Item: b.OrderItem, Price: brl(t, 900)},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("RecordPaidPrices = %v, %v; want %v", got, err, want)
	}
	if p := o.Lines()[1].Paid(); p == nil || p.Cents() != 900 || p.Currency() != "BRL" {
		t.Errorf("paid = %v", p)
	}

	// A cancelled line takes no price; a received one still does (the paid
	// price is often known only after the goods arrive).
	if _, err := o.Receive(ids[0], purchasing.CheckOK, "", date(t, "2026-12-02")); err != nil {
		t.Fatal(err)
	}
	if _, err := o.RecordPaidPrices(map[shared.PurchaseLineID]shared.Money{ids[0]: brl(t, 1_300)}); err != nil {
		t.Errorf("price on a received line: %v", err)
	}
	if _, err := o.Resolve(ids[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := o.RecordPaidPrices(map[shared.PurchaseLineID]shared.Money{ids[1]: brl(t, 1)}); !errors.Is(err, purchasing.ErrLineClosed) {
		t.Errorf("price on a cancelled line = %v, want ErrLineClosed", err)
	}
}

func TestReceive_DefectNeedsNote(t *testing.T) {
	t.Parallel()
	today := date(t, "2026-12-05")
	long := strings.Repeat("ã", 501)
	tests := []struct {
		name  string
		check purchasing.Check
		note  string
		want  error
		field string
		keep  string
	}{
		{"ok keeps no note", purchasing.CheckOK, "sobrou nota", nil, "", ""},
		{"defect with a note", purchasing.CheckDefective, "  riscado no couro ", nil, "", "riscado no couro"},
		{"defect at 500 characters", purchasing.CheckDefective, strings.Repeat("ã", 500), nil, "", strings.Repeat("ã", 500)},
		{"defect without a note", purchasing.CheckDefective, "", shared.ErrInvalid, "note", ""},
		{"defect with a blank note", purchasing.CheckDefective, "   ", shared.ErrInvalid, "note", ""},
		{"defect over 500 characters", purchasing.CheckDefective, long, shared.ErrInvalid, "note", ""},
		{"unknown check", purchasing.Check("meh"), "", shared.ErrInvalid, "check", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := sent(t, item("A", 1))
			id := o.Lines()[0].ID()
			got, err := o.Receive(id, tc.check, tc.note, today)
			if tc.want != nil {
				if !errors.Is(err, tc.want) || fieldOf(t, err) != tc.field {
					t.Fatalf("Receive = %v; want %v on %q", err, tc.want, tc.field)
				}
				if o.Lines()[0].Status() != purchasing.LineSent {
					t.Error("a refused Receive changed the line")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := purchasing.LineReceived
			if tc.check == purchasing.CheckDefective {
				wantStatus = purchasing.LineDefective
			}
			if got.Status() != wantStatus || got.CheckNote() != tc.keep || got.ReceivedOn() != today || o.Lines()[0].Status() != wantStatus {
				t.Errorf("line = %+v", got.Record())
			}
		})
	}

	t.Run("a line is received once", func(t *testing.T) {
		t.Parallel()
		o := sent(t, item("A", 1))
		id := o.Lines()[0].ID()
		if _, err := o.Receive(id, purchasing.CheckOK, "", today); err != nil {
			t.Fatal(err)
		}
		if _, err := o.Receive(id, purchasing.CheckOK, "", today); !errors.Is(err, purchasing.ErrLineClosed) || !errors.Is(err, shared.ErrConflict) {
			t.Errorf("second Receive = %v, want ErrLineClosed", err)
		}
	})
	t.Run("a stranger", func(t *testing.T) {
		t.Parallel()
		if _, err := sent(t, item("A", 1)).Receive(lineID(), purchasing.CheckOK, "", today); !errors.Is(err, purchasing.ErrLineNotFound) {
			t.Errorf("Receive = %v, want ErrLineNotFound", err)
		}
	})
	t.Run("in transit lines are received too", func(t *testing.T) {
		t.Parallel()
		o := sent(t, item("A", 1))
		if _, err := o.MarkInTransit(day1); err != nil {
			t.Fatal(err)
		}
		if _, err := o.Receive(o.Lines()[0].ID(), purchasing.CheckOK, "", today); err != nil {
			t.Errorf("Receive on an in transit line: %v", err)
		}
	})
	t.Run("today is required", func(t *testing.T) {
		t.Parallel()
		o := sent(t, item("A", 1))
		if _, err := o.Receive(o.Lines()[0].ID(), purchasing.CheckOK, "", shared.CivilDate{}); err == nil || errors.Is(err, shared.ErrInvalid) {
			t.Errorf("error = %v, want a caller bug", err)
		}
	})
}

func TestResolve(t *testing.T) {
	t.Parallel()
	o := sent(t, item("A", 1), item("B", 1))
	id := o.Lines()[0].ID()
	got, err := o.Resolve(id)
	if err != nil || got.Status() != purchasing.LineCancelled || got.CancelReason() != purchasing.CancelResolved {
		t.Fatalf("Resolve = %+v, %v", got.Record(), err)
	}
	if _, err := o.Resolve(id); !errors.Is(err, purchasing.ErrLineClosed) {
		t.Errorf("second Resolve = %v, want ErrLineClosed", err)
	}
	if _, err := o.Resolve(lineID()); !errors.Is(err, purchasing.ErrLineNotFound) {
		t.Errorf("Resolve of a stranger = %v, want ErrLineNotFound", err)
	}
	if o.State() != purchasing.StateSent {
		t.Errorf("State = %s, want sent while the other line waits", o.State())
	}
}

func TestRestore_RoundTripAndBrokenRows(t *testing.T) {
	t.Parallel()
	o := sent(t, item("A", 2, choice("Cor", "Preto")), item("B", 1))
	if _, err := o.Receive(o.Lines()[0].ID(), purchasing.CheckDefective, "trincado", date(t, "2026-12-05")); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Resolve(o.Lines()[1].ID()); err != nil {
		t.Fatal(err)
	}
	got, err := purchasing.Restore(o.Record())
	if err != nil || !reflect.DeepEqual(got.Record(), o.Record()) {
		t.Fatalf("Restore round trip = %v\n%+v\nwant\n%+v", err, got, o.Record())
	}

	tests := map[string]func(*purchasing.Record){
		"no tenant":        func(r *purchasing.Record) { r.Tenant = shared.TenantID{} },
		"no id":            func(r *purchasing.Record) { r.ID = shared.SupplierOrderID{} },
		"number 0":         func(r *purchasing.Record) { r.Number = 0 },
		"no lines":         func(r *purchasing.Record) { r.Lines = nil },
		"version 0":        func(r *purchasing.Record) { r.Version = 0 },
		"no sent time":     func(r *purchasing.Record) { r.SentAt = time.Time{} },
		"line elsewhere":   func(r *purchasing.Record) { r.Lines[0].SupplierOrder = shared.SupplierOrderID(testkit.NextID()) },
		"line of other":    func(r *purchasing.Record) { r.Lines[0].Supplier = shared.SupplierID(testkit.NextID()) },
		"unknown status":   func(r *purchasing.Record) { r.Lines[0].Status = "lost" },
		"no cancel reason": func(r *purchasing.Record) { r.Lines[1].CancelReason = "" },
		"no received day":  func(r *purchasing.Record) { r.Lines[0].ReceivedOn = shared.CivilDate{} },
		"defect no note":   func(r *purchasing.Record) { r.Lines[0].CheckNote = "" },
		"bad quantity":     func(r *purchasing.Record) { r.Lines[0].Item.Quantity = 0 },
		"no line id":       func(r *purchasing.Record) { r.Lines[0].ID = shared.PurchaseLineID{} },
	}
	for name, edit := range tests {
		r := o.Record()
		edit(&r)
		if _, err := purchasing.Restore(r); !errors.Is(err, purchasing.ErrInvalidRecord) || errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: Restore = %v, want ErrInvalidRecord and not a 400", name, err)
		}
	}
}

func TestRecordPromisedDate_RefusesUndone(t *testing.T) {
	t.Parallel()
	u := sent(t, item("C", 1))
	if _, err := u.Undo(testkit.Staff(t, u.Tenant(), shared.RoleOwner), day1); err != nil {
		t.Fatal(err)
	}
	if _, err := u.RecordPromisedDate(date(t, "2026-12-20"), nil, date(t, "2026-12-02")); !errors.Is(err, purchasing.ErrUndone) {
		t.Errorf("RecordPromisedDate on undone = %v, want ErrUndone", err)
	}
	if !u.Promised().IsZero() {
		t.Error("an undone order took a promised date")
	}
}
