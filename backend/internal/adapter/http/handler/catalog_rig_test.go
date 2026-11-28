package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// ctID is a fixed id for fixtures: a UUID the contract's `uuid` format
// accepts, whose last digits are n.
func ctID(n int) shared.ID { return mustID(fmt.Sprintf("01936d1e-8b5a-7c2e-9f3a-%012d", n)) }

var (
	ctCategory  = shared.CategoryID(ctID(101))
	ctAttribute = shared.AttributeID(ctID(102))
	ctValue     = shared.AttributeValueID(ctID(103))
	ctItem      = shared.ItemID(ctID(104))
	ctKit       = shared.KitID(ctID(105))
	ctPhoto     = shared.PhotoID(ctID(106))
	ctSupplier  = shared.SupplierID(ctID(107))
	ctOption    = shared.OptionID(ctID(108))
	ctChoice    = shared.ChoiceID(ctID(109))
	ctWhen      = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
)

// ctPaths fills the path parameters of a route pattern with fixture ids.
var ctPaths = strings.NewReplacer(
	"{categoryId}", ctCategory.String(), "{attributeId}", ctAttribute.String(), "{itemId}", ctItem.String(),
	"{kitId}", ctKit.String(), "{photoId}", ctPhoto.String(), "{supplierId}", ctSupplier.String(),
)

func ctMoney(cents int64) shared.Money {
	m, err := shared.NewMoney(cents, "BRL")
	if err != nil {
		panic(err)
	}
	return m
}

func ctPhotoView() appcatalog.PhotoView {
	return appcatalog.PhotoView{ID: ctPhoto, URL: "http://localhost:9000/photo?sig=1", ExpiresAt: ctWhen.Add(10 * time.Minute), Position: 0}
}

// ctItemView is an item an owner sees in full: with a cost, a margin and a
// photo.
func ctItemView() appcatalog.ItemView {
	cost, backupCost := ctMoney(4000), ctMoney(4500)
	return appcatalog.ItemView{
		ID: ctItem, Name: "Banco Scrambler", CategoryID: ctCategory, SourcingMode: catalog.ModeToOrder, Price: ctMoney(10000),
		ReadyDays: 7, MainSupplier: &appcatalog.SupplierItemView{SupplierID: ctSupplier, SupplierName: "Couros Paulista", LeadDays: 7},
		BackupSupplier:    &appcatalog.SupplierItemView{SupplierID: ctSupplier, SupplierName: "Couros Paulista", LeadDays: 9},
		AttributeValueIDs: []shared.AttributeValueID{ctValue},
		Options: []appcatalog.OptionView{{ID: ctOption, Name: "Cor", Required: true, Choices: []appcatalog.ChoiceView{
			{ID: ctChoice, Label: "Preta", ExtraPrice: ctMoney(0)},
		}}},
		Photos: []appcatalog.PhotoView{ctPhotoView()}, Version: 1,
		Costs: &appcatalog.ItemCosts{
			Cost: &cost, MainSupplierCost: &cost, BackupSupplierCost: &backupCost,
			Margin: &catalog.Margin{Cents: 6000, BasisPoints: 6000},
		},
	}
}

func ctKitView() appcatalog.KitView {
	cost := ctMoney(8000)
	return appcatalog.KitView{
		ID: ctKit, Name: "Kit Scrambler", Price: ctMoney(25000), SingleItemsTotal: ctMoney(27000), Discount: ctMoney(2000),
		SwapDiscount: catalog.SwapKeepDiscount, Days: catalog.KitDays{SlowestItem: 15, Assembly: 3, SafetyMargin: 2, Total: 20},
		Items: []appcatalog.KitItemView{{
			ItemID: ctItem, ItemName: "Banco Scrambler", Quantity: 1, Swappable: true,
			AlternativeItemIDs: []shared.ItemID{ctItem}, UnitPrice: ctMoney(10000), ReadyDays: 7,
		}},
		Photos: []appcatalog.PhotoView{ctPhotoView()}, Version: 1,
		Costs: &appcatalog.Costs{Cost: &cost, Margin: &catalog.Margin{Cents: 17000, BasisPoints: 6800}},
	}
}

func ctSupplierView() appsupplier.View {
	return appsupplier.View{ID: ctSupplier, Name: "Couros Paulista", WhatsAppPhone: "5511987654321", Version: 1}
}

// catalogCases is a fake appcatalog.UseCases: every call counts, records
// its input and answers a fixture view, or err.
type catalogCases struct {
	err   error
	calls int
	got   any
	owner catalog.PhotoOwner
	// confirmation is what ConfirmPhoto answers.
	confirmation appcatalog.PhotoConfirmation
}

