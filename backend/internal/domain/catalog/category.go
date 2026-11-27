package catalog

import (
	"fmt"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Category groups items in the store's own order ("Bancos", "Escapamentos").
// The store creates its categories itself; templates only prefill them
// (ADR 0024). An item has at most one. Whether a category is still in use
// (ErrCategoryInUse) and whether its name is taken (ErrNameTaken) are the
// app layer's and the repository's checks.
type Category struct {
	tenant   shared.TenantID
	id       shared.CategoryID
	name     string
	position int
	version  int
}

// NewCategory returns a category at version 1, at position (0 or more) in the
// store's order. The name is trimmed. It returns a ValidationError listing
// every invalid field ("name": 1 to 120 characters; "position": zero or
// more) and ErrInvalidCategory for a zero tenant or id.
func NewCategory(tenant shared.TenantID, id shared.CategoryID, name string, position int) (*Category, error) {
	if tenant.IsZero() || id.IsZero() {
		return nil, fmt.Errorf("new category %v: %w", id, ErrInvalidCategory)
	}
	name = strings.TrimSpace(name)
	if err := shared.MergeValidation(categoryRule.check(name), checkPosition(position)); err != nil {
		return nil, err
	}
	return &Category{tenant: tenant, id: id, name: name, position: position, version: 1}, nil
}

func checkPosition(position int) error {
	if position < 0 {
		return fieldError(fieldPosition, msgPosition)
	}
	return nil
}

// CategoryRecord is a category as a repository stores it.
type CategoryRecord struct {
	Tenant   shared.TenantID
	ID       shared.CategoryID
	Name     string
	Position int
	Version  int
}

// RestoreCategory rebuilds a stored category. It returns ErrInvalidCategory
// when the record breaks a rule a stored category always keeps.
func RestoreCategory(r CategoryRecord) (*Category, error) {
	if r.Version < 1 || r.Name != strings.TrimSpace(r.Name) {
		return nil, fmt.Errorf("restore category %v: %w", r.ID, ErrInvalidCategory)
	}
	c, err := NewCategory(r.Tenant, r.ID, r.Name, r.Position)
	if err != nil {
		// The cause is text only: wrapping a ValidationError would make a bad
		// stored row match shared.ErrInvalid and answer 400, not 500.
		return nil, fmt.Errorf("restore category %v: %w: %s", r.ID, ErrInvalidCategory, err.Error())
	}
	c.version = r.Version
	return c, nil
}

// TenantID returns the store the category belongs to.
func (c *Category) TenantID() shared.TenantID { return c.tenant }

// ID returns the category's id.
func (c *Category) ID() shared.CategoryID { return c.id }

// Name returns the category's name, trimmed.
func (c *Category) Name() string { return c.name }

// Position returns the category's place in the store's order, from 0.
func (c *Category) Position() int { return c.position }

// Version returns the version the category was read or created at.
func (c *Category) Version() int { return c.version }

// CheckVersion returns shared.ErrVersionConflict unless expected is the
// version the category was read at.
func (c *Category) CheckVersion(expected int) error {
	if expected != c.version {
		return shared.ErrVersionConflict
	}
	return nil
}

// Rename gives the category a new name, trimmed. It returns a
// ValidationError on "name" for a blank or too long name; nothing changes
// then.
func (c *Category) Rename(name string) error {
	name = strings.TrimSpace(name)
	if err := categoryRule.check(name); err != nil {
		return err
	}
	c.name = name
	return nil
}

// MoveTo puts the category at position in the store's order. It returns a
// ValidationError on "position" for a negative position; nothing changes
// then. The app layer shifts the other categories.
func (c *Category) MoveTo(position int) error {
	if err := checkPosition(position); err != nil {
		return err
	}
	c.position = position
	return nil
}
