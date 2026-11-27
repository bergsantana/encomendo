package customer

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

const (
	maxNameLength  = 120
	maxNotesLength = 2000
	maxAddresses   = 10
)

// Spec is what a person typed for a customer: the input of New and Replace.
// Nothing in it is trusted until one of them has checked it.
type Spec struct {
	Name, Phone, Email, Notes string
	Addresses                 []AddressSpec
}

// Customer is one person the store sells to, with the WhatsApp number its
// messages go to and the addresses it takes orders to
// (docs/product/design-document.md, customers section). It never holds another
// store's data about the same person.
type Customer struct {
	tenant        shared.TenantID
	id            shared.CustomerID
	name          string
	phone         string
	email         identity.Email
	notes         string
	addresses     []Address
	clientAccount shared.ClientAccountID
	version       int
}

// fields is a Spec after it passed every check: trimmed and safe to store.
type fields struct {
	name, phone, notes string
	email              identity.Email
	addresses          []Address
}

// New returns a customer at version 1. Text is trimmed. It returns a
// ValidationError listing every invalid field ("name", "phone", "email",
// "notes", "addresses" and "addresses[i].field", where field is "id" for an
// address id used twice) and ErrInvalidCustomer for a zero tenant, customer
// or address id.
func New(tenant shared.TenantID, id shared.CustomerID, s Spec) (*Customer, error) {
	if tenant.IsZero() || id.IsZero() {
		return nil, fmt.Errorf("new customer %v: %w", id, ErrInvalidCustomer)
	}
	f, err := check(s)
	if err != nil {
		return nil, err
	}
	return &Customer{
		tenant: tenant, id: id, name: f.name, phone: f.phone, email: f.email,
		notes: f.notes, addresses: f.addresses, version: 1,
	}, nil
}

// check validates and normalizes s. Every invalid field is reported at once.
func check(s Spec) (fields, error) {
	f := fields{
		name:  strings.TrimSpace(s.Name),
		phone: s.Phone,
		notes: strings.TrimSpace(s.Notes),
	}
	errs := []error{checkName(f.name), checkPhone(f.phone), checkNotes(f.notes)}
	email, emailErr := parseEmail(s.Email)
	f.email = email
	errs = append(errs, emailErr)

	addresses, addrErrs, err := checkAddresses(s.Addresses)
	if err != nil {
		return fields{}, err
	}
	f.addresses = addresses
	if err := shared.MergeValidation(append(errs, addrErrs...)...); err != nil {
		return fields{}, err
	}
	return f, nil
}

// checkName reports a ValidationError on "name" unless name, already
// trimmed, has 1 to 120 characters, valid UTF-8 and no control character. A
// name that breaks the last two gets its own message: shortening it would not
// help.
func checkName(name string) error {
	if name == "" {
		return fieldError(fieldName, msgNameRequired)
	}
	if !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return fieldError(fieldName, msgNameControl)
	}
	if utf8.RuneCountInString(name) > maxNameLength {
		return fieldError(fieldName, msgNameTooLong)
	}
	return nil
}

// checkPhone reports a ValidationError on "phone" unless phone is a number
// wa.me accepts. It is never reformatted: clients send digits only
// (docs/api/openapi.yaml, CustomerPhone).
func checkPhone(phone string) error {
	switch {
	case phone == "":
		return fieldError(fieldPhone, msgPhoneRequired)
	case !shared.ValidWhatsAppPhone(phone):
		return fieldError(fieldPhone, msgPhoneInvalid)
	}
	return nil
}

// checkNotes reports a ValidationError on "notes" when notes, already
// trimmed, has more than 2000 characters or is not valid text. Line breaks
// and tabs are allowed; other control characters are not.
func checkNotes(notes string) error {
	if !utf8.ValidString(notes) || strings.ContainsFunc(notes, isBadNoteRune) {
		return fieldError(fieldNotes, msgNotesInvalid)
	}
	if utf8.RuneCountInString(notes) > maxNotesLength {
		return fieldError(fieldNotes, msgNotesTooLong)
	}
	return nil
}

func isBadNoteRune(r rune) bool {
	return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
}

// parseEmail returns the zero Email for an empty address, and otherwise
// identity.ParseEmail's result under the field "email" with the customer
// form's message.
func parseEmail(s string) (identity.Email, error) {
	if strings.TrimSpace(s) == "" {
		return identity.Email{}, nil
	}
	e, err := identity.ParseEmail(s)
	if err != nil {
		return identity.Email{}, fieldError(fieldEmail, msgEmailInvalid)
	}
	return e, nil
}

