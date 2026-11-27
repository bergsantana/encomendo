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
// show or must hide:
//   - the published Kit Scrambler, whose seat may be swapped for the
//     published tall seat or for draftAlt, which is not published;
//   - the published "Kit capa", whose only member, draft, is not published
//     on its own;
//   - an archived item and an unpublished kit.
type storeFixture struct {
	scrambler
	draft, draftAlt, archived appcatalog.ItemView
	coverKit, hiddenKit       appcatalog.KitView
	category                  appcatalog.CategoryView
	attribute                 appcatalog.AttributeView
}

func (h *harness) storeFixture(t *testing.T) storeFixture {
	t.Helper()
	f := storeFixture{category: h.categories(t, "Bancos")[0], attribute: h.attribute(t, "Serve em", "CB 500X")}
	f.scrambler = h.scrambler(t)
	owner := h.owner(t)
	for _, it := range []appcatalog.ItemView{f.seat, f.tall, f.wheel} {
		if _, err := h.svc.SetItemPublished(owner, appcatalog.SetItemPublishedInput{ID: it.ID, Published: true, Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	f.draftAlt = h.createItem(t, inHouseInput("Banco rascunho", 70000, 3))
	in := kitOf("Kit Scrambler CB 500X", 65000,
		catalog.KitItem{Item: f.seat.ID, Quantity: 1, Swappable: true, Alternatives: []shared.ItemID{f.tall.ID, f.draftAlt.ID}},
		catalog.KitItem{Item: f.wheel.ID, Quantity: 2},
	)
	in.Published = true
	if _, err := h.svc.ReplaceKit(owner, appcatalog.ReplaceKitInput{ID: f.kit.ID, Kit: in, Version: 1}); err != nil {
		t.Fatal(err)
	}
	f.draft = h.createItem(t, h.costedInput("Rascunho"))
	cover := kitOf("Kit capa", 9000, one(f.draft.ID))
	cover.Published = true
	f.coverKit = h.createKit(t, cover)
	f.archived = h.createItem(t, inHouseInput("Antigo", 100, 1))
	f.hiddenKit = h.createKit(t, kitOf("Kit rascunho", 100, one(f.wheel.ID)))
	if _, err := h.svc.ArchiveItem(owner, appcatalog.ItemVersionInput{ID: f.archived.ID, Version: 1}); err != nil {
		t.Fatal(err)
	}
	return f
}

// storeKitByID finds a kit of the store page catalog.
func storeKitByID(t *testing.T, c appcatalog.StoreCatalog, id shared.KitID) appcatalog.StoreKit {
	t.Helper()
	for _, k := range c.Kits {
		if k.ID == id {
			return k
		}
	}
	t.Fatalf("kit %s is not on the store page: %+v", id, c.Kits)
	return appcatalog.StoreKit{}
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
	if len(c.Items) != 3 || c.Items[0].ID != f.tall.ID || c.Items[1].ID != f.seat.ID || c.Items[2].ID != f.wheel.ID {
		t.Fatalf("items = %+v; want only the three published ones, by name", c.Items)
	}
	wheel := c.Items[2]
	if wheel.ReadyDays != 7 || wheel.Price != brl(t, 10000) || len(wheel.Options) != 1 || wheel.Options[0].Choices[0].ExtraPrice != brl(t, 500) {
		t.Errorf("the wheel = %+v", wheel)
	}
	if len(c.Kits) != 2 || c.Kits[0].ID != f.coverKit.ID || c.Kits[1].ID != f.kit.ID {
		t.Fatalf("kits = %+v; want only the two published kits, by name", c.Kits)
	}
	k := c.Kits[1]
	if k.TotalDays != 20 || k.SingleItemsTotal != brl(t, 70000) || k.Discount != brl(t, 5000) || k.SwapDiscount != catalog.SwapKeepDiscount {
		t.Errorf("the kit = %+v", k)
	}
	seat := k.Items[0]
	if seat.ItemName != "Banco de couro" || !seat.Swappable || len(seat.Alternatives) != 1 || seat.Alternatives[0].ItemID != f.tall.ID ||
		seat.Alternatives[0].UnitPrice != brl(t, 60000) || k.Items[1].Quantity != 2 {
		t.Errorf("the kit's items = %+v; want the tall seat offered, the unpublished one left out", k.Items)
	}
	cover := c.Kits[0]
	if len(cover.Items) != 1 || cover.Items[0].ItemID != f.draft.ID || cover.Items[0].ItemName != "Rascunho" || cover.TotalDays != 12 {
		t.Errorf("the cover kit = %+v; an unpublished member still shows inside its published kit", cover)
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

// internalKeys returns the path of every forbidden key in value's JSON, and
// how many keys it walked.
func internalKeys(t *testing.T, name string, value any) (found []string, walked int) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	walkKeys(doc, name, func(key, path string) {
		walked++
		for _, bad := range forbiddenKeys {
			if strings.EqualFold(key, bad) {
				found = append(found, path)
			}
		}
	})
	return found, walked
}

func assertNoInternalKeys(t *testing.T, name string, value any) {
	t.Helper()
	found, walked := internalKeys(t, name, value)
	if walked == 0 {
		t.Errorf("%s has no keys; the walk proves nothing", name)
	}
	for _, path := range found {
		t.Errorf("%s holds an internal key", path)
	}
}

func TestStoreTypes_NoInternalKeyAtAnyDepth(t *testing.T) {
	t.Parallel()
	// The owner's read goes to the repositories in a staff scope, whose
	// item facts carry costs; the store page's has none to carry.
	for _, scope := range []string{"the store page", "the owner"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.storeFixture(t)
			ctx := h.storePage(h.store1)
			if scope == "the owner" {
				ctx = h.owner(t)
			}
			c, err := h.svc.PublicCatalog(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(c.Items) != 3 || len(c.Kits) != 2 || len(c.Kits[1].Items[0].Alternatives) == 0 || len(c.Items[2].Options) == 0 {
				t.Fatalf("the catalog is too thin to walk: %+v", c)
			}
			assertNoInternalKeys(t, "StoreCatalog", c)
			assertNoInternalKeys(t, "StoreItem", c.Items[2])
			assertNoInternalKeys(t, "StoreKit", c.Kits[1])
		})
	}
	t.Run("the walk catches a nested key", func(t *testing.T) {
		t.Parallel()
		probe := map[string]any{"items": []any{map[string]any{"mainSupplier": map[string]any{"LeadDays": 3}}}}
		if found, _ := internalKeys(t, "probe", probe); len(found) != 2 {
			t.Errorf("found %v, want mainSupplier and LeadDays", found)
		}
	})
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

	if err != nil || len(again.Items) != len(first.Items) || again.Kits[1].TotalDays != 20 {
		t.Errorf("the cached catalog = %+v, %v", again, err)
	}
	if len(h.tx.Scopes()) != before {
		t.Error("a cached read opened a transaction")
	}
	k, err := h.svc.PublicKit(ctx, first.Kits[1].ID)
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

	if err != nil || len(c.Items) != 3 || len(c.Kits) != 2 {
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

func TestPublicCatalog_RefusesAScopeWithoutAStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	f := h.storeFixture(t)
	account := shared.ClientAccountID(h.ids.NewID())
	cases := map[string]struct {
		scope appshared.Scope
		want  error
	}{
		"no scope":        {appshared.Scope{}, authz.ErrTenantRequired},
		"the client area": {appshared.Scope{ClientAccountID: account}, authz.ErrTenantRequired},
		"a mixed scope":   {appshared.Scope{TenantID: h.store1, ClientAccountID: account}, appshared.ErrInvalidScope},
		"a mixed scope 2": {appshared.Scope{UserID: shared.UserID(h.ids.NewID()), ClientAccountID: account}, appshared.ErrInvalidScope},
	}
	if _, err := h.svc.PublicCatalog(h.storePage(h.store1)); err != nil { // fills the cache a mixed scope must not reach
		t.Fatal(err)
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := appshared.WithScope(context.Background(), c.scope)
			if _, err := h.svc.PublicCatalog(ctx); !errors.Is(err, c.want) {
				t.Errorf("PublicCatalog: %v, want %v", err, c.want)
			}
			if _, err := h.svc.PublicKit(ctx, f.kit.ID); !errors.Is(err, c.want) {
				t.Errorf("PublicKit: %v, want %v", err, c.want)
			}
		})
	}
}

func TestPublicKit_OnlyPublishedKitsOfTheStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	f := h.storeFixture(t)
	cases := map[string]struct {
		tenant shared.TenantID
		id     shared.KitID
	}{
		"an unpublished kit":  {h.store1, f.hiddenKit.ID},
		"an unknown kit":      {h.store1, shared.KitID(h.ids.NewID())},
		"another store's kit": {h.store2, f.kit.ID},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := h.svc.PublicKit(h.storePage(c.tenant), c.id); !errors.Is(err, catalog.ErrKitNotFound) {
				t.Errorf("%v, want ErrKitNotFound", err)
			}
		})
	}
}

