package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Fixture ids of the purchasing handlers.
var (
	puSupplierOrder = shared.SupplierOrderID(ctID(301))
	puLine          = shared.PurchaseLineID(ctID(302))
	puLine2         = shared.PurchaseLineID(ctID(303))
	puReceipt       = shared.ReceiptID(ctID(304))
	puSupplier      = shared.SupplierID(ctID(305))
)

// puPaths fills the path parameters of a purchasing route with fixture ids.
var puPaths = strings.NewReplacer(
	"{supplierOrderId}", puSupplierOrder.String(), "{lineId}", puLine.String(), "{orderId}", orOrder.String(),
)

func puChoices() []apppurchasing.ChoiceView {
	return []apppurchasing.ChoiceView{{Name: "Cor da costura", Label: "Vermelha"}}
}

// puLines are the two lines of the fixture pedido de compra: one with a
// promised date and costs, one with a notice and none.
func puLines(costs bool) []apppurchasing.PurchaseLineView {
	cost, paid := ctMoney(4000), ctMoney(3800)
	first := apppurchasing.PurchaseLineView{
		ID: puLine, Order: orOrder, OrderNumber: "P-0231", OrderItem: orItem, CatalogItem: ctItem, Name: "Banco de couro", Quantity: 2,
		Choices: puChoices(), Status: purchasing.LineSent, Promised: orDate(18), Expected: orDate(20), ArrivalLimit: orDate(25),
		Alerts: []schedule.Alert{schedule.AlertAtRisk},
	}
	removed := apppurchasing.PurchaseLineView{
		ID: puLine2, Order: orOrder, OrderNumber: "P-0231", CatalogItem: ctItem, Name: "Capa", Quantity: 1, Choices: nil,
		Status: purchasing.LineDefective, CheckNote: "Veio rasgado", Notice: purchasing.NoticeItemRemoved,
	}
	if costs {
		first.Cost, first.Paid = &cost, &paid
		removed.Cost = &cost
	}
	return []apppurchasing.PurchaseLineView{first, removed}
}

// puSupplierOrderView is the pedido de compra C-0012 as the use cases
// answer it: costs and prices paid only when costs is set, as the owner's
// view has them.
func puSupplierOrderView(costs bool) apppurchasing.SupplierOrderView {
	return apppurchasing.SupplierOrderView{
		SupplierOrderSummary: apppurchasing.SupplierOrderSummary{
			ID: puSupplierOrder, Number: 12, Supplier: puSupplier, SupplierName: "Couros Paulista", SentAt: ctWhen,
			Promised: orDate(18), Received: 1, Total: 2, State: purchasing.StateSent,
		},
		SentBy: apppurchasing.ActorView{ID: orUser, Name: "Ana Souza"}, WhatsAppURL: "https://wa.me/5511987654321",
		Lines: puLines(costs), CanUndo: false, Version: 3,
	}
}

func puToBuyView(costs bool) apppurchasing.ToBuyView {
	cost := ctMoney(4000)
	item := apppurchasing.ToBuyItemView{
		Order: orOrder, OrderNumber: "P-0231", OrderItem: orItem, CatalogItem: ctItem, Name: "Banco de couro", Quantity: 2,
		Choices: puChoices(), ArrivalLimit: orDate(25), Replacement: true,
	}
	if costs {
		item.Cost = &cost
	}
	return apppurchasing.ToBuyView{Suppliers: []apppurchasing.ToBuySupplierView{
		{Supplier: puSupplier, Name: "Couros Paulista", Phone: "5511987654321", Items: []apppurchasing.ToBuyItemView{item}},
		{Supplier: shared.SupplierID(ctID(306)), Name: "Sem telefone", Items: nil},
	}}
}

// purchasingCases is a fake apppurchasing.UseCases. Every call counts and
// records its input, and answers the fixtures (costs for the owner's view
// when costs is set; set regardless of the caller, so a test of a lesser
// role proves the handler's own gate), or err. A send or a receipt without an
// Idempotency-Key answers ErrIdempotencyKeyRequired, as the real use case.
type purchasingCases struct {
	err   error
	calls int
	got   any
	costs bool
}

