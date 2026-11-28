package order_test

import (
	"context"
	"errors"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/app/order/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

func (h *harness) payBy(ctx context.Context, t *testing.T, v apporder.View, kind order.PaymentKind, cents int64, m order.PaymentMethod) apporder.View {
	t.Helper()
	out, err := h.svc.RecordPayment(ctx, apporder.PaymentInput{Order: v.ID, Kind: kind, AmountCents: cents, Method: m, Version: v.Version, Idempotency: h.key()})
	if err != nil {
		t.Fatalf("RecordPayment: %v", err)
	}
	return out
}

// addTampa adds a to_order item sold at R$ 200,00 whose cost nobody filled in.
func (h *harness) addTampa(t *testing.T) shared.ItemID {
	t.Helper()
	id := shared.ItemID(h.ids.NewID())
	h.catalog.AddItem(fake.CatalogItem{
		ID: id, Name: "Tampa", Mode: catalog.ModeToOrder, Price: testkit.BRL(t, 20_000), LeadDays: 5,
		Sourcing: order.Sourcing{Supplier: shared.SupplierID(h.ids.NewID()), SupplierName: "Tampas Ltda"},
	})
	return id
}

// deliver pays the rest of a released order, walks it to the handed over
// status and hands it over at the counter.
func (h *harness) deliver(ctx context.Context, t *testing.T, v apporder.View) apporder.View {
	t.Helper()
	v = h.walk(ctx, t, v, "awaiting_deposit", "purchasing", "items_arriving")
	v = h.arrive(ctx, t, v)
	v = h.walk(ctx, t, v, "assembling", "ready")
	v = h.payBy(ctx, t, v, order.PaymentBalance, v.BalanceDue.Cents(), order.MethodPix)
	pickup := order.HandoverPickup
	return h.advance(ctx, t, v, "delivered", &pickup)
}

func day2(t *testing.T, s string) shared.CivilDate { return testkit.Date(t, s) }

func TestFinance_TotalsForThePeriod(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	staff, owner := h.staff(t), h.owner(t)
	maria := h.addCustomer("Maria Souza", "5592987654321")
	two := func(in *apporder.CreateInput) { in.Lines = []apporder.LineRequest{{Item: h.banco, Quantity: 2}} }

	// A: R$ 500 of R$ 1.000 paid. B: the deposit was voided. C: R$ 700 paid.
	a := h.released(staff, t, nil)
	b := h.create(staff, t, func(in *apporder.CreateInput) { in.Customer = maria.ID }).Order
	b = h.payBy(staff, t, b, order.PaymentDeposit, 50_000, order.MethodCash)
	if _, err := h.svc.VoidPayment(owner, apporder.VoidPaymentInput{Order: b.ID, Payment: b.Payments[0].ID, Reason: "Valor errado", Version: b.Version}); err != nil {
		t.Fatal(err)
	}
	c := h.released(staff, t, nil)
	h.payBy(staff, t, c, order.PaymentBalance, 20_000, order.MethodBankTransfer)
	// D: paid R$ 2.000, then changed to R$ 1.000: R$ 1.000 to give back.
	d := h.create(staff, t, two).Order
	d = h.payBy(staff, t, d, order.PaymentDeposit, 100_000, order.MethodCash)
	d = h.payBy(staff, t, d, order.PaymentBalance, 100_000, order.MethodCash)
	if _, err := h.svc.Change(h.admin(t), apporder.ChangeInput{
		Order: d.ID, Lines: []apporder.LineRequest{h.bancoLine()}, Terms: apporder.TermsInput{Handover: order.HandoverPickup},
		CustomerApproved: true, Version: d.Version,
	}); err != nil {
		t.Fatal(err)
	}
	// E and F were handed over today; F's item has no cost.
	e := h.deliver(staff, t, h.released(staff, t, nil))
	h.clock.Advance(time.Hour)
	tampa := h.addTampa(t)
	f := h.deliver(staff, t, h.released(staff, t, func(in *apporder.CreateInput) { in.Lines = []apporder.LineRequest{{Item: tampa, Quantity: 1}} }))

	got, err := h.svc.Finance(owner, day2(t, "2026-12-01"), day2(t, "2026-12-01"))
	if err != nil {
		t.Fatal(err)
	}
	if got.From != day2(t, "2026-12-01") || got.To != day2(t, "2026-12-01") {
		t.Errorf("period = %s..%s", got.From, got.To)
	}
	// 50 + 70 + 200 + 100 (E) + 20 (F) = R$ 4.400, the void left out.
	if got.Received.Total.Cents() != 440_000 || got.Received.Total.Currency() != "BRL" {
		t.Errorf("received = %v", got.Received.Total)
	}
	assertMoney(t, "by kind", got.Received.ByKind, map[string]int64{"deposit": 260_000, "balance": 180_000})
	assertMoney(t, "by method", got.Received.ByMethod, map[string]int64{"pix": 220_000, "dinheiro": 200_000, "transferencia": 20_000})
	if got.ToReceive.Total.Cents() != 180_000 || len(got.ToReceive.Orders) != 3 {
		t.Errorf("to receive = %v over %d orders, want A, B and C: R$ 1.800", got.ToReceive.Total, len(got.ToReceive.Orders))
	}
	for i, id := range []shared.OrderID{a.ID, b.ID, c.ID} {
		if s := got.ToReceive.Orders[i]; s.ID != id || s.Customer.Name == "" {
			t.Errorf("to receive %d = %+v", i, s)
		}
	}
	if len(got.ToRefund) != 1 || got.ToRefund[0].Order.ID != d.ID || got.ToRefund[0].Amount.Cents() != 100_000 {
		t.Errorf("to refund = %+v", got.ToRefund)
	}
	if len(got.Margins) != 2 {
		t.Fatalf("margins = %+v", got.Margins)
	}
	known, unknown := got.Margins[0], got.Margins[1]
	if known.Order != e.ID || known.Number != e.Number || known.CustomerName != "João Pereira" || known.HandedOverOn != day2(t, "2026-12-01") ||
		known.Total.Cents() != 100_000 || known.Cost == nil || known.Cost.Cents() != 40_000 || known.Margin == nil || *known.Margin != (catalog.Margin{Cents: 60_000, BasisPoints: 6000}) {
		t.Errorf("known margin row = %+v", known)
	}
	if unknown.Order != f.ID || unknown.Total.Cents() != 20_000 || unknown.Cost != nil || unknown.Margin != nil {
		t.Errorf("unknown cost row = %+v, want no cost and no margin", unknown)
	}

	later, err := h.svc.Finance(owner, day2(t, "2026-12-02"), day2(t, "2026-12-30"))
	if err != nil {
		t.Fatal(err)
	}
	if later.Received.Total.Cents() != 0 || len(later.Received.ByKind) != 0 || len(later.Margins) != 0 || later.ToReceive.Total.Cents() != 180_000 || len(later.ToRefund) != 1 {
		t.Errorf("a later period = %+v: nothing received or handed over, but the debts stay", later)
	}
}

func assertMoney(t *testing.T, what string, got []apporder.MoneyByKey, want map[string]int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s = %+v, want %v", what, got, want)
		return
	}
	for i, m := range got {
		if m.Total.Cents() != want[m.Key] || (i > 0 && got[i-1].Key >= m.Key) {
			t.Errorf("%s = %+v, want %v sorted by key", what, got, want)
		}
	}
}

