package catalog_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// costedInput is suppliedInput with the owner's costs: R$ 40 from couros
// and R$ 45 from ferragens.
func (h *harness) costedInput(name string) appcatalog.ItemInput {
	in := h.suppliedInput(name)
	in.MainSupplier.Cost = appcatalog.SetCost(4000)
	in.BackupSupplier.Cost = appcatalog.SetCost(4500)
	return in
}

func (h *harness) replaceItem(ctx context.Context, t *testing.T, id shared.ItemID, version int, in appcatalog.ItemInput) appcatalog.ItemView {
	t.Helper()
	v, err := h.svc.ReplaceItem(ctx, appcatalog.ReplaceItemInput{ID: id, Item: in, Version: version})
	if err != nil {
		t.Fatalf("ReplaceItem: %v", err)
	}
	return v
}

// ownerCosts reads the item as the owner and returns its costs.
func (h *harness) ownerCosts(t *testing.T, id shared.ItemID) *appcatalog.ItemCosts {
	t.Helper()
	v, err := h.svc.GetItem(h.owner(t), id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Costs == nil {
		t.Fatal("the owner's view has no costs")
	}
	return v.Costs
}

func TestCreateItem_TheOwnerSeesCostsAndMargin(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	v := h.createItem(t, h.costedInput("Banco de couro"))

	if v.Version != 1 || v.ReadyDays != 7 || v.Price != brl(t, 10000) || v.SourcingMode != catalog.ModeToOrder || v.Published {
		t.Errorf("view = %+v", v)
	}
	if v.MainSupplier == nil || v.MainSupplier.SupplierName != "Couros Paulista" || v.MainSupplier.LeadDays != 7 ||
		v.BackupSupplier == nil || v.BackupSupplier.SupplierName != "Ferragens Norte" {
		t.Errorf("suppliers = %+v, %+v", v.MainSupplier, v.BackupSupplier)
	}
	c := v.Costs
	if c == nil || !sameMoney(c.Cost, ptr(brl(t, 4000))) || !sameMoney(c.MainSupplierCost, ptr(brl(t, 4000))) ||
		!sameMoney(c.BackupSupplierCost, ptr(brl(t, 4500))) || c.ProductionCost != nil {
		t.Fatalf("costs = %+v", c)
	}
	if c.Margin == nil || *c.Margin != (catalog.Margin{Cents: 6000, BasisPoints: 6000}) {
		t.Errorf("margin = %+v, want R$ 60 (60%%)", c.Margin)
	}
	if got := h.store.StoredReadyDays(v.ID); got != 7 {
		t.Errorf("items.ready_days = %d, want the main supplier's 7", got)
	}
	if len(v.Options) != 1 || len(v.Options[0].Choices) != 2 || v.Options[0].Choices[0].ExtraPrice != brl(t, 500) || v.Options[0].ID.IsZero() {
		t.Errorf("options = %+v", v.Options)
	}
}

func TestItemViews_OnlyTheOwnerGetsCosts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, h.costedInput("Banco"))
	own := h.createItem(t, appcatalog.ItemInput{
		Name: "Capa", SourcingMode: catalog.ModeInHouse, PriceCents: 3000, ProductionDays: 4, ProductionCost: appcatalog.SetCost(1000),
	})
	page := mustPage(t, "", 10)

	// The subtests only read the shared harness.
	for _, role := range []shared.Role{shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			ctx := h.as(t, h.store1, role)
			for _, id := range []shared.ItemID{it.ID, own.ID} {
				v, err := h.svc.GetItem(ctx, id)
				if err != nil || v.Costs != nil {
					t.Errorf("reading an item: costs %+v, %v; want none", v.Costs, err)
				}
			}
			list, err := h.svc.ListItems(ctx, appcatalog.ItemFilter{}, page)
			if err != nil || len(list.Items) != 2 {
				t.Fatalf("listing: %+v, %v", list, err)
			}
			for _, row := range list.Items {
				if row.Costs != nil {
					t.Errorf("listing %s: costs %+v; want none", row.Name, row.Costs)
				}
			}
		})
	}
	t.Run("owner", func(t *testing.T) {
		t.Parallel()
		h.ownerSeesItemCosts(t, own.ID, page)
	})
}

