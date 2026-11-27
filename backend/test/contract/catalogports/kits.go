package catalogports

import (
	"context"
	"errors"
	"slices"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// RunRepositoryContract runs every catalog repository suite.
func RunRepositoryContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("Categories", func(t *testing.T) { RunCategoriesContract(t, newHarness) })
	t.Run("Attributes", func(t *testing.T) { RunAttributesContract(t, newHarness) })
	t.Run("Items", func(t *testing.T) { RunItemsContract(t, newHarness) })
	t.Run("Kits", func(t *testing.T) { RunKitsContract(t, newHarness) })
}

// RunKitsContract checks the Kits repository: a kit reads back with its
// items and alternatives, optimistic updates, the list, the published list,
// and tenant isolation. Kits hold no staff data, so the store page scope
// reads them.
func RunKitsContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("a kit reads back with its items", func(t *testing.T) { kitRoundTrip(t, newHarness(t)) })
	t.Run("kit updates are optimistic", func(t *testing.T) { kitUpdate(t, newHarness(t)) })
	t.Run("a kit holds only its store's items", func(t *testing.T) { kitForeignItem(t, newHarness(t)) })
	t.Run("the kit list orders, pages and filters", func(t *testing.T) { kitList(t, newHarness(t)) })
	t.Run("the published kits", func(t *testing.T) { kitPublished(t, newHarness(t)) })
	t.Run("another store's kits are invisible", func(t *testing.T) { kitIsolation(t, newHarness(t)) })
}

// sameKit reports how got differs from want, or "".
func sameKit(got, want *catalog.Kit, version int) string {
	ga, gok := got.AssemblyDays()
	wa, wok := want.AssemblyDays()
	gm, gmok := got.SafetyMarginDays()
	wm, wmok := want.SafetyMarginDays()
	switch {
	case got.ID() != want.ID() || got.Name() != want.Name() || got.Price() != want.Price() || got.Version() != version:
		return "details or version"
	case got.SwapMode() != want.SwapMode() || ga != wa || gok != wok || gm != wm || gmok != wmok:
		return "swap mode or days"
	case got.Published() != want.Published() || got.Archived() != want.Archived():
		return "published or archived"
	case !slices.Equal(got.AttributeValueIDs(), want.AttributeValueIDs()):
		return "values"
	}
	gi, wi := got.Items(), want.Items()
	if len(gi) != len(wi) {
		return "items"
	}
	for i := range gi {
		if gi[i].Item != wi[i].Item || gi[i].Quantity != wi[i].Quantity || gi[i].Swappable != wi[i].Swappable ||
			!slices.Equal(gi[i].Alternatives, wi[i].Alternatives) {
			return "items"
		}
	}
	return ""
}

func kitRoundTrip(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	attr := h.addAttribute(t, tn, "Serve em", 0, "CB 500X")
	seat := h.addItem(t, tn, itemSpec{name: "Banco", inHouse: true})
	alt := h.addItem(t, tn, itemSpec{name: "Banco alto", inHouse: true})
	wheel := h.addItem(t, tn, itemSpec{name: "Roda", inHouse: true})
	k := h.addKit(t, tn, kitSpec{name: "Kit Scrambler", values: valueIDs(attr), published: true, items: []catalog.KitItem{
		{Item: wheel.ID(), Quantity: 2},
		{Item: seat.ID(), Quantity: 1, Swappable: true, Alternatives: []shared.ItemID{alt.ID()}},
	}})
	h.in(t, tn.storePage(), func(ctx context.Context) error {
		got, err := h.Kits.ByID(ctx, tn.id, k.ID())
		if err != nil {
			t.Fatalf("ByID: %v", err)
		}
		if diff := sameKit(got, k, 1); diff != "" {
			t.Errorf("the kit read back with other %s", diff)
		}
		if _, err := h.Kits.ByID(ctx, tn.id, shared.KitID(h.NewID())); !errors.Is(err, catalog.ErrKitNotFound) {
			t.Errorf("an unknown kit: %v, want ErrKitNotFound", err)
		}
		return nil
	})
}

func kitUpdate(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	a := h.addItem(t, tn, itemSpec{name: "Banco", inHouse: true})
	b := h.addItem(t, tn, itemSpec{name: "Roda", inHouse: true})
	k := h.addKit(t, tn, kitSpec{name: "Kit", items: []catalog.KitItem{member(a.ID())}})
	h.in(t, tn.scope(), func(ctx context.Context) error {
		cur, _ := h.Kits.ByID(ctx, tn.id, k.ID())
		stale, _ := h.Kits.ByID(ctx, tn.id, k.ID())
		lookup := catalog.ItemLookup{a.ID(): {Price: a.Price()}, b.ID(): {Price: b.Price()}}
		err := cur.Replace(catalog.KitInput{Name: "Kit novo", Price: brl(t, 9000), Items: []catalog.KitItem{member(b.ID()), {Item: a.ID(), Quantity: 3}}}, lookup)
		if err != nil {
			return err
		}
		if err := h.Kits.Update(ctx, cur); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, _ := h.Kits.ByID(ctx, tn.id, k.ID())
		if diff := sameKit(got, cur, 2); diff != "" {
			t.Errorf("after the update: other %s", diff)
		}
		if err := h.Kits.Update(ctx, stale); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("a stale update: %v, want ErrVersionConflict", err)
		}
		ghost := h.buildKit(t, tn, kitSpec{name: "Fantasma", items: []catalog.KitItem{member(a.ID())}})
		if err := h.Kits.Update(ctx, ghost); !errors.Is(err, catalog.ErrKitNotFound) {
			t.Errorf("updating an unknown kit: %v, want ErrKitNotFound", err)
		}
		return nil
	})
	if err := h.try(tn.scope(), func(ctx context.Context) error { return h.Kits.Insert(ctx, k) }); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("the same id again: %v, want ErrConflict", err)
	}
}

