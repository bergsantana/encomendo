package http

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

const (
	sampleID   = "01936d1e-8b5a-7c2e-9f3a-000000000104"
	samplePath = "/api/v1/admin/catalog"
)

// catalogAdminPaths are the 29 catalog operations of the contract, and
// supplierAdminPaths the four supplier ones, with an id in every path
// parameter.
var catalogAdminPaths = []struct{ method, path string }{
	{"GET", samplePath + "/categories"},
	{"POST", samplePath + "/categories"},
	{"PATCH", samplePath + "/categories/" + sampleID},
	{"DELETE", samplePath + "/categories/" + sampleID},
	{"GET", samplePath + "/attributes"},
	{"POST", samplePath + "/attributes"},
	{"PUT", samplePath + "/attributes/" + sampleID},
	{"DELETE", samplePath + "/attributes/" + sampleID},
	{"GET", samplePath + "/items"},
	{"POST", samplePath + "/items"},
	{"GET", samplePath + "/items/" + sampleID},
	{"PUT", samplePath + "/items/" + sampleID},
	{"PUT", samplePath + "/items/" + sampleID + "/published"},
	{"POST", samplePath + "/items/" + sampleID + "/archive"},
	{"POST", samplePath + "/items/" + sampleID + "/restore"},
	{"POST", samplePath + "/items/" + sampleID + "/photos"},
	{"PUT", samplePath + "/items/" + sampleID + "/photos/order"},
	{"DELETE", samplePath + "/items/" + sampleID + "/photos/" + sampleID},
	{"GET", samplePath + "/kits"},
	{"POST", samplePath + "/kits"},
	{"GET", samplePath + "/kits/" + sampleID},
	{"PUT", samplePath + "/kits/" + sampleID},
	{"PUT", samplePath + "/kits/" + sampleID + "/published"},
	{"POST", samplePath + "/kits/" + sampleID + "/archive"},
	{"POST", samplePath + "/kits/" + sampleID + "/restore"},
	{"POST", samplePath + "/kits/" + sampleID + "/photos"},
	{"PUT", samplePath + "/kits/" + sampleID + "/photos/order"},
	{"DELETE", samplePath + "/kits/" + sampleID + "/photos/" + sampleID},
	{"POST", samplePath + "/photo-uploads"},
}

var supplierAdminPaths = []struct{ method, path string }{
	{"GET", "/api/v1/admin/suppliers"},
	{"POST", "/api/v1/admin/suppliers"},
	{"GET", "/api/v1/admin/suppliers/" + sampleID},
	{"PUT", "/api/v1/admin/suppliers/" + sampleID},
}

// newCatalogBody returns a body that passes the handler's checks for the
// route, so a request reaches its use case; "" for a route with no body.
func newCatalogBody(method, path string) (string, bool) {
	item := fmt.Sprintf(`{"name":"Banco","description":"","categoryId":null,"sourcingMode":"to_order","priceCents":10000,`+
		`"mainSupplier":{"supplierId":%q,"leadDays":7},"attributeValueIds":[],"options":[],"published":false`, sampleID)
	kit := fmt.Sprintf(`{"name":"Kit","description":"","priceCents":1,"swapDiscount":"keep","assemblyDays":null,"safetyMarginDays":null,`+
		`"items":[{"itemId":%q,"quantity":1,"swappable":false,"alternativeItemIds":[]}],"attributeValueIds":[],"published":false`, sampleID)
	switch {
	case path == samplePath+"/categories" || path == samplePath+"/categories/"+sampleID:
		return `{"name":"Bancos"}`, method != "DELETE"
	case strings.HasPrefix(path, samplePath+"/attributes") && method != "GET" && method != "DELETE":
		return `{"name":"Serve em","values":[]}`, true
	case strings.HasSuffix(path, "/items") && method == "POST":
		return item + "}", true
	case strings.HasSuffix(path, "/items/"+sampleID) && method == "PUT":
		return item + `,"version":1}`, true
	case strings.HasSuffix(path, "/kits") && method == "POST":
		return kit + "}", true
	case strings.HasSuffix(path, "/kits/"+sampleID) && method == "PUT":
		return kit + `,"version":1}`, true
	case strings.HasSuffix(path, "/published"):
		return `{"published":true,"version":1}`, true
	case strings.HasSuffix(path, "/archive"), strings.HasSuffix(path, "/restore"):
		return `{"version":1}`, true
	case strings.HasSuffix(path, "/photos") && method == "POST":
		return `{"key":"k"}`, true
	case strings.HasSuffix(path, "/photos/order"):
		return `{"photoIds":[]}`, true
	case strings.HasSuffix(path, "/photo-uploads"):
		return fmt.Sprintf(`{"ownerType":"item","ownerId":%q,"contentType":"image/jpeg"}`, sampleID), true
	case path == "/api/v1/admin/suppliers" && method == "POST":
		return `{"name":"Couros","whatsappPhone":"5511987654321"}`, true
	case strings.HasPrefix(path, "/api/v1/admin/suppliers/") && method == "PUT":
		return `{"name":"Couros","whatsappPhone":"5511987654321","version":1}`, true
	}
	return "", false
}

