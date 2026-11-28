package order_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

const day = 24 * time.Hour

// firstPage is the zero page request: the first page, the default size.
var firstPage shared.PageRequest

func (h *harness) addCustomer(name, phone string) apporder.CustomerFacts {
	c := apporder.CustomerFacts{ID: shared.CustomerID(h.ids.NewID()), Name: name, Phone: phone}
	h.customers.Add(c)
	return c
}

// advance moves v to the status key (order:write), keeping the handover.
func (h *harness) advance(ctx context.Context, t *testing.T, v apporder.View, to string, handover *order.Handover) apporder.View {
	t.Helper()
	out, err := h.svc.Advance(ctx, apporder.AdvanceInput{Order: v.ID, To: workflow.StatusKey(to), Handover: handover, Version: v.Version})
	if err != nil {
		t.Fatalf("Advance to %s: %v", to, err)
	}
	return out
}

// walk advances v through keys, one status at a time.
func (h *harness) walk(ctx context.Context, t *testing.T, v apporder.View, keys ...string) apporder.View {
	t.Helper()
	for _, k := range keys {
		v = h.advance(ctx, t, v, k, nil)
	}
	return v
}

// arrive receives every item of v, checked ok.
func (h *harness) arrive(ctx context.Context, t *testing.T, v apporder.View) apporder.View {
	t.Helper()
	ok := order.CheckOK
	// Bought items arrive through purchasing's path since milestone 5 (ADR 0031).
	moves := make([]apporder.PurchaseMoveInput, 0, len(v.Items))
	for _, i := range v.Items {
		moves = append(moves, apporder.PurchaseMoveInput{Order: v.ID, Move: order.PurchaseMove{Item: i.ID, To: order.ItemReceived, Check: &ok}})
	}
	if err := h.svc.MovePurchased(ctx, moves); err != nil {
		t.Fatalf("MovePurchased: %v", err)
	}
	got, err := h.svc.Get(ctx, v.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return got
}

// released makes an order and pays its deposit, which releases it today.
func (h *harness) released(ctx context.Context, t *testing.T, edit func(*apporder.CreateInput)) apporder.View {
	t.Helper()
	v := h.create(ctx, t, edit).Order
	return h.pay(ctx, t, v, order.PaymentDeposit, v.DepositDue.Cents())
}

// ready makes a released order whose items arrived and that is in "Pronto".
func (h *harness) ready(ctx context.Context, t *testing.T) apporder.View {
	t.Helper()
	v := h.released(ctx, t, nil)
	v = h.walk(ctx, t, v, "awaiting_deposit", "purchasing", "items_arriving")
	v = h.arrive(ctx, t, v)
	return h.walk(ctx, t, v, "assembling", "ready")
}

// list lists as owner and returns the numbers in the page.
func (h *harness) numbers(t *testing.T, f apporder.ListFilter, p shared.PageRequest) ([]string, string) {
	t.Helper()
	page, err := h.svc.List(h.viewer(t), f, p)
	if err != nil {
		t.Fatalf("List(%+v): %v", f, err)
	}
	var out []string
	for _, s := range page.Items {
		out = append(out, s.Number)
	}
	return out, page.NextCursor
}

func equal(got, want []string) bool { return strings.Join(got, ",") == strings.Join(want, ",") }

func TestList_FiltersByStatusPhaseScopeCustomerAndQuery(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	maria := h.addCustomer("Maria Souza", "5592987654321")
	a := h.create(h.staff(t), t, nil).Order // P-0001, João, delivery 12-21
	h.clock.Advance(day)
	b := h.create(h.staff(t), t, func(in *apporder.CreateInput) { in.Customer = maria.ID }).Order // P-0002, 12-22
	h.clock.Advance(day)
	h.create(h.staff(t), t, nil) // P-0003, 12-23
	h.advance(h.staff(t), t, a, "awaiting_deposit", nil)
	if _, err := h.svc.Cancel(h.admin(t), apporder.CancelInput{Order: b.ID, Reason: "Desistiu", Version: b.Version}); err != nil {
		t.Fatal(err)
	}
	all := firstPage
	cases := []struct {
		name string
		f    apporder.ListFilter
		want []string
	}{
		{"active by default, cancelled left out", apporder.ListFilter{}, []string{"P-0001", "P-0003"}},
		{"cancelled", apporder.ListFilter{Scope: apporder.ScopeCancelled}, []string{"P-0002"}},
		{"handed over", apporder.ListFilter{Scope: apporder.ScopeHandedOver}, nil},
		{"all", apporder.ListFilter{Scope: apporder.ScopeAll}, []string{"P-0001", "P-0002", "P-0003"}},
		{"status", apporder.ListFilter{Status: "awaiting_deposit"}, []string{"P-0001"}},
		{"phase", apporder.ListFilter{Phase: "draft"}, []string{"P-0003"}},
		{"cancelled status", apporder.ListFilter{Scope: apporder.ScopeAll, Status: "cancelled"}, []string{"P-0002"}},
		{"cancelled phase", apporder.ListFilter{Scope: apporder.ScopeAll, Phase: "cancelled"}, []string{"P-0002"}},
		{"customer", apporder.ListFilter{Scope: apporder.ScopeAll, Customer: maria.ID}, []string{"P-0002"}},
		{"number", apporder.ListFilter{Query: "3"}, []string{"P-0003"}},
		{"number with prefix", apporder.ListFilter{Query: " P-0001 "}, []string{"P-0001"}},
		{"number in lower case", apporder.ListFilter{Query: "p-0003"}, []string{"P-0003"}},
		{"name prefix", apporder.ListFilter{Scope: apporder.ScopeAll, Query: "mar"}, []string{"P-0002"}},
		{"name prefix ignores case", apporder.ListFilter{Query: "JOÃO"}, []string{"P-0001", "P-0003"}},
		{"name in the middle is no match", apporder.ListFilter{Query: "Pereira"}, nil},
		{"unknown number", apporder.ListFilter{Query: "999"}, nil},
		{"filters combine", apporder.ListFilter{Query: "joão", Phase: "draft"}, []string{"P-0003"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, next := h.numbers(t, tc.f, all)
			if !equal(got, tc.want) || next != "" {
				t.Errorf("List(%+v) = %v next %q, want %v", tc.f, got, next, tc.want)
			}
		})
	}
}

func TestList_SummaryHasWhatTheScreenNeeds(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, func(in *apporder.CreateInput) {
		in.Lines = []apporder.LineRequest{h.bancoLine(), {Item: h.banco, Quantity: 2}}
	})
	page, err := h.svc.List(h.viewer(t), apporder.ListFilter{}, firstPage)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("List: %v %+v", err, page)
	}
	s := page.Items[0]
	if s.ID != v.ID || s.Number != "P-0001" || s.Customer.Name != "João Pereira" || s.Customer.ID != h.joao.ID ||
		s.ItemsTotal != 2 || s.ItemsReady != 0 || s.DeliveryDate != v.DeliveryDate ||
		s.Total != v.Total || s.BalanceDue != v.BalanceDue || s.Status.Key != "draft" || len(s.Alerts) != 0 {
		t.Errorf("summary = %+v", s)
	}
	v = h.arrive(h.staff(t), t, v)
	page, _ = h.svc.List(h.viewer(t), apporder.ListFilter{}, firstPage)
	if got := page.Items[0].ItemsReady; got != 2 {
		t.Errorf("itemsReady = %d, want 2", got)
	}
}

