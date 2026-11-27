package handler_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	ctItemBody = fmt.Sprintf(`{"name":"Banco","description":"","categoryId":null,"sourcingMode":"to_order","priceCents":10000,`+
		`"productionDays":null,"mainSupplier":{"supplierId":%q,"leadDays":7},"backupSupplier":null,"attributeValueIds":[],`+
		`"options":[{"name":"Cor","required":true,"choices":[{"label":"Preta","extraPriceCents":0,"extraDays":0}]}],"published":false`, ctSupplier.String())
	ctKitBody = fmt.Sprintf(`{"name":"Kit","description":"","priceCents":25000,"swapDiscount":"keep","assemblyDays":null,`+
		`"safetyMarginDays":null,"items":[{"itemId":%q,"quantity":1,"swappable":false,"alternativeItemIds":[]}],`+
		`"attributeValueIds":[],"published":false`, ctItem.String())
	ctAttributeBody = `{"name":"Serve em","values":[{"label":"CB 500X"}]}`
	ctUploadBody    = fmt.Sprintf(`{"ownerType":"item","ownerId":%q,"contentType":"image/jpeg"}`, ctItem.String())
)

// ctOp is one operation of the contract with a body that passes, and the
// status it answers.
type ctOp struct {
	pattern string
	body    string
	status  int
}

func (o ctOp) method() string { m, _, _ := strings.Cut(o.pattern, " "); return m }

func (o ctOp) path() string { _, p, _ := strings.Cut(o.pattern, " "); return ctPaths.Replace(p) }

// hasBody reports whether the operation reads a JSON body.
func (o ctOp) hasBody() bool { return o.body != "" }

// ctOps is all 33 catalog and supplier operations.
func ctOps() []ctOp {
	const base = "/api/v1/admin/catalog"
	return []ctOp{
		{"GET " + base + "/categories", "", 200},
		{"POST " + base + "/categories", `{"name":"Bancos"}`, 201},
		{"PATCH " + base + "/categories/{categoryId}", `{"name":"Bancos"}`, 200},
		{"DELETE " + base + "/categories/{categoryId}", "", 204},
		{"GET " + base + "/attributes", "", 200},
		{"POST " + base + "/attributes", ctAttributeBody, 201},
		{"PUT " + base + "/attributes/{attributeId}", ctAttributeBody, 200},
		{"DELETE " + base + "/attributes/{attributeId}", "", 204},
		{"GET " + base + "/items?limit=10&q=ban&includeArchived=true", "", 200},
		{"POST " + base + "/items", ctItemBody + `}`, 201},
		{"GET " + base + "/items/{itemId}", "", 200},
		{"PUT " + base + "/items/{itemId}", ctItemBody + `,"version":1}`, 200},
		{"PUT " + base + "/items/{itemId}/published", `{"published":true,"version":1}`, 200},
		{"POST " + base + "/items/{itemId}/archive", `{"version":1}`, 200},
		{"POST " + base + "/items/{itemId}/restore", `{"version":1}`, 200},
		{"POST " + base + "/items/{itemId}/photos", `{"key":"t/a/items/b/c"}`, 201},
		{"PUT " + base + "/items/{itemId}/photos/order", fmt.Sprintf(`{"photoIds":[%q]}`, ctPhoto.String()), 200},
		{"DELETE " + base + "/items/{itemId}/photos/{photoId}", "", 204},
		{"GET " + base + "/kits?includeArchived=false", "", 200},
		{"POST " + base + "/kits", ctKitBody + `}`, 201},
		{"GET " + base + "/kits/{kitId}", "", 200},
		{"PUT " + base + "/kits/{kitId}", ctKitBody + `,"version":1}`, 200},
		{"PUT " + base + "/kits/{kitId}/published", `{"published":true,"version":1}`, 200},
		{"POST " + base + "/kits/{kitId}/archive", `{"version":1}`, 200},
		{"POST " + base + "/kits/{kitId}/restore", `{"version":1}`, 200},
		{"POST " + base + "/kits/{kitId}/photos", `{"key":"t/a/kits/b/c"}`, 201},
		{"PUT " + base + "/kits/{kitId}/photos/order", `{"photoIds":[]}`, 200},
		{"DELETE " + base + "/kits/{kitId}/photos/{photoId}", "", 204},
		{"POST " + base + "/photo-uploads", ctUploadBody, 201},
		{"GET /api/v1/admin/suppliers?q=cou", "", 200},
		{"POST /api/v1/admin/suppliers", `{"name":"Couros","whatsappPhone":"5511987654321"}`, 201},
		{"GET /api/v1/admin/suppliers/{supplierId}", "", 200},
		{"PUT /api/v1/admin/suppliers/{supplierId}", `{"name":"Couros","whatsappPhone":"5511987654321","version":1}`, 200},
	}
}

