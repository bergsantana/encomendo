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

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// ErrRowSecurity is what the fake returns for a write RLS would refuse, as
// Postgres does with SQLSTATE 42501. It matches no sentinel: a use case
// that writes in the wrong scope is a bug and a 500.
var ErrRowSecurity = errors.New("fake: new row violates row-level security policy")

// Store is an in-memory appsupplier.Repository with the staff_only policy of
// 0009_suppliers emulated from the scope in the context: a store's suppliers
// are visible only in a scope of that store with a user, and writes
// elsewhere fail with ErrRowSecurity.
//
// Like adapter/postgres it refuses calls outside a fake transaction, and a
// failed transaction undoes its writes (sharedfake.KeepOnRollback; ADR
// 0009).
type Store struct {
	mu     sync.Mutex
	rows   map[shared.SupplierID]supplier.Record
	faults map[string]error
}

var _ appsupplier.Repository = (*Store)(nil)

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{rows: map[shared.SupplierID]supplier.Record{}, faults: map[string]error{}}
}

// Fail makes every later call of method return err; nil heals it.
func (s *Store) Fail(method string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults[method] = err
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

// sees reports whether the staff_only policy shows tenant's rows in scope:
// the tenant matches and a user is set.
func sees(scope appshared.Scope, tenant shared.TenantID) bool {
	return !tenant.IsZero() && scope.TenantID == tenant && !scope.UserID.IsZero()
}

func record(sup *supplier.Supplier) supplier.Record {
	return supplier.Record{
		Tenant: sup.TenantID(), ID: sup.ID(), Name: sup.Name(), Phone: sup.WhatsAppPhone(), Version: sup.Version(),
	}
}

// Insert implements appsupplier.Repository.
func (s *Store) Insert(ctx context.Context, sup *supplier.Supplier) error {
	scope, err := s.enter(ctx, "Insert")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	if !sees(scope, sup.TenantID()) {
		return ErrRowSecurity
	}
	if _, taken := s.rows[sup.ID()]; taken { // the primary key sees every row, whatever RLS shows
		return fmt.Errorf("suppliers_pkey: %w", shared.ErrConflict)
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.rows, sup.ID())
	s.rows[sup.ID()] = record(sup)
	return nil
}

// Update implements appsupplier.Repository.
func (s *Store) Update(ctx context.Context, sup *supplier.Supplier) error {
	scope, err := s.enter(ctx, "Update")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	r, ok := s.rows[sup.ID()]
	if !ok || r.Tenant != sup.TenantID() || !sees(scope, r.Tenant) {
		return supplier.ErrNotFound
	}
	if r.Version != sup.Version() {
		return shared.ErrVersionConflict
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.rows, sup.ID())
	r.Name, r.Phone, r.Version = sup.Name(), sup.WhatsAppPhone(), r.Version+1
	s.rows[sup.ID()] = r
	return nil
}

// ByID implements appsupplier.Repository.
func (s *Store) ByID(ctx context.Context, tenant shared.TenantID, id shared.SupplierID) (*supplier.Supplier, error) {
	scope, err := s.enter(ctx, "ByID")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	r, ok := s.rows[id]
	if !ok || r.Tenant != tenant || !sees(scope, tenant) {
		return nil, supplier.ErrNotFound
	}
	return supplier.Restore(r)
}

// List implements appsupplier.Repository.
func (s *Store) List(ctx context.Context, tenant shared.TenantID, filter appsupplier.ListFilter, page shared.PageRequest) (shared.Page[*supplier.Supplier], error) {
	scope, err := s.enter(ctx, "List")
	if err != nil {
		return shared.Page[*supplier.Supplier]{}, err
	}
	defer s.mu.Unlock()
	after, err := decodeCursor(page.Cursor())
	if err != nil {
		return shared.Page[*supplier.Supplier]{}, err
	}
	query := strings.ToLower(filter.Query)
	var rows []supplier.Record
	for _, r := range s.rows {
		if r.Tenant == tenant && sees(scope, tenant) && strings.Contains(strings.ToLower(r.Name), query) {
			rows = append(rows, r)
		}
	}
	slices.SortFunc(rows, compare)
	if page.Cursor() != "" { // keyset: only the rows after the last one of the previous page
		rows = slices.DeleteFunc(rows, func(r supplier.Record) bool { return !after.before(r) })
	}
	return pageOf(rows, page.Limit())
}

// ByIDs implements appsupplier.Repository.
func (s *Store) ByIDs(ctx context.Context, tenant shared.TenantID, ids []shared.SupplierID) ([]*supplier.Supplier, error) {
	scope, err := s.enter(ctx, "ByIDs")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	out := []*supplier.Supplier{}
	seen := map[shared.SupplierID]bool{}
	for _, id := range ids {
		r, ok := s.rows[id]
		if !ok || seen[id] || r.Tenant != tenant || !sees(scope, tenant) {
			continue
		}
		seen[id] = true
		sup, err := supplier.Restore(r)
		if err != nil {
			return nil, err
		}
		out = append(out, sup)
	}
	return out, nil
}

// compare orders suppliers by name, then id.
func compare(a, b supplier.Record) int {
	return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.ID.String(), b.ID.String()))
}

// pageOf cuts one page from rows, which are sorted by compare, and issues
// the cursor of its last row when more follow.
func pageOf(rows []supplier.Record, limit int) (shared.Page[*supplier.Supplier], error) {
	end := min(limit, len(rows))
	out := shared.Page[*supplier.Supplier]{Items: make([]*supplier.Supplier, 0, end)}
	for _, r := range rows[:end] {
		sup, err := supplier.Restore(r)
		if err != nil {
			return shared.Page[*supplier.Supplier]{}, err
		}
		out.Items = append(out.Items, sup)
	}
	if end < len(rows) {
		out.NextCursor = encodeCursor(rows[end-1])
	}
	return out, nil
}

// position is what a cursor holds: the last row of the page before.
type position struct{ name, id string }

// before reports whether the cursor's row sorts before r.
func (p position) before(r supplier.Record) bool {
	return cmp.Or(strings.Compare(p.name, r.Name), strings.Compare(p.id, r.ID.String())) < 0
}

func encodeCursor(r supplier.Record) string {
	return base64.RawURLEncoding.EncodeToString([]byte(r.Name + "\x00" + r.ID.String()))
}

// decodeCursor reads a cursor this fake issued. A cursor it did not issue is
// a ValidationError, as a real repository answers one.
func decodeCursor(c string) (position, error) {
	if c == "" {
		return position{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(c)
	name, id, found := strings.Cut(string(raw), "\x00")
	if err != nil || !found || id == "" {
		return position{}, shared.NewValidationError(shared.FieldError{Field: "cursor", Message: "Cursor inválido."})
	}
	return position{name: name, id: id}, nil
}
