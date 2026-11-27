package models

import (
	"time"

	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Customer maps customers.
//
// ClientAccountID is read-only (<-:false): encomendo_app has no INSERT or
// UPDATE grant on client_account_id (0012), and milestone 7 adds a reviewed
// way to link an account. Leaving it out of every write also keeps a
// repository from unlinking one.
type Customer struct {
	TenantID        string  `gorm:"column:tenant_id"`
	ID              string  `gorm:"column:id;primaryKey"`
	Name            string  `gorm:"column:name"`
	WhatsAppPhone   string  `gorm:"column:whatsapp_phone"`
	Email           *string `gorm:"column:email"`
	Notes           string  `gorm:"column:notes"`
	ClientAccountID *string `gorm:"column:client_account_id;<-:false"`
	Version         int     `gorm:"column:version"`
}

// TableName returns customers.
func (Customer) TableName() string { return "customers" }

// CustomerFromDomain returns the row of c. An empty e-mail is stored as
// NULL, as the column's check expects.
func CustomerFromDomain(c *customer.Customer) Customer {
	m := Customer{
		TenantID: c.TenantID().String(), ID: c.ID().String(), Name: c.Name(),
		WhatsAppPhone: c.Phone(), Notes: c.Notes(), Version: c.Version(),
	}
	if e := c.Email(); !e.IsZero() {
		s := e.String()
		m.Email = &s
	}
	return m
}

// ToDomain rebuilds the customer from its row and its address rows, which
// must be in position order.
func (m Customer) ToDomain(addresses []CustomerAddress) (*customer.Customer, error) {
	tenant, err := parseID[shared.TenantID](m.TenantID)
	if err != nil {
		return nil, err
	}
	id, err := parseID[shared.CustomerID](m.ID)
	if err != nil {
		return nil, err
	}
	r := customer.Record{
		Tenant: tenant, ID: id, Name: m.Name, Phone: m.WhatsAppPhone, Notes: m.Notes,
		Addresses: make([]customer.Address, 0, len(addresses)), Version: m.Version,
	}
	if m.Email != nil {
		r.Email = *m.Email
	}
	if m.ClientAccountID != nil {
		if r.ClientAccountID, err = parseID[shared.ClientAccountID](*m.ClientAccountID); err != nil {
			return nil, err
		}
	}
	for _, a := range addresses {
		addr, err := a.ToDomain()
		if err != nil {
			return nil, err
		}
		r.Addresses = append(r.Addresses, addr)
	}
	return customer.Restore(r)
}

// CustomerAddress maps customer_addresses. Position is 1-based and keeps the
// order the person gave the addresses in. The timestamps are the table's
// defaults and are left out.
type CustomerAddress struct {
	TenantID   string `gorm:"column:tenant_id"`
	ID         string `gorm:"column:id;primaryKey"`
	CustomerID string `gorm:"column:customer_id"`
	Position   int    `gorm:"column:position"`
	Label      string `gorm:"column:label"`
	CEP        string `gorm:"column:cep"`
	Street     string `gorm:"column:street"`
	Number     string `gorm:"column:number"`
	Complement string `gorm:"column:complement"`
	District   string `gorm:"column:district"`
	City       string `gorm:"column:city"`
	State      string `gorm:"column:state"`
}

// TableName returns customer_addresses.
func (CustomerAddress) TableName() string { return "customer_addresses" }

// CustomerAddressesFromDomain returns the rows of c's addresses, numbered
// from 1 in the order c keeps them.
func CustomerAddressesFromDomain(c *customer.Customer) []CustomerAddress {
	addresses := c.Addresses()
	out := make([]CustomerAddress, 0, len(addresses))
	for i, a := range addresses {
		out = append(out, CustomerAddress{
			TenantID: c.TenantID().String(), ID: a.ID.String(), CustomerID: c.ID().String(), Position: i + 1,
			Label: a.Label, CEP: a.CEP, Street: a.Street, Number: a.Number, Complement: a.Complement,
			District: a.District, City: a.City, State: a.State,
		})
	}
	return out
}

// ToDomain rebuilds the address.
func (m CustomerAddress) ToDomain() (customer.Address, error) {
	id, err := parseID[shared.AddressID](m.ID)
	if err != nil {
		return customer.Address{}, err
	}
	return customer.Address{
		ID: id, Label: m.Label, CEP: m.CEP, Street: m.Street, Number: m.Number, Complement: m.Complement,
		District: m.District, City: m.City, State: m.State,
	}, nil
}

// CustomerSummary is a row of the customer list: the customer's columns the
// list shows, with the count and the newest creation time of its orders from
// a lateral join on orders. It is only read.
type CustomerSummary struct {
	ID            string     `gorm:"column:id;primaryKey"`
	Name          string     `gorm:"column:name"`
	WhatsAppPhone string     `gorm:"column:whatsapp_phone"`
	OrdersCount   int        `gorm:"column:orders_count"`
	LastOrderAt   *time.Time `gorm:"column:last_order_at"`
}

// TableName returns customers, the table the list reads.
func (CustomerSummary) TableName() string { return "customers" }

// ToSummary returns the list row.
func (m CustomerSummary) ToSummary() (appcustomer.Summary, error) {
	id, err := parseID[shared.CustomerID](m.ID)
	if err != nil {
		return appcustomer.Summary{}, err
	}
	s := appcustomer.Summary{ID: id, Name: m.Name, Phone: m.WhatsAppPhone, Orders: m.OrdersCount}
	if m.LastOrderAt != nil {
		at := m.LastOrderAt.UTC()
		s.LastOrderAt = &at
	}
	return s, nil
}