var (
	_ appcatalog.UseCases         = (*catalogCases)(nil)
	_ handler.SupplierItemsReader = (*catalogCases)(nil)
)

// SupplierItems answers the items of a supplier, with the owner's cost the
// way the real use case gives it to the owner only: the fake always sets
// it, so the handler's own gate is what a test of a lesser role exercises.
func (u *catalogCases) SupplierItems(_ context.Context, id shared.SupplierID) ([]appcatalog.SupplierItemUseView, error) {
	u.record(id)
	cost := ctMoney(4000)
	return []appcatalog.SupplierItemUseView{
		{Item: ctItem, Name: "Banco de couro", Role: supplier.RoleMain, LeadDays: 7, Cost: &cost},
		{Item: ctItem, Name: "Capa", Role: supplier.RoleBackup, LeadDays: 9},
	}, u.err
}

func (u *catalogCases) record(in any) { u.calls++; u.got = in }

func (u *catalogCases) ListCategories(context.Context) ([]appcatalog.CategoryView, error) {
	u.record(nil)
	return []appcatalog.CategoryView{{ID: ctCategory, Name: "Bancos"}}, u.err
}

func (u *catalogCases) CreateCategory(_ context.Context, in appcatalog.CreateCategoryInput) (appcatalog.CategoryView, error) {
	u.record(in)
	return appcatalog.CategoryView{ID: ctCategory, Name: in.Name}, u.err
}

func (u *catalogCases) UpdateCategory(_ context.Context, in appcatalog.UpdateCategoryInput) (appcatalog.CategoryView, error) {
	u.record(in)
	return appcatalog.CategoryView{ID: ctCategory, Name: "Bancos"}, u.err
}

func (u *catalogCases) DeleteCategory(_ context.Context, id shared.CategoryID) error {
	u.record(id)
	return u.err
}

func ctAttributeView() appcatalog.AttributeView {
	return appcatalog.AttributeView{ID: ctAttribute, Name: "Serve em", Values: []appcatalog.AttributeValueView{{ID: ctValue, Label: "CB 500X", UsageCount: 2}}}
}

func (u *catalogCases) ListAttributes(context.Context) ([]appcatalog.AttributeView, error) {
	u.record(nil)
	return []appcatalog.AttributeView{ctAttributeView()}, u.err
}

func (u *catalogCases) CreateAttribute(_ context.Context, in appcatalog.AttributeInput) (appcatalog.AttributeView, error) {
	u.record(in)
	return ctAttributeView(), u.err
}

func (u *catalogCases) ReplaceAttribute(_ context.Context, in appcatalog.ReplaceAttributeInput) (appcatalog.AttributeView, error) {
	u.record(in)
	return ctAttributeView(), u.err
}

func (u *catalogCases) DeleteAttribute(_ context.Context, id shared.AttributeID) error {
	u.record(id)
	return u.err
}

func (u *catalogCases) ListItems(_ context.Context, f appcatalog.ItemFilter, page shared.PageRequest) (shared.Page[appcatalog.ItemSummaryView], error) {
	u.record([]any{f, page})
	row := appcatalog.ItemSummaryView{
		ID: ctItem, Name: "Banco Scrambler", SourcingMode: catalog.ModeToOrder, Price: ctMoney(10000), ReadyDays: 7,
		MainSupplierName: "Couros Paulista", Version: 1,
	}
	return shared.Page[appcatalog.ItemSummaryView]{Items: []appcatalog.ItemSummaryView{row}, NextCursor: "next"}, u.err
}

func (u *catalogCases) GetItem(_ context.Context, id shared.ItemID) (appcatalog.ItemView, error) {
	u.record(id)
	return ctItemView(), u.err
}

func (u *catalogCases) CreateItem(_ context.Context, in appcatalog.ItemInput) (appcatalog.ItemView, error) {
	u.record(in)
	return ctItemView(), u.err
}

func (u *catalogCases) ReplaceItem(_ context.Context, in appcatalog.ReplaceItemInput) (appcatalog.ItemView, error) {
	u.record(in)
	return ctItemView(), u.err
}

func (u *catalogCases) SetItemPublished(_ context.Context, in appcatalog.SetItemPublishedInput) (appcatalog.ItemView, error) {
	u.record(in)
	return ctItemView(), u.err
}

func (u *catalogCases) ArchiveItem(_ context.Context, in appcatalog.ItemVersionInput) (appcatalog.ItemView, error) {
	u.record(in)
	return ctItemView(), u.err
}

