package catalogports

import (
	"context"
	"errors"
	"slices"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// RunCategoriesContract checks the Categories repository: order, unique
// names per store ignoring case, optimistic updates, the in-use refusal,
// rollback and tenant isolation.
func RunCategoriesContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("categories list by position, then name", func(t *testing.T) { categoriesOrder(t, newHarness(t)) })
	t.Run("a category name is unique per store, ignoring case", func(t *testing.T) { categoryNames(t, newHarness(t)) })
	t.Run("category updates are optimistic", func(t *testing.T) { categoryUpdate(t, newHarness(t)) })
	t.Run("a category in use cannot be deleted", func(t *testing.T) { categoryDelete(t, newHarness(t)) })
	t.Run("a failed transaction undoes a category write", func(t *testing.T) { categoryRollback(t, newHarness(t)) })
	t.Run("another store's categories are invisible", func(t *testing.T) { categoryIsolation(t, newHarness(t)) })
	t.Run("categories need a transaction", func(t *testing.T) { categoryNeedsTx(t, newHarness(t)) })
}

func categoryNames(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	h.addCategory(t, a, "Bancos", 0)
	h.addCategory(t, b, "Bancos", 0) // another store may use the name
	dup, err := catalog.NewCategory(a.id, shared.CategoryID(h.NewID()), "BANCOS", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.try(a.scope(), func(ctx context.Context) error { return h.Categories.Insert(ctx, dup) }); !errors.Is(err, catalog.ErrNameTaken) {
		t.Errorf("the same name in other case: %v, want ErrNameTaken", err)
	}
}

func categoriesOrder(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	h.addCategory(t, tn, "Carenagens", 1)
	h.addCategory(t, tn, "Bancos", 1)
	h.addCategory(t, tn, "Rodas", 0)
	h.in(t, tn.scope(), func(ctx context.Context) error {
		list, err := h.Categories.List(ctx, tn.id)
		var got []string
		for _, c := range list {
			got = append(got, c.Name())
		}
		if err != nil || !slices.Equal(got, []string{"Rodas", "Bancos", "Carenagens"}) {
			t.Errorf("List = %v, %v; want by position, then name", got, err)
		}
		return nil
	})
}

func categoryUpdate(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	c := h.addCategory(t, tn, "Bancos", 0)
	h.addCategory(t, tn, "Rodas", 1)
	h.in(t, tn.scope(), func(ctx context.Context) error {
		cur, _ := h.Categories.ByID(ctx, tn.id, c.ID())
		stale, _ := h.Categories.ByID(ctx, tn.id, c.ID())
		if err := cur.Rename("Assentos"); err != nil {
			return err
		}
		if err := cur.MoveTo(4); err != nil {
			return err
		}
		if err := h.Categories.Update(ctx, cur); err != nil {
			t.Errorf("Update: %v", err)
		}
		got, err := h.Categories.ByID(ctx, tn.id, c.ID())
		if err != nil || got.Name() != "Assentos" || got.Position() != 4 || got.Version() != 2 {
			t.Errorf("after the update: %+v, %v; want Assentos at 4, version 2", got, err)
		}
		if err := h.Categories.Update(ctx, stale); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("a stale update: %v, want ErrVersionConflict", err)
		}
		ghost, _ := catalog.NewCategory(tn.id, shared.CategoryID(h.NewID()), "Fantasma", 0)
		if err := h.Categories.Update(ctx, ghost); !errors.Is(err, catalog.ErrCategoryNotFound) {
			t.Errorf("updating an unknown category: %v, want ErrCategoryNotFound", err)
		}
		return nil
	})
	// A unique violation aborts a Postgres transaction, so it gets its own.
	err := h.try(tn.scope(), func(ctx context.Context) error {
		cur, err := h.Categories.ByID(ctx, tn.id, c.ID())
		if err != nil {
			return err
		}
		if err := cur.Rename("rodas"); err != nil {
			return err
		}
		return h.Categories.Update(ctx, cur)
	})
	if !errors.Is(err, catalog.ErrNameTaken) {
		t.Errorf("renaming onto another name: %v, want ErrNameTaken", err)
	}
}