func TestList_Pages(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for range 5 {
		h.create(h.staff(t), t, nil)
		h.clock.Advance(day)
	}
	var seen []string
	cursor := ""
	for range 5 {
		p, err := shared.NewPageRequest(cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		got, next := h.numbers(t, apporder.ListFilter{}, p)
		seen = append(seen, got...)
		if cursor = next; cursor == "" {
			break
		}
	}
	if want := []string{"P-0001", "P-0002", "P-0003", "P-0004", "P-0005"}; !equal(seen, want) {
		t.Errorf("pages gave %v, want %v", seen, want)
	}
}

func TestList_AlertFilterKeepsActiveOrdersWithAnAlert(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	late := h.released(h.staff(t), t, nil) // item expected 12-16
	h.clock.Advance(2 * day)
	h.released(h.staff(t), t, nil) // released 12-03, expected 12-18
	h.create(h.staff(t), t, nil)   // not released: no alerts
	h.clock.Advance(15 * day)      // 12-18: the first is late
	for _, scope := range []apporder.ListScope{"", apporder.ScopeAll} {
		got, _ := h.numbers(t, apporder.ListFilter{Alert: true, Scope: scope}, firstPage)
		if !equal(got, []string{"P-0001"}) {
			t.Errorf("scope %q: alert list = %v, want P-0001 only", scope, got)
		}
	}
	page, _ := h.svc.List(h.viewer(t), apporder.ListFilter{Alert: true}, firstPage)
	a := page.Items[0].Alerts
	if len(a) != 1 || a[0].Kind != schedule.AlertLate || a[0].Item != late.Items[0].ID {
		t.Errorf("alerts = %+v", a)
	}
}

func TestList_RefusesBadFilters(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for name, f := range map[string]apporder.ListFilter{
		"scope": {Scope: "everything"}, "phase": {Phase: "nope"}, "query": {Query: strings.Repeat("a", 101)},
	} {
		if _, err := h.svc.List(h.viewer(t), f, firstPage); !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: err = %v, want invalid", name, err)
		}
	}
}

