package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	catfake "github.com/bergsantana/encomendo/backend/internal/app/catalog/fake"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	supfake "github.com/bergsantana/encomendo/backend/internal/app/supplier/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// realRig serves the handlers over the real catalog and supplier use cases
// and their in-memory fakes, so who sees a cost is decided by the app layer,
// as in production, and the HTTP layer is tested on what it really gets.
type realRig struct {
	t                 *testing.T
	c                 *handler.Catalog
	s                 *handler.Suppliers
	storage           *catfake.Storage
	couros, ferragens shared.SupplierID
}

func newRealRig(t *testing.T) *realRig {
	t.Helper()
	clock := sharedfake.NewClock(ctWhen)
	ids := sharedfake.NewIDs(clock)
	store, dir, defaults := catfake.NewStore(), catfake.NewSuppliers(), catfake.NewDefaults()
	r := &realRig{t: t, storage: catfake.NewStorage(clock)}
	for name, slot := range map[string]*shared.SupplierID{"Couros Paulista": &r.couros, "Ferragens Norte": &r.ferragens} {
		*slot = shared.SupplierID(ids.NewID())
		dir.Add(tenantID, *slot, name)
		store.AddSupplier(tenantID, *slot)
	}
	defaults.Set(tenantID, appcatalog.Defaults{Currency: "BRL", AssemblyDays: 3, SafetyMarginDays: 2, StockEnabled: true, ProductionEnabled: true})
	tx := sharedfake.NewTxManager()
	svc := appcatalog.NewService(appcatalog.Deps{
		Categories: store.Categories, Attributes: store.Attributes, Items: store.Items, Kits: store.Kits,
		Suppliers: dir, Defaults: defaults, Photos: store.Photos, Storage: r.storage,
		Cache: sharedfake.NewCache(clock), Tx: tx, IDs: ids,
	})
	suppliers := appsupplier.NewService(appsupplier.Deps{Suppliers: supfake.NewStore(), Tx: tx, IDs: ids})
	var err error
	if r.c, err = handler.NewCatalog(handler.CatalogDeps{UseCases: svc, Limiter: &limiter{refuse: map[string]bool{}}, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	if r.s, err = handler.NewSuppliers(handler.SuppliersDeps{UseCases: suppliers, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	return r
}

// do sends the request as role and returns the recorder and the decoded
// body (nil when there is none).
func (r *realRig) do(role shared.Role, method, path string, body any) (*httptest.ResponseRecorder, any) {
	r.t.Helper()
	payload := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			r.t.Fatal(err)
		}
		payload = string(b)
	}
	rec := ctServe(r.t, r.c, r.s, staffIn(r.t, role), method, path, payload)
	var out any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			r.t.Fatalf("%s %s: not JSON: %s", method, path, rec.Body.String())
		}
	}
	return rec, out
}

// ok sends the request and fails unless it answers want.
func (r *realRig) ok(role shared.Role, want int, method, path string, body any) map[string]any {
	r.t.Helper()
	rec, out := r.do(role, method, path, body)
	if rec.Code != want {
		r.t.Fatalf("%s %s as %s answered %d %s, want %d", method, path, role, rec.Code, rec.Body.String(), want)
	}
	m, _ := out.(map[string]any)
	return m
}

const ctBase = "/api/v1/admin/catalog"

// itemFields is an item body: bought from a supplier unless fields change
// it. Costs are left out.
func (r *realRig) itemFields(name string, over map[string]any) map[string]any {
	m := map[string]any{
		"name": name, "description": "", "categoryId": nil, "sourcingMode": "to_order", "priceCents": 10000,
		"productionDays": nil, "mainSupplier": map[string]any{"supplierId": r.couros.String(), "leadDays": 7},
		"backupSupplier": nil, "attributeValueIds": []string{}, "options": []any{}, "published": false,
	}
	for k, v := range over {
		m[k] = v
	}
	return m
}

// supplierLink is a mainSupplier or backupSupplier body.
func supplierLink(id shared.SupplierID, leadDays int, cost map[string]any) map[string]any {
	m := map[string]any{"supplierId": id.String(), "leadDays": leadDays}
	for k, v := range cost {
		m[k] = v
	}
	return m
}

// seeded is what seed creates.
type seeded struct {
	category          string
	banco, bolo, bare string // item ids: bought with costs, in house with a cost, bought with none
	kit               string
	photo             string
}