func (u *catalogCases) RestoreItem(_ context.Context, in appcatalog.ItemVersionInput) (appcatalog.ItemView, error) {
	u.record(in)
	return ctItemView(), u.err
}

func (u *catalogCases) ListKits(_ context.Context, f appcatalog.KitFilter, page shared.PageRequest) (shared.Page[appcatalog.KitSummaryView], error) {
	u.record([]any{f, page})
	row := appcatalog.KitSummaryView{ID: ctKit, Name: "Kit Scrambler", ItemCount: 1, Price: ctMoney(25000), SingleItemsTotal: ctMoney(27000), TotalDays: 20, Version: 1}
	return shared.Page[appcatalog.KitSummaryView]{Items: []appcatalog.KitSummaryView{row}}, u.err
}

func (u *catalogCases) GetKit(_ context.Context, id shared.KitID) (appcatalog.KitView, error) {
	u.record(id)
	return ctKitView(), u.err
}

func (u *catalogCases) CreateKit(_ context.Context, in appcatalog.KitInput) (appcatalog.KitView, error) {
	u.record(in)
	return ctKitView(), u.err
}

func (u *catalogCases) ReplaceKit(_ context.Context, in appcatalog.ReplaceKitInput) (appcatalog.KitView, error) {
	u.record(in)
	return ctKitView(), u.err
}

func (u *catalogCases) SetKitPublished(_ context.Context, in appcatalog.SetKitPublishedInput) (appcatalog.KitView, error) {
	u.record(in)
	return ctKitView(), u.err
}

func (u *catalogCases) ArchiveKit(_ context.Context, in appcatalog.KitVersionInput) (appcatalog.KitView, error) {
	u.record(in)
	return ctKitView(), u.err
}

func (u *catalogCases) RestoreKit(_ context.Context, in appcatalog.KitVersionInput) (appcatalog.KitView, error) {
	u.record(in)
	return ctKitView(), u.err
}

func (u *catalogCases) RequestPhotoUpload(_ context.Context, in appcatalog.PhotoUploadInput) (appcatalog.PhotoUploadView, error) {
	u.record(in)
	return appcatalog.PhotoUploadView{
		Key: "t/x/items/y/z", URL: "http://localhost:9000/upload?sig=1", Method: "PUT",
		Headers: map[string]string{"Content-Type": in.ContentType}, ExpiresAt: ctWhen.Add(5 * time.Minute),
	}, u.err
}

func (u *catalogCases) ConfirmPhoto(_ context.Context, owner catalog.PhotoOwner, key string) (appcatalog.PhotoConfirmation, error) {
	u.record(key)
	u.owner = owner
	if u.confirmation.Photo.ID.IsZero() {
		u.confirmation.Photo = ctPhotoView()
	}
	return u.confirmation, u.err
}

func (u *catalogCases) OrderPhotos(_ context.Context, owner catalog.PhotoOwner, ids []shared.PhotoID) ([]appcatalog.PhotoView, error) {
	u.record(ids)
	u.owner = owner
	return []appcatalog.PhotoView{ctPhotoView()}, u.err
}

func (u *catalogCases) DeletePhoto(_ context.Context, owner catalog.PhotoOwner, id shared.PhotoID) error {
	u.record(id)
	u.owner = owner
	return u.err
}

// supplierCases is a fake appsupplier.UseCases.
type supplierCases struct {
	err   error
	calls int
	got   any
}

var _ appsupplier.UseCases = (*supplierCases)(nil)

func (u *supplierCases) Create(_ context.Context, in appsupplier.CreateInput) (appsupplier.View, error) {
	u.calls++
	u.got = in
	return ctSupplierView(), u.err
}

func (u *supplierCases) Replace(_ context.Context, in appsupplier.ReplaceInput) (appsupplier.View, error) {
	u.calls++
	u.got = in
	return ctSupplierView(), u.err
}

func (u *supplierCases) Get(_ context.Context, id shared.SupplierID) (appsupplier.View, error) {
	u.calls++
	u.got = id
	return ctSupplierView(), u.err
}

func (u *supplierCases) List(_ context.Context, f appsupplier.ListFilter, page shared.PageRequest) (shared.Page[appsupplier.View], error) {
	u.calls++
	u.got = []any{f, page}
	return shared.Page[appsupplier.View]{Items: []appsupplier.View{ctSupplierView()}}, u.err
}

func (u *supplierCases) Names(context.Context, []shared.SupplierID) (map[shared.SupplierID]string, error) {
	u.calls++
	return nil, u.err
}