var _ apppurchasing.UseCases = (*purchasingCases)(nil)

func (u *purchasingCases) record(in any) { u.calls++; u.got = in }

func (u *purchasingCases) ToBuy(context.Context) (apppurchasing.ToBuyView, error) {
	u.record(nil)
	return puToBuyView(u.costs), u.err
}

func (u *purchasingCases) Preview(_ context.Context, in apppurchasing.PickInput) (string, error) {
	u.record(in)
	return "Pedido C-0012 para Couros Paulista\n2 x Banco de couro", u.err
}

func (u *purchasingCases) Send(_ context.Context, in apppurchasing.SendInput) (apppurchasing.SentView, error) {
	u.record(in)
	if u.err == nil && in.Idempotency.IsZero() {
		return apppurchasing.SentView{}, apppurchasing.ErrIdempotencyKeyRequired
	}
	return apppurchasing.SentView{
		SupplierOrder: puSupplierOrderView(u.costs), WhatsAppURL: "https://wa.me/5511987654321?text=Pedido", Text: "Pedido C-0012",
	}, u.err
}

func (u *purchasingCases) SupplierOrders(_ context.Context, open bool, p shared.PageRequest) (shared.Page[apppurchasing.SupplierOrderSummary], error) {
	u.record([]any{open, p})
	undone := puSupplierOrderView(false).SupplierOrderSummary
	undone.Promised, undone.State, undone.Total = shared.CivilDate{}, purchasing.StateUndone, 0
	return shared.Page[apppurchasing.SupplierOrderSummary]{
		Items: []apppurchasing.SupplierOrderSummary{puSupplierOrderView(false).SupplierOrderSummary, undone}, NextCursor: "next",
	}, u.err
}

func (u *purchasingCases) SupplierOrder(_ context.Context, id shared.SupplierOrderID) (apppurchasing.SupplierOrderView, error) {
	u.record(id)
	return puSupplierOrderView(u.costs), u.err
}

func (u *purchasingCases) RecordPromisedDate(_ context.Context, in apppurchasing.PromisedDateInput) (apppurchasing.SupplierOrderView, error) {
	u.record(in)
	return puSupplierOrderView(u.costs), u.err
}

func (u *purchasingCases) MarkInTransit(_ context.Context, in apppurchasing.VersionInput) (apppurchasing.SupplierOrderView, error) {
	u.record(in)
	return puSupplierOrderView(u.costs), u.err
}

func (u *purchasingCases) Undo(_ context.Context, in apppurchasing.VersionInput) (apppurchasing.SupplierOrderView, error) {
	u.record(in)
	return puSupplierOrderView(u.costs), u.err
}

func (u *purchasingCases) RecordPaidPrices(_ context.Context, in apppurchasing.PaidPricesInput) (apppurchasing.SupplierOrderView, error) {
	u.record(in)
	return puSupplierOrderView(u.costs), u.err
}

func puToReceiveLine(line shared.PurchaseLineID) apppurchasing.ToReceiveLineView {
	return apppurchasing.ToReceiveLineView{
		Line: line, Order: orOrder, OrderNumber: "P-0231", OrderItem: orItem, Name: "Banco de couro", Quantity: 2, Choices: puChoices(),
	}
}

func (u *purchasingCases) ToReceive(context.Context) (apppurchasing.ToReceiveView, error) {
	u.record(nil)
	return apppurchasing.ToReceiveView{
		Groups: []apppurchasing.ToReceiveGroupView{{
			SupplierOrder: puSupplierOrder, Number: 12, SupplierName: "Couros Paulista", Lines: []apppurchasing.ToReceiveLineView{puToReceiveLine(puLine)},
		}},
		ToBuy: []apppurchasing.ToReceiveLineView{puToReceiveLine(shared.PurchaseLineID{})},
	}, u.err
}

