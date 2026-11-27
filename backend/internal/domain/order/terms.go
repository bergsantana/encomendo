package order

import "github.com/bergsantana/encomendo/backend/internal/domain/shared"

// Handover is how an order reaches the client: Entrega or Retirada. The
// values are part of the API contract.
type Handover string

// The handovers.
const (
	HandoverPickup   Handover = "pickup"
	HandoverDelivery Handover = "delivery"
)

// Valid reports whether h is a known handover.
func (h Handover) Valid() bool { return h == HandoverPickup || h == HandoverDelivery }

// DeliveryAddress is the customer's address an order is delivered to, as it
// was when it was chosen. The order keeps its own copy, so editing or
// removing the address on the customer screen never changes where a past
// order went (docs/api/openapi.yaml, replaceCustomer).
type DeliveryAddress struct {
	ID                                                            shared.AddressID
	Label, CEP, Street, Number, Complement, District, City, State string
}

// Terms is how an order is handed over and paid, and the days its deadline
// counts. The app layer resolves each field from the request, the kit and
// the store's defaults, and checks who may set what (ADR 0030).
type Terms struct {
	Handover         Handover
	Address          *DeliveryAddress // required for delivery, nil for pickup
	Freight          shared.Money     // Frete; zero for pickup
	Discount         shared.Money     // Desconto; never above the subtotal
	DepositPercent   int              // Sinal, 0..100
	AssemblyDays     int              // 0..60, the effective value (order > kit > store)
	SafetyMarginDays int              // 0..60
}

const (
	maxPercent   = 100
	maxOrderDays = 60
)

// check returns a ValidationError listing every term that breaks a rule,
// given the order's subtotal.
func (t Terms) check(subtotal shared.Money) error {
	var errs []error
	if !t.Handover.Valid() {
		errs = append(errs, fieldError("handover", msgHandover))
	}
	if t.Handover == HandoverDelivery && t.Address == nil {
		errs = append(errs, fieldError("addressId", msgAddress))
	}
	errs = append(errs, t.checkMoney(subtotal)...)
	if t.DepositPercent < 0 || t.DepositPercent > maxPercent {
		errs = append(errs, fieldError("depositPercent", msgDepositPercent))
	}
	if !validDays(t.AssemblyDays) {
		errs = append(errs, fieldError("assemblyDays", msgDays))
	}
	if !validDays(t.SafetyMarginDays) {
		errs = append(errs, fieldError("safetyMarginDays", msgDays))
	}
	return shared.MergeValidation(errs...)
}

// checkMoney checks freight and discount. Total = subtotal + freight −
// discount, and the discount never takes the total below the freight
// (ADR 0030).
func (t Terms) checkMoney(subtotal shared.Money) []error {
	var errs []error
	switch {
	case t.Freight.Currency() != subtotal.Currency():
		errs = append(errs, fieldError("freightCents", msgCurrency))
	case t.Freight.IsNegative():
		errs = append(errs, fieldError("freightCents", msgNegative))
	case t.Freight.Cents() > maxAmountCents:
		errs = append(errs, fieldError("freightCents", msgAmountTooHigh))
	case t.Handover == HandoverPickup && !t.Freight.IsZero():
		errs = append(errs, fieldError("freightCents", msgFreightPickup))
	}
	switch {
	case t.Discount.Currency() != subtotal.Currency():
		errs = append(errs, fieldError("discountCents", msgCurrency))
	case t.Discount.IsNegative():
		errs = append(errs, fieldError("discountCents", msgNegative))
	case t.Discount.Cents() > subtotal.Cents():
		errs = append(errs, fieldError("discountCents", msgDiscount))
	}
	return errs
}

func validDays(d int) bool { return d >= 0 && d <= maxOrderDays }
