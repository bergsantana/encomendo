package catalog_test

import (
	"errors"
	"slices"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func (h *harness) categories(t *testing.T, names ...string) []appcatalog.CategoryView {
	t.Helper()
	out := make([]appcatalog.CategoryView, 0, len(names))
	for _, n := range names {
		c, err := h.svc.CreateCategory(h.admin(t), appcatalog.CreateCategoryInput{Name: n})
		if err != nil {
			t.Fatalf("CreateCategory(%s): %v", n, err)
		}
		out = append(out, c)
	}
	return out
}

func (h *harness) categoryOrder(t *testing.T) []string {
	t.Helper()
	list, err := h.svc.ListCategories(h.viewer(t))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for i, c := range list {
		if c.Position != i {
			t.Errorf("%s at position %d, want %d", c.Name, c.Position, i)
		}
		out = append(out, c.Name)
	}
	return out
}

func TestCreateCategory_AddsAtTheEnd(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	got := h.categories(t, " Bancos ", "Rodas")

	if got[0].Name != "Bancos" || got[0].Position != 0 || got[1].Position != 1 {
		t.Errorf("created = %+v; want trimmed names at positions 0 and 1", got)
	}
	if order := h.categoryOrder(t); !slices.Equal(order, []string{"Bancos", "Rodas"}) {
		t.Errorf("order = %v", order)
	}
	_, err := h.svc.CreateCategory(h.admin(t), appcatalog.CreateCategoryInput{Name: "bancos"})
	if !errors.Is(err, catalog.ErrNameTaken) {
		t.Errorf("a duplicate name: %v, want ErrNameTaken", err)
	}
	_, err = h.svc.CreateCategory(h.admin(t), appcatalog.CreateCategoryInput{Name: " "})
	if f := fieldsOf(t, err); !slices.Equal(f, []string{"name"}) {
		t.Errorf("a blank name: fields %v", f)
	}
}

func TestUpdateCategory_RenamesAndMoves(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	c := h.categories(t, "Bancos", "Rodas", "Carenagens", "Escapes")

	moved, err := h.svc.UpdateCategory(h.admin(t), appcatalog.UpdateCategoryInput{ID: c[3].ID, Name: ptr("Escapamentos"), Position: ptr(1)})
	if err != nil || moved.Name != "Escapamentos" || moved.Position != 1 {
		t.Fatalf("moved = %+v, %v; want Escapamentos at 1", moved, err)
	}
	if order := h.categoryOrder(t); !slices.Equal(order, []string{"Bancos", "Escapamentos", "Rodas", "Carenagens"}) {
		t.Errorf("after the move: %v", order)
	}
	last, err := h.svc.UpdateCategory(h.admin(t), appcatalog.UpdateCategoryInput{ID: c[0].ID, Position: ptr(99)})
	if err != nil || last.Position != 3 {
		t.Errorf("a position past the end = %+v, %v; want the end, 3", last, err)
	}
	renamed, err := h.svc.UpdateCategory(h.admin(t), appcatalog.UpdateCategoryInput{ID: c[1].ID, Name: ptr("Aros")})
	if err != nil || renamed.Name != "Aros" || renamed.Position != 1 {
		t.Errorf("a rename only = %+v, %v; want it in place", renamed, err)
	}
	if order := h.categoryOrder(t); !slices.Equal(order, []string{"Escapamentos", "Aros", "Carenagens", "Bancos"}) {
		t.Errorf("final order = %v", order)
	}
}

func TestUpdateCategory_Refusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	c := h.categories(t, "Bancos", "Rodas")
	ctx := h.admin(t)

	if _, err := h.svc.UpdateCategory(ctx, appcatalog.UpdateCategoryInput{ID: c[1].ID, Name: ptr("BANCOS")}); !errors.Is(err, catalog.ErrNameTaken) {
		t.Errorf("onto another name: %v, want ErrNameTaken", err)
	}
	_, err := h.svc.UpdateCategory(ctx, appcatalog.UpdateCategoryInput{ID: c[1].ID, Position: ptr(-1)})
	if f := fieldsOf(t, err); !slices.Equal(f, []string{"position"}) {
		t.Errorf("a negative position: fields %v", f)
	}
	_, err = h.svc.UpdateCategory(ctx, appcatalog.UpdateCategoryInput{ID: c[1].ID, Name: ptr("")})
	if f := fieldsOf(t, err); !slices.Equal(f, []string{"name"}) {
		t.Errorf("a blank name: fields %v", f)
	}
	if _, err := h.svc.UpdateCategory(ctx, appcatalog.UpdateCategoryInput{ID: shared.CategoryID(h.ids.NewID()), Name: ptr("X")}); !errors.Is(err, catalog.ErrCategoryNotFound) {
		t.Errorf("an unknown category: %v, want ErrCategoryNotFound", err)
	}
	other := h.as(t, h.store2, shared.RoleOwner)
	if _, err := h.svc.UpdateCategory(other, appcatalog.UpdateCategoryInput{ID: c[0].ID, Name: ptr("X")}); !errors.Is(err, catalog.ErrCategoryNotFound) {
		t.Errorf("another store's category: %v, want ErrCategoryNotFound", err)
	}
	h.store.Fail("CategoriesUpdate", errBoom)
	if _, err := h.svc.UpdateCategory(ctx, appcatalog.UpdateCategoryInput{ID: c[0].ID, Position: ptr(1)}); !errors.Is(err, errBoom) {
		t.Errorf("a failed save: %v, want it passed through", err)
	}
	h.store.Fail("CategoriesUpdate", nil)
	if order := h.categoryOrder(t); !slices.Equal(order, []string{"Bancos", "Rodas"}) {
		t.Errorf("after the refusals: %v, want nothing changed", order)
	}
}

