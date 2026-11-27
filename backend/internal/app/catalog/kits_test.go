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

// scrambler is the Kit Scrambler CB 500X: a leather seat made in 15 days
// (the slowest) that may be swapped for a tall seat, and two wheels from a
// supplier at 7 days.
type scrambler struct {
	seat, tall, wheel appcatalog.ItemView
	kit               appcatalog.KitView
}

func (h *harness) scrambler(t *testing.T) scrambler {
	t.Helper()
	seatIn := inHouseInput("Banco de couro", 50000, 15)
	seatIn.ProductionCost = appcatalog.SetCost(20000)
	wheelIn := h.suppliedInput("Roda raiada")
	wheelIn.MainSupplier.Cost = appcatalog.SetCost(4000)
	wheelIn.PriceCents = 10000
	s := scrambler{
		seat: h.createItem(t, seatIn), tall: h.createItem(t, inHouseInput("Banco alto", 60000, 10)), wheel: h.createItem(t, wheelIn),
	}
	s.kit = h.createKit(t, kitOf("Kit Scrambler CB 500X", 65000,
		catalog.KitItem{Item: s.seat.ID, Quantity: 1, Swappable: true, Alternatives: []shared.ItemID{s.tall.ID}},
		catalog.KitItem{Item: s.wheel.ID, Quantity: 2},
	))
	return s
}

func TestCreateKit_TotalsDaysAndTheOwnersCost(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	s := h.scrambler(t)
	v := s.kit

	if v.SingleItemsTotal != brl(t, 70000) || v.Discount != brl(t, 5000) || v.SwapDiscount != catalog.SwapKeepDiscount || v.Version != 1 {
		t.Errorf("totals = %+v", v)
	}
	if v.Days != (catalog.KitDays{SlowestItem: 15, Assembly: 3, SafetyMargin: 2, Total: 20}) || v.AssemblyDays != nil || v.SafetyMarginDays != nil {
		t.Errorf("days = %+v, overrides %v %v; want 15 + 3 + 2 = 20 from the store's defaults", v.Days, v.AssemblyDays, v.SafetyMarginDays)
	}
	want := []appcatalog.KitItemView{
		{ItemID: s.seat.ID, ItemName: "Banco de couro", Quantity: 1, Swappable: true, AlternativeItemIDs: []shared.ItemID{s.tall.ID}, UnitPrice: brl(t, 50000), ReadyDays: 15},
		{ItemID: s.wheel.ID, ItemName: "Roda raiada", Quantity: 2, AlternativeItemIDs: []shared.ItemID{}, UnitPrice: brl(t, 10000), ReadyDays: 7},
	}
	if len(v.Items) != 2 || !sameKitItem(v.Items[0], want[0]) || !sameKitItem(v.Items[1], want[1]) {
		t.Errorf("items = %+v, want %+v", v.Items, want)
	}
	if v.Costs == nil || !sameMoney(v.Costs.Cost, ptr(brl(t, 28000))) || *v.Costs.Margin != (catalog.Margin{Cents: 37000, BasisPoints: 5692}) {
		t.Errorf("the owner's costs = %+v", v.Costs)
	}
}

func sameKitItem(a, b appcatalog.KitItemView) bool {
	return a.ItemID == b.ItemID && a.ItemName == b.ItemName && a.Quantity == b.Quantity && a.Swappable == b.Swappable &&
		slices.Equal(a.AlternativeItemIDs, b.AlternativeItemIDs) && a.UnitPrice == b.UnitPrice && a.ReadyDays == b.ReadyDays && a.Archived == b.Archived
}

