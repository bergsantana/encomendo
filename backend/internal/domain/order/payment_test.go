package order_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// cb500x is the reference kit of docs/architecture.md section 4.2: the
// leather seat (15 days) is the slowest of three items.
func cb500x(t *testing.T) order.LineSpec {
	t.Helper()
	return order.LineSpec{
		ID:    shared.OrderLineID(testkit.NextID()),
		Kit:   &order.KitSnapshot{Kit: shared.KitID(testkit.NextID()), Name: "Kit Scrambler CB 500X"},
		Price: brl(t, 250_000),
		Items: []order.ItemSpec{
			testkit.ItemSpec(t, testkit.ItemOpts{Name: "Banco de couro", Price: 120_000, LeadDays: 15}),
			testkit.ItemSpec(t, testkit.ItemOpts{Name: "Guidão", Price: 80_000, LeadDays: 5}),
			testkit.ItemSpec(t, testkit.ItemOpts{Name: "Escapamento", Price: 90_000, LeadDays: 7}),
		},
	}
}

func pay(t *testing.T, o *order.Order, kind order.PaymentKind, cents int64, today string) (bool, error) {
	t.Helper()
	return o.RecordPayment(order.PaymentSpec{
		ID: shared.PaymentID(testkit.NextID()), Kind: kind, Amount: brl(t, cents), Method: order.MethodPix,
		By: owner(t, o), At: day1, Today: date(t, today),
	})
}

func TestRecordPayment_DepositConfirmsAndDates_CB500X(t *testing.T) {
	t.Parallel()

	b := testkit.NewOrder().WithLine(cb500x(t))
	spec := b.Spec(t)
	spec.Now = time.Date(2026, 11, 28, 15, 0, 0, 0, time.UTC)
	spec.Today = date(t, "2026-11-28")
	o, err := order.New(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := o.Promise(); got != date(t, "2026-12-18") {
		t.Errorf("promise at creation = %s, want 20 days from today", got)
	}
	if o.DepositDue().Cents() != 125_000 {
		t.Fatalf("deposit due = %v, want 50%% of R$ 2.500,00", o.DepositDue())
	}
	confirmed, err := pay(t, o, order.PaymentDeposit, 100_000, "2026-11-30")
	if err != nil || confirmed {
		t.Fatalf("part of the deposit: confirmed %v err %v, want neither", confirmed, err)
	}
	if _, ok := o.ReleaseDate(); ok {
		t.Error("part of the deposit released the order")
	}
	confirmed, err = pay(t, o, order.PaymentDeposit, 25_000, "2026-12-01")
	if err != nil || !confirmed {
		t.Fatalf("rest of the deposit: confirmed %v err %v, want confirmed", confirmed, err)
	}
	// Day 0 is the deposit: 15 + 3 + 2 = day 20, and the seat must arrive by
	// day 17.
	if got := o.Promise(); got != date(t, "2026-12-21") {
		t.Errorf("promise = %s, want 2026-12-21", got)
	}
	if got := o.ArrivalLimit(); got != date(t, "2026-12-18") {
		t.Errorf("arrival limit = %s, want 2026-12-18", got)
	}
	if got := o.Items()[0].Expected(); got != date(t, "2026-12-16") {
		t.Errorf("seat expected = %s, want 2026-12-16", got)
	}
	if o.BalanceDue().Cents() != 125_000 || o.Paid().Cents() != 125_000 {
		t.Errorf("paid %v balance %v, want half each", o.Paid(), o.BalanceDue())
	}
}

func TestRecordPayment_RefusesAboveBalance(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t) // R$ 1.000,00
	if _, err := pay(t, o, order.PaymentDeposit, 100_001, "2026-12-01"); !errors.Is(err, order.ErrPaymentExceedsDue) {
		t.Errorf("err = %v, want ErrPaymentExceedsDue", err)
	}
	if _, err := pay(t, o, order.PaymentDeposit, 0, "2026-12-01"); fieldsOf(err)["amountCents"] == "" {
		t.Errorf("zero: err = %v, want a field error on amountCents", err)
	}
	_, err := o.RecordPayment(order.PaymentSpec{ID: shared.PaymentID(testkit.NextID()), Kind: "tip", Method: "cheque", Amount: brl(t, 1), Today: date(t, "2026-12-01")})
	if f := fieldsOf(err); f["kind"] == "" || f["method"] == "" {
		t.Errorf("unknown kind and method: err = %v", err)
	}
	if len(o.Payments()) != 0 {
		t.Errorf("refused payments were kept: %d", len(o.Payments()))
	}
}

func TestVoidPayment_RemovesFromGuards_KeepsRelease(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	if _, err := pay(t, o, order.PaymentDeposit, 50_000, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	id := o.Payments()[0].ID()
	by := owner(t, o)
	if err := o.VoidPayment(id, "  ", by, day1); fieldsOf(err)["reason"] == "" {
		t.Errorf("empty reason: err = %v", err)
	}
	if err := o.VoidPayment(id, "Pix devolvido", by, day1); err != nil {
		t.Fatal(err)
	}
	if o.DepositPaid() || o.Paid().Cents() != 0 {
		t.Errorf("paid %v deposit paid %v, want nothing counted", o.Paid(), o.DepositPaid())
	}
	if _, ok := o.ReleaseDate(); !ok {
		t.Error("voiding the deposit undid the release")
	}
	if _, _, reason, ok := o.Payments()[0].Void(); !ok || reason != "Pix devolvido" {
		t.Errorf("void = %v %q", ok, reason)
	}
	// The release date stands, but the guard counts only payments that are
	// not void: the order cannot leave awaiting_deposit on a voided deposit.
	err := o.Advance("purchasing", nil, by, date(t, "2026-12-02"), day1)
	if guardOf(err) != order.GuardDepositUnpaid {
		t.Errorf("advance after void: err = %v, want deposit_unpaid", err)
	}
	if err := o.VoidPayment(id, "de novo", by, day1); fieldsOf(err)["paymentId"] == "" {
		t.Errorf("void twice: err = %v", err)
	}
	if err := o.VoidPayment(shared.PaymentID(testkit.NextID()), "x", by, day1); !errors.Is(err, order.ErrPaymentNotFound) {
		t.Errorf("unknown payment: err = %v", err)
	}
}

func TestDepositDue_RoundsHalfUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		price   int64
		percent int
		want    int64
	}{
		{100_001, 50, 50_001}, // 50.000,5 → up
		{100_003, 50, 50_002}, // 50.001,5 → up
		{33_333, 30, 10_000},  // 9.999,9 → up
		{10_001, 33, 3_300},   // 3.300,33 → down
		{100_000, 0, 0},
		{100_000, 100, 100_000},
	}
	for _, tt := range tests {
		o := testkit.NewOrder().WithItem(testkit.ItemOpts{Price: tt.price}).
			WithTerms(func(tr *order.Terms) { tr.DepositPercent = tt.percent }).Build(t)
		if got := o.DepositDue().Cents(); got != tt.want {
			t.Errorf("%d × %d%% = %d, want %d", tt.price, tt.percent, got, tt.want)
		}
	}
}
