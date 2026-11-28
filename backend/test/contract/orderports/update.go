package orderports

import (
	"errors"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var updateCases = []contractCase{
	{"stores the next version and every change", updateStoresNextVersion},
	{"a stale version is a conflict and writes nothing", updateStaleVersion},
	{"an order the store lacks is not found", updateMissingOrder},
	{"lines and items are replaced as sets by id", updateReplacesLinesAndItems},
	{"a payment is stored once and a void is recorded", updateVoidsPayment},
}

func updateStoresNextVersion(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s, rich())
	loaded := h.load(t, s, o.ID())
	recordDeposit(t, h, s, loaded, epoch.Add(time.Hour))
	h.mustUpdate(t, s, loaded)
	want := loaded.Record()
	want.Version = 2
	sameRecord(t, h.record(t, s, o.ID()), want)
}

func updateStaleVersion(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s)
	first, second := h.load(t, s, o.ID()), h.load(t, s, o.ID())
	recordDeposit(t, h, s, first, epoch.Add(time.Hour))
	h.mustUpdate(t, s, first)
	recordDeposit(t, h, s, second, epoch.Add(2*time.Hour))
	if err := h.update(s, second); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("stale update err = %v, want shared.ErrVersionConflict", err)
	}
	want := first.Record()
	want.Version = 2
	sameRecord(t, h.record(t, s, o.ID()), want)
	if n := len(h.history(t, s, o.ID(), 50)); n != 2 {
		t.Errorf("history has %d entries after a refused update, want 2 (created, payment)", n)
	}
}

func updateMissingOrder(t *testing.T, h Harness) {
	s := h.newStore(t)
	if err := h.update(s, h.newOrder(t, s, 1)); !errors.Is(err, apporder.ErrNotFound) {
		t.Errorf("update of a missing order err = %v, want apporder.ErrNotFound", err)
	}
}

func updateReplacesLinesAndItems(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s, rich())
	loaded := h.load(t, s, o.ID())
	changeLines(t, h, s, loaded, epoch.Add(2*time.Hour))
	h.mustUpdate(t, s, loaded)
	want := loaded.Record()
	want.Version = 2
	got := h.record(t, s, o.ID())
	sameRecord(t, got, want)
	if len(got.Lines) != 3 {
		t.Errorf("lines = %d, want 3 (one kept, one changed, one new)", len(got.Lines))
	}
	oldItems := map[shared.OrderItemID]bool{}
	for _, l := range o.Lines() {
		for _, it := range l.Items() {
			oldItems[it.ID()] = true
		}
	}
	kept := 0
	for _, l := range got.Lines {
		for _, it := range l.Items {
			if oldItems[it.ID] {
				kept++
			}
		}
	}
	if kept != 2 {
		t.Errorf("%d of the old items survive, want 2: the loose item was dropped", kept)
	}
}

func updateVoidsPayment(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s)
	loaded := h.load(t, s, o.ID())
	id := recordDeposit(t, h, s, loaded, epoch.Add(time.Hour))
	h.mustUpdate(t, s, loaded)
	loaded = h.load(t, s, o.ID())
	if err := loaded.VoidPayment(id, "Lançado errado", s.principal(t), epoch.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, s, loaded)
	want := loaded.Record()
	want.Version = 3
	got := h.record(t, s, o.ID())
	sameRecord(t, got, want)
	if len(got.Payments) != 1 || got.Payments[0].VoidedAt.IsZero() || got.Payments[0].VoidReason != "Lançado errado" {
		t.Errorf("payments = %+v, want the one payment voided", got.Payments)
	}
	// Saving the voided order again leaves its payment as it is.
	again := h.load(t, s, o.ID())
	h.mustUpdate(t, s, again)
	want = again.Record()
	want.Version = 4
	sameRecord(t, h.record(t, s, o.ID()), want)
}

// recordDeposit records the whole deposit due on o, paid at at, and returns
// the payment's id.
func recordDeposit(t *testing.T, h Harness, s store, o *order.Order, at time.Time) shared.PaymentID {
	t.Helper()
	id := shared.PaymentID(h.NewID())
	_, err := o.RecordPayment(order.PaymentSpec{
		ID: id, Kind: order.PaymentDeposit, Amount: o.DepositDue(), Method: order.MethodPix,
		By: s.principal(t), At: at, Today: today(at),
	})
	if err != nil {
		t.Fatalf("record payment: %v", err)
	}
	return id
}

// recordSmall records a payment of R$ 10,00 on o, which adds one history
// entry.
func recordSmall(t *testing.T, h Harness, s store, o *order.Order, at time.Time) {
	t.Helper()
	_, err := o.RecordPayment(order.PaymentSpec{
		ID: shared.PaymentID(h.NewID()), Kind: order.PaymentDeposit, Amount: brl(t, 1000), Method: order.MethodCash,
		By: s.principal(t), At: at, Today: today(at),
	})
	if err != nil {
		t.Fatalf("record payment: %v", err)
	}
}

// lineSpec turns a line back into the spec that makes it, as the app layer
// does for a line it keeps.
func lineSpec(l order.Line) order.LineSpec {
	ls := order.LineSpec{ID: l.ID(), Price: l.Price()}
	ls.AssemblyDays, ls.SafetyMarginDays = l.KitDays()
	if kit, ok := l.Kit(); ok {
		ls.Kit = &kit
	}
	for _, it := range l.Items() {
		ls.Items = append(ls.Items, order.ItemSpec{ID: it.ID(), Snapshot: it.Snapshot(), Sourcing: it.Sourcing()})
	}
	return ls
}

// changeLines keeps the first line, changes the item of the last one (its
// lead time and supplier), drops the middle one and adds a new line.
func changeLines(t *testing.T, h Harness, s store, o *order.Order, now time.Time) {
	t.Helper()
	old := o.Lines()
	last := lineSpec(old[2])
	last.Items[0].Snapshot.LeadDays = 9
	last.Items[0].Sourcing.SupplierName = "Outro fornecedor"
	fresh := h.looseLine(t, h.itemSpec(t, catalog.ModeStock, "Peça nova", 2, true))
	_, err := o.Change(order.ChangeSpec{
		Lines: []order.LineSpec{lineSpec(old[0]), last, fresh}, Terms: o.Terms(),
		By: s.principal(t), Now: now, Today: today(now),
	})
	if err != nil {
		t.Fatalf("change: %v", err)
	}
}