func TestDeleteCategory(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	c := h.categories(t, "Bancos", "Rodas")
	in := inHouseInput("Banco", 10000, 5)
	in.CategoryID = c[0].ID
	it := h.createItem(t, in)
	if _, err := h.svc.ArchiveItem(h.admin(t), appcatalog.ItemVersionInput{ID: it.ID, Version: 1}); err != nil {
		t.Fatal(err)
	}

	if err := h.svc.DeleteCategory(h.admin(t), c[0].ID); !errors.Is(err, catalog.ErrCategoryInUse) {
		t.Errorf("a category an archived item has: %v, want ErrCategoryInUse", err)
	}
	if err := h.svc.DeleteCategory(h.admin(t), c[1].ID); err != nil {
		t.Errorf("a free category: %v", err)
	}
	if err := h.svc.DeleteCategory(h.admin(t), c[1].ID); !errors.Is(err, catalog.ErrCategoryNotFound) {
		t.Errorf("deleting it again: %v, want ErrCategoryNotFound", err)
	}
	if order := h.categoryOrder(t); !slices.Equal(order, []string{"Bancos"}) {
		t.Errorf("left = %v", order)
	}
}

func TestCategories_AFailedReadPassesThrough(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.store.Fail("CategoriesList", errBoom)
	if _, err := h.svc.ListCategories(h.viewer(t)); !errors.Is(err, errBoom) {
		t.Errorf("ListCategories: %v", err)
	}
	if _, err := h.svc.CreateCategory(h.admin(t), appcatalog.CreateCategoryInput{Name: "X"}); !errors.Is(err, errBoom) {
		t.Errorf("CreateCategory: %v", err)
	}
	if _, err := h.svc.UpdateCategory(h.admin(t), appcatalog.UpdateCategoryInput{Name: ptr("X")}); !errors.Is(err, errBoom) {
		t.Errorf("UpdateCategory: %v", err)
	}
	h.store.Fail("CategoriesList", nil)
	h.store.Fail("CategoriesInsert", errBoom)
	if _, err := h.svc.CreateCategory(h.admin(t), appcatalog.CreateCategoryInput{Name: "X"}); !errors.Is(err, errBoom) {
		t.Errorf("CreateCategory insert: %v", err)
	}
}
