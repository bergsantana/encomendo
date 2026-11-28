package order

import (
	"context"
	"errors"

	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Field and messages of the "Novo endereço" refusals. The design document
// lists no wording for these two, so they follow its Entrega messages.
const (
	fieldNewAddress  = "terms.newAddress"
	msgAddressBoth   = "Escolha um endereço da lista ou informe um novo, não os dois."
	msgAddressPickup = "O endereço novo é só para entrega."
)

// withNewAddress turns terms.NewAddress ("Novo endereço") into one of the
// customer's own addresses, so resolveTerms finds it by id like any other:
//
//   - it is refused next to an addressId, or on a pickup;
//   - in a quote (w.preview) it is only checked, with a fresh id, and never
//     saved;
//   - otherwise the Customers port adds it to the customer in the caller's
//     transaction (order:write), so a refusal later in the same call undoes
//     it.
//
// It returns the terms with Address set and NewAddress cleared, and the
// customer with the address added. Without a NewAddress it returns both
// unchanged. Field errors are named terms.newAddress.<field>.
func (s *Service) withNewAddress(ctx context.Context, in TermsInput, c CustomerFacts, w writeCtx) (TermsInput, CustomerFacts, error) {
	if in.NewAddress == nil {
		return in, c, nil
	}
	if !in.Address.IsZero() {
		return in, c, newAddressError(msgAddressBoth)
	}
	if in.Handover == order.HandoverPickup {
		return in, c, newAddressError(msgAddressPickup)
	}
	var added order.DeliveryAddress
	var err error
	if w.preview {
		added, err = s.previewAddress(*in.NewAddress, len(c.Addresses))
	} else {
		added, err = s.d.Customers.AddAddress(ctx, c.ID, *in.NewAddress)
	}
	if err != nil {
		return in, c, underNewAddress(err)
	}
	c.Addresses = append(append([]order.DeliveryAddress(nil), c.Addresses...), added)
	in.Address, in.NewAddress = added.ID, nil
	return in, c, nil
}

// previewAddress checks a as AddAddress would, for a customer with have
// addresses, and gives it a fresh id. Nothing is stored.
func (s *Service) previewAddress(a order.DeliveryAddress, have int) (order.DeliveryAddress, error) {
	a.ID = shared.AddressID(s.d.IDs.NewID())
	checked, err := customer.CheckNewAddress(have, customer.AddressSpec(a))
	if err != nil {
		return order.DeliveryAddress{}, err
	}
	return order.DeliveryAddress(checked), nil
}

func newAddressError(message string) error {
	return shared.NewValidationError(shared.FieldError{Field: fieldNewAddress, Message: message})
}

// underNewAddress renames the fields of a ValidationError from the
// customer's address checks ("cep", or "addresses" for the limit) to the
// request's path, terms.newAddress.cep. Any other error passes through.
func underNewAddress(err error) error {
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	out := make([]shared.FieldError, len(ve.Fields))
	for i, f := range ve.Fields {
		name := fieldNewAddress + "." + f.Field
		if f.Field == "addresses" {
			name = fieldNewAddress
		}
		out[i] = shared.FieldError{Field: name, Message: f.Message}
	}
	return shared.NewValidationError(out...)
}
