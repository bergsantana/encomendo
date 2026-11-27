package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// storeFixture is a catalog with every kind of row the store page could
// show or must hide.
type storeFixture struct {
	scrambler
	draft, archived appcatalog.ItemView
	hiddenKit       appcatalog.KitView
	category        appcatalog.CategoryView
	attribute       appcatalog.AttributeView
}

func (h *harness) storeFixture(t *testing.T) storeFixture {
	t.Helper()
	f := storeFixture{category: h.categories(t, "Bancos")[0], attribute: h.attribute(t, "Serve em", "CB 500X")}
	f.scrambler = h.scrambler(t)
	owner := h.owner(t)
	for _, it := range []appcatalog.ItemView{f.seat, f.wheel} {
		if _, err := h.svc.SetItemPublished(owner, appcatalog.SetItemPublishedInput{ID: it.ID, Published: true, Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.svc.SetKitPublished(owner, appcatalog.SetKitPublishedInput{ID: f.kit.ID, Published: true, Version: 1}); err != nil {
		t.Fatal(err)
	}
	f.draft = h.createItem(t, h.costedInput("Rascunho"))
	f.archived = h.createItem(t, inHouseInput("Antigo", 100, 1))
	f.hiddenKit = h.createKit(t, kitOf("Kit rascunho", 100, one(f.wheel.ID)))
	if _, err := h.svc.ArchiveItem(owner, appcatalog.ItemVersionInput{ID: f.archived.ID, Version: 1}); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestPublicCatalog_PublishedRowsWithoutInternalData(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	f := h.storeFixture(t)

	c, err := h.svc.PublicCatalog(h.storePage(h.store1))
	if err != nil {
		t.Fatal(err)
	}

	if len(c.Categories) != 1 || c.Categories[0].Name != "Bancos" || len(c.Attributes) != 1 || c.Attributes[0].Values[0].Label != "CB 500X" {
		t.Errorf("categories and fields = %+v, %+v", c.Categories, c.Attributes)
	}
	if len(c.Items) != 2 || c.Items[0].ID != f.seat.ID || c.Items[1].ID != f.wheel.ID {
		t.Fatalf("items = %+v; want only the two published ones, by name", c.Items)
	}
	wheel := c.Items[1]
	if wheel.ReadyDays != 7 || wheel.Price != brl(t, 10000) || len(wheel.Options) != 1 || wheel.Options[0].Choices[0].ExtraPrice != brl(t, 500) {
		t.Errorf("the wheel = %+v", wheel)
	}
	if len(c.Kits) != 1 {
		t.Fatalf("kits = %+v; want only the published kit", c.Kits)
	}
	k := c.Kits[0]
	if k.ID != f.kit.ID || k.TotalDays != 20 || k.SingleItemsTotal != brl(t, 70000) || k.Discount != brl(t, 5000) || k.SwapDiscount != catalog.SwapKeepDiscount {
		t.Errorf("the kit = %+v", k)
	}
	seat := k.Items[0]
	if seat.ItemName != "Banco de couro" || !seat.Swappable || len(seat.Alternatives) != 1 || seat.Alternatives[0].UnitPrice != brl(t, 60000) || k.Items[1].Quantity != 2 {
		t.Errorf("the kit's items = %+v", k.Items)
	}
}

// forbiddenKeys are the JSON keys no store page value may hold, at any
// depth (docs/product/design-document.md, System rules; ADR 0025).
var forbiddenKeys = []string{
	"cost", "margin", "productionCost", "supplier", "supplierName", "supplierId", "leadDays", "mainSupplier", "backupSupplier", "atRisk",
}

// walkKeys calls fn with every object key in v, at any depth.
func walkKeys(v any, path string, fn func(key, path string)) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			fn(k, path+"."+k)
			walkKeys(child, path+"."+k, fn)
		}
	case []any:
		for _, child := range x {
			walkKeys(child, path+"[]", fn)
		}
	}
}

func assertNoInternalKeys(t *testing.T, name string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	seen := 0
	walkKeys(doc, name, func(key, path string) {
		seen++
		for _, bad := range forbiddenKeys {
			if strings.EqualFold(key, bad) {
				t.Errorf("%s holds the internal key %q", path, key)
			}
		}
	})
	if seen == 0 {
		t.Errorf("%s has no keys; the walk proves nothing", name)
	}
}

func TestStoreTypes_NoInternalKeyAtAnyDepth(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.storeFixture(t)
	for name, ctx := range map[string]context.Context{"the store page": h.storePage(h.store1), "the owner": h.owner(t)} {
		c, err := h.svc.PublicCatalog(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(c.Items) == 0 || len(c.Kits) == 0 || len(c.Kits[0].Items[0].Alternatives) == 0 || len(c.Items[1].Options) == 0 {
			t.Fatalf("%s: the catalog is too thin to walk: %+v", name, c)
		}
		assertNoInternalKeys(t, name+": StoreCatalog", c)
		assertNoInternalKeys(t, name+": StoreItem", c.Items[1])
		assertNoInternalKeys(t, name+": StoreKit", c.Kits[0])
		h.cache.SetDown(true) // the owner's call reads Postgres, in a staff scope that sees costs
	}
	// The walk itself catches a forbidden key, nested.
	probe := &testing.T{}
	assertNoInternalKeys(probe, "probe", map[string]any{"items": []any{map[string]any{"mainSupplier": map[string]any{"leadDays": 3}}}})
	if !probe.Failed() {
		t.Error("the walk missed a nested internal key")
	}
}

func TestPublicCatalog_IsCached(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.storeFixture(t)
	ctx := h.storePage(h.store1)
	first, err := h.svc.PublicCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !h.cached(t, h.store1) {
		t.Fatal("the catalog was not cached under t:{tenant}:catalog:store")
	}
	before := len(h.tx.Scopes())

	again, err := h.svc.PublicCatalog(ctx)

	if err != nil || len(again.Items) != len(first.Items) || again.Kits[0].TotalDays != 20 {
		t.Errorf("the cached catalog = %+v, %v", again, err)
	}
	if len(h.tx.Scopes()) != before {
		t.Error("a cached read opened a transaction")
	}
	k, err := h.svc.PublicKit(ctx, first.Kits[0].ID)
	if err != nil || k.Name != "Kit Scrambler CB 500X" {
		t.Errorf("PublicKit = %+v, %v", k, err)
	}
	h.clock.Advance(5*time.Minute + time.Second)
	if h.cached(t, h.store1) {
		t.Error("the entry outlived its 5 minutes")
	}
	if h.cached(t, h.store2) {
		t.Error("another store has an entry it never read")
	}
}

func TestPublicCatalog_ACacheThatFailsFallsBackToTheRepositories(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	f := h.storeFixture(t)
	h.cache.SetDown(true)

	c, err := h.svc.PublicCatalog(h.storePage(h.store1))

	if err != nil || len(c.Items) != 2 || len(c.Kits) != 1 {
		t.Errorf("with the cache down = %+v, %v; want the catalog from the repositories", c, err)
	}
	if _, err := h.svc.ReplaceItem(h.admin(t), appcatalog.ReplaceItemInput{ID: f.draft.ID, Item: h.suppliedInput("Rascunho 2"), Version: 1}); err != nil {
		t.Errorf("a write with the cache down: %v, want it to succeed", err)
	}
	h.cache.SetDown(false)
	if h.cached(t, h.store1) {
		t.Error("the down cache stored the catalog")
	}
}

func TestPublicCatalog_Refusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	f := h.storeFixture(t)
	clientArea := appshared.WithScope(context.Background(), appshared.Scope{ClientAccountID: shared.ClientAccountID(h.ids.NewID())})
	for name, ctx := range map[string]context.Context{"no scope": context.Background(), "the client area": clientArea} {
		if _, err := h.svc.PublicCatalog(ctx); !errors.Is(err, authz.ErrTenantRequired) {
			t.Errorf("%s: %v, want ErrTenantRequired", name, err)
		}
		if _, err := h.svc.PublicKit(ctx, f.kit.ID); !errors.Is(err, authz.ErrTenantRequired) {
			t.Errorf("PublicKit, %s: %v", name, err)
		}
	}
	for name, id := range map[string]shared.KitID{"an unpublished kit": f.hiddenKit.ID, "an unknown kit": shared.KitID(h.ids.NewID())} {
		if _, err := h.svc.PublicKit(h.storePage(h.store1), id); !errors.Is(err, catalog.ErrKitNotFound) {
			t.Errorf("%s: %v, want ErrKitNotFound", name, err)
		}
	}
	if _, err := h.svc.PublicKit(h.storePage(h.store2), f.kit.ID); !errors.Is(err, catalog.ErrKitNotFound) {
		t.Errorf("another store's kit: %v, want ErrKitNotFound", err)
	}
	h.cache.SetDown(true) // every read below goes to the repositories
	for _, fault := range []string{"CategoriesList", "AttributesList", "ItemsPublished", "KitsPublished", "ItemsByIDs"} {
		h.store.Fail(fault, errBoom)
		if _, err := h.svc.PublicCatalog(h.storePage(h.store1)); !errors.Is(err, errBoom) {
			t.Errorf("%s failing: %v", fault, err)
		}
		h.store.Fail(fault, nil)
	}
	h.defaults.Fail(errBoom)
	if _, err := h.svc.PublicCatalog(h.storePage(h.store1)); !errors.Is(err, errBoom) {
		t.Errorf("defaults failing: %v", err)
	}
}

func TestPublicCatalog_ArchivedAlternativesAreLeftOut(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	f := h.storeFixture(t)
	// The use cases never archive an item a live kit offers; a concurrent
	// archive can, so the store page must not offer it.
	h.store.ForceArchived(f.tall.ID)

	c, err := h.svc.PublicCatalog(h.storePage(h.store1))

	if err != nil || len(c.Kits) != 1 {
		t.Fatalf("kits = %+v, %v", c.Kits, err)
	}
	if seat := c.Kits[0].Items[0]; seat.Swappable || len(seat.Alternatives) != 0 {
		t.Errorf("the seat = %+v; an archived alternative is left out, and the item is no longer swappable", seat)
	}
}

func TestWrites_DropTheStoresCachedCatalog(t *testing.T) {
	t.Parallel()
	probe := newHarness(t)
	for _, uc := range probe.useCases(t) {
		if !uc.write {
			continue
		}
		h := newHarness(t)
		cases := h.useCases(t)
		for _, tenant := range []shared.TenantID{h.store1, h.store2} {
			if _, err := h.svc.PublicCatalog(h.storePage(tenant)); err != nil {
				t.Fatal(err)
			}
		}
		for _, c := range cases {
			if c.name != uc.name {
				continue
			}
			if err := c.call(h.owner(t)); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
		}
		if h.cached(t, h.store1) {
			t.Errorf("%s left the store's cached catalog", uc.name)
		}
		if !h.cached(t, h.store2) {
			t.Errorf("%s dropped another store's cached catalog", uc.name)
		}
	}
}
