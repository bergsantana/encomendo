package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	appworkflow "github.com/bergsantana/encomendo/backend/internal/app/workflow"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Fixture ids for the order, customer and workflow handlers.
var (
	orOrder    = shared.OrderID(ctID(201))
	orCustomer = shared.CustomerID(ctID(202))
	orItem     = shared.OrderItemID(ctID(203))
	orLine     = shared.OrderLineID(ctID(204))
	orPayment  = shared.PaymentID(ctID(205))
	orWorkflow = shared.WorkflowID(ctID(206))
	orAddress  = shared.AddressID(ctID(207))
	orUser     = shared.UserID(ctID(208))
	orKit      = shared.KitID(ctID(209))
	orKitItem  = shared.OrderItemID(ctID(210))
	orKitLine  = shared.OrderLineID(ctID(211))
)

// orPaths fills the path parameters of an M4 route with fixture ids.
var orPaths = strings.NewReplacer(
	"{orderId}", orOrder.String(), "{itemId}", orItem.String(), "{paymentId}", orPayment.String(),
	"{customerId}", orCustomer.String(),
)

func orDate(d int) shared.CivilDate {
	c, err := shared.NewCivilDate(2026, 12, d)
	if err != nil {
		panic(err)
	}
	return c
}

func orStatuses() []apporder.StatusView {
	return []apporder.StatusView{
		{Key: "rascunho", Label: "Rascunho", ClientLabel: "Em preparo", Phase: workflow.PhaseDraft},
		{Key: "aguardando_sinal", Label: "Aguardando sinal", ClientLabel: "Aguardando o sinal", Phase: workflow.PhaseAwaitingDeposit},
		{Key: "em_compra", Label: "Em compra", ClientLabel: "Separando as peças", Phase: workflow.PhaseSourcing},
		{Key: "em_montagem", Label: "Em montagem", ClientLabel: "Montando seu pedido", Phase: workflow.PhaseAssembling},
		{Key: "entregue", Label: "Entregue", ClientLabel: "Entregue", Phase: workflow.PhaseHandedOver},
	}
}

// orView is an order in its first status as the use case answers it: with a
// cost, a margin and the item's cost only when costs is set, as the owner's
// view has them.
func orView(costs bool) apporder.View {
	cost, perUnit := ctMoney(80000), ctMoney(40000)
	statuses := orStatuses()
	v := apporder.View{
		ID: orOrder, Number: "P-0231", Customer: apporder.CustomerRef{ID: orCustomer, Name: "João Pereira", Phone: "5592981234567"},
		Status:   statuses[0],
		Workflow: apporder.WorkflowView{ID: orWorkflow, Version: 1, Key: "encomenda_montagem", Name: "Encomenda com montagem", Statuses: statuses},
		Moves:    statuses[1:2],
		Lines: []apporder.LineView{
			{ID: orLine, Price: ctMoney(100000), Items: []shared.OrderItemID{orItem}},
			{ID: orKitLine, Price: ctMoney(25000), Items: []shared.OrderItemID{orKitItem}, Kit: &order.KitSnapshot{
				Kit: orKit, Name: "Kit Scrambler",
				Swaps: []order.SwapSnapshot{{Removed: shared.ItemID(ctID(212)), Added: shared.ItemID(ctID(213)), RemovedName: "Banco", AddedName: "Banco alto"}},
			}},
		},
		Items: []apporder.ItemView{
			{
				ID: orItem, Line: orLine, CatalogItem: ctItem, Name: "Banco de couro", SourcingMode: catalog.ModeToOrder, Quantity: 1,
				UnitPrice: ctMoney(100000), Status: order.ItemToBuy, Expected: orDate(20), Alerts: []schedule.Alert{schedule.AlertAtRisk},
				NextMoves: []order.ItemStatus{order.ItemSentToSupplier, order.ItemReceived},
				Options: []order.ChosenOption{{
					Option: ctOption, Choice: ctChoice, Name: "Cor da costura", Label: "Vermelha", ExtraPrice: ctMoney(5000), ExtraDays: 2,
				}},
				Supplier: &apporder.SupplierView{Name: "Couros Paulista", LeadDays: 15},
				Backup:   &apporder.SupplierView{Name: "Couros Rio", LeadDays: 6},
			},
			{
				ID: orKitItem, Line: orKitLine, CatalogItem: ctItem, Name: "Kit item", SourcingMode: catalog.ModeInHouse, Quantity: 2,
				UnitPrice: ctMoney(12500), Status: order.ItemReady, Expected: orDate(18), Arrived: true,
				ArrivedOn: dayPtr(orDate(17)), Check: checkPtr(order.CheckOK), NextMoves: []order.ItemStatus{order.ItemInstalled},
			},
		},
		Terms: apporder.TermsView{
			Handover: order.HandoverDelivery, DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2,
			Address: &order.DeliveryAddress{ID: orAddress, Label: "Casa", CEP: "69057000", Street: "Rua A", Number: "10", City: "Manaus", State: "AM"},
		},
		Subtotal: ctMoney(125000), Freight: ctMoney(2000), Discount: ctMoney(0), Total: ctMoney(127000),
		DepositDue: ctMoney(63500), Paid: ctMoney(63500), BalanceDue: ctMoney(63500), RefundDue: ctMoney(0),
		DepositPaid: true, Released: true, ReleaseDate: dayPtr(orDate(1)), DeliveryDate: orDate(28), DateByHand: true,
		ArrivalLimit: orDate(25), Editable: order.EditApproval, CancelReason: "",
		Payments: []apporder.PaymentView{{
			ID: orPayment, Kind: order.PaymentDeposit, Amount: ctMoney(63500), Method: order.MethodPix, RecordedBy: orUser, RecordedAt: ctWhen,
			Voided: &apporder.VoidView{By: orUser, At: ctWhen, Reason: "Valor errado"},
		}},
		CreatedAt: ctWhen, Version: 3,
	}
	if costs {
		v.Cost, v.Items[0].Cost = &cost, &perUnit
		v.Margin = &catalog.Margin{Cents: 47000, BasisPoints: 3700}
	}
	return v
}

