package catalog

import (
	"context"
	"fmt"
	"slices"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ListCategories returns every category of the store, in the store's order
// (catalog:read).
func (s *Service) ListCategories(ctx context.Context) ([]CategoryView, error) {
	by, err := reader(ctx)
	if err != nil {
		return nil, err
	}
	var list []*catalog.Category
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		read, err := s.categories.List(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("list categories: %w", err)
		}
		list = read
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]CategoryView, 0, len(list))
	for _, c := range list {
		out = append(out, categoryView(c))
	}
	return out, nil
}

// CreateCategory adds a category at the end of the store's list
// (catalog:write). It returns a ValidationError on "name" and
// catalog.ErrNameTaken when another category has the name, ignoring case.
func (s *Service) CreateCategory(ctx context.Context, in CreateCategoryInput) (CategoryView, error) {
	by, err := writer(ctx, false)
	if err != nil {
		return CategoryView{}, err
	}
	var c *catalog.Category
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		list, err := s.categories.List(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("list categories: %w", err)
		}
		position := 0
		for _, other := range list {
			position = max(position, other.Position()+1)
		}
		created, err := catalog.NewCategory(by.TenantID(), shared.CategoryID(s.ids.NewID()), in.Name, position)
		if err != nil {
			return err
		}
		if err := s.categories.Insert(ctx, created); err != nil {
			return fmt.Errorf("insert category: %w", err)
		}
		c = created
		return nil
	})
	if err != nil {
		return CategoryView{}, err
	}
	s.invalidate(ctx, by.TenantID())
	return categoryView(c), nil
}

// UpdateCategory renames a category, moves it, or both (catalog:write). A
// move renumbers the store's categories from 0, so a position past the end
// puts the category last. It returns catalog.ErrCategoryNotFound, a
// ValidationError on "name" or "position", catalog.ErrNameTaken, and
// shared.ErrVersionConflict when another write changed a category meanwhile.
func (s *Service) UpdateCategory(ctx context.Context, in UpdateCategoryInput) (CategoryView, error) {
	by, err := writer(ctx, false)
	if err != nil {
		return CategoryView{}, err
	}
	var out *catalog.Category
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		list, err := s.categories.List(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("list categories: %w", err)
		}
		i := slices.IndexFunc(list, func(c *catalog.Category) bool { return c.ID() == in.ID })
		if i < 0 {
			return fmt.Errorf("update category %s: %w", in.ID, catalog.ErrCategoryNotFound)
		}
		out = list[i]
		return s.saveCategories(ctx, list, i, in)
	})
	if err != nil {
		return CategoryView{}, err
	}
	s.invalidate(ctx, by.TenantID())
	return categoryView(out), nil
}

// saveCategories applies in to list[i], renumbers the list when it moves,
// and stores every category that changed.
func (s *Service) saveCategories(ctx context.Context, list []*catalog.Category, i int, in UpdateCategoryInput) error {
	target := list[i]
	changed := map[shared.CategoryID]bool{}
	if in.Name != nil {
		if err := target.Rename(*in.Name); err != nil {
			return err
		}
		changed[target.ID()] = true
	}
	if in.Position != nil {
		if *in.Position < 0 {
			return target.MoveTo(*in.Position) // the domain's ValidationError
		}
		list = slices.Insert(slices.Delete(slices.Clone(list), i, i+1), min(*in.Position, len(list)-1), target)
		for pos, c := range list {
			if c.Position() == pos {
				continue
			}
			if err := c.MoveTo(pos); err != nil {
				return err
			}
			changed[c.ID()] = true
		}
	}
	for _, c := range list {
		if !changed[c.ID()] {
			continue
		}
		if err := s.categories.Update(ctx, c); err != nil { // optimistic lock on version
			return fmt.Errorf("save category %s: %w", c.ID(), err)
		}
	}
	return nil
}

// DeleteCategory removes a category no item uses (catalog:write). It
// returns catalog.ErrCategoryNotFound, and catalog.ErrCategoryInUse while an
// item, archived ones included, has it.
func (s *Service) DeleteCategory(ctx context.Context, id shared.CategoryID) error {
	by, err := writer(ctx, false)
	if err != nil {
		return err
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.categories.Delete(ctx, by.TenantID(), id); err != nil {
			return fmt.Errorf("delete category %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.invalidate(ctx, by.TenantID())
	return nil
}