func TestCatalog_ServesEveryOperation(t *testing.T) {
	t.Parallel()

	ops := ctOps()
	if len(ops) != 33 {
		t.Fatalf("%d operations listed, the contract has 33", len(ops))
	}
	for _, op := range ops {
		t.Run(op.pattern, func(t *testing.T) {
			t.Parallel()

			f := newCtFixture(t)
			rec := f.call(t, op.method(), op.path(), op.body)
			if rec.Code != op.status {
				t.Fatalf("answer %d %s, want %d", rec.Code, rec.Body.String(), op.status)
			}
			if f.uses() != 1 {
				t.Errorf("%d use-case calls, want 1", f.uses())
			}
		})
	}
}

// A path id that is not an id names nothing: 404, and no use case runs.
func TestCatalog_MalformedPathIDIsNotFound(t *testing.T) {
	t.Parallel()

	for _, op := range ctOps() {
		if !strings.Contains(op.pattern, "{") {
			continue
		}
		t.Run(op.pattern, func(t *testing.T) {
			t.Parallel()

			f := newCtFixture(t)
			path := strings.NewReplacer("{categoryId}", "nope", "{attributeId}", "nope", "{itemId}", "nope",
				"{kitId}", "nope", "{photoId}", "nope", "{supplierId}", "nope").Replace(strings.SplitN(op.pattern, " ", 2)[1])
			rec := f.call(t, op.method(), path, op.body)
			if rec.Code != 404 || code(t, rec) != respond.CodeNotFound || f.uses() != 0 {
				t.Errorf("answer %d %s after %d use-case calls, want 404 not_found and none", rec.Code, rec.Body.String(), f.uses())
			}
		})
	}
}

// An unknown field in any body, at any depth, is 400 invalid: the schemas
// under allOf cannot say so, so only DisallowUnknownFields and this test
// enforce it.
func TestCatalog_RefusesUnknownFields(t *testing.T) {
	t.Parallel()

	for _, op := range ctOps() {
		if !op.hasBody() {
			continue
		}
		t.Run(op.pattern, func(t *testing.T) {
			t.Parallel()

			f := newCtFixture(t)
			rec := f.call(t, op.method(), op.path(), withUnknownField(t, op.body))
			if rec.Code != 400 || code(t, rec) != respond.CodeInvalid || f.uses() != 0 {
				t.Errorf("answer %d %s after %d use-case calls, want 400 invalid and none", rec.Code, rec.Body.String(), f.uses())
			}
		})
	}
}