// seed creates, as the owner, a category, an item bought from two suppliers
// with a cost each, an item made in house with a cost, an item with no cost,
// a kit of the first two and a photo on the first.
func (r *realRig) seed() seeded {
	r.t.Helper()
	var s seeded
	s.category = r.ok(shared.RoleOwner, 201, "POST", ctBase+"/categories", map[string]any{"name": "Bancos"})["id"].(string)
	s.banco = r.ok(shared.RoleOwner, 201, "POST", ctBase+"/items", r.itemFields("Banco", map[string]any{
		"categoryId":     s.category,
		"mainSupplier":   supplierLink(r.couros, 7, map[string]any{"costCents": 4000}),
		"backupSupplier": supplierLink(r.ferragens, 9, map[string]any{"costCents": 4500}),
	}))["id"].(string)
	s.bolo = r.ok(shared.RoleOwner, 201, "POST", ctBase+"/items", r.itemFields("Bolo", map[string]any{
		"sourcingMode": "in_house", "priceCents": 5000, "productionDays": 3, "mainSupplier": nil, "productionCostCents": 1500,
	}))["id"].(string)
	s.bare = r.ok(shared.RoleOwner, 201, "POST", ctBase+"/items", r.itemFields("Capa", nil))["id"].(string)
	s.kit = r.ok(shared.RoleOwner, 201, "POST", ctBase+"/kits", map[string]any{
		"name": "Kit", "description": "", "priceCents": 12000, "swapDiscount": "keep", "assemblyDays": nil, "safetyMarginDays": nil,
		"items": []any{
			map[string]any{"itemId": s.banco, "quantity": 1, "swappable": false, "alternativeItemIds": []string{}},
			map[string]any{"itemId": s.bolo, "quantity": 2, "swappable": false, "alternativeItemIds": []string{}},
		},
		"attributeValueIds": []string{}, "published": false,
	})["id"].(string)
	up := r.ok(shared.RoleOwner, 201, "POST", ctBase+"/photo-uploads",
		map[string]any{"ownerType": "item", "ownerId": s.banco, "contentType": "image/jpeg"})
	key := up["key"].(string)
	r.storage.Upload(key, catalog.ContentTypeJPEG, append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, make([]byte, 64)...))
	s.photo = r.ok(shared.RoleOwner, 201, "POST", ctBase+"/items/"+s.banco+"/photos", map[string]any{"key": key})["id"].(string)
	return s
}

// costKeys are the keys only the owner sees, at any depth of a response.
var costKeys = map[string]bool{"cost": true, "productionCost": true, "margin": true}

// findCostKeys walks a decoded JSON value and returns the path of every
// cost, productionCost or margin key in it.
func findCostKeys(v any, path string) []string {
	var found []string
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			if costKeys[k] {
				found = append(found, path+"."+k)
			}
			found = append(found, findCostKeys(child, path+"."+k)...)
		}
	case []any:
		for i, child := range v {
			found = append(found, findCostKeys(child, fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return found
}

// The plan's visibility test (step 3.10, ADR 0025): for admin, staff and
// viewer, no response of any catalog or supplier operation has a cost,
// productionCost or margin key, at any depth. The use cases are the real
// ones, so this fails if either layer lets a cost through.
func TestCatalogVisibility_NonOwnersNeverSeeCostKeys(t *testing.T) {
	t.Parallel()

	for _, role := range []shared.Role{shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()

			r := newRealRig(t)
			s := r.seed()
			supplier := r.ok(shared.RoleOwner, 201, "POST", "/api/v1/admin/suppliers",
				map[string]any{"name": "Couros", "whatsappPhone": "5511987654321"})["id"].(string)
			reads := []string{
				ctBase + "/categories", ctBase + "/attributes", ctBase + "/items?includeArchived=true", ctBase + "/items/" + s.banco,
				ctBase + "/items/" + s.bolo, ctBase + "/items/" + s.bare, ctBase + "/kits?includeArchived=true", ctBase + "/kits/" + s.kit,
				"/api/v1/admin/suppliers", "/api/v1/admin/suppliers/" + supplier,
			}
			seen := 0
			for _, path := range reads {
				rec, out := r.do(role, "GET", path, nil)
				if rec.Code != 200 {
					t.Fatalf("GET %s answered %d %s", path, rec.Code, rec.Body.String())
				}
				if found := findCostKeys(out, ""); len(found) > 0 {
					t.Errorf("GET %s as %s has cost keys %v in %s", path, role, found, rec.Body.String())
				}
				seen++
			}
			// A role that may write sees no cost in the answer to a write
			// either; one that may not gets a problem, which has none.
			for _, w := range r.writes(s) {
				rec, out := r.do(role, w.method, w.path, w.body)
				if found := findCostKeys(out, ""); len(found) > 0 {
					t.Errorf("%s %s as %s (%d) has cost keys %v in %s", w.method, w.path, role, rec.Code, found, rec.Body.String())
				}
				if role == shared.RoleAdmin && rec.Code >= 300 {
					t.Errorf("%s %s as admin answered %d %s", w.method, w.path, rec.Code, rec.Body.String())
				}
				seen++
			}
			if seen < 19 {
				t.Errorf("only %d responses were checked", seen)
			}
		})
	}
}