// catalogReached is an appcatalog.UseCases that counts each call into n and
// refuses it with ErrForbidden, as every admin route may.
type catalogReached struct{ n *int }

var _ appcatalog.UseCases = catalogReached{}

func (u catalogReached) hit() error { *u.n++; return shared.ErrForbidden }

func (u catalogReached) ListCategories(context.Context) ([]appcatalog.CategoryView, error) {
	return nil, u.hit()
}

func (u catalogReached) CreateCategory(context.Context, appcatalog.CreateCategoryInput) (appcatalog.CategoryView, error) {
	return appcatalog.CategoryView{}, u.hit()
}

func (u catalogReached) UpdateCategory(context.Context, appcatalog.UpdateCategoryInput) (appcatalog.CategoryView, error) {
	return appcatalog.CategoryView{}, u.hit()
}

func (u catalogReached) DeleteCategory(context.Context, shared.CategoryID) error { return u.hit() }

func (u catalogReached) ListAttributes(context.Context) ([]appcatalog.AttributeView, error) {
	return nil, u.hit()
}

func (u catalogReached) CreateAttribute(context.Context, appcatalog.AttributeInput) (appcatalog.AttributeView, error) {
	return appcatalog.AttributeView{}, u.hit()
}

func (u catalogReached) ReplaceAttribute(context.Context, appcatalog.ReplaceAttributeInput) (appcatalog.AttributeView, error) {
	return appcatalog.AttributeView{}, u.hit()
}

func (u catalogReached) DeleteAttribute(context.Context, shared.AttributeID) error { return u.hit() }

func (u catalogReached) ListItems(context.Context, appcatalog.ItemFilter, shared.PageRequest) (shared.Page[appcatalog.ItemSummaryView], error) {
	return shared.Page[appcatalog.ItemSummaryView]{}, u.hit()
}

func (u catalogReached) GetItem(context.Context, shared.ItemID) (appcatalog.ItemView, error) {
	return appcatalog.ItemView{}, u.hit()
}

func (u catalogReached) CreateItem(context.Context, appcatalog.ItemInput) (appcatalog.ItemView, error) {
	return appcatalog.ItemView{}, u.hit()
}

func (u catalogReached) ReplaceItem(context.Context, appcatalog.ReplaceItemInput) (appcatalog.ItemView, error) {
	return appcatalog.ItemView{}, u.hit()
}

func (u catalogReached) SetItemPublished(context.Context, appcatalog.SetItemPublishedInput) (appcatalog.ItemView, error) {
	return appcatalog.ItemView{}, u.hit()
}

func (u catalogReached) ArchiveItem(context.Context, appcatalog.ItemVersionInput) (appcatalog.ItemView, error) {
	return appcatalog.ItemView{}, u.hit()
}

func (u catalogReached) RestoreItem(context.Context, appcatalog.ItemVersionInput) (appcatalog.ItemView, error) {
	return appcatalog.ItemView{}, u.hit()
}