// ctRoutes is every catalog and supplier route, as the router mounts it.
func ctRoutes(c *handler.Catalog, s *handler.Suppliers) map[string]http.HandlerFunc {
	const base = "/api/v1/admin/catalog"
	return map[string]http.HandlerFunc{
		"GET " + base + "/categories":                         c.ListCategories,
		"POST " + base + "/categories":                        c.CreateCategory,
		"PATCH " + base + "/categories/{categoryId}":          c.UpdateCategory,
		"DELETE " + base + "/categories/{categoryId}":         c.DeleteCategory,
		"GET " + base + "/attributes":                         c.ListAttributes,
		"POST " + base + "/attributes":                        c.CreateAttribute,
		"PUT " + base + "/attributes/{attributeId}":           c.ReplaceAttribute,
		"DELETE " + base + "/attributes/{attributeId}":        c.DeleteAttribute,
		"GET " + base + "/items":                              c.ListItems,
		"POST " + base + "/items":                             c.CreateItem,
		"GET " + base + "/items/{itemId}":                     c.GetItem,
		"PUT " + base + "/items/{itemId}":                     c.ReplaceItem,
		"PUT " + base + "/items/{itemId}/published":           c.SetItemPublished,
		"POST " + base + "/items/{itemId}/archive":            c.ArchiveItem,
		"POST " + base + "/items/{itemId}/restore":            c.RestoreItem,
		"POST " + base + "/items/{itemId}/photos":             c.ConfirmItemPhoto,
		"PUT " + base + "/items/{itemId}/photos/order":        c.OrderItemPhotos,
		"DELETE " + base + "/items/{itemId}/photos/{photoId}": c.DeleteItemPhoto,
		"GET " + base + "/kits":                               c.ListKits,
		"POST " + base + "/kits":                              c.CreateKit,
		"GET " + base + "/kits/{kitId}":                       c.GetKit,
		"PUT " + base + "/kits/{kitId}":                       c.ReplaceKit,
		"PUT " + base + "/kits/{kitId}/published":             c.SetKitPublished,
		"POST " + base + "/kits/{kitId}/archive":              c.ArchiveKit,
		"POST " + base + "/kits/{kitId}/restore":              c.RestoreKit,
		"POST " + base + "/kits/{kitId}/photos":               c.ConfirmKitPhoto,
		"PUT " + base + "/kits/{kitId}/photos/order":          c.OrderKitPhotos,
		"DELETE " + base + "/kits/{kitId}/photos/{photoId}":   c.DeleteKitPhoto,
		"POST " + base + "/photo-uploads":                     c.RequestPhotoUpload,
		"GET /api/v1/admin/suppliers":                         s.List,
		"POST /api/v1/admin/suppliers":                        s.Create,
		"GET /api/v1/admin/suppliers/{supplierId}":            s.Get,
		"GET /api/v1/admin/suppliers/{supplierId}/items":      s.Items,
		"PUT /api/v1/admin/suppliers/{supplierId}":            s.Replace,
	}
}

// ctServe sends method path to the catalog and supplier handlers as caller
// and checks the answer against the contract.
func ctServe(t *testing.T, c *handler.Catalog, s *handler.Suppliers, caller shared.Principal, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, h := range ctRoutes(c, s) {
		mux.HandleFunc(pattern, h)
	}
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r = r.WithContext(authz.WithPrincipal(r.Context(), caller))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	checkContract(t, r, rec)
	return rec
}

// ctFixture is the handlers over fake use cases.
type ctFixture struct {
	catalog   *catalogCases
	suppliers *supplierCases
	limiter   *limiter
	c         *handler.Catalog
	s         *handler.Suppliers
}

func newCtFixture(t *testing.T) *ctFixture {
	t.Helper()
	f := &ctFixture{catalog: &catalogCases{}, suppliers: &supplierCases{}, limiter: &limiter{refuse: map[string]bool{}}}
	var err error
	if f.c, err = handler.NewCatalog(handler.CatalogDeps{UseCases: f.catalog, Limiter: f.limiter, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	if f.s, err = handler.NewSuppliers(handler.SuppliersDeps{UseCases: f.suppliers, Items: f.catalog, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	return f
}

// fail makes every fake use case answer err.
func (f *ctFixture) fail(err error) { f.catalog.err, f.suppliers.err = err, err }

// call sends the request as the store's owner.
func (f *ctFixture) call(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return f.callAs(t, shared.RoleOwner, method, path, body)
}

func (f *ctFixture) callAs(t *testing.T, role shared.Role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return ctServe(t, f.c, f.s, staffIn(t, role), method, path, body)
}

// uses returns how many use-case calls the fakes saw.
func (f *ctFixture) uses() int { return f.catalog.calls + f.suppliers.calls }