func dayPtr(d shared.CivilDate) *shared.CivilDate { return &d }

func checkPtr(c order.Check) *order.Check { return &c }

func orSummary() apporder.SummaryView {
	return apporder.SummaryView{
		ID: orOrder, Number: "P-0231", Customer: apporder.CustomerBrief{ID: orCustomer, Name: "João Pereira"},
		Status: orStatuses()[1], ItemsReady: 1, ItemsTotal: 2, DeliveryDate: orDate(28), Total: ctMoney(127000), BalanceDue: ctMoney(63500),
		Alerts: []apporder.AlertView{{Kind: schedule.AlertLate, Item: orItem}},
	}
}

func orCustomerView() appcustomer.View {
	return appcustomer.View{
		ID: orCustomer, Name: "João Pereira", Phone: "5592981234567", Email: "joao@example.com", Notes: "Prefere WhatsApp",
		Addresses: []appcustomer.AddressView{{
			ID: orAddress, Label: "Casa", CEP: "69057000", Street: "Rua A", Number: "10", District: "Centro", City: "Manaus", State: "AM",
		}},
		Version: 2,
	}
}

func orWorkflowView() appworkflow.View {
	var statuses []appworkflow.StatusView
	for _, s := range orStatuses() {
		statuses = append(statuses, appworkflow.StatusView{Key: workflow.StatusKey(s.Key), Label: s.Label, ClientLabel: s.ClientLabel, Phase: s.Phase})
	}
	return appworkflow.View{ID: orWorkflow, Version: 1, Key: "encomenda_montagem", Name: "Encomenda com montagem", Statuses: statuses}
}

// orderCases is a fake apporder.UseCases. Every call counts and records its
// input, and answers the fixture views for view (costs for the owner when
// costs is set), or err. A create or payment without an Idempotency-Key
// answers ErrIdempotencyKeyRequired, as the real use case does.
type orderCases struct {
	err   error
	calls int
	got   any
	costs bool
}

var _ apporder.UseCases = (*orderCases)(nil)

func (u *orderCases) record(in any) { u.calls++; u.got = in }

func (u *orderCases) Quote(_ context.Context, in apporder.QuoteInput) (apporder.QuoteView, error) {
	u.record(in)
	item := apporder.QuoteItem{
		CatalogItem: ctItem, Name: "Banco de couro", SourcingMode: catalog.ModeToOrder, Quantity: 1, UnitPrice: ctMoney(100000),
		Options:  []order.ChosenOption{{Option: ctOption, Choice: ctChoice, Name: "Cor", Label: "Vermelha", ExtraPrice: ctMoney(0)}},
		Expected: orDate(20),
	}
	return apporder.QuoteView{
		Lines:    []apporder.QuoteLine{{Price: ctMoney(100000), Items: []apporder.QuoteItem{item}}, {Kit: orView(false).Lines[1].Kit, Price: ctMoney(25000)}},
		Subtotal: ctMoney(125000), Freight: ctMoney(0), Discount: ctMoney(0), Total: ctMoney(125000), DepositDue: ctMoney(62500),
		DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2, DeliveryDate: orDate(28), ArrivalLimit: orDate(25),
	}, u.err
}