func (u *purchasingCases) RequestPhotoUpload(_ context.Context, contentType string) (apppurchasing.PhotoUploadView, error) {
	u.record(contentType)
	return apppurchasing.PhotoUploadView{
		Key: "t/" + tenantID.String() + "/receipts/" + ctID(307).String(), URL: "http://silo.localhost/up", Method: "PUT",
		Headers: map[string]string{"Content-Type": contentType}, ExpiresAt: ctWhen.Add(5 * time.Minute),
	}, u.err
}

func (u *purchasingCases) Receive(_ context.Context, in apppurchasing.ReceiptInput) (apppurchasing.ReceiptView, error) {
	u.record(in)
	if u.err == nil && in.Idempotency.IsZero() {
		return apppurchasing.ReceiptView{}, apppurchasing.ErrIdempotencyKeyRequired
	}
	return apppurchasing.ReceiptView{
		ID: puReceipt, ReceivedAt: ctWhen,
		Entries: []apppurchasing.ReceiptEntryView{
			{Line: puLine, Order: orOrder, OrderNumber: "P-0231", OrderItem: orItem, Name: "Banco de couro", Check: purchasing.CheckOK},
			{Line: puLine2, Order: orOrder, OrderNumber: "P-0231", OrderItem: shared.OrderItemID(ctID(308)), Name: "Capa", Check: purchasing.CheckDefective},
		},
		CompletedOrders: []apppurchasing.OrderRefView{{Order: orOrder, Number: "P-0231"}},
		BackToBuy:       []apppurchasing.BackToBuyView{{OrderItem: shared.OrderItemID(ctID(308)), Name: "Capa"}},
	}, u.err
}

func (u *purchasingCases) Notices(context.Context) ([]apppurchasing.NoticeView, error) {
	u.record(nil)
	return []apppurchasing.NoticeView{{
		Line: puLine2, SupplierOrder: puSupplierOrder, Number: 12, SupplierName: "Couros Paulista", Order: orOrder, OrderNumber: "P-0231",
		Name: "Capa", Quantity: 1, Reason: purchasing.NoticeOrderCancelled, WhatsAppURL: "https://wa.me/5511987654321?text=Cancelar",
	}}, u.err
}

func (u *purchasingCases) ResolveNotice(_ context.Context, id shared.PurchaseLineID) error {
	u.record(id)
	return u.err
}

func (u *purchasingCases) OrderLines(_ context.Context, id shared.OrderID) ([]apppurchasing.OrderLineView, error) {
	u.record(id)
	return []apppurchasing.OrderLineView{
		{OrderItem: orItem, SupplierName: "Couros Paulista", Status: apppurchasing.OrderLineToBuy, ChatURL: "https://wa.me/5511987654321?text=Oi"},
		{
			OrderItem: orKitItem, SupplierName: "Couros Paulista", Status: apppurchasing.OrderLineStatus(purchasing.LineSent), Line: puLine,
			SupplierOrder: puSupplierOrder, Number: 12, Promised: orDate(18), ChatURL: "https://wa.me/5511987654321?text=C-0012",
		},
		{OrderItem: shared.OrderItemID(ctID(308)), SupplierName: "Balcão", Status: apppurchasing.OrderLineStatus(purchasing.LineReceived), Line: puLine2},
	}, u.err
}

func (u *purchasingCases) Today(context.Context) (apppurchasing.TodayView, error) {
	u.record(nil)
	return apppurchasing.TodayView{ToSend: 3, Needs: []apppurchasing.NeedView{
		{Kind: apppurchasing.NeedNotifySupplier, SupplierName: "Couros Paulista", SupplierOrder: puSupplierOrder, Number: 12, Line: puLine2, ItemName: "Capa"},
		{Kind: apppurchasing.NeedNoPromisedDate, SupplierName: "Couros Paulista", SupplierOrder: puSupplierOrder, Number: 12, Days: 3},
		{Kind: apppurchasing.NeedToSend, Supplier: puSupplier, SupplierName: "Couros Paulista", Count: 2},
	}}, u.err
}