// A period is whole days of the store's calendar, not of UTC.
func TestFinance_PeriodIsInTheStoresZone(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		zone      string
		at        time.Time
		inDec1    bool
		inDec2    bool
		whichName string
	}{
		{"America/Sao_Paulo", time.Date(2026, 12, 2, 2, 59, 59, 0, time.UTC), true, false, "23:59:59 on the 1st"},
		{"America/Sao_Paulo", time.Date(2026, 12, 2, 3, 0, 0, 0, time.UTC), false, true, "midnight of the 2nd"},
		{"America/Manaus", time.Date(2026, 12, 2, 3, 0, 0, 0, time.UTC), true, false, "23:00 on the 1st"},
		{"America/Manaus", time.Date(2026, 12, 2, 4, 0, 0, 0, time.UTC), false, true, "midnight of the 2nd"},
	} {
		t.Run(tc.zone+" "+tc.whichName, func(t *testing.T) {
			t.Parallel()
			h := newHarnessIn(t, tc.zone)
			v := h.create(h.staff(t), t, nil).Order
			h.clock.Set(tc.at)
			h.pay(h.staff(t), t, v, order.PaymentDeposit, 50_000)
			for date, want := range map[string]bool{"2026-12-01": tc.inDec1, "2026-12-02": tc.inDec2} {
				got, err := h.svc.Finance(h.owner(t), day2(t, date), day2(t, date))
				if err != nil {
					t.Fatal(err)
				}
				if has := got.Received.Total.Cents() == 50_000; has != want {
					t.Errorf("on %s: received %v, want counted = %v", date, got.Received.Total, want)
				}
			}
		})
	}
}

