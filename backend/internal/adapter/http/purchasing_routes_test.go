package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// purchasingAdminPaths are the 16 milestone 5 operations of the tag
// `purchasing`, with an id in every path parameter.
var purchasingAdminPaths = []struct{ method, path string }{
	{"GET", "/api/v1/admin/purchasing/to-buy"},
	{"POST", "/api/v1/admin/purchasing/supplier-orders/preview"},
	{"GET", "/api/v1/admin/purchasing/supplier-orders"},
	{"POST", "/api/v1/admin/purchasing/supplier-orders"},
	{"GET", "/api/v1/admin/purchasing/supplier-orders/" + sampleID},
	{"PUT", "/api/v1/admin/purchasing/supplier-orders/" + sampleID + "/promised-date"},
	{"POST", "/api/v1/admin/purchasing/supplier-orders/" + sampleID + "/in-transit"},
	{"POST", "/api/v1/admin/purchasing/supplier-orders/" + sampleID + "/undo"},
	{"PUT", "/api/v1/admin/purchasing/supplier-orders/" + sampleID + "/paid-prices"},
	{"GET", "/api/v1/admin/purchasing/to-receive"},
	{"POST", "/api/v1/admin/purchasing/receipt-photo-uploads"},
	{"POST", "/api/v1/admin/purchasing/receipts"},
	{"GET", "/api/v1/admin/purchasing/notices"},
	{"POST", "/api/v1/admin/purchasing/notices/" + sampleID + "/resolve"},
	{"GET", "/api/v1/admin/purchasing/orders/" + sampleID + "/lines"},
	{"GET", "/api/v1/admin/purchasing/today"},
}

// newPurchasingBody returns a body that passes the handler's checks for the
// route, so a request reaches its use case; "" for a route with no body.
func newPurchasingBody(method, path string) string {
	switch {
	case strings.HasSuffix(path, "/supplier-orders/preview"), method == "POST" && strings.HasSuffix(path, "/supplier-orders"):
		return fmt.Sprintf(`{"supplierId":%q,"orderItemIds":[%q]}`, sampleID, sampleID)
	case strings.HasSuffix(path, "/promised-date"):
		return `{"version":1,"date":"2026-12-28"}`
	case strings.HasSuffix(path, "/in-transit"), strings.HasSuffix(path, "/undo"):
		return `{"version":1}`
	case strings.HasSuffix(path, "/paid-prices"):
		return fmt.Sprintf(`{"version":1,"prices":[{"lineId":%q,"cents":100}]}`, sampleID)
	case strings.HasSuffix(path, "/receipt-photo-uploads"):
		return `{"contentType":"image/jpeg"}`
	case strings.HasSuffix(path, "/purchasing/receipts"):
		return fmt.Sprintf(`{"entries":[{"lineId":%q,"check":"ok"}]}`, sampleID)
	}
	return ""
}

// purchasingReached is an apppurchasing.UseCases that counts each call into
// n and refuses it with ErrForbidden, as every admin route may.
type purchasingReached struct{ n *int }

var _ apppurchasing.UseCases = purchasingReached{}

func (u purchasingReached) hit() error { *u.n++; return shared.ErrForbidden }

func (u purchasingReached) ToBuy(context.Context) (apppurchasing.ToBuyView, error) {
	return apppurchasing.ToBuyView{}, u.hit()
}

func (u purchasingReached) Preview(context.Context, apppurchasing.PickInput) (string, error) {
	return "", u.hit()
}

func (u purchasingReached) Send(context.Context, apppurchasing.SendInput) (apppurchasing.SentView, error) {
	return apppurchasing.SentView{}, u.hit()
}

func (u purchasingReached) SupplierOrders(context.Context, bool, shared.PageRequest) (shared.Page[apppurchasing.SupplierOrderSummary], error) {
	return shared.Page[apppurchasing.SupplierOrderSummary]{}, u.hit()
}