func categoryDelete(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	used := h.addCategory(t, tn, "Bancos", 0)
	free := h.addCategory(t, tn, "Rodas", 1)
	h.addItem(t, tn, itemSpec{name: "Banco", category: used.ID(), archived: true})
	err := h.try(tn.scope(), func(ctx context.Context) error { return h.Categories.Delete(ctx, tn.id, used.ID()) })
	if !errors.Is(err, catalog.ErrCategoryInUse) {
		t.Errorf("deleting a category an archived item has: %v, want ErrCategoryInUse", err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		if err := h.Categories.Delete(ctx, tn.id, free.ID()); err != nil {
			t.Errorf("deleting a free category: %v", err)
		}
		if _, err := h.Categories.ByID(ctx, tn.id, free.ID()); !errors.Is(err, catalog.ErrCategoryNotFound) {
			t.Errorf("after the delete: %v, want ErrCategoryNotFound", err)
		}
		if err := h.Categories.Delete(ctx, tn.id, free.ID()); !errors.Is(err, catalog.ErrCategoryNotFound) {
			t.Errorf("deleting it again: %v, want ErrCategoryNotFound", err)
		}
		return nil
	})
}

func categoryRollback(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	kept := h.addCategory(t, tn, "Rodas", 0)
	c, _ := catalog.NewCategory(tn.id, shared.CategoryID(h.NewID()), "Bancos", 1)
	err := h.try(tn.scope(), func(ctx context.Context) error {
		if err := h.Categories.Insert(ctx, c); err != nil {
			return err
		}
		if err := h.Categories.Delete(ctx, tn.id, kept.ID()); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("the transaction = %v, want the error it returned", err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		list, err := h.Categories.List(ctx, tn.id)
		if err != nil || len(list) != 1 || list[0].ID() != kept.ID() {
			t.Errorf("after the rollback: %v, %v; want only the kept category", list, err)
		}
		return nil
	})
}

func categoryIsolation(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	theirs := h.addCategory(t, b, "Bancos", 0)
	h.in(t, a.scope(), func(ctx context.Context) error {
		if _, err := h.Categories.ByID(ctx, b.id, theirs.ID()); !errors.Is(err, catalog.ErrCategoryNotFound) {
			t.Errorf("reading B's category as A: %v, want ErrCategoryNotFound", err)
		}
		if list, err := h.Categories.List(ctx, b.id); err != nil || len(list) != 0 {
			t.Errorf("B's list in A's scope = %v, %v; want none", list, err)
		}
		if err := h.Categories.Delete(ctx, b.id, theirs.ID()); !errors.Is(err, catalog.ErrCategoryNotFound) {
			t.Errorf("A deleting B's category: %v, want ErrCategoryNotFound", err)
		}
		if err := h.Categories.Update(ctx, theirs); !errors.Is(err, catalog.ErrCategoryNotFound) {
			t.Errorf("A updating B's category: %v, want ErrCategoryNotFound", err)
		}
		return nil
	})
	intruder, _ := catalog.NewCategory(b.id, shared.CategoryID(h.NewID()), "Intrusa", 1)
	if err := h.try(a.scope(), func(ctx context.Context) error { return h.Categories.Insert(ctx, intruder) }); err == nil {
		t.Error("store A inserted a category of store B")
	}
	// Categories are tenant data, not staff data: the store page lists them.
	h.in(t, b.storePage(), func(ctx context.Context) error {
		if list, err := h.Categories.List(ctx, b.id); err != nil || len(list) != 1 {
			t.Errorf("the store page's list = %v, %v; want the category", list, err)
		}
		return nil
	})
}

func categoryNeedsTx(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	if _, err := h.Categories.List(context.Background(), tn.id); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("outside a transaction: %v, want ErrNoTransaction", err)
	}
}

