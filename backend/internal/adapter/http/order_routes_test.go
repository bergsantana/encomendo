package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	appworkflow "github.com/bergsantana/encomendo/backend/internal/app/workflow"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// orderAdminPaths are the 23 milestone 4 operations of the contract: the
// status workflows (and choosing one), customers, orders, Hoje and
// Financeiro, with an id in every path parameter.
var orderAdminPaths = []struct{ method, path string }{
	{"PUT", "/api/v1/admin/settings/status-workflow"},
	{"GET", "/api/v1/admin/status-workflows"},
	{"GET", "/api/v1/admin/customers"},
	{"POST", "/api/v1/admin/customers"},
	{"GET", "/api/v1/admin/customers/" + sampleID},
	{"PUT", "/api/v1/admin/customers/" + sampleID},
	{"POST", "/api/v1/admin/orders/quote"},
	{"GET", "/api/v1/admin/orders"},
	{"POST", "/api/v1/admin/orders"},
	{"GET", "/api/v1/admin/orders/" + sampleID},
	{"PUT", "/api/v1/admin/orders/" + sampleID},
	{"POST", "/api/v1/admin/orders/" + sampleID + "/advance"},
	{"POST", "/api/v1/admin/orders/" + sampleID + "/release"},
	{"POST", "/api/v1/admin/orders/" + sampleID + "/items/" + sampleID + "/move"},
	{"POST", "/api/v1/admin/orders/" + sampleID + "/items/" + sampleID + "/use-backup-supplier"},
	{"POST", "/api/v1/admin/orders/" + sampleID + "/payments"},
	{"POST", "/api/v1/admin/orders/" + sampleID + "/payments/" + sampleID + "/void"},
	{"PUT", "/api/v1/admin/orders/" + sampleID + "/delivery-date"},
	{"POST", "/api/v1/admin/orders/" + sampleID + "/tracking-link"},
	{"POST", "/api/v1/admin/orders/" + sampleID + "/cancel"},
	{"GET", "/api/v1/admin/orders/" + sampleID + "/history"},
	{"GET", "/api/v1/admin/today"},
	{"GET", "/api/v1/admin/finance/summary?from=2026-12-01&to=2026-12-31"},
}

// newOrderBody returns a body that passes the handler's checks for the
// route, so a request reaches its use case; "" for a route with no body.
func newOrderBody(method, path string) string {
	lines := fmt.Sprintf(`[{"itemId":%q,"quantity":1}]`, sampleID)
	terms := `{"handover":"pickup"}`
	customer := `{"name":"João","phone":"5592981234567","addresses":[]}`
	switch {
	case path == "/api/v1/admin/settings/status-workflow":
		return fmt.Sprintf(`{"workflowId":%q,"version":1,"expectedVersion":1}`, sampleID)
	case path == "/api/v1/admin/customers" && method == "POST":
		return customer
	case strings.HasPrefix(path, "/api/v1/admin/customers/") && method == "PUT":
		return strings.TrimSuffix(customer, "}") + `,"version":1}`
	case strings.HasSuffix(path, "/orders/quote"):
		return fmt.Sprintf(`{"lines":%s,"terms":%s}`, lines, terms)
	case strings.HasSuffix(path, "/orders") && method == "POST":
		return fmt.Sprintf(`{"customerId":%q,"lines":%s,"terms":%s}`, sampleID, lines, terms)
	case strings.HasSuffix(path, "/orders/"+sampleID) && method == "PUT":
		return fmt.Sprintf(`{"lines":%s,"terms":%s,"customerApproved":false,"version":1}`, lines, terms)
	case strings.HasSuffix(path, "/advance"):
		return `{"to":"entregue","version":1}`
	case strings.HasSuffix(path, "/release"), strings.HasSuffix(path, "/use-backup-supplier"):
		return `{"version":1}`
	case strings.HasSuffix(path, "/move"):
		return `{"to":"received","check":"ok","version":1}`
	case strings.HasSuffix(path, "/payments"):
		return `{"kind":"deposit","amountCents":1,"method":"pix","version":1}`
	case strings.HasSuffix(path, "/void"), strings.HasSuffix(path, "/cancel"):
		return `{"reason":"x","version":1}`
	case strings.HasSuffix(path, "/delivery-date"):
		return `{"date":"2026-12-28","version":1}`
	}
	return ""
}

// orderReached is an apporder.UseCases that counts each call into n and
// refuses it with ErrForbidden, as every admin route may.
type orderReached struct{ n *int }

var _ apporder.UseCases = orderReached{}

func (u orderReached) hit() error { *u.n++; return shared.ErrForbidden }

func (u orderReached) Quote(context.Context, apporder.QuoteInput) (apporder.QuoteView, error) {
	return apporder.QuoteView{}, u.hit()
}

func (u orderReached) Create(context.Context, apporder.CreateInput) (apporder.CreatedView, error) {
	return apporder.CreatedView{}, u.hit()
}

func (u orderReached) Change(context.Context, apporder.ChangeInput) (apporder.ChangedView, error) {
	return apporder.ChangedView{}, u.hit()
}

func (u orderReached) RecordPayment(context.Context, apporder.PaymentInput) (apporder.View, error) {
	return apporder.View{}, u.hit()
}

func (u orderReached) VoidPayment(context.Context, apporder.VoidPaymentInput) (apporder.View, error) {
	return apporder.View{}, u.hit()
}