func (u *orderCases) Create(_ context.Context, in apporder.CreateInput) (apporder.CreatedView, error) {
	u.record(in)
	if u.err == nil && in.Idempotency.IsZero() {
		return apporder.CreatedView{}, apporder.ErrIdempotencyKeyRequired
	}
	return apporder.CreatedView{
		Order: orView(u.costs), TrackingURL: "http://altarpm.localhost:8080/p/abc", WhatsAppURL: "https://wa.me/5592981234567?text=Oi",
	}, u.err
}

func (u *orderCases) Change(_ context.Context, in apporder.ChangeInput) (apporder.ChangedView, error) {
	u.record(in)
	return apporder.ChangedView{
		Order: orView(u.costs), WhatsAppURL: "https://wa.me/5592981234567?text=Mudan%C3%A7a",
		Summary: order.ChangeResult{
			Paid: true, OldTotal: ctMoney(100000), NewTotal: ctMoney(127000), OldPromise: orDate(25), NewPromise: orDate(28), RefundDue: ctMoney(0),
		},
	}, u.err
}

func (u *orderCases) RecordPayment(_ context.Context, in apporder.PaymentInput) (apporder.View, error) {
	u.record(in)
	if u.err == nil && in.Idempotency.IsZero() {
		return apporder.View{}, apporder.ErrIdempotencyKeyRequired
	}
	return orView(u.costs), u.err
}

func (u *orderCases) VoidPayment(_ context.Context, in apporder.VoidPaymentInput) (apporder.View, error) {
	u.record(in)
	return orView(u.costs), u.err
}

func (u *orderCases) Release(_ context.Context, in apporder.VersionInput) (apporder.View, error) {
	u.record(in)
	return orView(u.costs), u.err
}

func (u *orderCases) Advance(_ context.Context, in apporder.AdvanceInput) (apporder.View, error) {
	u.record(in)
	return orView(u.costs), u.err
}

func (u *orderCases) MoveItem(_ context.Context, in apporder.MoveItemInput) (apporder.View, error) {
	u.record(in)
	return orView(u.costs), u.err
}

func (u *orderCases) UseBackupSupplier(_ context.Context, in apporder.ItemVersionInput) (apporder.View, error) {
	u.record(in)
	return orView(u.costs), u.err
}

func (u *orderCases) ChangeDeliveryDate(_ context.Context, in apporder.DateInput) (apporder.MessagedView, error) {
	u.record(in)
	return apporder.MessagedView{Order: orView(u.costs), WhatsAppURL: "https://wa.me/5592981234567?text=Nova+data"}, u.err
}

func (u *orderCases) SendTrackingLink(_ context.Context, id shared.OrderID) (apporder.LinkView, error) {
	u.record(id)
	return apporder.LinkView{TrackingURL: "http://altarpm.localhost:8080/p/def", WhatsAppURL: "https://wa.me/5592981234567?text=Link"}, u.err
}

func (u *orderCases) Cancel(_ context.Context, in apporder.CancelInput) (apporder.View, error) {
	u.record(in)
	return orView(u.costs), u.err
}

func (u *orderCases) Get(_ context.Context, id shared.OrderID) (apporder.View, error) {
	u.record(id)
	return orView(u.costs), u.err
}

func (u *orderCases) List(_ context.Context, f apporder.ListFilter, p shared.PageRequest) (shared.Page[apporder.SummaryView], error) {
	u.record([]any{f, p})
	return shared.Page[apporder.SummaryView]{Items: []apporder.SummaryView{orSummary()}, NextCursor: "next"}, u.err
}

