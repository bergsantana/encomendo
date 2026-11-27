package order

import (
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// newBaseline is what a new order's terms default to: the store's deposit,
// the kits' own days (the longest of them) or the store's, no freight and
// no discount (ADR 0023, ADR 0030).
func newBaseline(f StoreFacts, lines []order.LineSpec) (order.Terms, error) {
	zero, err := shared.ZeroMoney(f.Currency)
	if err != nil {
		return order.Terms{}, err
	}
	t := order.Terms{
		Freight: zero, Discount: zero, DepositPercent: f.DepositPercent,
		AssemblyDays: f.AssemblyDays, SafetyMarginDays: f.SafetyMarginDays,
	}
	assembly, margin := -1, -1
	for _, l := range lines {
		if l.AssemblyDays != nil {
			assembly = max(assembly, *l.AssemblyDays)
		}
		if l.SafetyMarginDays != nil {
			margin = max(margin, *l.SafetyMarginDays)
		}
	}
	if assembly >= 0 {
		t.AssemblyDays = assembly
	}
	if margin >= 0 {
		t.SafetyMarginDays = margin
	}
	return t, nil
}

// resolveTerms turns in into the order's terms over baseline: a field left
// out keeps the baseline's value (the defaults on a new order, the order's
// own terms on a change). A discount, deposit percent or days different
// from the baseline need order:manage, so staff can neither set nor undo
// what an owner or admin chose (ADR 0030, Amendments). The delivery address
// is the customer's, copied; on a change, the order's own copy is kept when
// the same address is sent, even if the customer no longer has it.
func resolveTerms(by shared.Principal, baseline order.Terms, in TermsInput, c CustomerFacts) (order.Terms, error) {
	currency := baseline.Freight.Currency()
	freight, err := shared.NewMoney(in.FreightCents, currency)
	if err != nil {
		return order.Terms{}, err
	}
	t := baseline
	t.Handover, t.Freight, t.Address = in.Handover, freight, nil
	if in.DiscountCents != nil {
		if t.Discount, err = shared.NewMoney(*in.DiscountCents, currency); err != nil {
			return order.Terms{}, err
		}
	}
	t.DepositPercent = pick(in.DepositPercent, baseline.DepositPercent)
	t.AssemblyDays = pick(in.AssemblyDays, baseline.AssemblyDays)
	t.SafetyMarginDays = pick(in.SafetyMarginDays, baseline.SafetyMarginDays)
	if managed(t, baseline) {
		if err := requireManage(by, "set the discount, deposit or days"); err != nil {
			return order.Terms{}, err
		}
	}
	if t.Handover == order.HandoverDelivery {
		addr, ok := deliveryAddress(in.Address, baseline.Address, c)
		if !ok {
			return order.Terms{}, shared.NewValidationError(shared.FieldError{Field: "addressId", Message: msgAddress})
		}
		t.Address = addr
	}
	return t, nil
}

// msgAddress is the design document's refusal (Novo pedido, Entrega).
const msgAddress = "Escolha o endereço de entrega."

func pick(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

// managed reports whether t differs from baseline in what only order:manage
// may set.
func managed(t, baseline order.Terms) bool {
	return t.Discount != baseline.Discount || t.DepositPercent != baseline.DepositPercent ||
		t.AssemblyDays != baseline.AssemblyDays || t.SafetyMarginDays != baseline.SafetyMarginDays
}

func deliveryAddress(id shared.AddressID, current *order.DeliveryAddress, c CustomerFacts) (*order.DeliveryAddress, bool) {
	if id.IsZero() {
		return nil, false
	}
	if current != nil && current.ID == id {
		a := *current
		return &a, true
	}
	for _, a := range c.Addresses {
		if a.ID == id {
			return &a, true
		}
	}
	return nil, false
}