// RunAttributesContract checks the Attributes repository: order, names,
// optimistic updates, removed values leaving items and kits, usage counts,
// rollback and tenant isolation.
func RunAttributesContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("attributes read back in order with their values", func(t *testing.T) { attributesOrder(t, newHarness(t)) })
	t.Run("an attribute name is unique per store", func(t *testing.T) { attributeNames(t, newHarness(t)) })
	t.Run("a removed value leaves items and kits", func(t *testing.T) { attributeRemovesValues(t, newHarness(t)) })
	t.Run("a deleted attribute leaves items and kits", func(t *testing.T) { attributeDelete(t, newHarness(t)) })
	t.Run("a failed transaction undoes an attribute write", func(t *testing.T) { attributeRollback(t, newHarness(t)) })
	t.Run("another store's attributes are invisible", func(t *testing.T) { attributeIsolation(t, newHarness(t)) })
}

func attributesOrder(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	second := h.addAttribute(t, tn, "Material", 1, "Couro", "Tecido")
	first := h.addAttribute(t, tn, "Serve em", 0, "CB 500X", "XRE 300")
	h.in(t, tn.scope(), func(ctx context.Context) error {
		list, err := h.Attributes.List(ctx, tn.id)
		if err != nil || len(list) != 2 || list[0].Attribute.ID() != first.ID() || list[1].Position != 1 {
			t.Fatalf("List = %+v, %v; want Serve em, then Material", list, err)
		}
		got, err := h.Attributes.ByID(ctx, tn.id, second.ID())
		if err != nil || !slices.Equal(valueIDs(got.Attribute), valueIDs(second)) || got.Attribute.Values()[1].Label() != "Tecido" {
			t.Errorf("ByID = %+v, %v; want the values in order", got, err)
		}
		if _, err := h.Attributes.ByID(ctx, tn.id, shared.AttributeID(h.NewID())); !errors.Is(err, catalog.ErrAttributeNotFound) {
			t.Errorf("an unknown attribute: %v, want ErrAttributeNotFound", err)
		}
		return nil
	})
}

func attributeNames(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	h.addAttribute(t, tn, "Serve em", 0, "CB 500X")
	other := h.addAttribute(t, tn, "Material", 1, "Couro")
	dup, _ := catalog.NewAttribute(tn.id, shared.AttributeID(h.NewID()), catalog.AttributeInput{Name: "serve EM"})
	if err := h.try(tn.scope(), func(ctx context.Context) error { return h.Attributes.Insert(ctx, dup, 2) }); !errors.Is(err, catalog.ErrNameTaken) {
		t.Errorf("a duplicate name: %v, want ErrNameTaken", err)
	}
	err := h.try(tn.scope(), func(ctx context.Context) error {
		cur, err := h.Attributes.ByID(ctx, tn.id, other.ID())
		if err != nil {
			return err
		}
		if _, err := cur.Attribute.Replace(catalog.AttributeInput{Name: "Serve em"}); err != nil {
			return err
		}
		return h.Attributes.Update(ctx, cur.Attribute, nil)
	})
	if !errors.Is(err, catalog.ErrNameTaken) {
		t.Errorf("renaming onto another name: %v, want ErrNameTaken", err)
	}
}

func attributeRemovesValues(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	a := h.addAttribute(t, tn, "Serve em", 0, "CB 500X", "XRE 300")
	ids := valueIDs(a)
	it := h.addItem(t, tn, itemSpec{name: "Banco", values: ids})
	k := h.addKit(t, tn, kitSpec{name: "Kit", items: []catalog.KitItem{member(it.ID())}, values: ids})
	h.in(t, tn.scope(), func(ctx context.Context) error {
		usage, err := h.Attributes.UsageCounts(ctx, tn.id)
		if err != nil || usage[ids[0]] != 2 || usage[ids[1]] != 2 {
			t.Errorf("usage = %v, %v; want 2 for each value", usage, err)
		}
		cur, _ := h.Attributes.ByID(ctx, tn.id, a.ID())
		stale, _ := h.Attributes.ByID(ctx, tn.id, a.ID())
		removed, err := cur.Attribute.Replace(catalog.AttributeInput{Name: "Serve em", Values: []catalog.ValueInput{{ID: ids[0], Label: "CB 500X"}}})
		if err != nil {
			return err
		}
		if err := h.Attributes.Update(ctx, cur.Attribute, removed); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if err := h.Attributes.Update(ctx, stale.Attribute, nil); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("a stale update: %v, want ErrVersionConflict", err)
		}
		gotItem, _ := h.Items.ByID(ctx, tn.id, it.ID())
		gotKit, _ := h.Kits.ByID(ctx, tn.id, k.ID())
		if !slices.Equal(gotItem.AttributeValueIDs(), ids[:1]) || gotItem.Version() != 1 {
			t.Errorf("the item after the removal: %v at version %d; want only the kept value at version 1", gotItem.AttributeValueIDs(), gotItem.Version())
		}
		if !slices.Equal(gotKit.AttributeValueIDs(), ids[:1]) || gotKit.Version() != 1 {
			t.Errorf("the kit after the removal: %v at version %d; want only the kept value", gotKit.AttributeValueIDs(), gotKit.Version())
		}
		usage, _ = h.Attributes.UsageCounts(ctx, tn.id)
		if _, left := usage[ids[1]]; left || usage[ids[0]] != 2 {
			t.Errorf("usage after the removal = %v", usage)
		}
		return nil
	})
}

