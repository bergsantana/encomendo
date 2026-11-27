package catalogports

import (
	"context"
	"errors"
	"slices"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// RunItemsContract checks the Items repository: an item reads back with its
// supplier links, costs and ready days; optimistic updates; the list; the
// facts for kits; the kits holding an item; the published list; and what a
// scope with no user sees (staff-only tables, ADR 0024).
func RunItemsContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("an item reads back with its sourcing and costs", func(t *testing.T) { itemRoundTrip(t, newHarness(t)) })
	t.Run("item updates are optimistic and keep ready days", func(t *testing.T) { itemUpdate(t, newHarness(t)) })
	t.Run("a duplicate item id is a conflict", func(t *testing.T) { itemDuplicate(t, newHarness(t)) })
	t.Run("an item's category and values must be the store's", func(t *testing.T) { itemReferences(t, newHarness(t)) })
	t.Run("a failed transaction undoes an item write", func(t *testing.T) { itemRollback(t, newHarness(t)) })
	t.Run("the item list orders, pages and filters", func(t *testing.T) { itemList(t, newHarness(t)) })
	t.Run("a cursor the repository did not issue is invalid", func(t *testing.T) { itemBadCursor(t, newHarness(t)) })
	t.Run("ByIDs gives the facts of the items that exist", func(t *testing.T) { itemFacts(t, newHarness(t)) })
	t.Run("the kits holding an item", func(t *testing.T) { itemKits(t, newHarness(t)) })
	t.Run("the published items", func(t *testing.T) { itemPublished(t, newHarness(t)) })
	t.Run("a scope with no user reads no supplier or cost", func(t *testing.T) { itemStorePage(t, newHarness(t)) })
	t.Run("another store's items are invisible", func(t *testing.T) { itemIsolation(t, newHarness(t)) })
	t.Run("a sourcing change replaces the staff-only rows", func(t *testing.T) { itemSourcingChanges(t, newHarness(t)) })
	t.Run("PublicFacts never gives a cost, in any scope", func(t *testing.T) { itemPublicFacts(t, newHarness(t)) })
	t.Run("an archived item cannot be inserted", func(t *testing.T) { itemArchivedInsert(t, newHarness(t)) })
	t.Run("field values come back in the store's field order", func(t *testing.T) { itemFieldOrder(t, newHarness(t)) })
}

func sameMoney(a, b *shared.Money) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// sameItem reports how got differs from want, or "".
func sameItem(got, want *catalog.Item, version int) string {
	gm, _ := got.MainSupplier()
	wm, _ := want.MainSupplier()
	gb, gotBackup := got.BackupSupplier()
	wb, wantBackup := want.BackupSupplier()
	switch {
	case got.ID() != want.ID() || got.TenantID() != want.TenantID() || got.Name() != want.Name() || got.Description() != want.Description():
		return "details"
	case got.Price() != want.Price() || got.CategoryID() != want.CategoryID() || got.Version() != version:
		return "price, category or version"
	case got.SourcingMode() != want.SourcingMode() || got.ProductionDays() != want.ProductionDays() || got.ReadyDays() != want.ReadyDays():
		return "sourcing"
	case !sameMoney(got.Cost(), want.Cost()) || !sameMoney(got.ProductionCost(), want.ProductionCost()):
		return "costs"
	case gm.SupplierID() != wm.SupplierID() || gm.LeadDays() != wm.LeadDays() || !sameMoney(gm.Cost(), wm.Cost()):
		return "main supplier"
	case gotBackup != wantBackup || gb.SupplierID() != wb.SupplierID() || !sameMoney(gb.Cost(), wb.Cost()):
		return "backup supplier"
	case !slices.Equal(got.AttributeValueIDs(), want.AttributeValueIDs()) || len(got.Options()) != len(want.Options()):
		return "values or options"
	case got.Published() != want.Published() || got.Archived() != want.Archived():
		return "published or archived"
	}
	go1, wo := got.Options(), want.Options()
	for i := range go1 {
		if go1[i].ID() != wo[i].ID() || go1[i].Name() != wo[i].Name() || !slices.Equal(go1[i].Choices(), wo[i].Choices()) {
			return "options"
		}
	}
	return ""
}

