package fake

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func attributeRecord(a *catalog.Attribute) catalog.AttributeRecord {
	values := a.Values()
	in := make([]catalog.ValueInput, 0, len(values))
	for _, v := range values {
		in = append(in, catalog.ValueInput{ID: v.ID(), Label: v.Label()})
	}
	return catalog.AttributeRecord{Tenant: a.TenantID(), ID: a.ID(), Name: a.Name(), Values: in, Version: a.Version()}
}

func placed(r attributeRow) (appcatalog.PlacedAttribute, error) {
	a, err := catalog.RestoreAttribute(r.rec)
	if err != nil {
		return appcatalog.PlacedAttribute{}, err
	}
	return appcatalog.PlacedAttribute{Attribute: a, Position: r.position}, nil
}

func (s *Store) attributeNameTaken(r catalog.AttributeRecord) bool {
	for _, other := range s.attributes {
		if other.rec.Tenant == r.Tenant && other.rec.ID != r.ID && strings.EqualFold(other.rec.Name, r.Name) {
			return true
		}
	}
	return false
}

// valueTaken reports whether a value id of r belongs to another field: the
// primary key of attribute_values sees every store's rows.
func (s *Store) valueTaken(r catalog.AttributeRecord) bool {
	for _, other := range s.attributes {
		if other.rec.ID == r.ID {
			continue
		}
		for _, v := range other.rec.Values {
			if slices.ContainsFunc(r.Values, func(n catalog.ValueInput) bool { return n.ID == v.ID }) {
				return true
			}
		}
	}
	return false
}

