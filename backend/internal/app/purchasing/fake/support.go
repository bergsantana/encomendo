package fake

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Suppliers is an in-memory apppurchasing.Suppliers.
type Suppliers struct {
	mu       sync.Mutex
	contacts map[shared.SupplierID]apppurchasing.SupplierContact
}

var _ apppurchasing.Suppliers = (*Suppliers)(nil)

// NewSuppliers returns a directory with no supplier.
func NewSuppliers() *Suppliers {
	return &Suppliers{contacts: map[shared.SupplierID]apppurchasing.SupplierContact{}}
}

// Add stores a supplier.
func (s *Suppliers) Add(c apppurchasing.SupplierContact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contacts[c.ID] = c
}

// Contact implements apppurchasing.Suppliers.
func (s *Suppliers) Contact(_ context.Context, id shared.SupplierID) (apppurchasing.SupplierContact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.contacts[id]
	if !ok {
		return apppurchasing.SupplierContact{}, fmt.Errorf("fake: supplier %s: %w", id, shared.ErrNotFound)
	}
	return c, nil
}

// Store is an apppurchasing.Store with fixed facts; its day follows the
// clock in its time zone.
type Store struct {
	name, currency string
	loc            *time.Location
	clock          appshared.Clock
}

var _ apppurchasing.Store = (*Store)(nil)

// NewStore returns a store with name and currency, whose day is the
// clock's in loc.
func NewStore(name, currency string, loc *time.Location, clock appshared.Clock) *Store {
	return &Store{name: name, currency: currency, loc: loc, clock: clock}
}

// Name implements apppurchasing.Store.
func (s *Store) Name(context.Context) (string, error) { return s.name, nil }

// Today implements apppurchasing.Store.
func (s *Store) Today(context.Context) (shared.CivilDate, error) {
	return shared.DateOf(s.clock.Now(), s.loc), nil
}

// Currency implements apppurchasing.Store.
func (s *Store) Currency(context.Context) (string, error) { return s.currency, nil }

// Location implements apppurchasing.Store.
func (s *Store) Location(context.Context) (*time.Location, error) { return s.loc, nil }

// SupplierLinks builds deterministic wa.me links:
// https://wa.me/{phone}?text={url-encoded text}, or no query for no text.
// Like the real builder it refuses a phone that is not 55 + DDD + number.
type SupplierLinks struct{}

var _ apppurchasing.SupplierLinks = SupplierLinks{}

// Link implements apppurchasing.SupplierLinks.
func (SupplierLinks) Link(phone, text string) (string, error) {
	if !shared.ValidWhatsAppPhone(phone) {
		return "", fmt.Errorf("fake: wa.me link: invalid phone: %w", shared.ErrInvalid)
	}
	if text == "" {
		return "https://wa.me/" + phone, nil
	}
	return "https://wa.me/" + phone + "?text=" + url.QueryEscape(text), nil
}