func itemRoundTrip(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	cat := h.addCategory(t, tn, "Bancos", 0)
	attr := h.addAttribute(t, tn, "Serve em", 0, "CB 500X")
	supplied := h.addItem(t, tn, itemSpec{name: "Banco", category: cat.ID(), values: valueIDs(attr), published: true})
	own := h.addItem(t, tn, itemSpec{name: "Capa", inHouse: true})
	h.in(t, tn.scope(), func(ctx context.Context) error {
		for _, want := range []*catalog.Item{supplied, own} {
			got, err := h.Items.ByID(ctx, tn.id, want.ID())
			if err != nil {
				t.Fatalf("ByID(%s): %v", want.Name(), err)
			}
			if diff := sameItem(got, want, 1); diff != "" {
				t.Errorf("%s read back with other %s", want.Name(), diff)
			}
		}
		if _, err := h.Items.ByID(ctx, tn.id, shared.ItemID(h.NewID())); !errors.Is(err, catalog.ErrItemNotFound) {
			t.Errorf("an unknown item: %v, want ErrItemNotFound", err)
		}
		return nil
	})
}

func itemUpdate(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	it := h.addItem(t, tn, itemSpec{name: "Banco"})
	h.in(t, tn.scope(), func(ctx context.Context) error {
		cur, _ := h.Items.ByID(ctx, tn.id, it.ID())
		stale, _ := h.Items.ByID(ctx, tn.id, it.ID())
		// Swap main and backup, and give the new main a longer lead time.
		m, _ := cur.MainSupplier()
		b, _ := cur.BackupSupplier()
		err := cur.ReplaceSourcing(catalog.SourcingInput{
			Mode: catalog.ModeStock,
			Main: &catalog.SupplierLink{Supplier: b.SupplierID(), LeadDays: 12}, Backup: &catalog.SupplierLink{Supplier: m.SupplierID(), LeadDays: 7},
		})
		if err != nil {
			return err
		}
		if err := h.Items.Update(ctx, cur); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := h.Items.ByID(ctx, tn.id, it.ID())
		if err != nil {
			return err
		}
		if diff := sameItem(got, cur, 2); diff != "" || got.ReadyDays() != 12 {
			t.Errorf("after the update: other %s, ready days %d; want the swap at version 2, 12 days", diff, got.ReadyDays())
		}
		if gb, _ := got.BackupSupplier(); !sameMoney(gb.Cost(), m.Cost()) {
			t.Errorf("the old main's cost did not follow it to the backup slot: %v", gb.Cost())
		}
		if err := h.Items.Update(ctx, stale); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("a stale update: %v, want ErrVersionConflict", err)
		}
		ghost := h.buildItem(t, tn, itemSpec{name: "Fantasma", inHouse: true})
		if err := h.Items.Update(ctx, ghost); !errors.Is(err, catalog.ErrItemNotFound) {
			t.Errorf("updating an unknown item: %v, want ErrItemNotFound", err)
		}
		return nil
	})
}

func itemDuplicate(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	it := h.addItem(t, tn, itemSpec{name: "Banco", inHouse: true})
	err := h.try(tn.scope(), func(ctx context.Context) error { return h.Items.Insert(ctx, it) })
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("the same id again: %v, want ErrConflict", err)
	}
}