func TestPublicCatalog_FailuresPassThrough(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"CategoriesList", "AttributesList", "ItemsPublished", "KitsPublished", "ItemsPublicFacts", "defaults"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.storeFixture(t)
			if fault == "defaults" {
				h.defaults.Fail(errBoom)
			} else {
				h.store.Fail(fault, errBoom)
			}
			if _, err := h.svc.PublicCatalog(h.storePage(h.store1)); !errors.Is(err, errBoom) {
				t.Errorf("%v, want it passed through", err)
			}
		})
	}
}

// The store page reads kit facts through Items.PublicFacts, which never
// touches a staff-only table, in every scope: a failing ByIDs (the read that
// carries costs) is never reached, for the store page and for an owner's
// preview alike (ADR 0024).
func TestPublicCatalog_NeverReadsFactsWithCosts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.storeFixture(t)
	h.store.Fail("ItemsByIDs", errBoom)
	for name, ctx := range map[string]context.Context{"store page": h.storePage(h.store1), "owner preview": h.owner(t)} {
		if _, err := h.svc.PublicCatalog(ctx); err != nil {
			t.Errorf("%s: PublicCatalog = %v, want it to read no costs", name, err)
		}
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

	if err != nil {
		t.Fatal(err)
	}
	if seat := storeKitByID(t, c, f.kit.ID).Items[0]; seat.Swappable || len(seat.Alternatives) != 0 {
		t.Errorf("the seat = %+v; an archived alternative is left out, and the item is no longer swappable", seat)
	}
}

func TestWrites_DropTheStoresCachedCatalog(t *testing.T) {
	t.Parallel()
	for _, uc := range newHarness(t).useCases(t) {
		if !uc.write {
			continue
		}
		t.Run(uc.name, func(t *testing.T) {
			t.Parallel()
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
					t.Fatal(err)
				}
			}
			if h.cached(t, h.store1) {
				t.Error("the write left the store's cached catalog")
			}
			if !h.cached(t, h.store2) {
				t.Error("the write dropped another store's cached catalog")
			}
		})
	}
}