type write struct {
	method, path string
	body         any
}

// writes are write operations a role with catalog:write can make without
// sending a cost, answering the item, kit or photo.
func (r *realRig) writes(s seeded) []write {
	item := r.itemFields("Banco", map[string]any{
		"mainSupplier": supplierLink(r.couros, 7, nil), "backupSupplier": supplierLink(r.ferragens, 9, nil), "version": 1,
	})
	return []write{
		{"POST", ctBase + "/items", r.itemFields("Selim", nil)},
		{"PUT", ctBase + "/items/" + s.banco, item},
		{"PUT", ctBase + "/items/" + s.bare + "/published", map[string]any{"published": true, "version": 1}},
		{"POST", ctBase + "/items/" + s.bare + "/archive", map[string]any{"version": 2}},
		{"POST", ctBase + "/items/" + s.bare + "/restore", map[string]any{"version": 3}},
		{"PUT", ctBase + "/kits/" + s.kit + "/published", map[string]any{"published": true, "version": 1}},
		{"PUT", ctBase + "/items/" + s.banco + "/photos/order", map[string]any{"photoIds": []string{s.photo}}},
		{"PATCH", ctBase + "/categories/" + s.category, map[string]any{"name": "Assentos"}},
		{"POST", "/api/v1/admin/suppliers", map[string]any{"name": "Tecidos", "whatsappPhone": "5511987654321"}},
	}
}

// The owner's responses carry the keys, with null where nothing is
// recorded; each supplier's cost sits on its own line (ADR 0025).
func TestCatalogVisibility_OwnerSeesCostsAndNulls(t *testing.T) {
	t.Parallel()

	r := newRealRig(t)
	s := r.seed()

	banco := r.ok(shared.RoleOwner, 200, "GET", ctBase+"/items/"+s.banco, nil)
	wantKeys(t, "banco", banco, "cost", "margin")
	if _, has := banco["productionCost"]; has {
		t.Error("a bought item has a productionCost key; the contract shows it for in_house only")
	}
	if got := moneyOf(t, banco["cost"]); got != 4000 {
		t.Errorf("banco cost = %d, want the main supplier's 4000", got)
	}
	if m := banco["margin"].(map[string]any); m["cents"] != float64(6000) || m["basisPoints"] != float64(6000) {
		t.Errorf("banco margin = %v, want 6000 cents and 6000 bp", m)
	}
	main, backup := banco["mainSupplier"].(map[string]any), banco["backupSupplier"].(map[string]any)
	if moneyOf(t, main["cost"]) != 4000 || moneyOf(t, backup["cost"]) != 4500 {
		t.Errorf("supplier costs = %v and %v, want 4000 and 4500", main["cost"], backup["cost"])
	}

	bolo := r.ok(shared.RoleOwner, 200, "GET", ctBase+"/items/"+s.bolo, nil)
	wantKeys(t, "bolo", bolo, "cost", "margin", "productionCost")
	if moneyOf(t, bolo["productionCost"]) != 1500 || bolo["mainSupplier"] != nil {
		t.Errorf("bolo productionCost %v mainSupplier %v", bolo["productionCost"], bolo["mainSupplier"])
	}

	bare := r.ok(shared.RoleOwner, 200, "GET", ctBase+"/items/"+s.bare, nil)
	wantKeys(t, "capa", bare, "cost", "margin")
	if bare["cost"] != nil || bare["margin"] != nil || bare["mainSupplier"].(map[string]any)["cost"] != nil {
		t.Errorf("an item with no cost has cost %v margin %v supplier cost %v, want null", bare["cost"], bare["margin"], bare["mainSupplier"])
	}

	rows := r.ok(shared.RoleOwner, 200, "GET", ctBase+"/items", nil)["items"].([]any)
	if len(rows) != 3 {
		t.Fatalf("%d rows, want 3", len(rows))
	}
	for _, row := range rows {
		wantKeys(t, "item row", row.(map[string]any), "cost", "margin")
	}
	kit := r.ok(shared.RoleOwner, 200, "GET", ctBase+"/kits/"+s.kit, nil)
	wantKeys(t, "kit", kit, "cost", "margin")
	if got := moneyOf(t, kit["cost"]); got != 4000+2*1500 {
		t.Errorf("kit cost = %d, want 7000 (the items' costs times their quantities)", got)
	}
	for _, row := range r.ok(shared.RoleOwner, 200, "GET", ctBase+"/kits", nil)["items"].([]any) {
		wantKeys(t, "kit row", row.(map[string]any), "cost", "margin")
	}
}