func itemReferences(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	theirCat := h.addCategory(t, b, "Bancos", 0)
	theirAttr := h.addAttribute(t, b, "Serve em", 0, "CB 500X")
	for name, spec := range map[string]itemSpec{
		"another store's category": {name: "Banco", category: theirCat.ID(), inHouse: true},
		"another store's value":    {name: "Banco", values: valueIDs(theirAttr), inHouse: true},
		"another store's supplier": {name: "Banco", supplier: h.NewSupplier(t, b.id)},
	} {
		it := h.buildItem(t, a, spec)
		if err := h.try(a.scope(), func(ctx context.Context) error { return h.Items.Insert(ctx, it) }); !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
}

func itemRollback(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	kept := h.addItem(t, tn, itemSpec{name: "Banco"})
	fresh := h.buildItem(t, tn, itemSpec{name: "Novo"})
	err := h.try(tn.scope(), func(ctx context.Context) error {
		if err := h.Items.Insert(ctx, fresh); err != nil {
			return err
		}
		cur, err := h.Items.ByID(ctx, tn.id, kept.ID())
		if err != nil {
			return err
		}
		cur.Archive()
		if err := cur.ReplaceSourcing(catalog.SourcingInput{Mode: catalog.ModeInHouse, ProductionDays: 3}); err != nil {
			return err
		}
		if err := h.Items.Update(ctx, cur); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("the transaction = %v", err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		if _, err := h.Items.ByID(ctx, tn.id, fresh.ID()); !errors.Is(err, catalog.ErrItemNotFound) {
			t.Errorf("the rolled-back insert: %v, want ErrItemNotFound", err)
		}
		got, err := h.Items.ByID(ctx, tn.id, kept.ID())
		if err != nil || sameItem(got, kept, 1) != "" {
			t.Errorf("the rolled-back update: %v; want the item as it was", err)
		}
		return nil
	})
}

func itemNames(p shared.Page[*catalog.Item]) []string {
	out := []string{}
	for _, it := range p.Items {
		out = append(out, it.Name())
	}
	return out
}

func itemList(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	bancos := h.addCategory(t, tn, "Bancos", 0)
	for _, spec := range []itemSpec{
		{name: "Delta", inHouse: true}, {name: "alfa", category: bancos.ID(), inHouse: true},
		{name: "Charlie", archived: true, inHouse: true}, {name: "Bravo Banco", category: bancos.ID()}, {name: "Echo"},
	} {
		h.addItem(t, tn, spec)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		var all []string
		cursor := ""
		for range 10 {
			p, err := h.Items.List(ctx, tn.id, appcatalog.ItemFilter{IncludeArchived: true}, page(t, cursor, 2))
			if err != nil || len(p.Items) > 2 {
				t.Fatalf("List = %v, %v", itemNames(p), err)
			}
			all = append(all, itemNames(p)...)
			if cursor = p.NextCursor; cursor == "" {
				break
			}
		}
		if !slices.Equal(all, []string{"alfa", "Bravo Banco", "Charlie", "Delta", "Echo"}) {
			t.Errorf("all pages = %v, want every item by lower(name)", all)
		}
		cases := map[string]struct {
			filter appcatalog.ItemFilter
			want   []string
		}{
			"active only":       {appcatalog.ItemFilter{}, []string{"alfa", "Bravo Banco", "Delta", "Echo"}},
			"by query":          {appcatalog.ItemFilter{Query: "BANCO"}, []string{"Bravo Banco"}},
			"by category":       {appcatalog.ItemFilter{Category: bancos.ID()}, []string{"alfa", "Bravo Banco"}},
			"a literal percent": {appcatalog.ItemFilter{Query: "%"}, []string{}},
		}
		for name, c := range cases {
			p, err := h.Items.List(ctx, tn.id, c.filter, page(t, "", 10))
			if err != nil || !slices.Equal(itemNames(p), c.want) || p.HasMore() {
				t.Errorf("%s = %v, %v; want %v", name, itemNames(p), err, c.want)
			}
		}
		return nil
	})
}

func itemBadCursor(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	for _, n := range []string{"Alfa", "Bravo"} {
		h.addItem(t, a, itemSpec{name: n, inHouse: true})
		h.addItem(t, b, itemSpec{name: n, inHouse: true})
	}
	var theirs string
	h.in(t, b.scope(), func(ctx context.Context) error {
		p, err := h.Items.List(ctx, b.id, appcatalog.ItemFilter{}, page(t, "", 1))
		theirs = p.NextCursor
		return err
	})
	h.in(t, a.scope(), func(ctx context.Context) error {
		for name, c := range map[string]string{"gibberish": "not-a-cursor", "another store's cursor": theirs} {
			if _, err := h.Items.List(ctx, a.id, appcatalog.ItemFilter{}, page(t, c, 10)); !isCursorError(err) {
				t.Errorf("%s: %v, want a ValidationError on cursor", name, err)
			}
		}
		return nil
	})
}

func factsByID(list []appcatalog.ItemFacts) map[shared.ItemID]appcatalog.ItemFacts {
	out := map[shared.ItemID]appcatalog.ItemFacts{}
	for _, f := range list {
		out[f.ID] = f
	}
	return out
}

func itemFacts(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	supplied := h.addItem(t, tn, itemSpec{name: "Banco", published: true})
	own := h.addItem(t, tn, itemSpec{name: "Capa", inHouse: true, archived: true})
	missing := shared.ItemID(h.NewID())
	h.in(t, tn.scope(), func(ctx context.Context) error {
		list, err := h.Items.ByIDs(ctx, tn.id, []shared.ItemID{supplied.ID(), own.ID(), missing, own.ID()})
		got := factsByID(list)
		if err != nil || len(list) != 2 {
			t.Fatalf("ByIDs = %+v, %v; want two items, once each", list, err)
		}
		s, o := got[supplied.ID()], got[own.ID()]
		if s.Name != "Banco" || !s.Published || o.Published || s.Facts.Price != supplied.Price() || s.Facts.ReadyDays != 7 || s.Facts.Archived || !sameMoney(s.Facts.Cost, supplied.Cost()) {
			t.Errorf("the supplied item's facts = %+v", s)
		}
		if o.Facts.ReadyDays != 5 || !o.Facts.Archived || !sameMoney(o.Facts.Cost, own.Cost()) {
			t.Errorf("the in-house item's facts = %+v", o)
		}
		if none, err := h.Items.ByIDs(ctx, tn.id, nil); err != nil || len(none) != 0 {
			t.Errorf("no ids = %v, %v", none, err)
		}
		return nil
	})
}

func itemKits(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	it := h.addItem(t, tn, itemSpec{name: "Banco", inHouse: true})
	alt := h.addItem(t, tn, itemSpec{name: "Banco reserva", inHouse: true})
	other := h.addItem(t, tn, itemSpec{name: "Roda", inHouse: true})
	zulu := h.addKit(t, tn, kitSpec{name: "Zulu", items: []catalog.KitItem{member(it.ID())}})
	alfa := h.addKit(t, tn, kitSpec{name: "Alfa", items: []catalog.KitItem{member(other.ID()), {Item: alt.ID(), Quantity: 1, Swappable: true, Alternatives: []shared.ItemID{it.ID()}}}})
	h.addKit(t, tn, kitSpec{name: "Arquivado", items: []catalog.KitItem{member(it.ID())}, archived: true})
	h.in(t, tn.scope(), func(ctx context.Context) error {
		refs, err := h.Items.ActiveKitsHolding(ctx, tn.id, it.ID())
		want := []appcatalog.Reference{{ID: shared.ID(alfa.ID()), Name: "Alfa"}, {ID: shared.ID(zulu.ID()), Name: "Zulu"}}
		if err != nil || !slices.Equal(refs, want) {
			t.Errorf("kits holding the item = %+v, %v; want Alfa (alternative) and Zulu (member), not the archived kit", refs, err)
		}
		if refs, err := h.Items.ActiveKitsHolding(ctx, tn.id, shared.ItemID(h.NewID())); err != nil || len(refs) != 0 {
			t.Errorf("an item in no kit = %v, %v", refs, err)
		}
		return nil
	})
}

func itemPublished(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	attr := h.addAttribute(t, tn, "Serve em", 0, "CB 500X")
	want := h.addItem(t, tn, itemSpec{name: "Banco", published: true, values: valueIDs(attr)})
	h.addItem(t, tn, itemSpec{name: "Rascunho", inHouse: true})
	h.addItem(t, tn, itemSpec{name: "Antigo", inHouse: true, archived: true})
	h.addItem(t, tn, itemSpec{name: "Alfa", inHouse: true, published: true})
	h.in(t, tn.storePage(), func(ctx context.Context) error {
		list, err := h.Items.Published(ctx, tn.id)
		if err != nil || len(list) != 2 || list[0].Name != "Alfa" || list[1].ID != want.ID() {
			t.Fatalf("Published = %+v, %v; want Alfa and Banco", list, err)
		}
		got := list[1]
		if got.Price != want.Price() || got.ReadyDays != 7 || got.Description != want.Description() ||
			!slices.Equal(got.AttributeValues, want.AttributeValueIDs()) || len(got.Options) != 1 || got.Options[0].ID() != want.Options()[0].ID() {
			t.Errorf("the published item = %+v; want its store page data", got)
		}
		return nil
	})
}

func itemStorePage(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	it := h.addItem(t, tn, itemSpec{name: "Banco", published: true})
	h.in(t, tn.storePage(), func(ctx context.Context) error {
		if _, err := h.Items.ByID(ctx, tn.id, it.ID()); !errors.Is(err, catalog.ErrItemNotFound) {
			t.Errorf("ByID with no user: %v, want ErrItemNotFound", err)
		}
		if p, err := h.Items.List(ctx, tn.id, appcatalog.ItemFilter{}, page(t, "", 10)); err != nil || len(p.Items) != 0 {
			t.Errorf("List with no user = %v, %v; want none", itemNames(p), err)
		}
		list, err := h.Items.ByIDs(ctx, tn.id, []shared.ItemID{it.ID()})
		if err != nil || len(list) != 1 || list[0].Facts.Cost != nil || list[0].Facts.ReadyDays != 7 {
			t.Errorf("ByIDs with no user = %+v, %v; want the facts without a cost, ready days kept", list, err)
		}
		return nil
	})
	// Writes need a staff scope: RLS would hide the supplier rows, so a
	// replace could never drop them. The refusal matches no sentinel.
	refused := func(err error) bool {
		return err != nil && !errors.Is(err, shared.ErrNotFound) && !errors.Is(err, shared.ErrConflict) && !errors.Is(err, shared.ErrInvalid)
	}
	for name, fresh := range map[string]*catalog.Item{
		"with suppliers": h.buildItem(t, tn, itemSpec{name: "Novo"}),
		"in house":       h.buildItem(t, tn, itemSpec{name: "Capa", inHouse: true}),
	} {
		if err := h.try(tn.storePage(), func(ctx context.Context) error { return h.Items.Insert(ctx, fresh) }); !refused(err) {
			t.Errorf("a scope with no user inserting an item %s: %v, want a refusal", name, err)
		}
	}
	it.Unpublish()
	if err := it.ReplaceSourcing(catalog.SourcingInput{Mode: catalog.ModeInHouse, ProductionDays: 2}); err != nil {
		t.Fatal(err)
	}
	if err := h.try(tn.storePage(), func(ctx context.Context) error { return h.Items.Update(ctx, it) }); !refused(err) {
		t.Errorf("a scope with no user updating an item with supplier rows: %v, want a refusal", err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		got, err := h.Items.ByID(ctx, tn.id, it.ID())
		if _, main := got.MainSupplier(); err != nil || !got.Published() || got.Version() != 1 || !main {
			t.Errorf("after the refused update: %+v, %v; want it unchanged", got, err)
		}
		return nil
	})
}

func itemIsolation(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	theirs := h.addItem(t, b, itemSpec{name: "Banco", published: true})
	h.addKit(t, b, kitSpec{name: "Kit", items: []catalog.KitItem{member(theirs.ID())}})
	h.in(t, a.scope(), func(ctx context.Context) error {
		if _, err := h.Items.ByID(ctx, b.id, theirs.ID()); !errors.Is(err, catalog.ErrItemNotFound) {
			t.Errorf("reading B's item as A: %v", err)
		}
		if list, err := h.Items.ByIDs(ctx, b.id, []shared.ItemID{theirs.ID()}); err != nil || len(list) != 0 {
			t.Errorf("B's facts in A's scope = %v, %v", list, err)
		}
		if list, err := h.Items.Published(ctx, b.id); err != nil || len(list) != 0 {
			t.Errorf("B's published items in A's scope = %v, %v", list, err)
		}
		if refs, err := h.Items.ActiveKitsHolding(ctx, b.id, theirs.ID()); err != nil || len(refs) != 0 {
			t.Errorf("B's kits in A's scope = %v, %v", refs, err)
		}
		if err := h.Items.Update(ctx, theirs); !errors.Is(err, catalog.ErrItemNotFound) {
			t.Errorf("A updating B's item: %v", err)
		}
		return nil
	})
	intruder := h.buildItem(t, b, itemSpec{name: "Intruso", inHouse: true})
	if err := h.try(a.scope(), func(ctx context.Context) error { return h.Items.Insert(ctx, intruder) }); err == nil {
		t.Error("store A inserted an item of store B")
	}
}

// itemSourcingChanges moves an item between sourcing modes and checks what
// each Update leaves in supplier_items, item_costs and items.ready_days.
func itemSourcingChanges(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	it := h.addItem(t, tn, itemSpec{name: "Banco"})
	main, backup := h.NewSupplier(t, tn.id), h.NewSupplier(t, tn.id)
	inHouse := catalog.SourcingInput{Mode: catalog.ModeInHouse, ProductionDays: 5, ProductionCost: catalog.SetCost(brl(t, 1200))}
	got := h.replaceSourcing(t, tn, it.ID(), 2, inHouse)
	if hasMain(got) || hasBackup(got) || !sameMoney(got.ProductionCost(), ptrMoney(brl(t, 1200))) || got.ReadyDays() != 5 {
		t.Errorf("to_order to in_house: links %v %v, production cost %v, ready %d; want no links, item_costs, 5 days",
			hasMain(got), hasBackup(got), got.ProductionCost(), got.ReadyDays())
	}
	got = h.replaceSourcing(t, tn, it.ID(), 3, catalog.SourcingInput{Mode: catalog.ModeToOrder,
		Main:   &catalog.SupplierLink{Supplier: main, LeadDays: 11},
		Backup: &catalog.SupplierLink{Supplier: backup, LeadDays: 2, Cost: catalog.SetCost(brl(t, 300))}})
	b, _ := got.BackupSupplier()
	if got.ProductionCost() != nil || got.ReadyDays() != 11 || !hasBackup(got) || !sameMoney(b.Cost(), ptrMoney(brl(t, 300))) {
		t.Errorf("in_house to to_order: production cost %v, ready %d; want none, 11 days, the backup with its cost", got.ProductionCost(), got.ReadyDays())
	}
	got = h.replaceSourcing(t, tn, it.ID(), 4, catalog.SourcingInput{Mode: catalog.ModeToOrder, Main: &catalog.SupplierLink{Supplier: main, LeadDays: 4}})
	if hasBackup(got) || got.ReadyDays() != 4 {
		t.Errorf("unlinking the backup: backup %v, ready %d; want none, 4 days", hasBackup(got), got.ReadyDays())
	}
}

func hasMain(it *catalog.Item) bool   { _, ok := it.MainSupplier(); return ok }
func hasBackup(it *catalog.Item) bool { _, ok := it.BackupSupplier(); return ok }

// replaceSourcing replaces the sourcing of the item, stores it, and returns
// it as read back, checking it reads back as saved at version.
func (h Harness) replaceSourcing(t *testing.T, tn tenant, id shared.ItemID, version int, src catalog.SourcingInput) *catalog.Item {
	t.Helper()
	var got *catalog.Item
	h.in(t, tn.scope(), func(ctx context.Context) error {
		cur, err := h.Items.ByID(ctx, tn.id, id)
		if err != nil {
			return err
		}
		if err := cur.ReplaceSourcing(src); err != nil {
			return err
		}
		if err := h.Items.Update(ctx, cur); err != nil {
			return err
		}
		if got, err = h.Items.ByID(ctx, tn.id, id); err != nil {
			return err
		}
		if diff := sameItem(got, cur, version); diff != "" {
			t.Errorf("read back with other %s", diff)
		}
		return nil
	})
	return got
}

func ptrMoney(m shared.Money) *shared.Money { return &m }

func itemPublicFacts(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	supplied := h.addItem(t, a, itemSpec{name: "Banco", published: true})
	own := h.addItem(t, a, itemSpec{name: "Capa", inHouse: true, archived: true})
	theirs := h.addItem(t, b, itemSpec{name: "Alheio", inHouse: true})
	missing := shared.ItemID(h.NewID())
	ids := []shared.ItemID{supplied.ID(), own.ID(), missing, theirs.ID(), own.ID()}
	// The staff scope is the one that could pull a cost in: it still gets none.
	for name, scope := range map[string]appshared.Scope{"a staff scope": a.scope(), "the store page": a.storePage()} {
		h.in(t, scope, func(ctx context.Context) error {
			list, err := h.Items.PublicFacts(ctx, a.id, ids)
			if err != nil || len(list) != 2 {
				t.Fatalf("%s: PublicFacts = %+v, %v; want the two items of the store, once each", name, list, err)
			}
			checkPublicFacts(t, name, factsByID(list), supplied, own)
			if none, err := h.Items.PublicFacts(ctx, a.id, nil); err != nil || len(none) != 0 {
				t.Errorf("%s: no ids = %v, %v", name, none, err)
			}
			return nil
		})
	}
}

// checkPublicFacts checks the facts of the supplied item and of the archived
// in-house one, and that neither has a cost.
func checkPublicFacts(t *testing.T, scope string, got map[shared.ItemID]appcatalog.ItemFacts, supplied, own *catalog.Item) {
	t.Helper()
	s, o := got[supplied.ID()], got[own.ID()]
	if s.Name != "Banco" || !s.Published || s.Facts.Price != supplied.Price() || s.Facts.ReadyDays != 7 || s.Facts.Archived {
		t.Errorf("%s: the supplied item's facts = %+v", scope, s)
	}
	if o.Name != "Capa" || o.Published || o.Facts.ReadyDays != 5 || !o.Facts.Archived {
		t.Errorf("%s: the in-house item's facts = %+v", scope, o)
	}
	if s.Facts.Cost != nil || o.Facts.Cost != nil {
		t.Errorf("%s: costs %v and %v, want none: the store page never reads a cost", scope, s.Facts.Cost, o.Facts.Cost)
	}
}

func itemArchivedInsert(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	it := h.buildItem(t, tn, itemSpec{name: "Banco", inHouse: true})
	it.Archive()
	err := h.try(tn.scope(), func(ctx context.Context) error { return h.Items.Insert(ctx, it) })
	if err == nil || errors.Is(err, shared.ErrNotFound) || errors.Is(err, shared.ErrConflict) || errors.Is(err, shared.ErrInvalid) {
		t.Errorf("inserting an archived item: %v, want a refusal that matches no sentinel", err)
	}
}

// itemFieldOrder checks that the values an item picked come back in the
// store's field order, whatever order they were sent in.
func itemFieldOrder(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	first := h.addAttribute(t, tn, "Serve em", 0, "CB 500X", "XRE 300")
	second := h.addAttribute(t, tn, "Material", 1, "Couro", "Tecido")
	want := append(valueIDs(first), valueIDs(second)...)
	sent := slices.Clone(want)
	slices.Reverse(sent)
	it := h.addItem(t, tn, itemSpec{name: "Banco", values: sent, published: true})
	h.in(t, tn.scope(), func(ctx context.Context) error {
		got, err := h.Items.ByID(ctx, tn.id, it.ID())
		if err != nil || !slices.Equal(got.AttributeValueIDs(), want) {
			t.Errorf("ByID values = %v, %v; want the store's field order %v", got.AttributeValueIDs(), err, want)
		}
		listed, err := h.Items.List(ctx, tn.id, appcatalog.ItemFilter{}, page(t, "", 10))
		if err != nil || len(listed.Items) != 1 || !slices.Equal(listed.Items[0].AttributeValueIDs(), want) {
			t.Errorf("List values = %v, %v; want %v", listed.Items, err, want)
		}
		return nil
	})
	h.in(t, tn.storePage(), func(ctx context.Context) error {
		list, err := h.Items.Published(ctx, tn.id)
		if err != nil || len(list) != 1 || !slices.Equal(list[0].AttributeValues, want) {
			t.Errorf("Published values = %+v, %v; want %v", list, err, want)
		}
		return nil
	})
}