// List implements appcatalog.Attributes ("AttributesList").
func (s *Attributes) List(ctx context.Context, tenant shared.TenantID) ([]appcatalog.PlacedAttribute, error) {
	scope, err := s.enter(ctx, "AttributesList")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	var rows []attributeRow
	for _, r := range s.attributes {
		if r.rec.Tenant == tenant && sees(scope, tenant) {
			rows = append(rows, r)
		}
	}
	slices.SortFunc(rows, func(a, b attributeRow) int {
		return cmp.Or(cmp.Compare(a.position, b.position), strings.Compare(a.rec.ID.String(), b.rec.ID.String()))
	})
	out := make([]appcatalog.PlacedAttribute, 0, len(rows))
	for _, r := range rows {
		p, err := placed(r)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// ByID implements appcatalog.Attributes ("AttributesByID").
func (s *Attributes) ByID(ctx context.Context, tenant shared.TenantID, id shared.AttributeID) (appcatalog.PlacedAttribute, error) {
	scope, err := s.enter(ctx, "AttributesByID")
	if err != nil {
		return appcatalog.PlacedAttribute{}, err
	}
	defer s.mu.Unlock()
	r, ok := s.attributes[id]
	if !ok || r.rec.Tenant != tenant || !sees(scope, tenant) {
		return appcatalog.PlacedAttribute{}, catalog.ErrAttributeNotFound
	}
	return placed(r)
}

// Insert implements appcatalog.Attributes ("AttributesInsert").
func (s *Attributes) Insert(ctx context.Context, a *catalog.Attribute, position int) error {
	scope, err := s.enter(ctx, "AttributesInsert")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	r := attributeRecord(a)
	_, exists := s.attributes[r.ID]
	switch {
	case !sees(scope, r.Tenant):
		return ErrRowSecurity
	case exists || s.valueTaken(r):
		return fmt.Errorf("attributes_pkey: %w", shared.ErrConflict)
	case s.attributeNameTaken(r):
		return catalog.ErrNameTaken
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.attributes, r.ID)
	s.attributes[r.ID] = attributeRow{rec: r, position: position}
	return nil
}

// Update implements appcatalog.Attributes ("AttributesUpdate").
func (s *Attributes) Update(ctx context.Context, a *catalog.Attribute, removed []shared.AttributeValueID) error {
	scope, err := s.enter(ctx, "AttributesUpdate")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	stored, ok := s.attributes[a.ID()]
	next := attributeRecord(a)
	switch {
	case !ok || stored.rec.Tenant != next.Tenant || !sees(scope, stored.rec.Tenant):
		return catalog.ErrAttributeNotFound
	case stored.rec.Version != a.Version():
		return shared.ErrVersionConflict
	case s.attributeNameTaken(next):
		return catalog.ErrNameTaken
	case s.valueTaken(next):
		return fmt.Errorf("attribute_values_pkey: %w", shared.ErrConflict)
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.attributes, a.ID())
	next.Version = stored.rec.Version + 1
	s.attributes[a.ID()] = attributeRow{rec: next, position: stored.position}
	s.dropValues(ctx, next.Tenant, removed)
	return nil
}

// Delete implements appcatalog.Attributes ("AttributesDelete").
func (s *Attributes) Delete(ctx context.Context, tenant shared.TenantID, id shared.AttributeID) error {
	scope, err := s.enter(ctx, "AttributesDelete")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	r, ok := s.attributes[id]
	if !ok || r.rec.Tenant != tenant || !sees(scope, tenant) {
		return catalog.ErrAttributeNotFound
	}
	values := make([]shared.AttributeValueID, 0, len(r.rec.Values))
	for _, v := range r.rec.Values {
		values = append(values, v.ID)
	}
	sharedfake.KeepOnRollback(ctx, &s.mu, s.attributes, id)
	delete(s.attributes, id)
	s.dropValues(ctx, tenant, values)
	return nil
}

// dropValues removes values from every item and kit of tenant that picked
// them, as the ON DELETE CASCADE of item_attribute_values and
// kit_attribute_values does: their versions stay. Call it with s.mu held.
func (s *Store) dropValues(ctx context.Context, tenant shared.TenantID, values []shared.AttributeValueID) {
	gone := func(id shared.AttributeValueID) bool { return slices.Contains(values, id) }
	for id, row := range s.items {
		if row.rec.Tenant != tenant || !slices.ContainsFunc(row.rec.Attributes, gone) {
			continue
		}
		sharedfake.KeepOnRollback(ctx, &s.mu, s.items, id)
		row.rec.Attributes = slices.DeleteFunc(slices.Clone(row.rec.Attributes), gone)
		s.items[id] = row
	}
	for id, rec := range s.kits {
		if rec.Tenant != tenant || !slices.ContainsFunc(rec.Attributes, gone) {
			continue
		}
		sharedfake.KeepOnRollback(ctx, &s.mu, s.kits, id)
		rec.Attributes = slices.DeleteFunc(slices.Clone(rec.Attributes), gone)
		s.kits[id] = rec
	}
}

// UsageCounts implements appcatalog.Attributes ("AttributesUsageCounts").
func (s *Attributes) UsageCounts(ctx context.Context, tenant shared.TenantID) (map[shared.AttributeValueID]int, error) {
	scope, err := s.enter(ctx, "AttributesUsageCounts")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	out := map[shared.AttributeValueID]int{}
	if !sees(scope, tenant) {
		return out, nil
	}
	for _, row := range s.items {
		if row.rec.Tenant == tenant {
			for _, v := range row.rec.Attributes {
				out[v]++
			}
		}
	}
	for _, rec := range s.kits {
		if rec.Tenant == tenant {
			for _, v := range rec.Attributes {
				out[v]++
			}
		}
	}
	return out, nil
}

// inFieldOrder returns ids in the store's field order, as the Postgres
// repositories read them: the field's position, then the value's position.
// The input is not changed. Call it with s.mu held.
func (s *Store) inFieldOrder(tenant shared.TenantID, ids []shared.AttributeValueID) []shared.AttributeValueID {
	type place struct{ field, value int }
	places := map[shared.AttributeValueID]place{}
	for _, r := range s.attributes {
		if r.rec.Tenant != tenant {
			continue
		}
		for i, v := range r.rec.Values {
			places[v.ID] = place{field: r.position, value: i}
		}
	}
	out := slices.Clone(ids)
	slices.SortStableFunc(out, func(a, b shared.AttributeValueID) int {
		pa, pb := places[a], places[b]
		return cmp.Or(cmp.Compare(pa.field, pb.field), cmp.Compare(pa.value, pb.value), strings.Compare(a.String(), b.String()))
	})
	return out
}

// knownValues reports whether every id is a value of tenant's fields. Call
// it with s.mu held.
func (s *Store) knownValues(tenant shared.TenantID, ids []shared.AttributeValueID) bool {
	known := map[shared.AttributeValueID]bool{}
	for _, r := range s.attributes {
		if r.rec.Tenant != tenant {
			continue
		}
		for _, v := range r.rec.Values {
			known[v.ID] = true
		}
	}
	for _, id := range ids {
		if !known[id] {
			return false
		}
	}
	return true
}