func TestKitViews_OnlyTheOwnerGetsCosts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	s := h.scrambler(t)
	for role, ctx := range map[shared.Role]context.Context{shared.RoleAdmin: h.admin(t), shared.RoleStaff: h.staff(t), shared.RoleViewer: h.viewer(t)} {
		v, err := h.svc.GetKit(ctx, s.kit.ID)
		if err != nil || v.Costs != nil || v.SingleItemsTotal != brl(t, 70000) {
			t.Errorf("%s reading the kit: %+v, %v; want totals and no costs", role, v.Costs, err)
		}
		list, err := h.svc.ListKits(ctx, appcatalog.KitFilter{}, mustPage(t, "", 10))
		if err != nil || len(list.Items) != 1 || list.Items[0].Costs != nil {
			t.Errorf("%s listing kits: %+v, %v", role, list, err)
		}
	}
	free := h.createItem(t, inHouseInput("Capa", 3000, 2))
	k := h.createKit(t, kitOf("Sem custo", 9000, one(s.seat.ID), one(free.ID)))
	if k.Costs == nil || k.Costs.Cost != nil || k.Costs.Margin != nil {
		t.Errorf("a kit with an item without a cost: %+v; want costs present with no value", k.Costs)
	}
}

func TestCreateKit_RefusesArchivedItemsNamingThem(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	seat := h.createItem(t, inHouseInput("Banco", 10000, 5))
	old := h.createItem(t, inHouseInput("Banco antigo", 10000, 5))
	if _, err := h.svc.ArchiveItem(h.admin(t), appcatalog.ItemVersionInput{ID: old.ID, Version: 1}); err != nil {
		t.Fatal(err)
	}

	_, err := h.svc.CreateKit(h.admin(t), kitOf("Kit", 9000, catalog.KitItem{Item: seat.ID, Quantity: 1, Swappable: true, Alternatives: []shared.ItemID{old.ID}}))

	var refusal *appcatalog.RefusalError
	if !errors.As(err, &refusal) || !errors.Is(err, catalog.ErrItemArchived) || !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("create = %v, want a RefusalError matching ErrItemArchived", err)
	}
	if want := []appcatalog.Reference{{ID: shared.ID(old.ID), Name: "Banco antigo"}}; !slices.Equal(refusal.References, want) {
		t.Errorf("references = %+v, want %+v", refusal.References, want)
	}
}

func TestKitWrites_FieldErrors(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	theirs, err := h.svc.CreateItem(h.as(t, h.store2, shared.RoleOwner), inHouseInput("Alheio", 100, 1))
	if err != nil {
		t.Fatal(err)
	}
	in := kitOf("", 0, one(theirs.ID))
	in.AttributeValueIDs = []shared.AttributeValueID{shared.AttributeValueID(h.ids.NewID())}

	_, err = h.svc.CreateKit(h.admin(t), in)

	want := []string{"attributeValueIds", "name", "priceCents", "items[0].itemId"}
	if f := fieldsOf(t, err); !slices.Equal(f, want) {
		t.Errorf("fields = %v, want %v", f, want)
	}
}

func TestReplaceKit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	s := h.scrambler(t)
	in := kitOf("Kit Scrambler", 60000, one(s.wheel.ID), one(s.tall.ID))
	in.SwapDiscount, in.AssemblyDays, in.SafetyMarginDays, in.Published = catalog.SwapLoseDiscount, ptr(5), ptr(0), true

	v, err := h.svc.ReplaceKit(h.admin(t), appcatalog.ReplaceKitInput{ID: s.kit.ID, Kit: in, Version: 1})
	if err != nil {
		t.Fatal(err)
	}

	if v.Version != 2 || v.Name != "Kit Scrambler" || v.SwapDiscount != catalog.SwapLoseDiscount || !v.Published || v.Costs != nil {
		t.Errorf("replaced = %+v", v)
	}
	if v.Days != (catalog.KitDays{SlowestItem: 10, Assembly: 5, SafetyMargin: 0, Total: 15}) || *v.AssemblyDays != 5 || *v.SafetyMarginDays != 0 {
		t.Errorf("days = %+v; want the kit's own assembly and margin", v.Days)
	}
	if _, err := h.svc.ReplaceKit(h.admin(t), appcatalog.ReplaceKitInput{ID: s.kit.ID, Kit: kitOf("", 0), Version: 1}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version with invalid values: %v, want ErrVersionConflict", err)
	}
	if _, err := h.svc.ReplaceKit(h.admin(t), appcatalog.ReplaceKitInput{ID: shared.KitID(h.ids.NewID()), Kit: in, Version: 1}); !errors.Is(err, catalog.ErrKitNotFound) {
		t.Errorf("an unknown kit: %v, want ErrKitNotFound", err)
	}
	if _, err := h.svc.GetKit(h.as(t, h.store2, shared.RoleOwner), s.kit.ID); !errors.Is(err, catalog.ErrKitNotFound) {
		t.Errorf("another store's kit: %v, want ErrKitNotFound", err)
	}
}

