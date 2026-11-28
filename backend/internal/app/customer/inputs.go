package customer

import (
	"strings"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// AddressView is one address of a customer (the Address schema).
type AddressView struct {
	ID         shared.AddressID
	Label      string
	CEP        string
	Street     string
	Number     string
	Complement string
	District   string
	City       string
	State      string
}

// View is a customer with its addresses (the Customer schema). Version is
// the one an update must send back.
type View struct {
	ID        shared.CustomerID
	Name      string
	Phone     string
	Email     string
	Notes     string
	Addresses []AddressView
	Version   int
}

// Summary is one row of the customer list: what the screen shows without
// opening the customer. Orders counts every order of the customer, cancelled
// ones included; LastOrderAt is the creation time of the newest, nil when the
// customer has none.
type Summary struct {
	ID          shared.CustomerID
	Name        string
	Phone       string
	Orders      int
	LastOrderAt *time.Time
}

// ListFilter narrows the list (GET /api/v1/admin/customers). The zero value
// lists every customer.
type ListFilter struct {
	// Query keeps the customers whose name starts with it, ignoring case,
	// or whose number contains its digits (see PhoneDigits).
	Query string
}

// NamePrefix returns the text a customer's name must start with, in
// lower case, or "" when there is no query.
func (f ListFilter) NamePrefix() string { return strings.ToLower(strings.TrimSpace(f.Query)) }

// PhoneDigits returns the digits a customer's number must contain, or ""
// when the query is not a phone number. A query is one when it holds at least
// one digit and nothing but digits and the separators people type in a number
// (space, "+", "-", "(", ")" and "."), so "(92) 99123" finds numbers and
// "Ana 2" does not match every number with a 2 in it.
func (f ListFilter) PhoneDigits() string {
	var digits strings.Builder
	for _, r := range f.Query {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case strings.ContainsRune(" +-().", r):
		default:
			return ""
		}
	}
	return digits.String()
}

// CreateInput adds a customer (POST /api/v1/admin/customers). The ids of
// Spec's addresses are ignored: every address gets a new one.
type CreateInput struct {
	Spec customer.Spec
}

// UpdateInput replaces a customer's fields and addresses, with the version
// read (PUT /api/v1/admin/customers/{customerId}). An address whose id the
// customer has is updated, one with any other id, or none, is added, and a
// current address left out is removed.
type UpdateInput struct {
	ID      shared.CustomerID
	Spec    customer.Spec
	Version int
}

func viewOf(c *customer.Customer, version int) View {
	addresses := c.Addresses()
	v := View{
		ID: c.ID(), Name: c.Name(), Phone: c.Phone(), Email: c.Email().String(), Notes: c.Notes(),
		Addresses: make([]AddressView, 0, len(addresses)), Version: version,
	}
	for _, a := range addresses {
		v.Addresses = append(v.Addresses, AddressView(a))
	}
	return v
}