func (h *harness) ownerSeesItemCosts(t *testing.T, own shared.ItemID, page shared.PageRequest) {
	t.Helper()
	v, _ := h.svc.GetItem(h.owner(t), own)
	if c := v.Costs; c == nil || !sameMoney(c.ProductionCost, ptr(brl(t, 1000))) || !sameMoney(c.Cost, ptr(brl(t, 1000))) || c.MainSupplierCost != nil {
		t.Errorf("the owner's in-house costs = %+v", v.Costs)
	}
	list, _ := h.svc.ListItems(h.owner(t), appcatalog.ItemFilter{}, page)
	if c := list.Items[0].Costs; c == nil || !sameMoney(c.Cost, ptr(brl(t, 4000))) || c.Margin == nil {
		t.Errorf("the owner's row costs = %+v", list.Items[0].Costs)
	}
	if c := list.Items[1].Costs; c == nil || !sameMoney(c.Cost, ptr(brl(t, 1000))) || c.Margin.BasisPoints != 6666 {
		t.Errorf("the owner's in-house row = %+v", list.Items[1].Costs)
	}
}

func TestItemWrites_ACostFromANonOwnerIsForbidden(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, h.costedInput("Banco"))
	before := len(h.tx.Scopes())
	sends := map[string]func(*appcatalog.ItemInput){
		"a main supplier cost":   func(in *appcatalog.ItemInput) { in.MainSupplier.Cost = appcatalog.SetCost(1) },
		"a null main cost":       func(in *appcatalog.ItemInput) { in.MainSupplier.Cost = appcatalog.ClearCost() },
		"a null backup cost":     func(in *appcatalog.ItemInput) { in.BackupSupplier.Cost = appcatalog.ClearCost() },
		"a null production cost": func(in *appcatalog.ItemInput) { in.ProductionCost = appcatalog.ClearCost() },
	}
	// The subtests share the harness and count its transactions, so they
	// run one after another.
	for name, send := range sends {
		t.Run(name, func(t *testing.T) {
			in := h.suppliedInput("Banco")
			send(&in)
			if !in.ProductionCost.Sent() && !in.MainSupplier.Cost.Sent() && !in.BackupSupplier.Cost.Sent() {
				t.Fatal("the case sends no cost")
			}
			if _, err := h.svc.CreateItem(h.admin(t), in); !errors.Is(err, shared.ErrForbidden) || !errors.Is(err, appcatalog.ErrCostForbidden) {
				t.Errorf("admin creating: %v, want ErrCostForbidden", err)
			}
			if _, err := h.svc.ReplaceItem(h.admin(t), appcatalog.ReplaceItemInput{ID: it.ID, Item: in, Version: 1}); !errors.Is(err, appcatalog.ErrCostForbidden) {
				t.Errorf("admin replacing: %v, want ErrCostForbidden", err)
			}
		})
	}
	if got := len(h.tx.Scopes()); got != before {
		t.Errorf("%d transactions started; a refused cost starts none", got-before)
	}
	if c := h.ownerCosts(t, it.ID); !sameMoney(c.MainSupplierCost, ptr(brl(t, 4000))) {
		t.Errorf("after the refusals the cost is %v", c.MainSupplierCost)
	}
}

