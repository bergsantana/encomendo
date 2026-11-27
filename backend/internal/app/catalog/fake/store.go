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

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// ErrRowSecurity is what the fake returns for a write RLS would refuse, as
// Postgres does with SQLSTATE 42501. It matches no sentinel: a use case that
// writes in the wrong scope is a bug and a 500.
var ErrRowSecurity = errors.New("fake: new row violates row-level security policy")

// errArchivedInsert is what Items.Insert and Kits.Insert return for an
// archived aggregate: the constructors never produce one, and archiving is an
// Update. It matches no sentinel.
var errArchivedInsert = errors.New("fake: an archived row cannot be inserted")

// errForeignKey is what the fake returns for a row that points at a row its
// store does not have, as Postgres does with 23503 (mapped to ErrInvalid).
var errForeignKey = fmt.Errorf("fake: foreign key violation: %w", shared.ErrInvalid)

// itemRow is an items row: the item without its staff-only parts, which
// live in links and costs as they live in supplier_items and item_costs.
type itemRow struct {
	rec       catalog.ItemRecord
	readyDays int
}

// links are an item's supplier_items rows.
type links struct {
	main, backup *supplier.SupplierItem
}

type attributeRow struct {
	rec      catalog.AttributeRecord
	position int
}

// Store is an in-memory implementation of the catalog repositories, with
// the RLS of 0008_catalog and 0009_suppliers emulated from the scope in the
// context. Like adapter/postgres it refuses calls outside a fake
// transaction, and a failed transaction undoes its writes
// (sharedfake.KeepOnRollback; ADR 0009).
type Store struct {
	mu         sync.Mutex
	categories map[shared.CategoryID]catalog.CategoryRecord
	attributes map[shared.AttributeID]attributeRow
	items      map[shared.ItemID]itemRow
	links      map[shared.ItemID]links
	costs      map[shared.ItemID]shared.Money
	kits       map[shared.KitID]catalog.KitRecord
	faults     map[string]error
	// suppliers are the suppliers rows supplier_items points at, per store.
	suppliers map[shared.TenantID]map[shared.SupplierID]bool

	// The repositories over these maps.
	Categories *Categories
	Attributes *Attributes
	Items      *Items
	Kits       *Kits
	Photos     *Photos
}

// Categories is the appcatalog.Categories of a Store.
type Categories struct{ *Store }

// Attributes is the appcatalog.Attributes of a Store.
type Attributes struct{ *Store }

// Items is the appcatalog.Items of a Store.
type Items struct{ *Store }

// Kits is the appcatalog.Kits of a Store.
type Kits struct{ *Store }

var (
	_ appcatalog.Categories = (*Categories)(nil)
	_ appcatalog.Attributes = (*Attributes)(nil)
	_ appcatalog.Items      = (*Items)(nil)
	_ appcatalog.Kits       = (*Kits)(nil)
)

// NewStore returns an empty store.
func NewStore() *Store {
	s := &Store{
		categories: map[shared.CategoryID]catalog.CategoryRecord{},
		attributes: map[shared.AttributeID]attributeRow{},
		items:      map[shared.ItemID]itemRow{},
		links:      map[shared.ItemID]links{},
		costs:      map[shared.ItemID]shared.Money{},
		kits:       map[shared.KitID]catalog.KitRecord{},
		faults:     map[string]error{},
		suppliers:  map[shared.TenantID]map[shared.SupplierID]bool{},
	}
	s.Categories, s.Attributes, s.Items, s.Kits, s.Photos = &Categories{s}, &Attributes{s}, &Items{s}, &Kits{s}, &Photos{s}
	return s
}

// AddSupplier records a supplier of tenant, so items of that store may
// link to it; a link to any other supplier is refused as the foreign key of
// supplier_items refuses it.
func (s *Store) AddSupplier(tenant shared.TenantID, id shared.SupplierID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.suppliers[tenant] == nil {
		s.suppliers[tenant] = map[shared.SupplierID]bool{}
	}
	s.suppliers[tenant][id] = true
}

// Fail makes every later call of method (such as "ItemsByIDs" or
// "CategoriesList"; see each method) return err; nil heals it.
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

// sees reports whether tenant_isolation shows tenant's rows in scope.
func sees(scope appshared.Scope, tenant shared.TenantID) bool {
	return !tenant.IsZero() && scope.TenantID == tenant
}

// seesStaff reports whether staff_only shows tenant's rows in scope: the
// tenant matches and a user is set (ADR 0024).
func seesStaff(scope appshared.Scope, tenant shared.TenantID) bool {
	return sees(scope, tenant) && !scope.UserID.IsZero()
}

func invalidCursor() error {
	return shared.NewValidationError(shared.FieldError{Field: "cursor", Message: "Cursor inválido."})
}

func encodeCursor(id fmt.Stringer) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id.String()))
}

func decodeCursor(cursor string) (shared.ID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return shared.ID{}, invalidCursor()
	}
	id, err := shared.ParseID(string(raw))
	if err != nil {
		return shared.ID{}, invalidCursor()
	}
	return id, nil
}

// named is a row the lists order by name, then id.
type named struct {
	id   shared.ID
	name string
}