func TestQueries_NeedOrderRead(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	for _, ctx := range []context.Context{h.viewer(t), h.staff(t), h.admin(t), h.owner(t)} {
		if _, err := h.svc.List(ctx, apporder.ListFilter{}, firstPage); err != nil {
			t.Errorf("List: %v", err)
		}
		if _, err := h.svc.History(ctx, v.ID, firstPage); err != nil {
			t.Errorf("History: %v", err)
		}
		if _, err := h.svc.Today(ctx); err != nil {
			t.Errorf("Today: %v", err)
		}
	}
	nobody := context.Background()
	if _, err := h.svc.List(nobody, apporder.ListFilter{}, firstPage); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("List without a principal: %v", err)
	}
	if _, err := h.svc.History(nobody, v.ID, firstPage); err == nil {
		t.Error("History without a principal worked")
	}
	if _, err := h.svc.Today(nobody); err == nil {
		t.Error("Today without a principal worked")
	}
}

// Another store's staff sees none of this store's orders.
func TestQueries_NeverCrossStores(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, nil)
	p, err := shared.NewBackofficePrincipal(shared.UserID(h.ids.NewID()), shared.TenantID(h.ids.NewID()), shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	other := authz.WithPrincipal(context.Background(), p)
	page, err := h.svc.List(other, apporder.ListFilter{Scope: apporder.ScopeAll}, firstPage)
	if err != nil || len(page.Items) != 0 {
		t.Errorf("List = %+v, %v; want nothing", page, err)
	}
	if _, err := h.svc.History(other, v.ID, firstPage); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("History: err = %v, want not found", err)
	}
	today, err := h.svc.Today(other)
	if err != nil || today.Counts.Active != 0 || len(today.Upcoming) != 0 {
		t.Errorf("Today = %+v, %v; want empty", today, err)
	}
	if _, err := h.svc.Finance(other, testkit.Date(t, "2026-12-01"), testkit.Date(t, "2026-12-31")); err != nil {
		t.Errorf("Finance: %v", err)
	}
}