func TestReplaceItem_ACostLeftOutIsKept(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, h.costedInput("Banco"))
	in := h.suppliedInput("Banco renomeado")
	in.PriceCents = 20000
	in.MainSupplier.Cost = appcatalog.KeepCost() // what a handler maps an absent key to

	v := h.replaceItem(h.admin(t), t, it.ID, 1, in)

	if v.Costs != nil || v.Version != 2 || v.Name != "Banco renomeado" {
		t.Errorf("the admin's view = %+v", v)
	}
	c := h.ownerCosts(t, it.ID)
	if !sameMoney(c.MainSupplierCost, ptr(brl(t, 4000))) || !sameMoney(c.BackupSupplierCost, ptr(brl(t, 4500))) || c.Margin.Cents != 16000 {
		t.Errorf("costs after the admin's save = %+v; want both kept, margin on the new price", c)
	}
	cleared := h.suppliedInput("Banco")
	cleared.MainSupplier.Cost = appcatalog.ClearCost()
	if v := h.replaceItem(h.owner(t), t, it.ID, 2, cleared); v.Costs.MainSupplierCost != nil || v.Costs.Margin != nil ||
		!sameMoney(v.Costs.BackupSupplierCost, ptr(brl(t, 4500))) {
		t.Errorf("the owner clearing the main cost = %+v", v.Costs)
	}
}

func TestReplaceItem_ACostFollowsItsSupplier(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, h.costedInput("Banco"))
	admin := h.admin(t)
	sourcing := func(main, backup shared.SupplierID) appcatalog.ItemInput {
		in := h.suppliedInput("Banco")
		in.MainSupplier.SupplierID = main
		in.BackupSupplier = nil
		if !backup.IsZero() {
			in.BackupSupplier = &appcatalog.SupplierInput{SupplierID: backup, LeadDays: 9}
		}
		return in
	}

	h.replaceItem(admin, t, it.ID, 1, sourcing(h.ferragens, h.couros))
	c := h.ownerCosts(t, it.ID)
	if !sameMoney(c.MainSupplierCost, ptr(brl(t, 4500))) || !sameMoney(c.BackupSupplierCost, ptr(brl(t, 4000))) {
		t.Errorf("after swapping main and backup: %+v; want each supplier's own cost", c)
	}
	h.replaceItem(admin, t, it.ID, 2, sourcing(h.ferragens, h.tecidos))
	if c := h.ownerCosts(t, it.ID); c.BackupSupplierCost != nil {
		t.Errorf("a newly linked backup has cost %v, want none", c.BackupSupplierCost)
	}
	h.replaceItem(admin, t, it.ID, 3, sourcing(h.ferragens, h.couros))
	if c := h.ownerCosts(t, it.ID); c.BackupSupplierCost != nil {
		t.Errorf("couros linked again has cost %v; unlinking dropped it", c.BackupSupplierCost)
	}
	if main, backup := h.store.StoredLinks(it.ID); main.SupplierID() != h.ferragens || backup.SupplierID() != h.couros {
		t.Errorf("stored links = %v, %v", main, backup)
	}
}

func TestReplaceItem_LeavingInHouseDropsTheProductionCost(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	in := inHouseInput("Capa", 3000, 4)
	in.ProductionCost = appcatalog.SetCost(1000)
	it := h.createItem(t, in)

	v := h.replaceItem(h.admin(t), t, it.ID, 1, h.suppliedInput("Capa"))
	if v.ReadyDays != 7 || h.store.StoredReadyDays(it.ID) != 7 {
		t.Errorf("ready days = %d (stored %d), want the supplier's 7", v.ReadyDays, h.store.StoredReadyDays(it.ID))
	}
	back := h.replaceItem(h.admin(t), t, it.ID, 2, inHouseInput("Capa", 3000, 6))
	if back.ProductionDays != 6 || back.MainSupplier != nil || h.store.StoredReadyDays(it.ID) != 6 {
		t.Errorf("back in house = %+v", back)
	}
	if c := h.ownerCosts(t, it.ID); c.ProductionCost != nil || c.Cost != nil || c.Margin != nil {
		t.Errorf("back in house the costs are %+v, want none", c)
	}
}