func wantKeys(t *testing.T, what string, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Errorf("%s has no %q key for the owner: %v", what, k, m)
		}
	}
}

func moneyOf(t *testing.T, v any) int64 {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%v is not an amount", v)
	}
	return int64(m["cents"].(float64))
}

// A cost key from any role but the owner, even null, is 403 forbidden and
// nothing is written (ADR 0025).
func TestCatalogCosts_NonOwnerSendingTheKeyIsForbidden(t *testing.T) {
	t.Parallel()

	r := newRealRig(t)
	s := r.seed()
	costed := map[string]map[string]any{
		"main supplier cost null": {"mainSupplier": supplierLink(r.couros, 7, map[string]any{"costCents": nil})},
		"main supplier cost":      {"mainSupplier": supplierLink(r.couros, 7, map[string]any{"costCents": 1})},
		"backup cost null":        {"mainSupplier": supplierLink(r.couros, 7, nil), "backupSupplier": supplierLink(r.ferragens, 9, map[string]any{"costCents": nil})},
		"production cost null":    {"sourcingMode": "in_house", "productionDays": 2, "mainSupplier": nil, "productionCostCents": nil},
		"production cost":         {"sourcingMode": "in_house", "productionDays": 2, "mainSupplier": nil, "productionCostCents": 100},
	}
	for name, over := range costed {
		t.Run(name, func(t *testing.T) {
			rec, _ := r.do(shared.RoleAdmin, "POST", ctBase+"/items", r.itemFields("Novo "+name, over))
			if rec.Code != 403 || code(t, rec) != "forbidden" {
				t.Errorf("create answered %d %s, want 403 forbidden", rec.Code, rec.Body.String())
			}
			over["version"] = 1
			rec, _ = r.do(shared.RoleAdmin, "PUT", ctBase+"/items/"+s.banco, r.itemFields("Banco", over))
			if rec.Code != 403 || code(t, rec) != "forbidden" {
				t.Errorf("replace answered %d %s, want 403 forbidden", rec.Code, rec.Body.String())
			}
		})
	}
	// Nothing was written: still the three seeded items, the first at version 1.
	if rows := r.ok(shared.RoleOwner, 200, "GET", ctBase+"/items", nil)["items"].([]any); len(rows) != 3 {
		t.Errorf("%d items after the refused writes, want 3", len(rows))
	}
	if v := r.ok(shared.RoleOwner, 200, "GET", ctBase+"/items/"+s.banco, nil)["version"]; v != float64(1) {
		t.Errorf("banco version = %v, want 1", v)
	}
	// The owner may send them.
	r.ok(shared.RoleOwner, 201, "POST", ctBase+"/items", r.itemFields("Do dono", map[string]any{
		"mainSupplier": supplierLink(r.couros, 7, map[string]any{"costCents": nil}),
	}))
}

// costsOf returns the cost of the item's main supplier, backup supplier
// and production, as the owner reads them: nil for null or no such line.
func (r *realRig) costsOf(item string) (main, backup, production any) {
	r.t.Helper()
	it := r.ok(shared.RoleOwner, 200, "GET", ctBase+"/items/"+item, nil)
	line := func(key string) any {
		if m, ok := it[key].(map[string]any); ok {
			return m["cost"]
		}
		return nil
	}
	return line("mainSupplier"), line("backupSupplier"), it["productionCost"]
}