func TestFinance_RefusesBadPeriods(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var zero shared.CivilDate
	d := day2(t, "2026-01-01")
	for name, tc := range map[string]struct{ from, to shared.CivilDate }{
		"from after to": {day2(t, "2026-12-02"), day2(t, "2026-12-01")},
		"367 days":      {d, day2(t, "2027-01-02")},
		"no from":       {zero, d},
		"no to":         {d, zero},
	} {
		if _, err := h.svc.Finance(h.owner(t), tc.from, tc.to); !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: err = %v, want invalid", name, err)
		}
	}
	for name, to := range map[string]string{"one day": "2026-01-01", "366 days": "2027-01-01"} {
		if _, err := h.svc.Finance(h.owner(t), d, day2(t, to)); err != nil {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

// Financeiro is the owner's: finance:read is in no other role.
func TestFinance_OnlyTheOwner(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.released(h.staff(t), t, nil)
	for name, ctx := range map[string]context.Context{"admin": h.admin(t), "staff": h.staff(t), "viewer": h.viewer(t)} {
		if _, err := h.svc.Finance(ctx, day2(t, "2026-12-01"), day2(t, "2026-12-31")); !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("%s: err = %v, want forbidden", name, err)
		}
	}
	// Refused before it reads anything, even with a period that is wrong.
	if _, err := h.svc.Finance(h.admin(t), day2(t, "2026-12-31"), day2(t, "2026-12-01")); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("admin with a bad period: err = %v, want forbidden first", err)
	}
	if _, err := h.svc.Finance(h.owner(t), day2(t, "2026-12-01"), day2(t, "2026-12-31")); err != nil {
		t.Errorf("owner: %v", err)
	}
	if _, err := h.svc.Finance(context.Background(), day2(t, "2026-12-01"), day2(t, "2026-12-31")); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("no caller: err = %v", err)
	}
}

func TestFinance_EmptyStoreIsZeroInTheStoresCurrency(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	got, err := h.svc.Finance(h.owner(t), day2(t, "2026-12-01"), day2(t, "2026-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Received.Total.IsZero() || got.Received.Total.Currency() != "BRL" || !got.ToReceive.Total.IsZero() || got.ToReceive.Total.Currency() != "BRL" {
		t.Errorf("finance = %+v", got)
	}
}

func TestQueries_ReturnTheRepositorysFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.owner(t), t, nil)
	h.repo.Fail(errDown)
	calls := map[string]func() error{
		"list":    func() error { _, err := h.svc.List(h.owner(t), apporder.ListFilter{}, firstPage); return err },
		"history": func() error { _, err := h.svc.History(h.owner(t), v.ID, firstPage); return err },
		"today":   func() error { _, err := h.svc.Today(h.owner(t)); return err },
		"finance": func() error {
			_, err := h.svc.Finance(h.owner(t), day2(t, "2026-12-01"), day2(t, "2026-12-02"))
			return err
		},
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, errDown) {
			t.Errorf("%s: err = %v, want the repository's", name, err)
		}
	}
}