func TestItemWrites_FieldErrorsTogether(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.defaults.Set(h.store1, appcatalog.Defaults{Currency: "BRL", AssemblyDays: 3})
	in := h.suppliedInput("")
	in.SourcingMode = catalog.ModeStock
	in.MainSupplier.SupplierID = h.alheio
	in.BackupSupplier.SupplierID = shared.SupplierID(h.ids.NewID())
	in.CategoryID = shared.CategoryID(h.ids.NewID())
	in.AttributeValueIDs = []shared.AttributeValueID{shared.AttributeValueID(h.ids.NewID())}

	_, err := h.svc.CreateItem(h.admin(t), in)

	want := []string{"sourcingMode", "mainSupplier.supplierId", "backupSupplier.supplierId", "categoryId", "attributeValueIds", "name"}
	if f := fieldsOf(t, err); !slices.Equal(f, want) {
		t.Errorf("fields = %v, want %v", f, want)
	}
	calls := h.suppliers.Calls()
	_, err = h.svc.CreateItem(h.admin(t), inHouseInput("Capa", 100, 3))
	if f := fieldsOf(t, err); !slices.Equal(f, []string{"sourcingMode"}) {
		t.Errorf("in_house with production off: fields %v", f)
	}
	if h.suppliers.Calls() != calls {
		t.Error("an item with no supplier asked the directory for names")
	}
	missing := h.suppliedInput("Banco")
	missing.MainSupplier = nil
	missing.SourcingMode = catalog.ModeToOrder
	_, err = h.svc.CreateItem(h.admin(t), missing)
	if f := fieldsOf(t, err); !slices.Equal(f, []string{"mainSupplier.supplierId"}) {
		t.Errorf("no main supplier: fields %v", f)
	}
	if p, _ := h.svc.ListItems(h.viewer(t), appcatalog.ItemFilter{IncludeArchived: true}, mustPage(t, "", 10)); len(p.Items) != 0 {
		t.Errorf("refused items were stored: %+v", p.Items)
	}
}

func TestItemWrites_KnownCategoryAndValuesPass(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	cat := h.categories(t, "Bancos")[0]
	attr := h.attribute(t, "Serve em", "CB 500X")
	in := h.suppliedInput("Banco")
	in.CategoryID, in.AttributeValueIDs, in.Published = cat.ID, []shared.AttributeValueID{attr.Values[0].ID}, true

	v := h.createItem(t, in)

	if v.CategoryID != cat.ID || !slices.Equal(v.AttributeValueIDs, in.AttributeValueIDs) || !v.Published {
		t.Errorf("view = %+v", v)
	}
}

func TestReplaceItem_KeepsOptionAndChoiceIDs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, h.suppliedInput("Banco"))
	opt := it.Options[0]
	in := h.suppliedInput("Banco")
	in.Options = []appcatalog.OptionInput{
		{ID: opt.ID, Name: "Costura", Choices: []appcatalog.ChoiceInput{{ID: opt.Choices[1].ID, Label: "Preta"}, {ID: opt.Choices[1].ID, Label: "Branca"}}},
		{ID: opt.ID, Name: "Cópia", Choices: []appcatalog.ChoiceInput{{ID: opt.Choices[0].ID, Label: "Azul"}}},
	}

	v := h.replaceItem(h.admin(t), t, it.ID, 1, in)

	if v.Options[0].ID != opt.ID || v.Options[0].Choices[0].ID != opt.Choices[1].ID {
		t.Errorf("kept option = %+v; want the ids of the item's option and choice", v.Options[0])
	}
	if v.Options[0].Choices[1].ID == opt.Choices[1].ID || v.Options[1].ID == opt.ID || v.Options[1].Choices[0].ID == opt.Choices[0].ID {
		t.Errorf("repeated ids = %+v; a second use of an id is new", v.Options)
	}
}