func (u *orderCases) History(_ context.Context, id shared.OrderID, p shared.PageRequest) (shared.Page[apporder.HistoryView], error) {
	u.record([]any{id, p})
	amount := ctMoney(63500)
	by := &apporder.ActorView{ID: orUser, Name: "Maria"}
	rows := []apporder.HistoryView{
		{Kind: order.HistoryPayment, At: ctWhen, By: by, Payment: orPayment, PaymentKind: order.PaymentDeposit, PaymentMethod: order.MethodPix, Amount: &amount},
		{Kind: order.HistoryDate, At: ctWhen, By: by, OldDate: orDate(25), NewDate: orDate(28), Reason: "Atraso do fornecedor"},
		{Kind: order.HistoryStatus, At: ctWhen, From: &orStatuses()[0], To: &orStatuses()[1], Handover: order.HandoverPickup},
		{Kind: order.HistoryItem, At: ctWhen, Item: orItem, ItemName: "Banco de couro", ItemFrom: order.ItemToBuy, ItemTo: order.ItemReceived, Check: order.CheckOK},
		{Kind: order.HistoryChanged, At: ctWhen, CustomerApproved: true},
	}
	return shared.Page[apporder.HistoryView]{Items: rows}, u.err
}

func (u *orderCases) Today(context.Context) (apporder.TodayView, error) {
	u.record(nil)
	refund := ctMoney(5000)
	return apporder.TodayView{
		Today:  orDate(21),
		Counts: apporder.TodayCounts{Active: 4, AwaitingDeposit: 1, WithAlert: 2, DueIn7Days: 3},
		NeedsYou: []apporder.NeedView{
			{Kind: apporder.NeedLate, Order: orSummary(), Item: "Banco de couro", Days: 2},
			{Kind: apporder.NeedRefundDue, Order: orSummary(), Amount: &refund},
		},
		Upcoming: []apporder.SummaryView{orSummary()},
	}, u.err
}

func (u *orderCases) Finance(_ context.Context, from, to shared.CivilDate) (apporder.FinanceView, error) {
	u.record([]shared.CivilDate{from, to})
	cost := ctMoney(80000)
	return apporder.FinanceView{
		From: from, To: to,
		Received:  apporder.ReceivedView{Total: ctMoney(63500), ByKind: []apporder.MoneyByKey{{Key: "deposit", Total: ctMoney(63500)}}, ByMethod: []apporder.MoneyByKey{{Key: "pix", Total: ctMoney(63500)}}},
		ToReceive: apporder.ToReceiveView{Total: ctMoney(63500), Orders: []apporder.SummaryView{orSummary()}},
		ToRefund:  []apporder.RefundView{{Order: orSummary(), Amount: ctMoney(5000)}},
		Margins: []apporder.MarginRow{
			{Order: orOrder, Number: "P-0231", CustomerName: "João Pereira", HandedOverOn: orDate(20), Total: ctMoney(127000), Cost: &cost, Margin: &catalog.Margin{Cents: 47000, BasisPoints: 3700}},
			{Order: orOrder, Number: "P-0232", CustomerName: "Ana", HandedOverOn: orDate(21), Total: ctMoney(10000)},
		},
		MarginsTotal: apporder.MarginsTotal{Total: ctMoney(137000), Cost: cost, Margin: &catalog.Margin{Cents: 47000, BasisPoints: 3700}},
	}, u.err
}

// customerCases is a fake appcustomer.UseCases.
type customerCases struct {
	err   error
	calls int
	got   any
}

var _ appcustomer.UseCases = (*customerCases)(nil)

func (u *customerCases) List(_ context.Context, f appcustomer.ListFilter, p shared.PageRequest) (shared.Page[appcustomer.Summary], error) {
	u.calls++
	u.got = []any{f, p}
	last := ctWhen
	rows := []appcustomer.Summary{
		{ID: orCustomer, Name: "João Pereira", Phone: "5592981234567", Orders: 2, LastOrderAt: &last},
		{ID: orCustomer, Name: "Ana", Phone: "5592981234568"},
	}
	return shared.Page[appcustomer.Summary]{Items: rows, NextCursor: "next"}, u.err
}

func (u *customerCases) Get(_ context.Context, id shared.CustomerID) (appcustomer.View, error) {
	u.calls++
	u.got = id
	return orCustomerView(), u.err
}

func (u *customerCases) ByPhone(_ context.Context, phone string) (appcustomer.View, error) {
	u.calls++
	u.got = phone
	return orCustomerView(), u.err
}

func (u *customerCases) Create(_ context.Context, in appcustomer.CreateInput) (appcustomer.View, error) {
	u.calls++
	u.got = in
	return orCustomerView(), u.err
}

func (u *customerCases) Update(_ context.Context, in appcustomer.UpdateInput) (appcustomer.View, error) {
	u.calls++
	u.got = in
	return orCustomerView(), u.err
}

// workflowCases is a fake appworkflow.UseCases.
type workflowCases struct {
	err   error
	calls int
}