func TestRestoreKit_RefusedWhileAnItemIsArchived(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	s := h.scrambler(t)
	archived, err := h.svc.ArchiveKit(h.admin(t), appcatalog.KitVersionInput{ID: s.kit.ID, Version: 1})
	if err != nil || !archived.Archived || archived.Version != 2 {
		t.Fatalf("archive = %+v, %v", archived, err)
	}
	for _, it := range []appcatalog.ItemView{s.tall, s.wheel} {
		if _, err := h.svc.ArchiveItem(h.admin(t), appcatalog.ItemVersionInput{ID: it.ID, Version: 1}); err != nil {
			t.Fatalf("archiving %s once its kit is archived: %v", it.Name, err)
		}
	}

	_, err = h.svc.RestoreKit(h.admin(t), appcatalog.KitVersionInput{ID: s.kit.ID, Version: 2})

	var refusal *appcatalog.RefusalError
	want := []appcatalog.Reference{{ID: shared.ID(s.tall.ID), Name: "Banco alto"}, {ID: shared.ID(s.wheel.ID), Name: "Roda raiada"}}
	if !errors.As(err, &refusal) || !errors.Is(err, catalog.ErrItemArchived) || !slices.Equal(refusal.References, want) {
		t.Fatalf("restore = %v (%+v), want item_archived naming %+v", err, refusal, want)
	}
	v, _ := h.svc.GetKit(h.viewer(t), s.kit.ID)
	if !v.Archived || !v.Items[1].Archived || v.Items[0].Archived {
		t.Errorf("the kit after the refusal: archived %v, items %+v", v.Archived, v.Items)
	}
	_, err = h.svc.ReplaceKit(h.admin(t), appcatalog.ReplaceKitInput{ID: s.kit.ID, Kit: kitOf("Kit", 100, one(s.wheel.ID)), Version: 2})
	if !errors.Is(err, catalog.ErrItemArchived) {
		t.Errorf("saving the kit with the archived item: %v, want ErrItemArchived", err)
	}
	for _, it := range []appcatalog.ItemView{s.tall, s.wheel} {
		if _, err := h.svc.RestoreItem(h.admin(t), appcatalog.ItemVersionInput{ID: it.ID, Version: 2}); err != nil {
			t.Fatal(err)
		}
	}
	if r, err := h.svc.RestoreKit(h.admin(t), appcatalog.KitVersionInput{ID: s.kit.ID, Version: 2}); err != nil || r.Archived || r.Published || r.Version != 3 {
		t.Errorf("restore once the items are back = %+v, %v", r, err)
	}
}