func kitForeignItem(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	theirs := h.addItem(t, b, itemSpec{name: "Alheio", inHouse: true})
	k := h.buildKit(t, a, kitSpec{name: "Kit", items: []catalog.KitItem{member(theirs.ID())}})
	if err := h.try(a.scope(), func(ctx context.Context) error { return h.Kits.Insert(ctx, k) }); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("a kit holding another store's item: %v, want ErrInvalid", err)
	}
}

func kitNames(p shared.Page[*catalog.Kit]) []string {
	out := []string{}
	for _, k := range p.Items {
		out = append(out, k.Name())
	}
	return out
}

func kitList(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	it := h.addItem(t, tn, itemSpec{name: "Banco", inHouse: true})
	for _, spec := range []kitSpec{{name: "Charlie"}, {name: "Alfa Couro"}, {name: "Bravo", archived: true}} {
		spec.items = []catalog.KitItem{member(it.ID())}
		h.addKit(t, tn, spec)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		first, err := h.Kits.List(ctx, tn.id, appcatalog.KitFilter{IncludeArchived: true}, page(t, "", 2))
		if err != nil || !slices.Equal(kitNames(first), []string{"Alfa Couro", "Bravo"}) || !first.HasMore() {
			t.Fatalf("first page = %v, %v", kitNames(first), err)
		}
		last, err := h.Kits.List(ctx, tn.id, appcatalog.KitFilter{IncludeArchived: true}, page(t, first.NextCursor, 2))
		if err != nil || !slices.Equal(kitNames(last), []string{"Charlie"}) || last.HasMore() {
			t.Errorf("last page = %v, %v", kitNames(last), err)
		}
		active, err := h.Kits.List(ctx, tn.id, appcatalog.KitFilter{}, page(t, "", 10))
		if err != nil || !slices.Equal(kitNames(active), []string{"Alfa Couro", "Charlie"}) {
			t.Errorf("active kits = %v, %v", kitNames(active), err)
		}
		found, err := h.Kits.List(ctx, tn.id, appcatalog.KitFilter{Query: "couro"}, page(t, "", 10))
		if err != nil || !slices.Equal(kitNames(found), []string{"Alfa Couro"}) {
			t.Errorf("kits matching couro = %v, %v", kitNames(found), err)
		}
		if _, err := h.Kits.List(ctx, tn.id, appcatalog.KitFilter{}, page(t, "bad", 10)); !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("a bad cursor: %v, want ErrInvalid", err)
		}
		return nil
	})
}

func kitPublished(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	it := h.addItem(t, tn, itemSpec{name: "Banco", inHouse: true})
	items := []catalog.KitItem{member(it.ID())}
	h.addKit(t, tn, kitSpec{name: "Zulu", items: items, published: true})
	h.addKit(t, tn, kitSpec{name: "Rascunho", items: items})
	h.addKit(t, tn, kitSpec{name: "Antigo", items: items, archived: true})
	h.addKit(t, tn, kitSpec{name: "Alfa", items: items, published: true})
	h.in(t, tn.storePage(), func(ctx context.Context) error {
		list, err := h.Kits.Published(ctx, tn.id)
		var names []string
		for _, k := range list {
			names = append(names, k.Name())
		}
		if err != nil || !slices.Equal(names, []string{"Alfa", "Zulu"}) {
			t.Errorf("Published = %v, %v; want Alfa and Zulu", names, err)
		}
		return nil
	})
}

func kitIsolation(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	it := h.addItem(t, b, itemSpec{name: "Banco", inHouse: true})
	theirs := h.addKit(t, b, kitSpec{name: "Kit", items: []catalog.KitItem{member(it.ID())}, published: true})
	h.in(t, a.scope(), func(ctx context.Context) error {
		if _, err := h.Kits.ByID(ctx, b.id, theirs.ID()); !errors.Is(err, catalog.ErrKitNotFound) {
			t.Errorf("reading B's kit as A: %v", err)
		}
		if p, err := h.Kits.List(ctx, b.id, appcatalog.KitFilter{}, page(t, "", 10)); err != nil || len(p.Items) != 0 {
			t.Errorf("B's kits in A's scope = %v, %v", kitNames(p), err)
		}
		if list, err := h.Kits.Published(ctx, b.id); err != nil || len(list) != 0 {
			t.Errorf("B's published kits in A's scope = %v, %v", list, err)
		}
		if err := h.Kits.Update(ctx, theirs); !errors.Is(err, catalog.ErrKitNotFound) {
			t.Errorf("A updating B's kit: %v", err)
		}
		return nil
	})
}