// checkAddresses checks every address and the count. The last return is
// ErrInvalidCustomer for a bug (a zero id); the slice of errors holds what
// the person got wrong, including an id used twice.
func checkAddresses(specs []AddressSpec) ([]Address, []error, error) {
	if len(specs) > maxAddresses {
		return nil, []error{fieldError(fieldAddresses, msgTooManyAddrs)}, nil
	}
	out := make([]Address, 0, len(specs))
	seen := make(map[shared.AddressID]struct{}, len(specs))
	var errs []error
	for i, s := range specs {
		a, err := checkAddress(i, s)
		if err != nil && !isValidation(err) {
			return nil, nil, fmt.Errorf("check customer: %w", err)
		}
		errs = append(errs, err)
		if _, dup := seen[s.ID]; dup {
			errs = append(errs, fieldError(at(fieldAddresses, i)+"."+fieldID, msgAddressRepeat))
		}
		seen[s.ID] = struct{}{}
		out = append(out, a)
	}
	return out, errs, nil
}

func isValidation(err error) bool {
	_, ok := err.(*shared.ValidationError) //nolint:errorlint // checkAddress returns a direct ValidationError or a plain bug error, never a wrapped one
	return ok
}

// Record is a customer as a repository stores it: every field.
type Record struct {
	Tenant          shared.TenantID
	ID              shared.CustomerID
	Name            string
	Phone           string
	Email           string
	Notes           string
	Addresses       []Address
	ClientAccountID shared.ClientAccountID
	Version         int
}

// Restore rebuilds a stored customer, in the order its addresses are stored.
// It returns ErrInvalidCustomer when the record breaks a rule a stored
// customer always keeps.
func Restore(r Record) (*Customer, error) {
	if r.Version < 1 {
		return nil, fmt.Errorf("restore customer %v: %w: version %d", r.ID, ErrInvalidCustomer, r.Version)
	}
	specs := make([]AddressSpec, len(r.Addresses))
	for i, a := range r.Addresses {
		specs[i] = AddressSpec(a)
	}
	c, err := New(r.Tenant, r.ID, Spec{
		Name: r.Name, Phone: r.Phone, Email: r.Email, Notes: r.Notes, Addresses: specs,
	})
	if err != nil {
		// The cause is text only: wrapping New's ValidationError would make a
		// bad stored row match shared.ErrInvalid and answer 400, not 500.
		return nil, fmt.Errorf("restore customer %v: %w: %s", r.ID, ErrInvalidCustomer, err.Error())
	}
	c.clientAccount = r.ClientAccountID
	c.version = r.Version
	return c, nil
}

// Replace gives the customer every field of s at once, addresses included:
// an address whose id is not in s is dropped, one whose id is known is
// updated and a new id adds one. expectedVersion is the version the client
// read. It returns shared.ErrVersionConflict for a stale version, and
// otherwise the errors of New; nothing changes when it fails. The version
// itself is raised by the repository's update.
func (c *Customer) Replace(s Spec, expectedVersion int) error {
	if expectedVersion != c.version {
		return shared.ErrVersionConflict
	}
	f, err := check(s)
	if err != nil {
		return err
	}
	c.name, c.phone, c.email, c.notes, c.addresses = f.name, f.phone, f.email, f.notes, f.addresses
	return nil
}

// TenantID returns the store the customer belongs to.
func (c *Customer) TenantID() shared.TenantID { return c.tenant }

// ID returns the customer's id.
func (c *Customer) ID() shared.CustomerID { return c.id }

// Name returns the customer's name, trimmed.
func (c *Customer) Name() string { return c.name }

// Phone returns the customer's WhatsApp number: 55, the DDD and 8 or 9
// digits, digits only.
func (c *Customer) Phone() string { return c.phone }

// Email returns the customer's e-mail, or the zero Email when there is none.
func (c *Customer) Email() identity.Email { return c.email }

// Notes returns the store's notes about the customer, trimmed.
func (c *Customer) Notes() string { return c.notes }

// Addresses returns a copy of the customer's addresses, in the order they
// were given.
func (c *Customer) Addresses() []Address { return slices.Clone(c.addresses) }

// Address returns the customer's address with the given id, and false when
// the customer has none. An order's delivery address is looked up here, so
// another customer's address can never be used.
func (c *Customer) Address(id shared.AddressID) (Address, bool) {
	for _, a := range c.addresses {
		if a.ID == id {
			return a, true
		}
	}
	return Address{}, false
}

// ClientAccountID returns the client account linked to the customer, or the
// zero id while there is none. Nothing links one before M7.
func (c *Customer) ClientAccountID() shared.ClientAccountID { return c.clientAccount }

// Version returns the version the customer was read or created at. A
// repository's update stores the next one, and only while the stored version
// is still this one.
func (c *Customer) Version() int { return c.version }

// FirstName returns the part of name before the first space, for greetings
// in WhatsApp messages ("Oi, João!"). It returns the whole name when it has
// no space and "" for a blank name.
func FirstName(name string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(name), " ")
	return first
}