func TestSetKitPublished(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	s := h.scrambler(t)
	v, err := h.svc.SetKitPublished(h.admin(t), appcatalog.SetKitPublishedInput{ID: s.kit.ID, Published: true, Version: 1})
	if err != nil || !v.Published || v.Version != 2 {
		t.Fatalf("publish = %+v, %v", v, err)
	}
	if _, err := h.svc.ArchiveKit(h.admin(t), appcatalog.KitVersionInput{ID: s.kit.ID, Version: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SetKitPublished(h.admin(t), appcatalog.SetKitPublishedInput{ID: s.kit.ID, Published: true, Version: 3}); !errors.Is(err, catalog.ErrPublishArchived) {
		t.Errorf("publishing an archived kit: %v, want ErrPublishArchived", err)
	}
	if _, err := h.svc.SetKitPublished(h.admin(t), appcatalog.SetKitPublishedInput{ID: s.kit.ID, Version: 2}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version: %v, want ErrVersionConflict", err)
	}
}

func TestListKits_RowsWithTotalsAndCovers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	s := h.scrambler(t)
	h.createKit(t, kitOf("Alfa", 100, one(s.wheel.ID)))
	h.createKit(t, kitOf("Zulu", 100, one(s.wheel.ID)))
	photo, err := catalog.NewPhoto(shared.PhotoID(h.ids.NewID()), "t/a/kits/b/c", catalog.ContentTypeWebP, 5)
	if err != nil {
		t.Fatal(err)
	}
	h.store.SetKitPhotos(s.kit.ID, []catalog.Photo{photo})

	first, err := h.svc.ListKits(h.owner(t), appcatalog.KitFilter{Query: " "}, mustPage(t, "", 2))
	if err != nil || len(first.Items) != 2 || !first.HasMore() {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	row := first.Items[1]
	if row.Name != "Kit Scrambler CB 500X" || row.ItemCount != 2 || row.SingleItemsTotal != brl(t, 70000) || row.TotalDays != 20 ||
		row.Cover == nil || row.Cover.ID != photo.ID() || row.Costs == nil || !sameMoney(row.Costs.Cost, ptr(brl(t, 28000))) {
		t.Errorf("the row = %+v", row)
	}
	rest, err := h.svc.ListKits(h.owner(t), appcatalog.KitFilter{}, mustPage(t, first.NextCursor, 2))
	if err != nil || len(rest.Items) != 1 || rest.Items[0].Name != "Zulu" {
		t.Errorf("last page = %+v, %v", rest, err)
	}
	if v, _ := h.svc.GetKit(h.viewer(t), s.kit.ID); len(v.Photos) != 1 || v.Photos[0].URL != "https://photos.test/t/a/kits/b/c" {
		t.Errorf("kit photos = %+v", v.Photos)
	}
	h.photos.Fail(errBoom)
	if _, err := h.svc.ListKits(h.owner(t), appcatalog.KitFilter{}, mustPage(t, "", 10)); !errors.Is(err, errBoom) {
		t.Errorf("a failed cover signature: %v", err)
	}
	if _, err := h.svc.GetKit(h.owner(t), s.kit.ID); !errors.Is(err, errBoom) {
		t.Errorf("a failed photo signature: %v", err)
	}
}

func TestKitUseCases_FailuresPassThrough(t *testing.T) {
	t.Parallel()
	calls := map[string]func(h *harness, s scrambler) error{
		"list": func(h *harness, _ scrambler) error {
			_, err := h.svc.ListKits(h.viewer(t), appcatalog.KitFilter{}, mustPage(t, "", 10))
			return err
		},
		"get": func(h *harness, s scrambler) error { _, err := h.svc.GetKit(h.viewer(t), s.kit.ID); return err },
		"create": func(h *harness, s scrambler) error {
			_, err := h.svc.CreateKit(h.admin(t), kitOf("Novo", 100, one(s.wheel.ID)))
			return err
		},
		"replace": func(h *harness, s scrambler) error {
			_, err := h.svc.ReplaceKit(h.admin(t), appcatalog.ReplaceKitInput{ID: s.kit.ID, Kit: kitOf("Novo", 100, one(s.wheel.ID)), Version: 1})
			return err
		},
		"archive": func(h *harness, s scrambler) error {
			_, err := h.svc.ArchiveKit(h.admin(t), appcatalog.KitVersionInput{ID: s.kit.ID, Version: 1})
			return err
		},
	}
	faults := map[string][]string{
		"KitsList": {"list"}, "KitsByID": {"get", "replace", "archive"}, "KitsInsert": {"create"}, "KitsUpdate": {"replace", "archive"},
		"ItemsByIDs": {"list", "get", "create", "replace", "archive"}, "defaults": {"list", "get", "create", "replace", "archive"},
	}
	for fault, names := range faults {
		h := newHarness(t)
		s := h.scrambler(t)
		if fault == "defaults" {
			h.defaults.Fail(errBoom)
		} else {
			h.store.Fail(fault, errBoom)
		}
		for _, name := range names {
			if err := calls[name](h, s); !errors.Is(err, errBoom) {
				t.Errorf("%s failing, %s: %v; want it passed through", fault, name, err)
			}
		}
	}
}