func withUnknownField(t *testing.T, body string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatal(err)
	}
	m["bogus"] = 1
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// Nested objects refuse unknown fields too, including the ones inside an
// Optional value, which decodes with its own decoder.
func TestCatalog_RefusesUnknownNestedFields(t *testing.T) {
	t.Parallel()

	sup := fmt.Sprintf(`{"supplierId":%q,"leadDays":7`, ctSupplier.String())
	tests := map[string]string{
		"supplier":    strings.Replace(ctItemBody, sup, sup+`,"bogus":1`, 1) + `}`,
		"cost":        strings.Replace(ctItemBody, sup, sup+`,"costCents":5`, 1) + `,"productionCostCents":1,"bogus":1}`,
		"option":      strings.Replace(ctItemBody, `"required":true`, `"required":true,"bogus":1`, 1) + `}`,
		"choice":      strings.Replace(ctItemBody, `"extraDays":0`, `"extraDays":0,"bogus":1`, 1) + `}`,
		"category id": strings.Replace(ctItemBody, `"categoryId":null`, `"categoryId":{"bogus":1}`, 1) + `}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newCtFixture(t)
			rec := f.call(t, "POST", "/api/v1/admin/catalog/items", body)
			if rec.Code != 400 || code(t, rec) != respond.CodeInvalid || f.uses() != 0 {
				t.Errorf("answer %d %s after %d use-case calls, want 400 invalid and none", rec.Code, rec.Body.String(), f.uses())
			}
		})
	}
}

// A body with nothing in it is 400 invalid and names what is missing.
func TestCatalog_EmptyBodiesAreInvalid(t *testing.T) {
	t.Parallel()

	for _, op := range ctOps() {
		if !op.hasBody() {
			continue
		}
		t.Run(op.pattern, func(t *testing.T) {
			t.Parallel()

			f := newCtFixture(t)
			rec := f.call(t, op.method(), op.path(), `{}`)
			var p respond.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 400 || p.Code != respond.CodeInvalid || len(p.Errors) == 0 || f.uses() != 0 {
				t.Errorf("answer %d %s after %d use-case calls, want 400 invalid naming fields and none", rec.Code, rec.Body.String(), f.uses())
			}
		})
	}
}

func TestCatalog_ValidationNamesTheField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, path, body, method string
		field                    string
	}{
		{"bad supplier id", "/api/v1/admin/catalog/items", strings.Replace(ctItemBody, ctSupplier.String(), "nope", 1) + `}`, "POST", "mainSupplier.supplierId"},
		{"missing lead days", "/api/v1/admin/catalog/items", strings.Replace(ctItemBody, `,"leadDays":7`, ``, 1) + `}`, "POST", "mainSupplier.leadDays"},
		{"missing choice label", "/api/v1/admin/catalog/items", strings.Replace(ctItemBody, `"label":"Preta",`, ``, 1) + `}`, "POST", "options[0].choices[0].label"},
		{"category key left out", "/api/v1/admin/catalog/items", strings.Replace(ctItemBody, `"categoryId":null,`, ``, 1) + `}`, "POST", "categoryId"},
		{"version below 1", "/api/v1/admin/catalog/items/" + ctItem.String(), ctItemBody + `,"version":0}`, "PUT", "version"},
		{"bad owner type", "/api/v1/admin/catalog/photo-uploads", strings.Replace(ctUploadBody, `"item"`, `"order"`, 1), "POST", "ownerType"},
		{"bad kit item id", "/api/v1/admin/catalog/kits", strings.Replace(ctKitBody, ctItem.String(), "nope", 1) + `}`, "POST", "items[0].itemId"},
		{"price of the wrong type", "/api/v1/admin/catalog/kits", strings.Replace(ctKitBody, `"priceCents":25000`, `"priceCents":"x"`, 1) + `}`, "POST", "priceCents"},
		{"empty key", "/api/v1/admin/catalog/items/" + ctItem.String() + "/photos", `{"key":""}`, "POST", "key"},
		{"category update with nothing", "/api/v1/admin/catalog/categories/" + ctCategory.String(), `{}`, "PATCH", "name"},
		{"bad category filter", "/api/v1/admin/catalog/items?categoryId=nope", "", "GET", "categoryId"},
		{"bad includeArchived", "/api/v1/admin/catalog/kits?includeArchived=maybe", "", "GET", "includeArchived"},
		{"search too long", "/api/v1/admin/suppliers?q=" + strings.Repeat("a", 101), "", "GET", "q"},
		{"forged cursor limit", "/api/v1/admin/suppliers?limit=0x", "", "GET", "limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newCtFixture(t)
			rec := f.call(t, tt.method, tt.path, tt.body)
			var p respond.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(p.Errors))
			for _, e := range p.Errors {
				got = append(got, e.Field)
			}
			if rec.Code != 400 || !contains(got, tt.field) || f.uses() != 0 {
				t.Errorf("answer %d with fields %v after %d use-case calls, want 400 naming %s and none", rec.Code, got, f.uses(), tt.field)
			}
		})
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// refusal builds an app error that names rows, as the use cases return it.
func refusal(sentinel error) error {
	return &appcatalog.RefusalError{Err: sentinel, References: []appcatalog.Reference{{ID: shared.ID(ctKit), Name: "Kit Scrambler"}}}
}

func TestCatalog_MapsErrorsToProblems(t *testing.T) {
	t.Parallel()

	const base = "/api/v1/admin/catalog"
	item, kit := base+"/items/"+ctItem.String(), base+"/kits/"+ctKit.String()
	tests := []struct {
		name         string
		method, path string
		body         string
		err          error
		status       int
		code         string
	}{
		{"category name taken", "POST", base + "/categories", `{"name":"Bancos"}`, catalog.ErrNameTaken, 409, respond.CodeNameTaken},
		{"attribute name taken", "POST", base + "/attributes", ctAttributeBody, catalog.ErrNameTaken, 409, respond.CodeNameTaken},
		{"category in use", "DELETE", base + "/categories/" + ctCategory.String(), "", catalog.ErrCategoryInUse, 409, respond.CodeCategoryInUse},
		{"category gone", "PATCH", base + "/categories/" + ctCategory.String(), `{"name":"x"}`, catalog.ErrCategoryNotFound, 404, respond.CodeNotFound},
		{"item gone", "GET", item, "", catalog.ErrItemNotFound, 404, respond.CodeNotFound},
		{"stale item", "PUT", item, ctItemBody + `,"version":1}`, shared.ErrVersionConflict, 409, respond.CodeVersionConflict},
		{"stale kit", "PUT", kit, ctKitBody + `,"version":1}`, shared.ErrVersionConflict, 409, respond.CodeVersionConflict},
		{"stale supplier", "PUT", "/api/v1/admin/suppliers/" + ctSupplier.String(), `{"name":"x","whatsappPhone":"5511987654321","version":1}`, shared.ErrVersionConflict, 409, respond.CodeVersionConflict},
		{"item in a kit", "POST", item + "/archive", `{"version":1}`, refusal(catalog.ErrItemInKit), 409, respond.CodeItemInKit},
		{"kit with an archived item", "POST", base + "/kits", ctKitBody + `}`, refusal(catalog.ErrItemArchived), 409, respond.CodeItemArchived},
		{"kit restored with an archived item", "POST", kit + "/restore", `{"version":1}`, refusal(catalog.ErrItemArchived), 409, respond.CodeItemArchived},
		{"publish an archived item", "PUT", item + "/published", `{"published":true,"version":1}`, catalog.ErrPublishArchived, 400, respond.CodeInvalid},
		{"cost from a role that cannot", "POST", base + "/items", ctItemBody + `}`, appcatalog.ErrCostForbidden, 403, respond.CodeForbidden},
		{"no permission", "POST", base + "/categories", `{"name":"Bancos"}`, shared.ErrForbidden, 403, respond.CodeForbidden},
		{"photo limit", "POST", item + "/photos", `{"key":"k"}`, catalog.ErrPhotoLimit, 409, respond.CodePhotoLimit},
		{"photo limit on upload", "POST", base + "/photo-uploads", ctUploadBody, catalog.ErrPhotoLimit, 409, respond.CodePhotoLimit},
		{"photo invalid", "POST", item + "/photos", `{"key":"k"}`, catalog.ErrPhotoInvalid, 400, respond.CodePhotoInvalid},
		{"photo not uploaded", "POST", kit + "/photos", `{"key":"k"}`, catalog.ErrPhotoNotUploaded, 400, respond.CodePhotoNotUploaded},
		{"photo owner gone", "POST", base + "/photo-uploads", ctUploadBody, catalog.ErrItemNotFound, 404, respond.CodeNotFound},
		{"storage down", "POST", item + "/photos", `{"key":"k"}`, fmt.Errorf("storage: head: %w", shared.ErrUnavailable), 503, respond.CodeNotReady},
		{"supplier gone", "GET", "/api/v1/admin/suppliers/" + ctSupplier.String(), "", shared.ErrNotFound, 404, respond.CodeNotFound},
		{"no store", "GET", base + "/items", "", shared.ErrForbidden, 403, respond.CodeForbidden},
		{"a bug", "GET", base + "/kits", "", fmt.Errorf("boom"), 500, respond.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newCtFixture(t)
			f.fail(tt.err)
			rec := f.call(t, tt.method, tt.path, tt.body)
			if rec.Code != tt.status || code(t, rec) != tt.code {
				t.Errorf("answer %d %s, want %d %s", rec.Code, rec.Body.String(), tt.status, tt.code)
			}
			if tt.status == 503 && rec.Header().Get("Retry-After") == "" {
				t.Error("503 without Retry-After")
			}
		})
	}
}

// item_in_kit and item_archived name the rows in references.
func TestCatalog_RefusalsCarryReferences(t *testing.T) {
	t.Parallel()

	for _, sentinel := range []error{catalog.ErrItemInKit, catalog.ErrItemArchived} {
		f := newCtFixture(t)
		f.fail(refusal(sentinel))
		path := "/api/v1/admin/catalog/items/" + ctItem.String() + "/archive"
		if errors.Is(sentinel, catalog.ErrItemArchived) {
			path = "/api/v1/admin/catalog/kits/" + ctKit.String() + "/restore"
		}
		rec := f.call(t, "POST", path, `{"version":1}`)
		var p respond.Problem
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		if len(p.References) != 1 || p.References[0].ID != ctKit.String() || p.References[0].Name != "Kit Scrambler" {
			t.Errorf("%v: references = %+v, want the kit", sentinel, p.References)
		}
	}
}

// The photo codes have their own answers, not the generic invalid.
func TestCatalog_PhotoRefusalsAreNotGenericInvalid(t *testing.T) {
	t.Parallel()

	f := newCtFixture(t)
	f.fail(fmt.Errorf("confirm: %w", catalog.ErrPhotoNotUploaded))
	rec := f.call(t, "POST", "/api/v1/admin/catalog/items/"+ctItem.String()+"/photos", `{"key":"k"}`)
	if rec.Code != 400 || code(t, rec) != respond.CodePhotoNotUploaded {
		t.Errorf("answer %d %s, want 400 photo_not_uploaded", rec.Code, rec.Body.String())
	}
}

// A repeat confirm of a recorded key answers 200 with the same photo; a new
// photo is 201.
func TestCatalog_ConfirmPhotoAnswers200ForARepeat(t *testing.T) {
	t.Parallel()

	for _, owner := range []string{"items/" + ctItem.String(), "kits/" + ctKit.String()} {
		for _, again := range []bool{false, true} {
			f := newCtFixture(t)
			f.catalog.confirmation = appcatalog.PhotoConfirmation{AlreadyRecorded: again}
			rec := f.call(t, "POST", "/api/v1/admin/catalog/"+owner+"/photos", `{"key":"t/a/b/c/d"}`)
			want := http.StatusCreated
			if again {
				want = http.StatusOK
			}
			if rec.Code != want {
				t.Errorf("%s with AlreadyRecorded=%v answered %d, want %d", owner, again, rec.Code, want)
			}
			wantOwner := catalog.ItemPhotos(ctItem)
			if strings.HasPrefix(owner, "kits") {
				wantOwner = catalog.KitPhotos(ctKit)
			}
			if f.catalog.owner != wantOwner || f.catalog.got != "t/a/b/c/d" {
				t.Errorf("%s: use case got owner %+v key %v", owner, f.catalog.owner, f.catalog.got)
			}
		}
	}
}

func TestCatalog_PassesInput(t *testing.T) {
	t.Parallel()

	f := newCtFixture(t)
	f.call(t, "GET", "/api/v1/admin/catalog/items?cursor=abc&limit=5&q=ban&includeArchived=true&categoryId="+ctCategory.String(), "")
	got, _ := f.catalog.got.([]any)
	filter, page := got[0].(appcatalog.ItemFilter), got[1].(shared.PageRequest)
	if filter.Query != "ban" || !filter.IncludeArchived || filter.Category != ctCategory || page.Cursor() != "abc" || page.Limit() != 5 {
		t.Errorf("use case got %+v %q %d", filter, page.Cursor(), page.Limit())
	}

	f.call(t, "PUT", "/api/v1/admin/catalog/items/"+ctItem.String()+"/photos/order", fmt.Sprintf(`{"photoIds":[%q]}`, ctPhoto.String()))
	if ids, _ := f.catalog.got.([]shared.PhotoID); len(ids) != 1 || ids[0] != ctPhoto || f.catalog.owner != catalog.ItemPhotos(ctItem) {
		t.Errorf("order got %+v for %+v", f.catalog.got, f.catalog.owner)
	}

	f.call(t, "PATCH", "/api/v1/admin/catalog/categories/"+ctCategory.String(), `{"position":2}`)
	if in, _ := f.catalog.got.(appcatalog.UpdateCategoryInput); in.ID != ctCategory || in.Name != nil || in.Position == nil || *in.Position != 2 {
		t.Errorf("update category got %+v", f.catalog.got)
	}

	f.call(t, "PUT", "/api/v1/admin/catalog/kits/"+ctKit.String(), strings.Replace(ctKitBody, `"assemblyDays":null`, `"assemblyDays":4`, 1)+`,"version":3}`)
	in, _ := f.catalog.got.(appcatalog.ReplaceKitInput)
	if in.ID != ctKit || in.Version != 3 || in.Kit.AssemblyDays == nil || *in.Kit.AssemblyDays != 4 || in.Kit.SafetyMarginDays != nil ||
		in.Kit.SwapDiscount != catalog.SwapKeepDiscount || len(in.Kit.Items) != 1 || in.Kit.Items[0].Item != ctItem {
		t.Errorf("replace kit got %+v", in)
	}

	f.call(t, "POST", "/api/v1/admin/suppliers", `{"name":"Couros","whatsappPhone":"5511987654321"}`)
	if in, _ := f.suppliers.got.(appsupplier.CreateInput); in.Name != "Couros" || in.WhatsAppPhone != "5511987654321" {
		t.Errorf("create supplier got %+v", f.suppliers.got)
	}
}

// Replace bodies keep version: a flat struct, not an embedded input whose
// UnmarshalJSON would swallow it.
func TestCatalog_ReplaceReadsTheVersion(t *testing.T) {
	t.Parallel()

	f := newCtFixture(t)
	f.call(t, "PUT", "/api/v1/admin/catalog/items/"+ctItem.String(), ctItemBody+`,"version":7}`)
	if in, _ := f.catalog.got.(appcatalog.ReplaceItemInput); in.Version != 7 || in.ID != ctItem || in.Item.Name != "Banco" {
		t.Errorf("replace item got %+v", f.catalog.got)
	}
	f.call(t, "PUT", "/api/v1/admin/suppliers/"+ctSupplier.String(), `{"name":"Couros","whatsappPhone":"5511987654321","version":9}`)
	if in, _ := f.suppliers.got.(appsupplier.ReplaceInput); in.Version != 9 || in.ID != ctSupplier {
		t.Errorf("replace supplier got %+v", f.suppliers.got)
	}
}

// The cost keys of a request are tri-state: absent keeps the cost, null
// clears it, and a value sets it (ADR 0025).
func TestCatalog_CostKeysAreTriState(t *testing.T) {
	t.Parallel()

	sup := fmt.Sprintf(`{"supplierId":%q,"leadDays":7`, ctSupplier.String())
	tests := []struct {
		name       string
		supplier   string // the main supplier's cost key, with its comma
		production string
		wantMain   appcatalog.CostField
		wantProd   appcatalog.CostField
	}{
		{"both absent", ``, ``, appcatalog.KeepCost(), appcatalog.KeepCost()},
		{"both null", `,"costCents":null`, `,"productionCostCents":null`, appcatalog.ClearCost(), appcatalog.ClearCost()},
		{"both values", `,"costCents":4000`, `,"productionCostCents":1500`, appcatalog.SetCost(4000), appcatalog.SetCost(1500)},
		{"zero is a value", `,"costCents":0`, ``, appcatalog.SetCost(0), appcatalog.KeepCost()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newCtFixture(t)
			body := strings.Replace(ctItemBody, sup, sup+tt.supplier, 1) + tt.production + `}`
			if rec := f.call(t, "POST", "/api/v1/admin/catalog/items", body); rec.Code != 201 {
				t.Fatalf("answer %d %s", rec.Code, rec.Body.String())
			}
			in, _ := f.catalog.got.(appcatalog.ItemInput)
			if in.MainSupplier == nil || !reflect.DeepEqual(in.MainSupplier.Cost, tt.wantMain) || !reflect.DeepEqual(in.ProductionCost, tt.wantProd) {
				t.Errorf("main cost %+v, production cost %+v; want %+v and %+v", in.MainSupplier, in.ProductionCost, tt.wantMain, tt.wantProd)
			}
		})
	}
}

func TestCatalog_RateLimitsPhotoUploads(t *testing.T) {
	t.Parallel()

	for _, rule := range []ratelimit.Rule{ratelimit.PhotoUploadByUser, ratelimit.PhotoUploadByStore} {
		t.Run(rule.Kind, func(t *testing.T) {
			t.Parallel()

			f := newCtFixture(t)
			f.limiter.refuse[rule.Name+":"+rule.Kind] = true
			rec := f.call(t, "POST", "/api/v1/admin/catalog/photo-uploads", ctUploadBody)
			if rec.Code != 429 || code(t, rec) != respond.CodeRateLimited || rec.Header().Get("Retry-After") != "90" || f.uses() != 0 {
				t.Errorf("answer %d %s (Retry-After %q) after %d use-case calls, want 429 and none",
					rec.Code, rec.Body.String(), rec.Header().Get("Retry-After"), f.uses())
			}
		})
	}
}

// Both limits count each call from a caller who may write the catalog,
// under their own keys; a role that may not write is never counted, so a
// viewer cannot spend the store's budget.
func TestCatalog_PhotoUploadLimitsCountWritersOnly(t *testing.T) {
	t.Parallel()

	f := newCtFixture(t)
	f.call(t, "POST", "/api/v1/admin/catalog/photo-uploads", ctUploadBody)
	want := []string{ratelimit.PhotoUploadByUser.Key(userID.String()), ratelimit.PhotoUploadByStore.Key(tenantID.String())}
	if fmt.Sprint(f.limiter.keys) != fmt.Sprint(want) {
		t.Errorf("counted %v, want %v", f.limiter.keys, want)
	}

	f = newCtFixture(t)
	f.callAs(t, shared.RoleViewer, "POST", "/api/v1/admin/catalog/photo-uploads", ctUploadBody)
	if len(f.limiter.keys) != 0 {
		t.Errorf("a viewer's call was counted: %v", f.limiter.keys)
	}
}

// The other routes are not limited.
func TestCatalog_OnlyPhotoUploadsAreRateLimited(t *testing.T) {
	t.Parallel()

	f := newCtFixture(t)
	for _, op := range ctOps() {
		if op.pattern != "POST /api/v1/admin/catalog/photo-uploads" {
			f.call(t, op.method(), op.path(), op.body)
		}
	}
	if len(f.limiter.keys) != 0 {
		t.Errorf("counted %v", f.limiter.keys)
	}
}

func TestNewCatalog_NeedsItsDependencies(t *testing.T) {
	t.Parallel()

	log := discardLog()
	if _, err := handler.NewCatalog(handler.CatalogDeps{Limiter: &limiter{}, Log: log}); err == nil {
		t.Error("accepted no use cases")
	}
	if _, err := handler.NewCatalog(handler.CatalogDeps{UseCases: &catalogCases{}, Log: log}); err == nil {
		t.Error("accepted no limiter")
	}
	if _, err := handler.NewSuppliers(handler.SuppliersDeps{Log: log}); err == nil {
		t.Error("accepted no supplier use cases")
	}
}