func (u purchasingReached) SupplierOrder(context.Context, shared.SupplierOrderID) (apppurchasing.SupplierOrderView, error) {
	return apppurchasing.SupplierOrderView{}, u.hit()
}

func (u purchasingReached) RecordPromisedDate(context.Context, apppurchasing.PromisedDateInput) (apppurchasing.SupplierOrderView, error) {
	return apppurchasing.SupplierOrderView{}, u.hit()
}

func (u purchasingReached) MarkInTransit(context.Context, apppurchasing.VersionInput) (apppurchasing.SupplierOrderView, error) {
	return apppurchasing.SupplierOrderView{}, u.hit()
}

func (u purchasingReached) Undo(context.Context, apppurchasing.VersionInput) (apppurchasing.SupplierOrderView, error) {
	return apppurchasing.SupplierOrderView{}, u.hit()
}

func (u purchasingReached) RecordPaidPrices(context.Context, apppurchasing.PaidPricesInput) (apppurchasing.SupplierOrderView, error) {
	return apppurchasing.SupplierOrderView{}, u.hit()
}

func (u purchasingReached) ToReceive(context.Context) (apppurchasing.ToReceiveView, error) {
	return apppurchasing.ToReceiveView{}, u.hit()
}

func (u purchasingReached) RequestPhotoUpload(context.Context, string) (apppurchasing.PhotoUploadView, error) {
	return apppurchasing.PhotoUploadView{}, u.hit()
}

func (u purchasingReached) Receive(context.Context, apppurchasing.ReceiptInput) (apppurchasing.ReceiptView, error) {
	return apppurchasing.ReceiptView{}, u.hit()
}

func (u purchasingReached) Notices(context.Context) ([]apppurchasing.NoticeView, error) {
	return nil, u.hit()
}

func (u purchasingReached) ResolveNotice(context.Context, shared.PurchaseLineID) error {
	return u.hit()
}

func (u purchasingReached) OrderLines(context.Context, shared.OrderID) ([]apppurchasing.OrderLineView, error) {
	return nil, u.hit()
}

func (u purchasingReached) Today(context.Context) (apppurchasing.TodayView, error) {
	return apppurchasing.TodayView{}, u.hit()
}

// supplierItemsReached is a handler.SupplierItemsReader that counts and refuses.
type supplierItemsReached struct{ n *int }

func (u supplierItemsReached) SupplierItems(context.Context, shared.SupplierID) ([]appcatalog.SupplierItemUseView, error) {
	*u.n++
	return nil, shared.ErrForbidden
}

// The literal "preview" is its own route: the mux never reads it as a
// {supplierOrderId}, so a POST to it reaches Preview, and a GET to it is a
// path that names no supplier order.
func TestRouter_PreviewIsNotASupplierOrderID(t *testing.T) {
	t.Parallel()

	uc := &tenancyReached{}
	rec := httptest.NewRecorder()
	adminRouter(t, uc).ServeHTTP(rec, adminRequest("GET", "/api/v1/admin/purchasing/supplier-orders/preview", "staff"))
	if rec.Code != http.StatusNotFound || uc.calls != 0 {
		t.Errorf("GET .../preview answered %d after %d use-case calls, want 404 and none", rec.Code, uc.calls)
	}
}

// Every purchasing route answers only its own method: any other is 405.
func TestRouter_PurchasingRoutesAnswer405ToAnotherMethod(t *testing.T) {
	t.Parallel()

	for _, rt := range purchasingAdminPaths {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			t.Parallel()

			uc := &tenancyReached{}
			rec := httptest.NewRecorder()
			adminRouter(t, uc).ServeHTTP(rec, adminRequest("DELETE", rt.path, "staff"))
			if rec.Code != http.StatusMethodNotAllowed || uc.calls != 0 || !strings.Contains(rec.Header().Get("Allow"), rt.method) {
				t.Errorf("DELETE answered %d (Allow %q) after %d use-case calls, want 405 allowing %s",
					rec.Code, rec.Header().Get("Allow"), uc.calls, rt.method)
			}
		})
	}
}