func (r *realRig) versionOf(item string) float64 {
	r.t.Helper()
	return r.ok(shared.RoleOwner, 200, "GET", ctBase+"/items/"+item, nil)["version"].(float64)
}

func cents(v any) any {
	if m, ok := v.(map[string]any); ok {
		return m["cents"]
	}
	return nil
}

// An admin's replace, which leaves costs out, keeps each supplier's own
// cost (ADR 0025). A cost belongs to the item and one supplier, not to the
// main or backup slot.
func TestCatalogCosts_AdminReplaceKeepsEachSuppliersCost(t *testing.T) {
	t.Parallel()

	r := newRealRig(t)
	s := r.seed()
	replace := func(over map[string]any) {
		over["version"] = r.versionOf(s.banco)
		r.ok(shared.RoleAdmin, 200, "PUT", ctBase+"/items/"+s.banco, r.itemFields("Banco", over))
	}
	main, backup, _ := r.costsOf(s.banco)
	if cents(main) != float64(4000) || cents(backup) != float64(4500) {
		t.Fatalf("seeded costs = %v and %v", main, backup)
	}

	// The same suppliers, costs left out: each keeps its own.
	replace(map[string]any{"mainSupplier": supplierLink(r.couros, 8, nil), "backupSupplier": supplierLink(r.ferragens, 10, nil)})
	if main, backup, _ = r.costsOf(s.banco); cents(main) != float64(4000) || cents(backup) != float64(4500) {
		t.Errorf("after a replace with no costs: %v and %v, want 4000 and 4500", main, backup)
	}

	// Main and backup swap: each supplier keeps its own cost, in its new slot.
	replace(map[string]any{"mainSupplier": supplierLink(r.ferragens, 10, nil), "backupSupplier": supplierLink(r.couros, 8, nil)})
	if main, backup, _ = r.costsOf(s.banco); cents(main) != float64(4500) || cents(backup) != float64(4000) {
		t.Errorf("after a swap: main %v backup %v, want 4500 and 4000", main, backup)
	}

	// A backup that is unlinked takes its cost with it; linked again, it starts with none.
	replace(map[string]any{"mainSupplier": supplierLink(r.ferragens, 10, nil)})
	if main, backup, _ = r.costsOf(s.banco); cents(main) != float64(4500) || backup != nil {
		t.Errorf("after unlinking the backup: main %v backup %v, want 4500 and none", main, backup)
	}
	replace(map[string]any{"mainSupplier": supplierLink(r.ferragens, 10, nil), "backupSupplier": supplierLink(r.couros, 8, nil)})
	if _, backup, _ = r.costsOf(s.banco); backup != nil {
		t.Errorf("a supplier linked again has cost %v, want null", backup)
	}

	// A new main supplier starts with none: the old one's cost does not move to it.
	replace(map[string]any{"mainSupplier": supplierLink(r.couros, 8, nil)})
	if main, backup, _ = r.costsOf(s.banco); main != nil || backup != nil {
		t.Errorf("after replacing the main supplier: %v and %v, want null and none", main, backup)
	}
}

// An item that leaves in_house drops its production cost; coming back
// starts with none (ADR 0025). An admin's replace that leaves the cost out
// keeps it while the item stays in house.
func TestCatalogCosts_ProductionCostFollowsTheMode(t *testing.T) {
	t.Parallel()

	r := newRealRig(t)
	s := r.seed()
	replace := func(over map[string]any) {
		over["version"] = r.versionOf(s.bolo)
		r.ok(shared.RoleAdmin, 200, "PUT", ctBase+"/items/"+s.bolo, r.itemFields("Bolo", over))
	}
	inHouse := map[string]any{"sourcingMode": "in_house", "priceCents": 5000, "productionDays": 4, "mainSupplier": nil}

	replace(inHouse)
	if _, _, prod := r.costsOf(s.bolo); cents(prod) != float64(1500) {
		t.Errorf("an admin's replace changed the production cost to %v, want 1500", prod)
	}
	replace(map[string]any{"priceCents": 5000})
	if _, _, prod := r.costsOf(s.bolo); prod != nil {
		t.Errorf("a bought item has a productionCost %v", prod)
	}
	replace(inHouse)
	it := r.ok(shared.RoleOwner, 200, "GET", ctBase+"/items/"+s.bolo, nil)
	if prod, has := it["productionCost"]; !has || prod != nil {
		t.Errorf("back in house, productionCost = %v (present: %v), want null", prod, has)
	}
}