func (u orderReached) Release(context.Context, apporder.VersionInput) (apporder.View, error) {
	return apporder.View{}, u.hit()
}

func (u orderReached) Advance(context.Context, apporder.AdvanceInput) (apporder.View, error) {
	return apporder.View{}, u.hit()
}

func (u orderReached) MoveItem(context.Context, apporder.MoveItemInput) (apporder.View, error) {
	return apporder.View{}, u.hit()
}

func (u orderReached) UseBackupSupplier(context.Context, apporder.ItemVersionInput) (apporder.View, error) {
	return apporder.View{}, u.hit()
}

func (u orderReached) ChangeDeliveryDate(context.Context, apporder.DateInput) (apporder.MessagedView, error) {
	return apporder.MessagedView{}, u.hit()
}

func (u orderReached) SendTrackingLink(context.Context, shared.OrderID) (apporder.LinkView, error) {
	return apporder.LinkView{}, u.hit()
}

func (u orderReached) Cancel(context.Context, apporder.CancelInput) (apporder.View, error) {
	return apporder.View{}, u.hit()
}

func (u orderReached) Get(context.Context, shared.OrderID) (apporder.View, error) {
	return apporder.View{}, u.hit()
}

func (u orderReached) List(context.Context, apporder.ListFilter, shared.PageRequest) (shared.Page[apporder.SummaryView], error) {
	return shared.Page[apporder.SummaryView]{}, u.hit()
}

func (u orderReached) History(context.Context, shared.OrderID, shared.PageRequest) (shared.Page[apporder.HistoryView], error) {
	return shared.Page[apporder.HistoryView]{}, u.hit()
}

func (u orderReached) Today(context.Context) (apporder.TodayView, error) {
	return apporder.TodayView{}, u.hit()
}

func (u orderReached) Finance(context.Context, shared.CivilDate, shared.CivilDate) (apporder.FinanceView, error) {
	return apporder.FinanceView{}, u.hit()
}

// customersReached is an appcustomer.UseCases that counts and refuses.
type customersReached struct{ n *int }

var _ appcustomer.UseCases = customersReached{}

func (u customersReached) hit() error { *u.n++; return shared.ErrForbidden }

func (u customersReached) List(context.Context, appcustomer.ListFilter, shared.PageRequest) (shared.Page[appcustomer.Summary], error) {
	return shared.Page[appcustomer.Summary]{}, u.hit()
}

func (u customersReached) Get(context.Context, shared.CustomerID) (appcustomer.View, error) {
	return appcustomer.View{}, u.hit()
}

func (u customersReached) ByPhone(context.Context, string) (appcustomer.View, error) {
	return appcustomer.View{}, u.hit()
}

func (u customersReached) Create(context.Context, appcustomer.CreateInput) (appcustomer.View, error) {
	return appcustomer.View{}, u.hit()
}

func (u customersReached) Update(context.Context, appcustomer.UpdateInput) (appcustomer.View, error) {
	return appcustomer.View{}, u.hit()
}

// workflowsReached is an appworkflow.UseCases that counts and refuses.
type workflowsReached struct{ n *int }

var _ appworkflow.UseCases = workflowsReached{}

func (u workflowsReached) Presets(context.Context) ([]appworkflow.View, error) { return nil, u.hit() }

func (u workflowsReached) hit() error { *u.n++; return shared.ErrForbidden }

// orderHandlers builds the M4 handler groups over reached fakes that count
// into n.
func orderHandlers(t *testing.T, n *int) Handlers {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	var (
		h   Handlers
		err error
	)
	if h.Workflows, err = handler.NewWorkflows(handler.WorkflowsDeps{UseCases: workflowsReached{n: n}, Log: log}); err != nil {
		t.Fatal(err)
	}
	if h.Customers, err = handler.NewCustomers(handler.CustomersDeps{UseCases: customersReached{n: n}, Log: log}); err != nil {
		t.Fatal(err)
	}
	if h.Orders, err = handler.NewOrders(handler.OrdersDeps{UseCases: orderReached{n: n}, Log: log}); err != nil {
		t.Fatal(err)
	}
	if h.Today, err = handler.NewToday(handler.TodayDeps{UseCases: orderReached{n: n}, Log: log}); err != nil {
		t.Fatal(err)
	}
	if h.Finance, err = handler.NewFinance(handler.FinanceDeps{UseCases: orderReached{n: n}, Log: log}); err != nil {
		t.Fatal(err)
	}
	return h
}

// Every M4 route answers only its own method: any other is 405 with the
// right Allow, before authn runs and before any use case.
func TestRouter_OrderRoutesAnswer405ToAnotherMethod(t *testing.T) {
	t.Parallel()

	for _, rt := range orderAdminPaths {
		other := "DELETE"
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			t.Parallel()

			uc := &tenancyReached{}
			rec := httptest.NewRecorder()
			adminRouter(t, uc).ServeHTTP(rec, adminRequest(other, rt.path, "staff"))

			if rec.Code != http.StatusMethodNotAllowed || uc.calls != 0 || !strings.Contains(rec.Header().Get("Allow"), rt.method) {
				t.Errorf("DELETE answered %d (Allow %q) after %d use-case calls, want 405 allowing %s",
					rec.Code, rec.Header().Get("Allow"), uc.calls, rt.method)
			}
		})
	}
}