var _ appworkflow.UseCases = (*workflowCases)(nil)

func (u *workflowCases) Presets(context.Context) ([]appworkflow.View, error) {
	u.calls++
	return []appworkflow.View{orWorkflowView()}, u.err
}

// orHandlers are the M4 handlers over fake use cases.
type orHandlers struct {
	orders    *orderCases
	customers *customerCases
	workflows *workflowCases
	o         *handler.Orders
	c         *handler.Customers
	w         *handler.Workflows
	td        *handler.Today
	f         *handler.Finance
}

func newOrHandlers(t *testing.T) *orHandlers {
	t.Helper()
	h := &orHandlers{orders: &orderCases{}, customers: &customerCases{}, workflows: &workflowCases{}}
	var err error
	if h.o, err = handler.NewOrders(handler.OrdersDeps{UseCases: h.orders, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	if h.c, err = handler.NewCustomers(handler.CustomersDeps{UseCases: h.customers, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	if h.w, err = handler.NewWorkflows(handler.WorkflowsDeps{UseCases: h.workflows, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	if h.td, err = handler.NewToday(handler.TodayDeps{UseCases: h.orders, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	if h.f, err = handler.NewFinance(handler.FinanceDeps{UseCases: h.orders, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	return h
}

// fail makes every fake use case answer err.
func (h *orHandlers) fail(err error) { h.orders.err, h.customers.err, h.workflows.err = err, err, err }

// uses returns how many use-case calls the fakes saw.
func (h *orHandlers) uses() int { return h.orders.calls + h.customers.calls + h.workflows.calls }

// orRoutes is every M4 route but the settings one, as the router mounts it.
func orRoutes(h *orHandlers) map[string]http.HandlerFunc {
	const base = "/api/v1/admin/orders"
	return map[string]http.HandlerFunc{
		"GET /api/v1/admin/status-workflows":                             h.w.List,
		"GET /api/v1/admin/customers":                                    h.c.List,
		"POST /api/v1/admin/customers":                                   h.c.Create,
		"GET /api/v1/admin/customers/{customerId}":                       h.c.Get,
		"PUT /api/v1/admin/customers/{customerId}":                       h.c.Update,
		"POST " + base + "/quote":                                        h.o.Quote,
		"GET " + base:                                                    h.o.List,
		"POST " + base:                                                   h.o.Create,
		"GET " + base + "/{orderId}":                                     h.o.Get,
		"PUT " + base + "/{orderId}":                                     h.o.Change,
		"POST " + base + "/{orderId}/advance":                            h.o.Advance,
		"POST " + base + "/{orderId}/release":                            h.o.Release,
		"POST " + base + "/{orderId}/items/{itemId}/move":                h.o.MoveItem,
		"POST " + base + "/{orderId}/items/{itemId}/use-backup-supplier": h.o.UseBackupSupplier,
		"POST " + base + "/{orderId}/payments":                           h.o.RecordPayment,
		"POST " + base + "/{orderId}/payments/{paymentId}/void":          h.o.VoidPayment,
		"PUT " + base + "/{orderId}/delivery-date":                       h.o.ChangeDeliveryDate,
		"POST " + base + "/{orderId}/tracking-link":                      h.o.SendTrackingLink,
		"POST " + base + "/{orderId}/cancel":                             h.o.Cancel,
		"GET " + base + "/{orderId}/history":                             h.o.History,
		"GET /api/v1/admin/today":                                        h.td.Get,
		"GET /api/v1/admin/finance/summary":                              h.f.Summary,
	}
}

// orRequest is one request to the M4 handlers.
type orRequest struct {
	caller  shared.Principal
	method  string
	path    string
	body    string
	headers map[string]string
}

// serve sends the request through a mux of the M4 routes and checks the
// answer against the contract.
func (h *orHandlers) serve(t *testing.T, q orRequest) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, fn := range orRoutes(h) {
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

// as sends a request as the store's owner.
func (h *orHandlers) call(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return h.serve(t, orRequest{caller: staffIn(t, shared.RoleOwner), method: method, path: path, body: body, headers: orKeyFor(method, path)})
}

// orKeyFor gives the creating calls the Idempotency-Key they require.
func orKeyFor(method, path string) map[string]string {
	if method == "POST" && (strings.HasSuffix(path, "/orders") || strings.HasSuffix(path, "/payments")) {
		return map[string]string{"Idempotency-Key": "key-1"}
	}
	return nil
}
