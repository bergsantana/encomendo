package catalogports

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// RunRepositoryContract runs every catalog repository suite.
func RunRepositoryContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("Categories", func(t *testing.T) { RunCategoriesContract(t, newHarness) })
	t.Run("Attributes", func(t *testing.T) { RunAttributesContract(t, newHarness) })
	t.Run("Items", func(t *testing.T) { RunItemsContract(t, newHarness) })
	t.Run("Kits", func(t *testing.T) { RunKitsContract(t, newHarness) })
	t.Run("every method needs a transaction", func(t *testing.T) { needsTx(t, newHarness(t)) })
}

// needsTx calls every repository method outside a transaction.
func needsTx(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	ctx := context.Background()
	it := h.buildItem(t, tn, itemSpec{name: "Banco", inHouse: true})
	k := h.buildKit(t, tn, kitSpec{name: "Kit", items: []catalog.KitItem{member(it.ID())}})
	c, _ := catalog.NewCategory(tn.id, shared.CategoryID(h.NewID()), "Bancos", 0)
	a, _ := catalog.NewAttribute(tn.id, shared.AttributeID(h.NewID()), catalog.AttributeInput{Name: "Serve em"})
	p := page(t, "", 10)
	ignore := func(_ any, err error) error { return err }
	calls := map[string]func() error{
		"Categories.List":        func() error { return ignore(h.Categories.List(ctx, tn.id)) },
		"Categories.ByID":        func() error { return ignore(h.Categories.ByID(ctx, tn.id, c.ID())) },
		"Categories.Insert":      func() error { return h.Categories.Insert(ctx, c) },
		"Categories.Update":      func() error { return h.Categories.Update(ctx, c) },
		"Categories.Delete":      func() error { return h.Categories.Delete(ctx, tn.id, c.ID()) },
		"Attributes.List":        func() error { return ignore(h.Attributes.List(ctx, tn.id)) },
		"Attributes.ByID":        func() error { return ignore(h.Attributes.ByID(ctx, tn.id, a.ID())) },
		"Attributes.Insert":      func() error { return h.Attributes.Insert(ctx, a, 0) },
		"Attributes.Update":      func() error { return h.Attributes.Update(ctx, a, nil) },
		"Attributes.Delete":      func() error { return h.Attributes.Delete(ctx, tn.id, a.ID()) },
		"Attributes.UsageCounts": func() error { return ignore(h.Attributes.UsageCounts(ctx, tn.id)) },
		"Items.Insert":           func() error { return h.Items.Insert(ctx, it) },
		"Items.Update":           func() error { return h.Items.Update(ctx, it) },
		"Items.ByID":             func() error { return ignore(h.Items.ByID(ctx, tn.id, it.ID())) },
		"Items.ByIDs":            func() error { return ignore(h.Items.ByIDs(ctx, tn.id, []shared.ItemID{it.ID()})) },
		"Items.PublicFacts":      func() error { return ignore(h.Items.PublicFacts(ctx, tn.id, []shared.ItemID{it.ID()})) },
		"Items.List":             func() error { return ignore(h.Items.List(ctx, tn.id, appcatalog.ItemFilter{}, p)) },
		"Items.ActiveKitsHolding": func() error {
			return ignore(h.Items.ActiveKitsHolding(ctx, tn.id, it.ID()))
		},
		"Items.Published": func() error { return ignore(h.Items.Published(ctx, tn.id)) },
		"Kits.Insert":     func() error { return h.Kits.Insert(ctx, k) },
		"Kits.Update":     func() error { return h.Kits.Update(ctx, k) },
		"Kits.ByID":       func() error { return ignore(h.Kits.ByID(ctx, tn.id, k.ID())) },
		"Kits.List":       func() error { return ignore(h.Kits.List(ctx, tn.id, appcatalog.KitFilter{}, p)) },
		"Kits.Published":  func() error { return ignore(h.Kits.Published(ctx, tn.id)) },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, appshared.ErrNoTransaction) {
			t.Errorf("%s outside a transaction: %v, want ErrNoTransaction", name, err)
		}
	}
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
	t.Run("an archived kit cannot be inserted", func(t *testing.T) { kitArchivedInsert(t, newHarness(t)) })
	t.Run("kit values are in field order and alternatives are a set", func(t *testing.T) { kitOrder(t, newHarness(t)) })
}

