package dto

import (
	"fmt"
	"time"

	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Address is the Address schema.
type Address struct {
	ID         shared.AddressID `json:"id"`
	Label      string           `json:"label"`
	CEP        string           `json:"cep"`
	Street     string           `json:"street"`
	Number     string           `json:"number"`
	Complement string           `json:"complement"`
	District   string           `json:"district"`
	City       string           `json:"city"`
	State      string           `json:"state"`
}

// newOrderAddress maps the delivery address an order snapshotted; nil for a
// pickup.
func newOrderAddress(a *order.DeliveryAddress) *Address {
	if a == nil {
		return nil
	}
	return &Address{
		ID: a.ID, Label: a.Label, CEP: a.CEP, Street: a.Street, Number: a.Number,
		Complement: a.Complement, District: a.District, City: a.City, State: a.State,
	}
}

// Customer is the Customer schema. A customer with no e-mail answers null.
type Customer struct {
	ID        shared.CustomerID `json:"id"`
	Name      string            `json:"name"`
	Phone     string            `json:"phone"`
	Email     *string           `json:"email"`
	Notes     string            `json:"notes"`
	Addresses []Address         `json:"addresses"`
	Version   int               `json:"version"`
}

// NewCustomer maps a customer with its addresses.
func NewCustomer(v appcustomer.View) Customer {
	c := Customer{
		ID: v.ID, Name: v.Name, Phone: v.Phone, Notes: v.Notes, Version: v.Version,
		Addresses: make([]Address, 0, len(v.Addresses)),
	}
	if v.Email != "" {
		email := v.Email
		c.Email = &email
	}
	for _, a := range v.Addresses {
		c.Addresses = append(c.Addresses, Address(a))
	}
	return c
}

// CustomerSummary is the CustomerSummary schema.
type CustomerSummary struct {
	ID          shared.CustomerID `json:"id"`
	Name        string            `json:"name"`
	Phone       string            `json:"phone"`
	Orders      int               `json:"orders"`
	LastOrderAt *time.Time        `json:"lastOrderAt,omitempty"`
}

// NewCustomerSummary maps a row of the customer list.
func NewCustomerSummary(s appcustomer.Summary) CustomerSummary {
	out := CustomerSummary{ID: s.ID, Name: s.Name, Phone: s.Phone, Orders: s.Orders}
	if s.LastOrderAt != nil {
		t := s.LastOrderAt.UTC()
		out.LastOrderAt = &t
	}
	return out
}

// AddressRequest is the AddressInput schema.
type AddressRequest struct {
	ID         *string `json:"id"`
	Label      *string `json:"label"`
	CEP        *string `json:"cep"`
	Street     *string `json:"street"`
	Number     string  `json:"number"`
	Complement string  `json:"complement"`
	District   string  `json:"district"`
	City       *string `json:"city"`
	State      *string `json:"state"`
}

// CustomerRequest is the CustomerInput schema (POST /customers). The
// e-mail is optional and may be null; the address ids it carries are kept
// by an update and ignored by a create.
type CustomerRequest struct {
	Name      *string          `json:"name"`
	Phone     *string          `json:"phone"`
	Email     Optional[string] `json:"email"`
	Notes     string           `json:"notes"`
	Addresses []AddressRequest `json:"addresses"`
}

func (r CustomerRequest) spec(f *fields) customer.Spec {
	s := customer.Spec{Name: need(f, r.Name, "name"), Phone: need(f, r.Phone, "phone"), Notes: r.Notes}
	if email, ok := nullable(r.Email); ok {
		s.Email = email
	}
	for i, a := range needList(f, r.Addresses, "addresses") {
		path := fmt.Sprintf("addresses[%d]", i)
		as := customer.AddressSpec{
			Label: need(f, a.Label, path+".label"), CEP: need(f, a.CEP, path+".cep"), Street: need(f, a.Street, path+".street"),
			Number: a.Number, Complement: a.Complement, District: a.District,
			City: need(f, a.City, path+".city"), State: need(f, a.State, path+".state"),
		}
		if a.ID != nil {
			as.ID = parseID[shared.AddressID](f, *a.ID, path+".id")
		}
		s.Addresses = append(s.Addresses, as)
	}
	return s
}

// Input returns the use case's input, or a ValidationError.
func (r CustomerRequest) Input() (appcustomer.CreateInput, error) {
	var f fields
	in := appcustomer.CreateInput{Spec: r.spec(&f)}
	return in, f.err()
}

// ReplaceCustomerRequest is PUT /customers/{customerId}: the customer's
// fields and the version read.
type ReplaceCustomerRequest struct {
	CustomerRequest
	Version *int `json:"version"`
}

// Input returns the use case's input for the customer id, or a
// ValidationError.
func (r ReplaceCustomerRequest) Input(id shared.CustomerID) (appcustomer.UpdateInput, error) {
	var f fields
	in := appcustomer.UpdateInput{ID: id, Spec: r.spec(&f), Version: needVersion(&f, r.Version)}
	return in, f.err()
}