func TestHistory_NewestFirstWithNamesAndPages(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	staff := h.staff(t)
	actor, _ := authz.PrincipalFrom(staff)
	h.repo.NameUser(actor.UserID(), "Ana")
	v := h.create(staff, t, nil).Order
	h.clock.Advance(time.Hour)
	v = h.pay(staff, t, v, order.PaymentDeposit, v.DepositDue.Cents())
	h.clock.Advance(time.Hour)
	h.advance(staff, t, v, "awaiting_deposit", nil)

	page, err := h.svc.History(h.viewer(t), v.ID, firstPage)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, e := range page.Items {
		kinds = append(kinds, string(e.Kind))
	}
	if !equal(kinds, []string{"status", "payment", "created"}) {
		t.Fatalf("kinds = %v, want newest first", kinds)
	}
	move, pay := page.Items[0], page.Items[1]
	if move.By == nil || move.By.Name != "Ana" || move.To == nil || move.To.Key != "awaiting_deposit" || move.From == nil || move.From.Key != "draft" {
		t.Errorf("status entry = %+v", move)
	}
	if pay.Amount == nil || pay.Amount.Cents() != 50_000 || pay.PaymentKind != order.PaymentDeposit || pay.PaymentMethod != order.MethodPix || !pay.At.Before(move.At) {
		t.Errorf("payment entry = %+v", pay)
	}
	p, _ := shared.NewPageRequest("", 2)
	first, err := h.svc.History(h.viewer(t), v.ID, p)
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	p, _ = shared.NewPageRequest(first.NextCursor, 2)
	second, err := h.svc.History(h.viewer(t), v.ID, p)
	if err != nil || len(second.Items) != 1 || second.NextCursor != "" || second.Items[0].Kind != order.HistoryCreated {
		t.Errorf("second page = %+v, %v", second, err)
	}
	if _, err := h.svc.History(h.viewer(t), shared.OrderID(h.ids.NewID()), firstPage); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("unknown order: err = %v", err)
	}
}

func TestHistory_NamesTheItemOfAnItemMove(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	sent := order.PurchaseMove{Item: v.Items[0].ID, To: order.ItemSentToSupplier, Ref: "C-0001"}
	if err := h.svc.MovePurchased(h.admin(t), []apporder.PurchaseMoveInput{{Order: v.ID, Move: sent}}); err != nil {
		t.Fatal(err)
	}
	page, err := h.svc.History(h.viewer(t), v.ID, firstPage)
	if err != nil {
		t.Fatal(err)
	}
	e := page.Items[0]
	if e.Kind != order.HistoryItem || e.Item != v.Items[0].ID || e.ItemName != "Banco de couro" || e.ItemFrom != order.ItemToBuy || e.ItemTo != order.ItemSentToSupplier || e.Ref != "C-0001" {
		t.Errorf("item entry = %+v", e)
	}
}

// Lists, Hoje and the history are built for staff, but only the owner's
// order screen has costs: these answers have no field that could hold one.
func TestQueries_CarryNoCostOrMarginForAnyRole(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.released(h.staff(t), t, nil)
	h.advance(h.staff(t), t, v, "awaiting_deposit", nil)
	h.clock.Advance(20 * day) // a late item
	for name, ctx := range map[string]context.Context{"owner": h.owner(t), "admin": h.admin(t), "staff": h.staff(t), "viewer": h.viewer(t)} {
		list, err := h.svc.List(ctx, apporder.ListFilter{Scope: apporder.ScopeAll}, firstPage)
		if err != nil {
			t.Fatal(err)
		}
		today, err := h.svc.Today(ctx)
		if err != nil {
			t.Fatal(err)
		}
		hist, err := h.svc.History(ctx, v.ID, firstPage)
		if err != nil {
			t.Fatal(err)
		}
		for what, got := range map[string]any{"list": list, "today": today, "history": hist} {
			b, _ := json.Marshal(got)
			s := strings.ToLower(string(b))
			for _, bad := range []string{"cost", "margin", "supplier", "couros"} {
				if strings.Contains(s, bad) {
					t.Errorf("%s %s carries %q: %s", name, what, bad, s)
				}
			}
		}
	}
}

func TestNumberQuery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		q    string
		want order.Number
	}{{"231", 231}, {"P-0231", 231}, {"p-0231", 231}, {"  7 ", 7}, {"P-12345", 12345}, {"0001", 1}} {
		if got, ok := apporder.NumberQuery(tc.q); !ok || got != tc.want {
			t.Errorf("NumberQuery(%q) = %v, %v; want %v", tc.q, got, ok, tc.want)
		}
	}
	for _, q := range []string{"", "P-", "P", "-1", "0", "00", "Maria", "2 3", "P-0231x", "1234567890", "٣"} {
		if got, ok := apporder.NumberQuery(q); ok {
			t.Errorf("NumberQuery(%q) = %v, want a name search", q, got)
		}
	}
}
