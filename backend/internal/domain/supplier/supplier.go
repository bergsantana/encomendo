package supplier

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

const maxNameLength = 120

// Supplier is one of the store's suppliers: who it is and the WhatsApp
// number the purchase messages go to (docs/product/design-document.md,
// Catálogo and Compras).
type Supplier struct {
	tenant  shared.TenantID
	id      shared.SupplierID
	name    string
	phone   string
	version int
}

// New returns a supplier at version 1. The name is trimmed. It returns a
// ValidationError listing every invalid field ("name", "whatsappPhone") and
// ErrInvalidSupplier for a zero tenant or id.
func New(tenant shared.TenantID, id shared.SupplierID, name, whatsapp string) (*Supplier, error) {
	if tenant.IsZero() || id.IsZero() {
		return nil, fmt.Errorf("new supplier %v: %w", id, ErrInvalidSupplier)
	}
	name = strings.TrimSpace(name)
	if err := shared.MergeValidation(checkName(name), checkPhone(whatsapp)); err != nil {
		return nil, err
	}
	return &Supplier{tenant: tenant, id: id, name: name, phone: whatsapp, version: 1}, nil
}

// Record is a supplier as a repository stores it.
type Record struct {
	Tenant  shared.TenantID
	ID      shared.SupplierID
	Name    string
	Phone   string
	Version int
}

// Restore rebuilds a stored supplier. It returns ErrInvalidSupplier when the
// record breaks a rule a stored supplier always keeps.
func Restore(r Record) (*Supplier, error) {
	if r.Version < 1 || r.Name != strings.TrimSpace(r.Name) {
		return nil, fmt.Errorf("restore supplier %v: %w", r.ID, ErrInvalidSupplier)
	}
	s, err := New(r.Tenant, r.ID, r.Name, r.Phone)
	if err != nil {
		// The cause is text only: wrapping New's ValidationError would make a
		// bad stored row match shared.ErrInvalid and answer 400, not 500.
		return nil, fmt.Errorf("restore supplier %v: %w: %s", r.ID, ErrInvalidSupplier, err.Error())
	}
	s.version = r.Version
	return s, nil
}

// checkName reports a ValidationError on "name" unless name, already
// trimmed, has 1 to 120 characters, valid UTF-8 and no control character. A
// name that breaks the last two gets its own message: shortening it would not
// help.
func checkName(name string) error {
	if name == "" {
		return nameError(msgNameRequired)
	}
	if !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return nameError(msgNameControl)
	}
	if utf8.RuneCountInString(name) > maxNameLength {
		return nameError(msgNameTooLong)
	}
	return nil
}

func nameError(msg string) error {
	return shared.NewValidationError(shared.FieldError{Field: fieldName, Message: msg})
}

// checkPhone reports a ValidationError on "whatsappPhone" unless phone is a
// number wa.me accepts. The number is never reformatted: clients send digits
// only (docs/api/openapi.yaml, SupplierPhone).
func checkPhone(phone string) error {
	switch {
	case phone == "":
		return shared.NewValidationError(shared.FieldError{Field: fieldWhatsAppPhone, Message: msgPhoneRequired})
	case !shared.ValidWhatsAppPhone(phone):
		return shared.NewValidationError(shared.FieldError{Field: fieldWhatsAppPhone, Message: msgPhoneInvalid})
	}
	return nil
}

// TenantID returns the store the supplier belongs to.
func (s *Supplier) TenantID() shared.TenantID { return s.tenant }

// ID returns the supplier's id.
func (s *Supplier) ID() shared.SupplierID { return s.id }

// Name returns the supplier's name, trimmed.
func (s *Supplier) Name() string { return s.name }

// WhatsAppPhone returns the supplier's number: 55, the DDD and 8 or 9
// digits, digits only.
func (s *Supplier) WhatsAppPhone() string { return s.phone }

// Version returns the version the supplier was read or created at. A
// repository's update stores the next one, and only while the stored version
// is still this one.
func (s *Supplier) Version() int { return s.version }

// CheckVersion returns shared.ErrVersionConflict unless expected is the
// version the supplier was read at. The use case calls it with the version
// the client sent, before it changes anything.
func (s *Supplier) CheckVersion(expected int) error {
	if expected != s.version {
		return shared.ErrVersionConflict
	}
	return nil
}

// Rename gives the supplier a new name, trimmed. It returns a
// ValidationError on "name" when the name is blank or longer than 120
// characters; nothing changes then.
func (s *Supplier) Rename(name string) error {
	name = strings.TrimSpace(name)
	if err := checkName(name); err != nil {
		return err
	}
	s.name = name
	return nil
}

// ChangePhone gives the supplier a new WhatsApp number. It returns a
// ValidationError on "whatsappPhone" when the number is empty or not 55, a
// DDD and 8 or 9 digits; nothing changes then.
func (s *Supplier) ChangePhone(whatsapp string) error {
	if err := checkPhone(whatsapp); err != nil {
		return err
	}
	s.phone = whatsapp
	return nil
}
