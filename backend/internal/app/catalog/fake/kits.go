package fake

import (
	"context"
	"fmt"
	"slices"
	"strings"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func kitRecord(k *catalog.Kit) catalog.KitRecord {
	r := catalog.KitRecord{
		Tenant: k.TenantID(), ID: k.ID(), Name: k.Name(), Description: k.Description(), Price: k.Price(),
		SwapMode: k.SwapMode(), Items: k.Items(), Attributes: k.AttributeValueIDs(), Photos: k.Photos(),
		Published: k.Published(), Archived: k.Archived(), Version: k.Version(),
	}
	if d, ok := k.AssemblyDays(); ok {
		r.AssemblyDays = &d
	}
	if d, ok := k.SafetyMarginDays(); ok {
		r.SafetyMarginDays = &d
	}
	return r
}

// restoreKit rebuilds a stored kit. RestoreKit checks only that every item
// it names is in the lookup, which the foreign keys of kit_items guarantee,
// so the lookup holds the ids with no facts.
//
// Like the Postgres repository it returns the field values in the store's
// field order and each item's alternatives sorted by id (a set to callers).
// Call it with s.mu held.
func (s *Store) restoreKit(r catalog.KitRecord) (*catalog.Kit, error) {
	r.Attributes = s.inFieldOrder(r.Tenant, r.Attributes)
	r.Items = slices.Clone(r.Items)
	for i, ki := range r.Items {
		alts := slices.Clone(ki.Alternatives)
		slices.SortFunc(alts, func(a, b shared.ItemID) int { return strings.Compare(a.String(), b.String()) })
		r.Items[i].Alternatives = alts
	}
	lookup := catalog.ItemLookup{}
	for _, ki := range r.Items {
		lookup[ki.Item] = catalog.ItemFacts{}
		for _, alt := range ki.Alternatives {
			lookup[alt] = catalog.ItemFacts{}
		}
	}
	return catalog.RestoreKit(r, lookup)
}

// checkKitWrite refuses what RLS and the foreign keys would refuse. Call it
// with s.mu held.
func (s *Store) checkKitWrite(scope appshared.Scope, r catalog.KitRecord) error {
	if !sees(scope, r.Tenant) {
		return ErrRowSecurity
	}
	for _, ki := range r.Items {
		for _, id := range append([]shared.ItemID{ki.Item}, ki.Alternatives...) {
			if row, ok := s.items[id]; !ok || row.rec.Tenant != r.Tenant {
				return fmt.Errorf("kit_items item: %w", errForeignKey)
			}
		}
	}
	if !s.knownValues(r.Tenant, r.Attributes) {
		return fmt.Errorf("kit_attribute_values value: %w", errForeignKey)
	}
	return nil
}

// Insert implements appcatalog.Kits ("KitsInsert").
func (s *Kits) Insert(ctx context.Context, k *catalog.Kit) error {
	scope, err := s.enter(ctx, "KitsInsert")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	if k.Archived() {
		return errArchivedInsert
	}
	r := kitRecord(k)
	if err := s.checkKitWrite(scope, r); err != nil {
		return err
	}
	if _, taken := s.kits[r.ID]; taken {
		return fmt.Errorf("kits_pkey: %w", shared.ErrConflict)
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.kits, r.ID)
	s.kits[r.ID] = r
	return nil
}

// Update implements appcatalog.Kits ("KitsUpdate"). It keeps the stored
// photos.
func (s *Kits) Update(ctx context.Context, k *catalog.Kit) error {
	scope, err := s.enter(ctx, "KitsUpdate")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	stored, ok := s.kits[k.ID()]
	if !ok || stored.Tenant != k.TenantID() || !sees(scope, stored.Tenant) {
		return catalog.ErrKitNotFound
	}
	if stored.Version != k.Version() {
		return shared.ErrVersionConflict
	}
	r := kitRecord(k)
	if err := s.checkKitWrite(scope, r); err != nil {
		return err
	}
	r.Photos, r.Version = stored.Photos, stored.Version+1
	sharedfake.KeepOnRollback(ctx, &s.mu, s.kits, r.ID)
	s.kits[r.ID] = r
	return nil
}

// ByID implements appcatalog.Kits ("KitsByID").
func (s *Kits) ByID(ctx context.Context, tenant shared.TenantID, id shared.KitID) (*catalog.Kit, error) {
	scope, err := s.enter(ctx, "KitsByID")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	r, ok := s.kits[id]
	if !ok || r.Tenant != tenant || !sees(scope, tenant) {
		return nil, catalog.ErrKitNotFound
	}
	return s.restoreKit(r)
}

// List implements appcatalog.Kits ("KitsList").
func (s *Kits) List(ctx context.Context, tenant shared.TenantID, filter appcatalog.KitFilter, page shared.PageRequest) (shared.Page[*catalog.Kit], error) {
	scope, err := s.enter(ctx, "KitsList")
	if err != nil {
		return shared.Page[*catalog.Kit]{}, err
	}
	defer s.mu.Unlock()
	var rows []named
	for id, r := range s.kits {
		if r.Tenant == tenant && sees(scope, tenant) {
			rows = append(rows, named{id: shared.ID(id), name: r.Name})
		}
	}
	keep := func(id shared.ID) bool {
		r := s.kits[shared.KitID(id)]
		return contains(r.Name, filter.Query) && (filter.IncludeArchived || !r.Archived)
	}
	ids, next, err := pageIDs(rows, keep, page)
	if err != nil {
		return shared.Page[*catalog.Kit]{}, err
	}
	out := shared.Page[*catalog.Kit]{Items: make([]*catalog.Kit, 0, len(ids)), NextCursor: next}
	for _, id := range ids {
		k, err := s.restoreKit(s.kits[shared.KitID(id)])
		if err != nil {
			return shared.Page[*catalog.Kit]{}, err
		}
		out.Items = append(out.Items, k)
	}
	return out, nil
}

// Published implements appcatalog.Kits ("KitsPublished").
func (s *Kits) Published(ctx context.Context, tenant shared.TenantID) ([]*catalog.Kit, error) {
	scope, err := s.enter(ctx, "KitsPublished")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	var rows []named
	for id, r := range s.kits {
		if r.Tenant == tenant && sees(scope, tenant) && r.Published && !r.Archived {
			rows = append(rows, named{id: shared.ID(id), name: r.Name})
		}
	}
	slices.SortFunc(rows, compareNamed)
	out := make([]*catalog.Kit, 0, len(rows))
	for _, r := range rows {
		k, err := s.restoreKit(s.kits[shared.KitID(r.id)])
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

// SetKitPhotos replaces the photos stored for a kit, without changing its
// version. Tests use it to give kits photos.
func (s *Store) SetKitPhotos(id shared.KitID, photos []catalog.Photo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.kits[id]
	r.Photos = slices.Clone(photos)
	s.kits[id] = r
}
