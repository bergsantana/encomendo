package tenancy

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

const maxStoreNameLength = 80

// StoreName is the store's name as clients see it: trimmed, 1 to 80
// visible characters. The zero StoreName is "no name".
type StoreName struct {
	s string
}

// ParseStoreName trims s and checks it. It returns a ValidationError on the
// field "name".
func ParseStoreName(s string) (StoreName, error) {
	s = strings.TrimSpace(s)
	if !visible(s, maxStoreNameLength) {
		return StoreName{}, shared.NewValidationError(shared.FieldError{
			Field:   "name",
			Message: "Informe um nome de 1 a 80 caracteres.",
		})
	}
	return StoreName{s: s}, nil
}

// visible reports whether s has 1 to max characters, none of them control or
// invisible.
func visible(s string, max int) bool {
	if s == "" || utf8.RuneCountInString(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// String returns the name.
func (n StoreName) String() string { return n.s }

// IsZero reports whether n is the zero StoreName.
func (n StoreName) IsZero() bool { return n.s == "" }

// Status says whether a store works. Only the platform suspends a store;
// the store itself cannot change it (0007_rls_policies grants no UPDATE on
// status).
type Status string

// The store statuses.
const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
)

// Tenant is one store.
type Tenant struct {
	id        shared.TenantID
	name      StoreName
	slug      Slug
	status    Status
	version   int
	createdAt time.Time
}

// NewTenant returns an active store at version 1. It returns
// ErrInvalidTenant for a zero id, name or slug.
func NewTenant(id shared.TenantID, name StoreName, slug Slug, now time.Time) (*Tenant, error) {
	if id.IsZero() || name.IsZero() || slug.IsZero() {
		return nil, fmt.Errorf("new tenant %v: %w", id, ErrInvalidTenant)
	}
	return &Tenant{id: id, name: name, slug: slug, status: StatusActive, version: 1, createdAt: now}, nil
}

// TenantRecord is a store as a repository stores it.
type TenantRecord struct {
	ID        shared.TenantID
	Slug      string
	Name      string
	Status    Status
	Version   int
	CreatedAt time.Time
}

// RestoreTenant rebuilds a stored store. It returns ErrInvalidTenant when the
// record breaks a rule a stored store always keeps.
func RestoreTenant(r TenantRecord) (*Tenant, error) {
	slug, slugErr := RestoreSlug(r.Slug)
	name, nameErr := ParseStoreName(r.Name)
	if slugErr != nil || nameErr != nil || (r.Status != StatusActive && r.Status != StatusSuspended) || r.Version < 1 {
		return nil, fmt.Errorf("restore tenant %v: %w", r.ID, ErrInvalidTenant)
	}
	t, err := NewTenant(r.ID, name, slug, r.CreatedAt)
	if err != nil {
		return nil, err
	}
	t.status, t.version = r.Status, r.Version
	return t, nil
}

// ID returns the store's id.
func (t *Tenant) ID() shared.TenantID { return t.id }

// Name returns the store's name.
func (t *Tenant) Name() StoreName { return t.name }

// Slug returns the store's address label.
func (t *Tenant) Slug() Slug { return t.slug }

// Status returns whether the store works.
func (t *Tenant) Status() Status { return t.status }

// Suspended reports whether the platform suspended the store (ADR 0022).
func (t *Tenant) Suspended() bool { return t.status == StatusSuspended }

// Version returns the version the store was read or created at.
func (t *Tenant) Version() int { return t.version }

// CreatedAt returns when the store was created.
func (t *Tenant) CreatedAt() time.Time { return t.createdAt }
