package fake

import (
	"context"
	"fmt"
	"slices"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

func optionInputs(options []catalog.CustomizationOption) []catalog.OptionInput {
	out := make([]catalog.OptionInput, 0, len(options))
	for _, o := range options {
		in := catalog.OptionInput{ID: o.ID(), Name: o.Name(), Required: o.Required()}
		for _, c := range o.Choices() {
			in.Choices = append(in.Choices, catalog.ChoiceInput{ID: c.ID(), Label: c.Label(), ExtraPrice: c.ExtraPrice(), ExtraDays: c.ExtraDays()})
		}
		out = append(out, in)
	}
	return out
}

// split turns an item into its items row and its staff-only rows.
func split(it *catalog.Item) (itemRow, links, *shared.Money) {
	rec := catalog.ItemRecord{
		Tenant: it.TenantID(), ID: it.ID(), Name: it.Name(), Description: it.Description(), Category: it.CategoryID(),
		Price: it.Price(), Sourcing: catalog.SourcingRecord{Mode: it.SourcingMode(), ProductionDays: it.ProductionDays()},
		Attributes: it.AttributeValueIDs(), Options: optionInputs(it.Options()), Photos: it.Photos(),
		Published: it.Published(), Archived: it.Archived(), Version: it.Version(),
	}
	var l links
	if m, ok := it.MainSupplier(); ok {
		l.main = &m
	}
	if b, ok := it.BackupSupplier(); ok {
		l.backup = &b
	}
	return itemRow{rec: rec, readyDays: it.ReadyDays()}, l, it.ProductionCost()
}

// restore rebuilds a whole item from its rows. Call it with s.mu held, in a
// staff scope.
func (s *Store) restore(row itemRow) (*catalog.Item, error) {
	rec := row.rec
	l := s.links[rec.ID]
	rec.Sourcing.Main, rec.Sourcing.Backup = l.main, l.backup
	if c, ok := s.costs[rec.ID]; ok {
		rec.Sourcing.ProductionCost = &c
	}
	return catalog.RestoreItem(rec)
}

// checkItemWrite refuses what RLS and the foreign keys of 0008_catalog
// would refuse for row with its staff-only rows. Call it with s.mu held.
func (s *Store) checkItemWrite(scope appshared.Scope, row itemRow, l links, cost *shared.Money) error {
	staffRows := l.main != nil || l.backup != nil || cost != nil
	if !sees(scope, row.rec.Tenant) || (staffRows && !seesStaff(scope, row.rec.Tenant)) {
		return ErrRowSecurity
	}
	if c, ok := s.categories[row.rec.Category]; !row.rec.Category.IsZero() && (!ok || c.Tenant != row.rec.Tenant) {
		return fmt.Errorf("items category: %w", errForeignKey)
	}
	if !s.knownValues(row.rec.Tenant, row.rec.Attributes) {
		return fmt.Errorf("item_attribute_values value: %w", errForeignKey)
	}
	return nil
}

// put writes an item's rows, undone if the transaction fails. Call it with
// s.mu held.
func (s *Store) put(ctx context.Context, row itemRow, l links, cost *shared.Money) {
	id := row.rec.ID
	sharedfake.KeepOnRollback(ctx, &s.mu, s.items, id)
	sharedfake.KeepOnRollback(ctx, &s.mu, s.links, id)
	sharedfake.KeepOnRollback(ctx, &s.mu, s.costs, id)
	s.items[id] = row
	s.links[id] = l
	delete(s.costs, id)
	if cost != nil {
		s.costs[id] = *cost
	}
}

// Insert implements appcatalog.Items ("ItemsInsert").
func (s *Items) Insert(ctx context.Context, it *catalog.Item) error {
	scope, err := s.enter(ctx, "ItemsInsert")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	row, l, cost := split(it)
	if err := s.checkItemWrite(scope, row, l, cost); err != nil {
		return err
	}
	if _, taken := s.items[it.ID()]; taken {
		return fmt.Errorf("items_pkey: %w", shared.ErrConflict)
	}
	s.put(ctx, row, l, cost)
	return nil
}

// Update implements appcatalog.Items ("ItemsUpdate"). It keeps the stored
// photos: they are their own resource.
func (s *Items) Update(ctx context.Context, it *catalog.Item) error {
	scope, err := s.enter(ctx, "ItemsUpdate")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	stored, ok := s.items[it.ID()]
	if !ok || stored.rec.Tenant != it.TenantID() || !sees(scope, stored.rec.Tenant) {
		return catalog.ErrItemNotFound
	}
	if stored.rec.Version != it.Version() {
		return shared.ErrVersionConflict
	}
	row, l, cost := split(it)
	if err := s.checkItemWrite(scope, row, l, cost); err != nil {
		return err
	}
	row.rec.Photos, row.rec.Version = stored.rec.Photos, stored.rec.Version+1
	s.put(ctx, row, l, cost)
	return nil
}

// ByID implements appcatalog.Items ("ItemsByID"). With no user in the scope
// it finds nothing, as the contract says.
func (s *Items) ByID(ctx context.Context, tenant shared.TenantID, id shared.ItemID) (*catalog.Item, error) {
	scope, err := s.enter(ctx, "ItemsByID")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	row, ok := s.items[id]
	if !ok || row.rec.Tenant != tenant || !seesStaff(scope, tenant) {
		return nil, catalog.ErrItemNotFound
	}
	return s.restore(row)
}

// ByIDs implements appcatalog.Items ("ItemsByIDs"). The cost comes from the
// staff-only rows, so it is nil in a scope with no user.
func (s *Items) ByIDs(ctx context.Context, tenant shared.TenantID, ids []shared.ItemID) ([]appcatalog.ItemFacts, error) {
	scope, err := s.enter(ctx, "ItemsByIDs")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	out := []appcatalog.ItemFacts{}
	seen := map[shared.ItemID]bool{}
	for _, id := range ids {
		row, ok := s.items[id]
		if !ok || seen[id] || row.rec.Tenant != tenant || !sees(scope, tenant) {
			continue
		}
		seen[id] = true
		f := catalog.ItemFacts{Price: row.rec.Price, ReadyDays: row.readyDays, Archived: row.rec.Archived}
		if seesStaff(scope, tenant) {
			f.Cost = s.costOf(row)
		}
		out = append(out, appcatalog.ItemFacts{ID: id, Name: row.rec.Name, Facts: f})
	}
	return out, nil
}

// costOf is the item's cost from its staff-only rows: the main supplier's,
// or the production cost of an in_house item.
func (s *Store) costOf(row itemRow) *shared.Money {
	if row.rec.Sourcing.Mode == catalog.ModeInHouse {
		if c, ok := s.costs[row.rec.ID]; ok {
			return &c
		}
		return nil
	}
	if m := s.links[row.rec.ID].main; m != nil {
		return m.Cost()
	}
	return nil
}

// List implements appcatalog.Items ("ItemsList").
func (s *Items) List(ctx context.Context, tenant shared.TenantID, filter appcatalog.ItemFilter, page shared.PageRequest) (shared.Page[*catalog.Item], error) {
	scope, err := s.enter(ctx, "ItemsList")
	if err != nil {
		return shared.Page[*catalog.Item]{}, err
	}
	defer s.mu.Unlock()
	var rows []named
	for id, row := range s.items {
		if row.rec.Tenant == tenant && seesStaff(scope, tenant) {
			rows = append(rows, named{id: shared.ID(id), name: row.rec.Name})
		}
	}
	keep := func(id shared.ID) bool {
		r := s.items[shared.ItemID(id)].rec
		return contains(r.Name, filter.Query) && (filter.IncludeArchived || !r.Archived) &&
			(filter.Category.IsZero() || r.Category == filter.Category)
	}
	ids, next, err := pageIDs(rows, keep, page)
	if err != nil {
		return shared.Page[*catalog.Item]{}, err
	}
	out := shared.Page[*catalog.Item]{Items: make([]*catalog.Item, 0, len(ids)), NextCursor: next}
	for _, id := range ids {
		it, err := s.restore(s.items[shared.ItemID(id)])
		if err != nil {
			return shared.Page[*catalog.Item]{}, err
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

// ActiveKitsHolding implements appcatalog.Items ("ItemsActiveKitsHolding").
func (s *Items) ActiveKitsHolding(ctx context.Context, tenant shared.TenantID, id shared.ItemID) ([]appcatalog.Reference, error) {
	scope, err := s.enter(ctx, "ItemsActiveKitsHolding")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	var rows []named
	for kid, rec := range s.kits {
		if rec.Tenant != tenant || !sees(scope, tenant) || rec.Archived || !holds(rec, id) {
			continue
		}
		rows = append(rows, named{id: shared.ID(kid), name: rec.Name})
	}
	slices.SortFunc(rows, compareNamed)
	out := make([]appcatalog.Reference, 0, len(rows))
	for _, r := range rows {
		out = append(out, appcatalog.Reference{ID: r.id, Name: r.name})
	}
	return out, nil
}

func holds(rec catalog.KitRecord, id shared.ItemID) bool {
	for _, ki := range rec.Items {
		if ki.Item == id || slices.Contains(ki.Alternatives, id) {
			return true
		}
	}
	return false
}

// Published implements appcatalog.Items ("ItemsPublished"). It reads only
// the items rows, so it works in a scope with no user.
func (s *Items) Published(ctx context.Context, tenant shared.TenantID) ([]appcatalog.PublishedItem, error) {
	scope, err := s.enter(ctx, "ItemsPublished")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	var rows []named
	for id, row := range s.items {
		if row.rec.Tenant == tenant && sees(scope, tenant) && row.rec.Published && !row.rec.Archived {
			rows = append(rows, named{id: shared.ID(id), name: row.rec.Name})
		}
	}
	slices.SortFunc(rows, compareNamed)
	out := make([]appcatalog.PublishedItem, 0, len(rows))
	for _, r := range rows {
		row := s.items[shared.ItemID(r.id)]
		options := make([]catalog.CustomizationOption, 0, len(row.rec.Options))
		for _, in := range row.rec.Options {
			o, err := catalog.NewOption(in)
			if err != nil {
				return nil, err
			}
			options = append(options, o)
		}
		out = append(out, appcatalog.PublishedItem{
			ID: row.rec.ID, Name: row.rec.Name, Description: row.rec.Description, Category: row.rec.Category,
			Price: row.rec.Price, ReadyDays: row.readyDays, AttributeValues: slices.Clone(row.rec.Attributes), Options: options,
		})
	}
	return out, nil
}

// SetItemPhotos replaces the photos stored for an item, without changing
// its version, as the photo use cases of step 3.9 will. Tests use it to
// give items photos.
func (s *Store) SetItemPhotos(id shared.ItemID, photos []catalog.Photo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.items[id]
	row.rec.Photos = slices.Clone(photos)
	s.items[id] = row
}

// ForceArchived marks an item archived and unpublished without any check,
// as a concurrent archive could leave it. Tests use it for states the use
// cases never reach on their own.
func (s *Store) ForceArchived(id shared.ItemID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.items[id]
	row.rec.Archived, row.rec.Published = true, false
	s.items[id] = row
}

// StoredReadyDays returns items.ready_days as the store keeps it.
func (s *Store) StoredReadyDays(id shared.ItemID) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items[id].readyDays
}

// StoredLinks returns the item's supplier_items rows, main and backup (nil
// when absent), for tests that check what was written.
func (s *Store) StoredLinks(id shared.ItemID) (main, backup *supplier.SupplierItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.links[id]
	return l.main, l.backup
}
