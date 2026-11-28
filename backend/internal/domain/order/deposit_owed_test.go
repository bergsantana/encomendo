package order_test

import (
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

func TestOrder_DepositOwed(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t) // R$ 1.000,00, deposit 50%
	if got := o.DepositOwed().Cents(); got != 50_000 {
		t.Fatalf("new order: deposit owed = %d, want the whole deposit", got)
	}
	if _, err := pay(t, o, order.PaymentDeposit, 20_000, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	if got := o.DepositOwed().Cents(); got != 30_000 {
		t.Errorf("partial deposit: owed = %d, want 30000", got)
	}
	if got := o.DepositDue().Cents(); got != 50_000 {
		t.Errorf("deposit due = %d, want the full deposit still", got)
	}
	if _, err := pay(t, o, order.PaymentDeposit, 30_000, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	if got := o.DepositOwed().Cents(); got != 0 {
		t.Errorf("deposit paid: owed = %d, want 0", got)
	}
	if _, err := pay(t, o, order.PaymentBalance, 50_000, "2026-12-02"); err != nil {
		t.Fatal(err)
	}
	if got := o.DepositOwed().Cents(); got != 0 {
		t.Errorf("a balance payment changed the deposit owed to %d", got)
	}
}

func TestOrder_DepositOwed_VoidedPaymentIsOwedAgain(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	if _, err := pay(t, o, order.PaymentDeposit, 50_000, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	if err := o.VoidPayment(o.Payments()[0].ID(), "Pix devolvido", owner(t, o), day1); err != nil {
		t.Fatal(err)
	}
	if got := o.DepositOwed().Cents(); got != 50_000 {
		t.Errorf("after the void: owed = %d, want the whole deposit", got)
	}
}
