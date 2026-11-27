package fake

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrRowSecurity is what the fake returns for a write RLS would refuse, as
// Postgres does with SQLSTATE 42501. It matches no sentinel: a use case
// that writes in the wrong scope is a bug and a 500.
var ErrRowSecurity = errors.New("fake: new row violates row-level security policy")

// Store is an in-memory appcustomer.Repository with the tenant_isolation
// policy of 0012_customers emulated from the scope in the context: a
// store's customers are visible only in a scope of that store, with or
// without a user, and writes elsewhere fail with ErrRowSecurity. A number is
// unique per store, as in the table.
//
// Like adapter/postgres it refuses calls outside a fake transaction, and a
// failed transaction undoes its writes (sharedfake.KeepOnRollback; ADR
// 0009).
type Store struct {
	mu     sync.Mutex
	rows   map[shared.CustomerID]customer.Record
	orders map[shared.CustomerID][]time.Time
	faults map[string]error
}

var _ appcustomer.Repository = (*Store)(nil)

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		rows:   map[shared.CustomerID]customer.Record{},
		orders: map[shared.CustomerID][]time.Time{},
		faults: map[string]error{},
	}
}

// Fail makes every later call of method return err; nil heals it.
func (s *Store) Fail(method string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults[method] = err
}

// AddOrder records that the customer has an order created at at, which the
// list counts. The orders table belongs to another context, so tests tell
// the fake about its orders instead of storing any.
func (s *Store) AddOrder(id shared.CustomerID, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders[id] = append(s.orders[id], at)
}

// enter checks the transaction and the fault for method and returns the
// scope. On success it returns with s.mu held.
func (s *Store) enter(ctx context.Context, method string) (appshared.Scope, error) {
	if !sharedfake.InTx(ctx) {
		return appshared.Scope{}, appshared.ErrNoTransaction
	}
	s.mu.Lock()
	if err := s.faults[method]; err != nil {
		s.mu.Unlock()
		return appshared.Scope{}, err
	}
	return appshared.ScopeFrom(ctx), nil
}

// sees reports whether tenant_isolation shows tenant's rows in scope.
func sees(scope appshared.Scope, tenant shared.TenantID) bool {
	return !tenant.IsZero() && scope.TenantID == tenant
}

func record(c *customer.Customer, clientAccount shared.ClientAccountID) customer.Record {
	return customer.Record{
		Tenant: c.TenantID(), ID: c.ID(), Name: c.Name(), Phone: c.Phone(), Email: c.Email().String(),
		Notes: c.Notes(), Addresses: c.Addresses(), ClientAccountID: clientAccount, Version: c.Version(),
	}
}

// conflict returns what the unique keys of the table would answer for r, or
// nil: the number of another customer of the store (ErrPhoneTaken), or an
// address id another customer owns, which is clash: the primary key's
// conflict for an insert, and the repository's own refusal for an update.
// Rows with r's own id are skipped: a customer never conflicts with itself.
func (s *Store) conflict(r customer.Record, clash error) error {
	for id, other := range s.rows {
		if id == r.ID {
			continue
		}
		if other.Tenant == r.Tenant && other.Phone == r.Phone {
			return appcustomer.ErrPhoneTaken
		}
		for _, a := range r.Addresses {
			if slices.ContainsFunc(other.Addresses, func(o customer.Address) bool { return o.ID == a.ID }) {
				return fmt.Errorf("address %s belongs to another customer: %w", a.ID, clash)
			}
		}
	}
	return nil
}

// Insert implements appcustomer.Repository.
func (s *Store) Insert(ctx context.Context, c *customer.Customer) error {
	scope, err := s.enter(ctx, "Insert")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	if !sees(scope, c.TenantID()) {
		return ErrRowSecurity
	}
	if _, taken := s.rows[c.ID()]; taken { // the primary key sees every row, whatever RLS shows
		return fmt.Errorf("customers_pkey: %w", shared.ErrConflict)
	}
	r := record(c, shared.ClientAccountID{}) // a repository never writes the link
	if err := s.conflict(r, shared.ErrConflict); err != nil {
		return err
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.rows, c.ID())
	s.rows[c.ID()] = r
	return nil
}

// Update implements appcustomer.Repository.
func (s *Store) Update(ctx context.Context, c *customer.Customer) error {
	scope, err := s.enter(ctx, "Update")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	old, ok := s.rows[c.ID()]
	if !ok || old.Tenant != c.TenantID() || !sees(scope, old.Tenant) {
		return customer.ErrNotFound
	}
	if old.Version != c.Version() {
		return shared.ErrVersionConflict
	}
	r := record(c, old.ClientAccountID)
	r.Version = old.Version + 1
	if err := s.conflict(r, shared.ErrInvalid); err != nil {
		return err
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.rows, c.ID())
	s.rows[c.ID()] = r
	return nil
}

