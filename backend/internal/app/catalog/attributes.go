package catalog

import (
	"context"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ListAttributes returns every item field of the store, in order, each value
// with how many items and kits picked it (catalog:read). The apps use the
// count to ask before a value is removed.
func (s *Service) ListAttributes(ctx context.Context) ([]AttributeView, error) {
	by, err := reader(ctx)
	if err != nil {
		return nil, err
	}
	var list []PlacedAttribute
	var usage map[shared.AttributeValueID]int
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		read, err := s.attributes.List(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("list attributes: %w", err)
		}
		counts, err := s.attributes.UsageCounts(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("count attribute usage: %w", err)
		}
		list, usage = read, counts
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]AttributeView, 0, len(list))
	for _, p := range list {
		out = append(out, attributeView(p, usage))
	}
	return out, nil
}

// CreateAttribute adds an item field with its values at the end of the
// store's list (catalog:write). It returns a ValidationError ("name",
// "values[i].label", "values") and catalog.ErrNameTaken.
func (s *Service) CreateAttribute(ctx context.Context, in AttributeInput) (AttributeView, error) {
	by, err := writer(ctx, false)
	if err != nil {
		return AttributeView{}, err
	}
	var placed PlacedAttribute
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		list, err := s.attributes.List(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("list attributes: %w", err)
		}
		position := 0
		for _, p := range list {
			position = max(position, p.Position+1)
		}
		a, err := catalog.NewAttribute(by.TenantID(), shared.AttributeID(s.ids.NewID()), s.attributeInput(in, nil))
		if err != nil {
			return err
		}
		if err := s.attributes.Insert(ctx, a, position); err != nil {
			return fmt.Errorf("insert attribute: %w", err)
		}
		placed = PlacedAttribute{Attribute: a, Position: position}
		return nil
	})
	if err != nil {
		return AttributeView{}, err
	}
	s.invalidate(ctx, by.TenantID())
	return attributeView(placed, nil), nil
}

// ReplaceAttribute replaces a field's name and ordered values
// (catalog:write). A value sent with one of the field's ids keeps it and its
// items and kits; a value left out leaves the field and every item and kit
// that picked it. It returns catalog.ErrAttributeNotFound, a
// ValidationError, catalog.ErrNameTaken and shared.ErrVersionConflict.
func (s *Service) ReplaceAttribute(ctx context.Context, in ReplaceAttributeInput) (AttributeView, error) {
	by, err := writer(ctx, false)
	if err != nil {
		return AttributeView{}, err
	}
	var placed PlacedAttribute
	var usage map[shared.AttributeValueID]int
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		loaded, err := s.attributes.ByID(ctx, by.TenantID(), in.ID)
		if err != nil {
			return fmt.Errorf("load attribute %s: %w", in.ID, err)
		}
		removed, err := loaded.Attribute.Replace(s.attributeInput(in.Attribute, loaded.Attribute.Values()))
		if err != nil {
			return err
		}
		if err := s.attributes.Update(ctx, loaded.Attribute, removed); err != nil { // optimistic lock on version
			return fmt.Errorf("save attribute %s: %w", in.ID, err)
		}
		counts, err := s.attributes.UsageCounts(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("count attribute usage: %w", err)
		}
		placed, usage = loaded, counts
		return nil
	})
	if err != nil {
		return AttributeView{}, err
	}
	s.invalidate(ctx, by.TenantID())
	return attributeView(placed, usage), nil
}

// DeleteAttribute removes a field; its values leave every item and kit
// (catalog:write). It returns catalog.ErrAttributeNotFound.
func (s *Service) DeleteAttribute(ctx context.Context, id shared.AttributeID) error {
	by, err := writer(ctx, false)
	if err != nil {
		return err
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.attributes.Delete(ctx, by.TenantID(), id); err != nil {
			return fmt.Errorf("delete attribute %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.invalidate(ctx, by.TenantID())
	return nil
}

// attributeInput turns the client's values into the domain's. A value keeps
// its id only when it is one of current, and only the first time it is
// sent; every other value gets a new id. So a client can never take over a
// value of another field, and items keep pointing at the values they chose.
func (s *Service) attributeInput(in AttributeInput, current []catalog.AttributeValue) catalog.AttributeInput {
	known := make(map[shared.AttributeValueID]bool, len(current))
	for _, v := range current {
		known[v.ID()] = true
	}
	out := catalog.AttributeInput{Name: in.Name, Values: make([]catalog.ValueInput, 0, len(in.Values))}
	for _, v := range in.Values {
		id := v.ID
		if !known[id] {
			id = shared.AttributeValueID(s.ids.NewID())
		}
		known[v.ID] = false // a second use of the same id is a new value
		out.Values = append(out.Values, catalog.ValueInput{ID: id, Label: v.Label})
	}
	return out
}