// compareNamed orders by lower(name), then id, as the port says.
func compareNamed(a, b named) int {
	return cmp.Or(strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)), strings.Compare(a.id.String(), b.id.String()))
}

// pageIDs cuts one page of ids from rows, the rows the scope sees, sorted
// here. keep filters them; the cursor names the last row of the previous
// page by id, found among all the rows, so a row that left the filter
// still marks the place. A cursor naming no visible row is a
// ValidationError.
func pageIDs(rows []named, keep func(shared.ID) bool, page shared.PageRequest) ([]shared.ID, string, error) {
	slices.SortFunc(rows, compareNamed)
	if page.Cursor() != "" {
		id, err := decodeCursor(page.Cursor())
		if err != nil {
			return nil, "", err
		}
		i := slices.IndexFunc(rows, func(r named) bool { return r.id == id })
		if i < 0 {
			return nil, "", invalidCursor()
		}
		rows = rows[i+1:]
	}
	var ids []shared.ID
	for _, r := range rows {
		if keep(r.id) {
			ids = append(ids, r.id)
		}
	}
	end := min(page.Limit(), len(ids))
	next := ""
	if end < len(ids) {
		next = encodeCursor(ids[end-1])
	}
	return ids[:end], next, nil
}

// contains reports whether name contains query, ignoring case; literal.
func contains(name, query string) bool {
	return strings.Contains(strings.ToLower(name), strings.ToLower(query))
}

// ---- Categories ----

func (s *Store) categoryNameTaken(c catalog.CategoryRecord) bool {
	for _, other := range s.categories {
		if other.Tenant == c.Tenant && other.ID != c.ID && strings.EqualFold(other.Name, c.Name) {
			return true
		}
	}
	return false
}

func categoryRecord(c *catalog.Category) catalog.CategoryRecord {
	return catalog.CategoryRecord{Tenant: c.TenantID(), ID: c.ID(), Name: c.Name(), Position: c.Position(), Version: c.Version()}
}

// List implements appcatalog.Categories ("CategoriesList").
func (s *Categories) List(ctx context.Context, tenant shared.TenantID) ([]*catalog.Category, error) {
	scope, err := s.enter(ctx, "CategoriesList")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	var recs []catalog.CategoryRecord
	for _, r := range s.categories {
		if r.Tenant == tenant && sees(scope, tenant) {
			recs = append(recs, r)
		}
	}
	slices.SortFunc(recs, func(a, b catalog.CategoryRecord) int {
		return cmp.Or(cmp.Compare(a.Position, b.Position), strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			strings.Compare(a.ID.String(), b.ID.String()))
	})
	out := make([]*catalog.Category, 0, len(recs))
	for _, r := range recs {
		c, err := catalog.RestoreCategory(r)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// ByID implements appcatalog.Categories ("CategoriesByID").
func (s *Categories) ByID(ctx context.Context, tenant shared.TenantID, id shared.CategoryID) (*catalog.Category, error) {
	scope, err := s.enter(ctx, "CategoriesByID")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	r, ok := s.categories[id]
	if !ok || r.Tenant != tenant || !sees(scope, tenant) {
		return nil, catalog.ErrCategoryNotFound
	}
	return catalog.RestoreCategory(r)
}

// Insert implements appcatalog.Categories ("CategoriesInsert").
func (s *Categories) Insert(ctx context.Context, c *catalog.Category) error {
	scope, err := s.enter(ctx, "CategoriesInsert")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	r := categoryRecord(c)
	switch {
	case !sees(scope, r.Tenant):
		return ErrRowSecurity
	case s.categoryExists(r.ID):
		return fmt.Errorf("categories_pkey: %w", shared.ErrConflict)
	case s.categoryNameTaken(r):
		return catalog.ErrNameTaken
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.categories, r.ID)
	s.categories[r.ID] = r
	return nil
}

func (s *Store) categoryExists(id shared.CategoryID) bool {
	_, ok := s.categories[id]
	return ok
}

// Update implements appcatalog.Categories ("CategoriesUpdate").
func (s *Categories) Update(ctx context.Context, c *catalog.Category) error {
	scope, err := s.enter(ctx, "CategoriesUpdate")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	stored, ok := s.categories[c.ID()]
	next := categoryRecord(c)
	switch {
	case !ok || stored.Tenant != next.Tenant || !sees(scope, stored.Tenant):
		return catalog.ErrCategoryNotFound
	case stored.Version != c.Version():
		return shared.ErrVersionConflict
	case s.categoryNameTaken(next):
		return catalog.ErrNameTaken
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.categories, c.ID())
	next.Version = stored.Version + 1
	s.categories[c.ID()] = next
	return nil
}

// Delete implements appcatalog.Categories ("CategoriesDelete").
func (s *Categories) Delete(ctx context.Context, tenant shared.TenantID, id shared.CategoryID) error {
	scope, err := s.enter(ctx, "CategoriesDelete")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	r, ok := s.categories[id]
	if !ok || r.Tenant != tenant || !sees(scope, tenant) {
		return catalog.ErrCategoryNotFound
	}
	for _, it := range s.items {
		if it.rec.Tenant == tenant && it.rec.Category == id {
			return catalog.ErrCategoryInUse
		}
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.categories, id)
	delete(s.categories, id)
	return nil
}
