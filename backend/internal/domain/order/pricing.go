package order

import (
	"math"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// currency returns the order's currency. Every amount of an order is in it;
// New checks lines and terms against the freight's.
func (o *Order) currency() string { return o.terms.Freight.Currency() }

// Subtotal returns the sum of the line prices ("Itens"). Line prices are
// bounded by the catalog (R$ 1.000.000.000,00 each, at most 50 lines), so
// the sum fits in int64 cents.
func (o *Order) Subtotal() shared.Money {
	var cents int64
	for _, l := range o.lines {
		cents += l.price.Cents()
	}
	return mustMoney(cents, o.currency())
}

// Total returns subtotal + freight − discount. It returns
// shared.ErrMoneyOverflow when the sum does not fit, which bounded inputs
// never reach.
func (o *Order) Total() (shared.Money, error) {
	withFreight, err := o.Subtotal().Add(o.terms.Freight)
	if err != nil {
		return shared.Money{}, err
	}
	return withFreight.Sub(o.terms.Discount)
}

// total is Total for an order New already checked.
func (o *Order) total() shared.Money {
	t, err := o.Total()
	if err != nil {
		return mustMoney(0, o.currency())
	}
	return t
}

// DepositDue returns the deposit ("Sinal"): the total times the order's
// deposit percent, rounded half up to the cent (ADR 0030). Once the deposit
// was confirmed it stays the amount confirmed, never above the total: a
// higher total or percent later adds to the balance, not to the deposit.
func (o *Order) DepositDue() shared.Money {
	total := o.total().Cents()
	if o.depositFrozen != nil {
		return mustMoney(min(*o.depositFrozen, max(total, 0)), o.currency())
	}
	return mustMoney((total*int64(o.terms.DepositPercent)+50)/100, o.currency())
}

// Paid returns the sum of the payments that are not void.
func (o *Order) Paid() shared.Money { return o.sumPayments(func(Payment) bool { return true }) }

// depositPaidSoFar returns the sum of the deposit payments that are not void.
func (o *Order) depositPaidSoFar() shared.Money {
	return o.sumPayments(func(p Payment) bool { return p.kind == PaymentDeposit })
}

func (o *Order) sumPayments(keep func(Payment) bool) shared.Money {
	var cents int64
	for _, p := range o.payments {
		if !p.Voided() && keep(p) {
			cents += p.amount.Cents()
		}
	}
	return mustMoney(cents, o.currency())
}

// DepositPaid reports whether the deposit payments that are not void reach
// the deposit due. A deposit due of zero is paid from the start.
func (o *Order) DepositPaid() bool {
	return o.depositPaidSoFar().Cents() >= o.DepositDue().Cents()
}

// DepositOwed returns the deposit still to pay: DepositDue less the deposit
// payments that are not void, never below zero. It is what "Registrar
// pagamento" prefills for a Sinal (docs/product/design-document.md, Pedido).
func (o *Order) DepositOwed() shared.Money {
	return mustMoney(max(o.DepositDue().Cents()-o.depositPaidSoFar().Cents(), 0), o.currency())
}

// BalanceDue returns what is still to pay: total − paid, never below zero.
func (o *Order) BalanceDue() shared.Money {
	return mustMoney(max(o.total().Cents()-o.Paid().Cents(), 0), o.currency())
}

// RefundDue returns "Valor a devolver": paid − total when a change brought
// the total below what was paid, otherwise zero (ADR 0030).
func (o *Order) RefundDue() shared.Money {
	return mustMoney(max(o.Paid().Cents()-o.total().Cents(), 0), o.currency())
}

// Cost returns the sum of every item's unit cost times its quantity, and
// false when any item's cost is unknown. A unit cost is the price actually
// paid when purchasing recorded one (Sourcing.PaidCost), and the cost at
// order time otherwise (ADR 0031), so a quote counts the order-time cost and
// a bought order its real one. Only the owner sees it (identity.SeesCosts,
// ADR 0025).
func (o *Order) Cost() (shared.Money, bool) {
	var cents int64
	for _, i := range o.Items() {
		unit := i.sourcing.PaidCost
		if unit == nil {
			unit = i.sourcing.Cost
		}
		if unit == nil {
			return shared.Money{}, false
		}
		part, ok := times(unit.Cents(), i.snapshot.Quantity)
		if !ok {
			return shared.Money{}, false
		}
		cents += part
	}
	return mustMoney(cents, o.currency()), true
}

// Margin returns what the order leaves after its cost, over what was sold:
// the total less the freight, which only passes through. It returns false
// when Cost does or nothing was sold.
func (o *Order) Margin() (catalog.Margin, bool) {
	cost, ok := o.Cost()
	if !ok {
		return catalog.Margin{}, false
	}
	sold := mustMoney(o.total().Cents()-o.terms.Freight.Cents(), o.currency())
	return catalog.MarginOf(sold, cost)
}

// times returns cents × n, and false when it does not fit in int64.
func times(cents int64, n int) (int64, bool) {
	if n <= 0 {
		return 0, n == 0
	}
	if cents > math.MaxInt64/int64(n) || cents < math.MinInt64/int64(n) {
		return 0, false
	}
	return cents * int64(n), true
}

// mustMoney builds an amount in a currency the order already validated, so
// NewMoney cannot fail.
func mustMoney(cents int64, currency string) shared.Money {
	m, err := shared.NewMoney(cents, currency)
	if err != nil {
		return shared.Money{}
	}
	return m
}