// puRoutes is every purchasing route, as the router mounts it.
func puRoutes(p *handler.Purchasing) map[string]http.HandlerFunc {
	const base = "/api/v1/admin/purchasing"
	return map[string]http.HandlerFunc{
		"GET " + base + "/to-buy":                                          p.ToBuy,
		"POST " + base + "/supplier-orders/preview":                        p.PreviewSupplierOrder,
		"GET " + base + "/supplier-orders":                                 p.ListSupplierOrders,
		"POST " + base + "/supplier-orders":                                p.SendSupplierOrder,
		"GET " + base + "/supplier-orders/{supplierOrderId}":               p.GetSupplierOrder,
		"PUT " + base + "/supplier-orders/{supplierOrderId}/promised-date": p.RecordPromisedDate,
		"POST " + base + "/supplier-orders/{supplierOrderId}/in-transit":   p.MarkInTransit,
		"POST " + base + "/supplier-orders/{supplierOrderId}/undo":         p.UndoSupplierOrder,
		"PUT " + base + "/supplier-orders/{supplierOrderId}/paid-prices":   p.RecordPaidPrices,
		"GET " + base + "/to-receive":                                      p.ToReceive,
		"POST " + base + "/receipt-photo-uploads":                          p.RequestReceiptPhotoUpload,
		"POST " + base + "/receipts":                                       p.RecordReceipt,
		"GET " + base + "/notices":                                         p.ListNotices,
		"POST " + base + "/notices/{lineId}/resolve":                       p.ResolveNotice,
		"GET " + base + "/orders/{orderId}/lines":                          p.OrderPurchases,
		"GET " + base + "/today":                                           p.Today,
	}
}

// puFixture is the purchasing handlers over fake use cases.
type puFixture struct {
	uc      *purchasingCases
	limiter *limiter
	p       *handler.Purchasing
}

func newPuFixture(t *testing.T) *puFixture {
	t.Helper()
	f := &puFixture{uc: &purchasingCases{}, limiter: &limiter{refuse: map[string]bool{}}}
	var err error
	if f.p, err = handler.NewPurchasing(handler.PurchasingDeps{UseCases: f.uc, Limiter: f.limiter, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	return f
}

// puRequest is one request to the purchasing handlers.
type puRequest struct {
	caller  shared.Principal
	method  string
	path    string
	body    string
	headers map[string]string
}

// serveWith sends the request through a mux of the purchasing routes and
// checks the answer against the contract.
func serveWith(t *testing.T, routes map[string]http.HandlerFunc, q puRequest) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, fn := range routes {
		mux.HandleFunc(pattern, fn)
	}
	var r *http.Request
	if q.body == "" {
		r = httptest.NewRequest(q.method, q.path, nil)
	} else {
		r = httptest.NewRequest(q.method, q.path, strings.NewReader(q.body))
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range q.headers {
		r.Header.Set(k, v)
	}
	r = r.WithContext(authz.WithPrincipal(r.Context(), q.caller))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	checkContract(t, r, rec)
	return rec
}

// call sends the request as the store's owner, with a key for the two calls
// that need one.
func (f *puFixture) call(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return f.callAs(t, shared.RoleOwner, method, path, body)
}

func (f *puFixture) callAs(t *testing.T, role shared.Role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return serveWith(t, puRoutes(f.p), puRequest{caller: staffIn(t, role), method: method, path: path, body: body, headers: puKeyFor(method, path)})
}

// puKeyFor gives the two creating calls the Idempotency-Key they require.
func puKeyFor(method, path string) map[string]string {
	if method == "POST" && (strings.HasSuffix(path, "/supplier-orders") || strings.HasSuffix(path, "/receipts")) {
		return map[string]string{"Idempotency-Key": "key-1"}
	}
	return nil
}