func (u catalogReached) ListKits(context.Context, appcatalog.KitFilter, shared.PageRequest) (shared.Page[appcatalog.KitSummaryView], error) {
	return shared.Page[appcatalog.KitSummaryView]{}, u.hit()
}

func (u catalogReached) GetKit(context.Context, shared.KitID) (appcatalog.KitView, error) {
	return appcatalog.KitView{}, u.hit()
}

func (u catalogReached) CreateKit(context.Context, appcatalog.KitInput) (appcatalog.KitView, error) {
	return appcatalog.KitView{}, u.hit()
}

func (u catalogReached) ReplaceKit(context.Context, appcatalog.ReplaceKitInput) (appcatalog.KitView, error) {
	return appcatalog.KitView{}, u.hit()
}

func (u catalogReached) SetKitPublished(context.Context, appcatalog.SetKitPublishedInput) (appcatalog.KitView, error) {
	return appcatalog.KitView{}, u.hit()
}

func (u catalogReached) ArchiveKit(context.Context, appcatalog.KitVersionInput) (appcatalog.KitView, error) {
	return appcatalog.KitView{}, u.hit()
}

func (u catalogReached) RestoreKit(context.Context, appcatalog.KitVersionInput) (appcatalog.KitView, error) {
	return appcatalog.KitView{}, u.hit()
}

func (u catalogReached) RequestPhotoUpload(context.Context, appcatalog.PhotoUploadInput) (appcatalog.PhotoUploadView, error) {
	return appcatalog.PhotoUploadView{}, u.hit()
}

func (u catalogReached) ConfirmPhoto(context.Context, catalog.PhotoOwner, string) (appcatalog.PhotoConfirmation, error) {
	return appcatalog.PhotoConfirmation{}, u.hit()
}

func (u catalogReached) OrderPhotos(context.Context, catalog.PhotoOwner, []shared.PhotoID) ([]appcatalog.PhotoView, error) {
	return nil, u.hit()
}

func (u catalogReached) DeletePhoto(context.Context, catalog.PhotoOwner, shared.PhotoID) error {
	return u.hit()
}

// suppliersReached is an appsupplier.UseCases that counts and refuses.
type suppliersReached struct{ n *int }

var _ appsupplier.UseCases = suppliersReached{}

func (u suppliersReached) hit() error { *u.n++; return shared.ErrForbidden }

func (u suppliersReached) Create(context.Context, appsupplier.CreateInput) (appsupplier.View, error) {
	return appsupplier.View{}, u.hit()
}

func (u suppliersReached) Replace(context.Context, appsupplier.ReplaceInput) (appsupplier.View, error) {
	return appsupplier.View{}, u.hit()
}

func (u suppliersReached) Get(context.Context, shared.SupplierID) (appsupplier.View, error) {
	return appsupplier.View{}, u.hit()
}

func (u suppliersReached) List(context.Context, appsupplier.ListFilter, shared.PageRequest) (shared.Page[appsupplier.View], error) {
	return shared.Page[appsupplier.View]{}, u.hit()
}

func (u suppliersReached) Names(context.Context, []shared.SupplierID) (map[shared.SupplierID]string, error) {
	return nil, u.hit()
}

// Any admin group, not only tenancy, needs authn and the suspended-store
// check: the router refuses to mount it without them.
func TestNewRouter_RefusesCatalogWithoutAuthn(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	calls := 0
	cat, err := handler.NewCatalog(handler.CatalogDeps{UseCases: catalogReached{n: &calls}, Limiter: &countingLimiter{}, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	sup, err := handler.NewSuppliers(handler.SuppliersDeps{UseCases: suppliersReached{n: &calls}, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	for name, h := range map[string]Handlers{
		"catalog":   {Health: handler.NewHealth(log, time.Second), Catalog: cat},
		"suppliers": {Health: handler.NewHealth(log, time.Second), Suppliers: sup},
	} {
		if _, err := NewRouter(log, h, RouterOptions{}); err == nil {
			t.Errorf("the %s routes were mounted without authn", name)
		}
	}
}