func attributeDelete(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	a := h.addAttribute(t, tn, "Serve em", 0, "CB 500X")
	it := h.addItem(t, tn, itemSpec{name: "Banco", values: valueIDs(a)})
	h.in(t, tn.scope(), func(ctx context.Context) error {
		if err := h.Attributes.Delete(ctx, tn.id, a.ID()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		got, _ := h.Items.ByID(ctx, tn.id, it.ID())
		if len(got.AttributeValueIDs()) != 0 {
			t.Errorf("the item still has %v", got.AttributeValueIDs())
		}
		if err := h.Attributes.Delete(ctx, tn.id, a.ID()); !errors.Is(err, catalog.ErrAttributeNotFound) {
			t.Errorf("deleting it again: %v, want ErrAttributeNotFound", err)
		}
		return nil
	})
}

func attributeRollback(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	a := h.addAttribute(t, tn, "Serve em", 0, "CB 500X")
	it := h.addItem(t, tn, itemSpec{name: "Banco", values: valueIDs(a)})
	err := h.try(tn.scope(), func(ctx context.Context) error {
		if err := h.Attributes.Delete(ctx, tn.id, a.ID()); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("the transaction = %v", err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		got, _ := h.Items.ByID(ctx, tn.id, it.ID())
		if _, err := h.Attributes.ByID(ctx, tn.id, a.ID()); err != nil || len(got.AttributeValueIDs()) != 1 {
			t.Errorf("after the rollback: %v, item values %v; want both back", err, got.AttributeValueIDs())
		}
		return nil
	})
}

func attributeIsolation(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	theirs := h.addAttribute(t, b, "Serve em", 0, "CB 500X")
	h.addItem(t, b, itemSpec{name: "Banco", values: valueIDs(theirs)})
	h.in(t, a.scope(), func(ctx context.Context) error {
		if _, err := h.Attributes.ByID(ctx, b.id, theirs.ID()); !errors.Is(err, catalog.ErrAttributeNotFound) {
			t.Errorf("reading B's attribute as A: %v", err)
		}
		if usage, err := h.Attributes.UsageCounts(ctx, b.id); err != nil || len(usage) != 0 {
			t.Errorf("B's usage in A's scope = %v, %v; want none", usage, err)
		}
		if err := h.Attributes.Delete(ctx, b.id, theirs.ID()); !errors.Is(err, catalog.ErrAttributeNotFound) {
			t.Errorf("A deleting B's attribute: %v", err)
		}
		if err := h.Attributes.Update(ctx, theirs, nil); !errors.Is(err, catalog.ErrAttributeNotFound) {
			t.Errorf("A updating B's attribute: %v", err)
		}
		return nil
	})
	if err := h.try(a.scope(), func(ctx context.Context) error { return h.Attributes.Insert(ctx, theirs, 0) }); err == nil {
		t.Error("store A inserted an attribute of store B")
	}
	h.in(t, b.storePage(), func(ctx context.Context) error {
		if list, err := h.Attributes.List(ctx, b.id); err != nil || len(list) != 1 {
			t.Errorf("the store page's list = %v, %v; want the attribute", list, err)
		}
		return nil
	})
}