func TestReplaceItem_Refusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, h.suppliedInput("Banco"))
	h.replaceItem(h.admin(t), t, it.ID, 1, h.suppliedInput("Banco 2"))
	invalid := h.suppliedInput("")

	if _, err := h.svc.ReplaceItem(h.admin(t), appcatalog.ReplaceItemInput{ID: it.ID, Item: invalid, Version: 1}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version with invalid values: %v, want ErrVersionConflict first", err)
	}
	if _, err := h.svc.ReplaceItem(h.admin(t), appcatalog.ReplaceItemInput{ID: shared.ItemID(h.ids.NewID()), Item: invalid, Version: 1}); !errors.Is(err, catalog.ErrItemNotFound) {
		t.Errorf("an unknown item: %v, want ErrItemNotFound", err)
	}
	other := h.as(t, h.store2, shared.RoleOwner)
	if _, err := h.svc.GetItem(other, it.ID); !errors.Is(err, catalog.ErrItemNotFound) {
		t.Errorf("another store's item: %v, want ErrItemNotFound", err)
	}
	_, err := h.svc.ReplaceItem(h.admin(t), appcatalog.ReplaceItemInput{ID: it.ID, Item: invalid, Version: 2})
	if f := fieldsOf(t, err); !slices.Equal(f, []string{"name"}) {
		t.Errorf("invalid values: fields %v", f)
	}
	if v, _ := h.svc.GetItem(h.viewer(t), it.ID); v.Name != "Banco 2" || v.Version != 2 {
		t.Errorf("after the refusals: %+v", v)
	}
}