func kitArchivedInsert(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	it := h.addItem(t, tn, itemSpec{name: "Banco", inHouse: true})
	k := h.buildKit(t, tn, kitSpec{name: "Kit", items: []catalog.KitItem{member(it.ID())}})
	k.Archive()
	err := h.try(tn.scope(), func(ctx context.Context) error { return h.Kits.Insert(ctx, k) })
	if err == nil || errors.Is(err, shared.ErrNotFound) || errors.Is(err, shared.ErrConflict) || errors.Is(err, shared.ErrInvalid) {
		t.Errorf("inserting an archived kit: %v, want a refusal that matches no sentinel", err)
	}
}

// kitOrder checks the order the Kits port promises: values in the store's
// field order whatever order they were sent in, and the alternatives of a
// kit item as a set, compared sorted.
func kitOrder(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	first := h.addAttribute(t, tn, "Serve em", 0, "CB 500X", "XRE 300")
	second := h.addAttribute(t, tn, "Material", 1, "Couro", "Tecido")
	want := append(valueIDs(first), valueIDs(second)...)
	sent := slices.Clone(want)
	slices.Reverse(sent)
	seat := h.addItem(t, tn, itemSpec{name: "Banco", inHouse: true})
	var alts []shared.ItemID
	for _, name := range []string{"Banco alto", "Banco baixo", "Banco gel"} {
		alts = append(alts, h.addItem(t, tn, itemSpec{name: name, inHouse: true}).ID())
	}
	k := h.addKit(t, tn, kitSpec{name: "Kit", values: sent, items: []catalog.KitItem{
		{Item: seat.ID(), Quantity: 1, Swappable: true, Alternatives: []shared.ItemID{alts[2], alts[0], alts[1]}},
	}})
	h.in(t, tn.storePage(), func(ctx context.Context) error {
		got, err := h.Kits.ByID(ctx, tn.id, k.ID())
		if err != nil {
			t.Fatalf("ByID: %v", err)
		}
		if !slices.Equal(got.AttributeValueIDs(), want) {
			t.Errorf("values = %v, want the store's field order %v", got.AttributeValueIDs(), want)
		}
		gotAlts := slices.Clone(got.Items()[0].Alternatives)
		wantAlts := slices.Clone(alts)
		less := func(a, b shared.ItemID) int { return strings.Compare(a.String(), b.String()) }
		slices.SortFunc(gotAlts, less)
		slices.SortFunc(wantAlts, less)
		if !slices.Equal(gotAlts, wantAlts) {
			t.Errorf("alternatives = %v, want the set %v", gotAlts, wantAlts)
		}
		return nil
	})
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
	// A field value of another store is the same refusal.
	theirAttr := h.addAttribute(t, b, "Serve em", 0, "CB 500X")
	own := h.addItem(t, a, itemSpec{name: "Banco", inHouse: true})
	k = h.buildKit(t, a, kitSpec{name: "Kit", items: []catalog.KitItem{member(own.ID())}, values: valueIDs(theirAttr)})
	if err := h.try(a.scope(), func(ctx context.Context) error { return h.Kits.Insert(ctx, k) }); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("a kit with another store's field value: %v, want ErrInvalid", err)
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
	for _, spec := range []kitSpec{{name: "Charlie"}, {name: "alfa Couro"}, {name: "Bravo", archived: true}} {
		spec.items = []catalog.KitItem{member(it.ID())}
		h.addKit(t, tn, spec)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		first, err := h.Kits.List(ctx, tn.id, appcatalog.KitFilter{IncludeArchived: true}, page(t, "", 2))
		if err != nil || !slices.Equal(kitNames(first), []string{"alfa Couro", "Bravo"}) || !first.HasMore() {
			t.Fatalf("first page = %v, %v", kitNames(first), err)
		}
		last, err := h.Kits.List(ctx, tn.id, appcatalog.KitFilter{IncludeArchived: true}, page(t, first.NextCursor, 2))
		if err != nil || !slices.Equal(kitNames(last), []string{"Charlie"}) || last.HasMore() {
			t.Errorf("last page = %v, %v", kitNames(last), err)
		}
		active, err := h.Kits.List(ctx, tn.id, appcatalog.KitFilter{}, page(t, "", 10))
		if err != nil || !slices.Equal(kitNames(active), []string{"alfa Couro", "Charlie"}) {
			t.Errorf("active kits = %v, %v", kitNames(active), err)
		}
		found, err := h.Kits.List(ctx, tn.id, appcatalog.KitFilter{Query: "couro"}, page(t, "", 10))
		if err != nil || !slices.Equal(kitNames(found), []string{"alfa Couro"}) {
			t.Errorf("kits matching couro = %v, %v", kitNames(found), err)
		}
		if _, err := h.Kits.List(ctx, tn.id, appcatalog.KitFilter{}, page(t, "bad", 10)); !isCursorError(err) {
			t.Errorf("a bad cursor: %v, want a ValidationError on cursor", err)
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
