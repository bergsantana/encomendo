package catalog

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestNewCategory(t *testing.T) {
	t.Parallel()

	c, err := NewCategory(tenantID, categoryID(1), "  Bancos  ", 2)
	if err != nil {
		t.Fatal(err)
	}
	if c.TenantID() != tenantID || c.ID() != categoryID(1) || c.Name() != "Bancos" || c.Position() != 2 || c.Version() != 1 {
		t.Errorf("category = %v %v %q %d v%d", c.TenantID(), c.ID(), c.Name(), c.Position(), c.Version())
	}
}

func TestNewCategory_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cat      string
		position int
		want     map[string]string
	}{
		{"blank", "  ", 0, map[string]string{"name": msgCategoryNameRequired}},
		{"over 120", repeat("a", 121), 0, map[string]string{"name": msgNameTooLong}},
		{"control character", "a\nb", 0, map[string]string{"name": msgNameControl}},
		{"negative position", "Bancos", -1, map[string]string{"position": msgPosition}},
		{"both", "", -1, map[string]string{"name": msgCategoryNameRequired, "position": msgPosition}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewCategory(tenantID, categoryID(1), tt.cat, tt.position)
			wantFields(t, err, tt.want)
		})
	}
	for name, args := range map[string][2]any{"zero tenant": {shared.TenantID{}, categoryID(1)}, "zero id": {tenantID, shared.CategoryID{}}} {
		if _, err := NewCategory(args[0].(shared.TenantID), args[1].(shared.CategoryID), "x", 0); !errors.Is(err, ErrInvalidCategory) {
			t.Errorf("%s: error = %v, want ErrInvalidCategory", name, err)
		}
	}
}

func TestCategory_RenameMoveAndVersion(t *testing.T) {
	t.Parallel()

	c, err := NewCategory(tenantID, categoryID(1), "Bancos", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Rename(" Escapamentos "); err != nil || c.Name() != "Escapamentos" {
		t.Errorf("Rename = %v, name %q", err, c.Name())
	}
	wantFields(t, c.Rename(""), map[string]string{"name": msgCategoryNameRequired})
	if c.Name() != "Escapamentos" {
		t.Error("a refused Rename changed the name")
	}
	if err := c.MoveTo(4); err != nil || c.Position() != 4 {
		t.Errorf("MoveTo = %v, position %d", err, c.Position())
	}
	wantFields(t, c.MoveTo(-3), map[string]string{"position": msgPosition})
	if c.Position() != 4 {
		t.Error("a refused MoveTo changed the position")
	}
	if err := c.CheckVersion(1); err != nil {
		t.Errorf("CheckVersion(1) = %v", err)
	}
	if err := c.CheckVersion(2); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("CheckVersion(2) = %v, want ErrVersionConflict", err)
	}
}

func TestRestoreCategory(t *testing.T) {
	t.Parallel()

	c, err := RestoreCategory(CategoryRecord{Tenant: tenantID, ID: categoryID(1), Name: "Bancos", Position: 3, Version: 5})
	if err != nil || c.Version() != 5 || c.Position() != 3 {
		t.Fatalf("RestoreCategory = %+v, %v", c, err)
	}
	for name, r := range map[string]CategoryRecord{
		"version 0":      {Tenant: tenantID, ID: categoryID(1), Name: "x"},
		"untrimmed name": {Tenant: tenantID, ID: categoryID(1), Name: " x", Version: 1},
		"blank name":     {Tenant: tenantID, ID: categoryID(1), Name: "", Version: 1},
	} {
		_, err := RestoreCategory(r)
		if !errors.Is(err, ErrInvalidCategory) || errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalidCategory and not a 400", name, err)
		}
	}
}