func TestArchiveItem_RefusedWhileAKitHoldsIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	seat := h.createItem(t, inHouseInput("Banco", 10000, 5))
	alt := h.createItem(t, inHouseInput("Banco alto", 12000, 5))
	wheel := h.createItem(t, inHouseInput("Roda", 5000, 3))
	zulu := h.createKit(t, kitOf("Zulu", 9000, one(seat.ID)))
	alfa := h.createKit(t, kitOf("Alfa", 9000, one(wheel.ID), catalog.KitItem{Item: alt.ID, Quantity: 1, Swappable: true, Alternatives: []shared.ItemID{seat.ID}}))

	_, err := h.svc.ArchiveItem(h.admin(t), appcatalog.ItemVersionInput{ID: seat.ID, Version: 1})

	var refusal *appcatalog.RefusalError
	if !errors.As(err, &refusal) || !errors.Is(err, catalog.ErrItemInKit) || !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("archive = %v, want a RefusalError matching ErrItemInKit", err)
	}
	want := []appcatalog.Reference{{ID: shared.ID(alfa.ID), Name: "Alfa"}, {ID: shared.ID(zulu.ID), Name: "Zulu"}}
	if !slices.Equal(refusal.References, want) || refusal.Error() == "" {
		t.Errorf("references = %+v, want %+v", refusal.References, want)
	}
	for _, k := range []appcatalog.KitView{alfa, zulu} {
		if _, err := h.svc.ArchiveKit(h.admin(t), appcatalog.KitVersionInput{ID: k.ID, Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	v, err := h.svc.ArchiveItem(h.admin(t), appcatalog.ItemVersionInput{ID: seat.ID, Version: 1})
	if err != nil || !v.Archived || v.Published || v.Version != 2 {
		t.Errorf("archive once the kits are archived = %+v, %v", v, err)
	}
	r, err := h.svc.RestoreItem(h.admin(t), appcatalog.ItemVersionInput{ID: seat.ID, Version: 2})
	if err != nil || r.Archived || r.Published || r.Version != 3 {
		t.Errorf("restore = %+v, %v; want it back, unpublished, at version 3", r, err)
	}
	if _, err := h.svc.RestoreItem(h.admin(t), appcatalog.ItemVersionInput{ID: seat.ID, Version: 2}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale restore: %v, want ErrVersionConflict", err)
	}
}

func TestSetItemPublished(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))

	v, err := h.svc.SetItemPublished(h.admin(t), appcatalog.SetItemPublishedInput{ID: it.ID, Published: true, Version: 1})
	if err != nil || !v.Published || v.Version != 2 {
		t.Fatalf("publish = %+v, %v", v, err)
	}
	if _, err := h.svc.ArchiveItem(h.admin(t), appcatalog.ItemVersionInput{ID: it.ID, Version: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SetItemPublished(h.admin(t), appcatalog.SetItemPublishedInput{ID: it.ID, Published: true, Version: 3}); !errors.Is(err, catalog.ErrPublishArchived) {
		t.Errorf("publishing an archived item: %v, want ErrPublishArchived", err)
	}
	in := inHouseInput("Banco", 10000, 5)
	in.Published = true
	if _, err := h.svc.ReplaceItem(h.admin(t), appcatalog.ReplaceItemInput{ID: it.ID, Item: in, Version: 3}); !errors.Is(err, catalog.ErrPublishArchived) {
		t.Errorf("replacing an archived item as published: %v, want ErrPublishArchived", err)
	}
	if v, err := h.svc.SetItemPublished(h.admin(t), appcatalog.SetItemPublishedInput{ID: it.ID, Version: 3}); err != nil || v.Published {
		t.Errorf("unpublishing an archived item = %+v, %v", v, err)
	}
}

func TestListItems_RowsWithSupplierNamesAndCovers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	cat := h.categories(t, "Bancos")[0]
	in := h.suppliedInput("Banco")
	in.CategoryID = cat.ID
	seat := h.createItem(t, in)
	h.createItem(t, inHouseInput("Capa", 3000, 4))
	h.createItem(t, inHouseInput("Alça", 2000, 4))
	photo, err := catalog.NewPhoto(shared.PhotoID(h.ids.NewID()), "t/x/items/y/z", catalog.ContentTypeJPEG, 100)
	if err != nil {
		t.Fatal(err)
	}
	h.store.SetItemPhotos(seat.ID, []catalog.Photo{photo})

	first, err := h.svc.ListItems(h.viewer(t), appcatalog.ItemFilter{Query: "  "}, mustPage(t, "", 2))
	if err != nil || len(first.Items) != 2 || first.Items[0].Name != "Alça" || first.Items[1].Name != "Banco" || !first.HasMore() {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	row := first.Items[1]
	if row.MainSupplierName != "Couros Paulista" || row.ReadyDays != 7 || row.Cover == nil || row.Cover.URL != "https://photos.test/t/x/items/y/z" ||
		!row.Cover.ExpiresAt.Equal(start.Add(10*60e9)) || row.CategoryID != cat.ID {
		t.Errorf("the row = %+v", row)
	}
	if first.Items[0].MainSupplierName != "" || first.Items[0].Cover != nil {
		t.Errorf("an in-house row = %+v", first.Items[0])
	}
	rest, err := h.svc.ListItems(h.viewer(t), appcatalog.ItemFilter{}, mustPage(t, first.NextCursor, 2))
	if err != nil || len(rest.Items) != 1 || rest.Items[0].Name != "Capa" || rest.HasMore() {
		t.Errorf("last page = %+v, %v", rest, err)
	}
	byCat, err := h.svc.ListItems(h.viewer(t), appcatalog.ItemFilter{Category: cat.ID}, mustPage(t, "", 10))
	if err != nil || len(byCat.Items) != 1 {
		t.Errorf("by category = %+v, %v", byCat, err)
	}
	if _, err := h.svc.ListItems(h.viewer(t), appcatalog.ItemFilter{}, mustPage(t, "nope", 2)); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("a bad cursor: %v, want ErrInvalid", err)
	}
}

func TestItemPhotos_SignedAndNeverMoveTheVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	var photos []catalog.Photo
	for _, key := range []string{"t/a/items/b/1", "t/a/items/b/2"} {
		p, err := catalog.NewPhoto(shared.PhotoID(h.ids.NewID()), key, catalog.ContentTypePNG, 10)
		if err != nil {
			t.Fatal(err)
		}
		photos = append(photos, p)
	}
	h.store.SetItemPhotos(it.ID, photos)

	v := h.replaceItem(h.admin(t), t, it.ID, 1, inHouseInput("Banco", 11000, 5))

	if len(v.Photos) != 2 || v.Photos[1].URL != "https://photos.test/t/a/items/b/2" || v.Photos[1].Position != 1 || v.Photos[0].ID != photos[0].ID() {
		t.Errorf("photos = %+v; want both, signed, in order", v.Photos)
	}
	h.storage.Fail("SignGet", errBoom)
	if _, err := h.svc.GetItem(h.viewer(t), it.ID); !errors.Is(err, errBoom) {
		t.Errorf("a failed signature: %v", err)
	}
	if _, err := h.svc.ListItems(h.viewer(t), appcatalog.ItemFilter{}, mustPage(t, "", 10)); !errors.Is(err, errBoom) {
		t.Errorf("a failed cover signature: %v", err)
	}
}