// ByID implements appcustomer.Repository.
func (s *Store) ByID(ctx context.Context, tenant shared.TenantID, id shared.CustomerID) (*customer.Customer, error) {
	scope, err := s.enter(ctx, "ByID")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	r, ok := s.rows[id]
	if !ok || r.Tenant != tenant || !sees(scope, tenant) {
		return nil, customer.ErrNotFound
	}
	return customer.Restore(r)
}

// ByPhone implements appcustomer.Repository.
func (s *Store) ByPhone(ctx context.Context, tenant shared.TenantID, phone string) (*customer.Customer, error) {
	scope, err := s.enter(ctx, "ByPhone")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	for _, r := range s.rows {
		if r.Tenant == tenant && r.Phone == phone && sees(scope, tenant) {
			return customer.Restore(r)
		}
	}
	return nil, customer.ErrNotFound
}

// List implements appcustomer.Repository.
func (s *Store) List(ctx context.Context, tenant shared.TenantID, f appcustomer.ListFilter, page shared.PageRequest) (shared.Page[appcustomer.Summary], error) {
	scope, err := s.enter(ctx, "List")
	if err != nil {
		return shared.Page[appcustomer.Summary]{}, err
	}
	defer s.mu.Unlock()
	var rows []customer.Record // every row the scope sees
	for _, r := range s.rows {
		if r.Tenant == tenant && sees(scope, tenant) {
			rows = append(rows, r)
		}
	}
	slices.SortFunc(rows, compare)
	if page.Cursor() != "" {
		rows, err = afterCursor(rows, page.Cursor())
		if err != nil {
			return shared.Page[appcustomer.Summary]{}, err
		}
	}
	rows = slices.DeleteFunc(rows, func(r customer.Record) bool { return !matches(r, f) })
	return s.pageOf(rows, page.Limit()), nil
}

// matches reports whether r passes f: its name starts with the query, or its
// number contains the query's digits.
func matches(r customer.Record, f appcustomer.ListFilter) bool {
	prefix, digits := f.NamePrefix(), f.PhoneDigits()
	if prefix == "" && digits == "" {
		return true
	}
	if prefix != "" && strings.HasPrefix(strings.ToLower(r.Name), prefix) {
		return true
	}
	return digits != "" && strings.Contains(r.Phone, digits)
}

// compare orders customers by lower-cased name, then id.
func compare(a, b customer.Record) int {
	return cmp.Or(strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), strings.Compare(a.ID.String(), b.ID.String()))
}

// pageOf cuts one page from rows, which are sorted by compare, and issues
// the cursor of its last row when more follow.
func (s *Store) pageOf(rows []customer.Record, limit int) shared.Page[appcustomer.Summary] {
	end := min(limit, len(rows))
	out := shared.Page[appcustomer.Summary]{Items: make([]appcustomer.Summary, 0, end)}
	for _, r := range rows[:end] {
		out.Items = append(out.Items, s.summary(r))
	}
	if end < len(rows) {
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(rows[end-1].ID.String()))
	}
	return out
}

// summary counts the customer's orders and finds the newest.
func (s *Store) summary(r customer.Record) appcustomer.Summary {
	out := appcustomer.Summary{ID: r.ID, Name: r.Name, Phone: r.Phone}
	at := s.orders[r.ID]
	out.Orders = len(at)
	if len(at) > 0 {
		last := slices.MaxFunc(at, time.Time.Compare)
		out.LastOrderAt = &last
	}
	return out
}

func invalidCursor() error {
	return shared.NewValidationError(shared.FieldError{Field: "cursor", Message: "Cursor inválido."})
}

// afterCursor returns the rows after the one the cursor names, found by id
// among rows, which are sorted by compare and are all the scope sees. A
// cursor this fake did not issue, or one naming a row the scope does not
// see, is a ValidationError, as a real repository answers one.
func afterCursor(rows []customer.Record, cursor string) ([]customer.Record, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, invalidCursor()
	}
	id, err := shared.ParseCustomerID(string(raw))
	if err != nil || id.String() != string(raw) {
		return nil, invalidCursor()
	}
	i := slices.IndexFunc(rows, func(r customer.Record) bool { return r.ID == id })
	if i < 0 {
		return nil, invalidCursor()
	}
	return rows[i+1:], nil
}