func TestItemUseCases_FailuresPassThrough(t *testing.T) {
	t.Parallel()
	type op func(t *testing.T, h *harness, it appcatalog.ItemView) error
	create := func(t *testing.T, h *harness, _ appcatalog.ItemView) error {
		_, err := h.svc.CreateItem(h.owner(t), h.refsInput())
		return err
	}
	replace := func(t *testing.T, h *harness, it appcatalog.ItemView) error {
		_, err := h.svc.ReplaceItem(h.owner(t), appcatalog.ReplaceItemInput{ID: it.ID, Item: h.refsInput(), Version: 1})
		return err
	}
	archive := func(t *testing.T, h *harness, it appcatalog.ItemView) error {
		_, err := h.svc.ArchiveItem(h.owner(t), appcatalog.ItemVersionInput{ID: it.ID, Version: 1})
		return err
	}
	list := func(t *testing.T, h *harness, _ appcatalog.ItemView) error {
		_, err := h.svc.ListItems(h.viewer(t), appcatalog.ItemFilter{}, mustPage(t, "", 10))
		return err
	}
	get := func(t *testing.T, h *harness, it appcatalog.ItemView) error {
		_, err := h.svc.GetItem(h.viewer(t), it.ID)
		return err
	}
	cases := []struct {
		name string
		fail func(h *harness)
		ops  []op
	}{
		{"suppliers", func(h *harness) { h.suppliers.Fail(errBoom) }, []op{create, replace, list, get}},
		{"defaults", func(h *harness) { h.defaults.Fail(errBoom) }, []op{create, replace}},
		{"category", func(h *harness) { h.store.Fail("CategoriesByID", errBoom) }, []op{create, replace}},
		{"values", func(h *harness) { h.store.Fail("AttributesList", errBoom) }, []op{create, replace}},
		{"insert", func(h *harness) { h.store.Fail("ItemsInsert", errBoom) }, []op{create}},
		{"update", func(h *harness) { h.store.Fail("ItemsUpdate", errBoom) }, []op{replace, archive}},
		{"load", func(h *harness) { h.store.Fail("ItemsByID", errBoom) }, []op{replace, archive, get}},
		{"kits", func(h *harness) { h.store.Fail("ItemsActiveKitsHolding", errBoom) }, []op{archive}},
		{"list", func(h *harness) { h.store.Fail("ItemsList", errBoom) }, []op{list}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			cat := h.categories(t, "Bancos")[0]
			attr := h.attribute(t, "Serve em", "CB 500X")
			h.refs = func(in *appcatalog.ItemInput) {
				in.CategoryID, in.AttributeValueIDs = cat.ID, []shared.AttributeValueID{attr.Values[0].ID}
			}
			it := h.createItem(t, h.refsInput())
			c.fail(h)
			for i, run := range c.ops {
				if err := run(t, h, it); !errors.Is(err, errBoom) {
					t.Errorf("operation %d: %v; want it passed through", i, err)
				}
			}
		})
	}
}
