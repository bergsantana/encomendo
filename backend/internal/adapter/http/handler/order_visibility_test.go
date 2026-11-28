package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/app/order/fake"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// visRig serves the order handlers over the real order use cases and their
// in-memory fakes, so who sees a cost is decided by the app layer as in
// production, and the HTTP layer is tested on what it really gets.
type visRig struct {
	t       *testing.T
	o       *handler.Orders
	today   *handler.Today
	finance *handler.Finance
	banco   shared.ItemID
	joao    shared.CustomerID
	keys    int
}

func newVisRig(t *testing.T) *visRig {
	t.Helper()
	clock := sharedfake.NewClock(time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC))
	ids := sharedfake.NewIDs(clock)
	repo, cat, customers := fake.NewOrders(), fake.NewCatalog(), fake.NewCustomers(ids.NewID)
	workflows := fake.NewWorkflows(testkit.Preset(t, testkit.PresetEncomendaMontagem))
	repo.Link(workflows, customers)
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := apporder.NewService(apporder.Deps{
		Repo: repo, Catalog: cat, Customers: customers, Workflows: workflows,
		Store: fake.NewStore(apporder.StoreFacts{
			Name: "Alta RPM", Slug: "altarpm", WhatsApp: "5592991234567", Currency: "BRL",
			DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2, Location: loc,
		}),
		Tracking: fake.Tracking{}, WhatsApp: fake.WhatsApp{}, Idempotency: sharedfake.NewIdempotency(clock),
		Secrets: sharedfake.NewSecrets(), Tx: sharedfake.NewTxManager(), Events: sharedfake.NewEvents(), Clock: clock, IDs: ids,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := &visRig{t: t, banco: shared.ItemID(ids.NewID())}
	cost, backupCost := ctMoney(40000), ctMoney(45000)
	cat.AddItem(fake.CatalogItem{
		ID: r.banco, Name: "Banco de couro", Mode: catalog.ModeToOrder, Price: ctMoney(100000), LeadDays: 15,
		Sourcing: order.Sourcing{
			Supplier: shared.SupplierID(ids.NewID()), SupplierName: "Couros Paulista", Cost: &cost,
			Backup: &order.BackupSource{Supplier: shared.SupplierID(ids.NewID()), Name: "Couros Rio", LeadDays: 6, Cost: &backupCost},
		},
	})
	r.joao = shared.CustomerID(ids.NewID())
	customers.Add(apporder.CustomerFacts{ID: r.joao, Name: "João Pereira", Phone: "5592981234567"})
	if r.o, err = handler.NewOrders(handler.OrdersDeps{UseCases: svc, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	if r.today, err = handler.NewToday(handler.TodayDeps{UseCases: svc, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	if r.finance, err = handler.NewFinance(handler.FinanceDeps{UseCases: svc, Log: discardLog()}); err != nil {
		t.Fatal(err)
	}
	return r
}

const visBase = "/api/v1/admin/orders"

// do sends the request as role, with a key when the call needs one, and
// checks the answer against the contract. It returns the status and the
// decoded body (nil when there is none).
func (r *visRig) do(role shared.Role, method, path string, body any, key string) (int, map[string]any) {
	r.t.Helper()
	mux := http.NewServeMux()
	for pattern, h := range map[string]http.HandlerFunc{
		"POST " + visBase:                                                   r.o.Create,
		"POST " + visBase + "/quote":                                        r.o.Quote,
		"GET " + visBase:                                                    r.o.List,
		"GET " + visBase + "/{orderId}":                                     r.o.Get,
		"PUT " + visBase + "/{orderId}":                                     r.o.Change,
		"POST " + visBase + "/{orderId}/advance":                            r.o.Advance,
		"POST " + visBase + "/{orderId}/release":                            r.o.Release,
		"POST " + visBase + "/{orderId}/items/{itemId}/move":                r.o.MoveItem,
		"POST " + visBase + "/{orderId}/items/{itemId}/use-backup-supplier": r.o.UseBackupSupplier,
		"POST " + visBase + "/{orderId}/payments":                           r.o.RecordPayment,
		"POST " + visBase + "/{orderId}/payments/{paymentId}/void":          r.o.VoidPayment,
		"PUT " + visBase + "/{orderId}/delivery-date":                       r.o.ChangeDeliveryDate,
		"POST " + visBase + "/{orderId}/cancel":                             r.o.Cancel,
		"GET /api/v1/admin/today":                                           r.today.Get,
		"GET /api/v1/admin/finance/summary":                                 r.finance.Summary,
	} {
		mux.HandleFunc(pattern, h)
	}
	payload := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			r.t.Fatal(err)
		}
		payload = string(b)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	req = req.WithContext(authz.WithPrincipal(req.Context(), staffIn(r.t, role)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	checkContract(r.t, req, rec)
	var out map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			r.t.Fatalf("%s %s: not JSON: %s", method, path, rec.Body.String())
		}
	}
	return rec.Code, out
}

func (r *visRig) key() string { r.keys++; return fmt.Sprintf("vis-%d", r.keys) }

func (r *visRig) createBody() map[string]any {
	return map[string]any{
		"customerId": r.joao.String(), "lines": []any{map[string]any{"itemId": r.banco.String(), "quantity": 2}},
		"terms": map[string]any{"handover": "pickup"},
	}
}

// fresh makes an order as the owner and returns its path and version.
func (r *visRig) fresh() (path string, id string, version int) {
	r.t.Helper()
	status, out := r.do(shared.RoleOwner, "POST", visBase, r.createBody(), r.key())
	if status != 201 {
		r.t.Fatalf("create as owner answered %d %v", status, out)
	}
	o := out["order"].(map[string]any)
	id = o["id"].(string)
	return visBase + "/" + id, id, int(o["version"].(float64))
}

// walk calls visit on every key of every object in v, with its dotted path.
func walk(v any, path string, visit func(path, key string, val any)) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			visit(path, k, val)
			walk(val, path+"."+k, visit)
		}
	case []any:
		for i, val := range x {
			walk(val, fmt.Sprintf("%s[%d]", path, i), visit)
		}
	}
}

// ownerKeys are the keys only the owner sees, at any depth.
var ownerKeys = map[string]bool{"cost": true, "margin": true}

// assertVisibility fails when the answer has a cost key for a role that is
// not the owner, or none for the owner where the contract gives it one.
func assertVisibility(t *testing.T, role shared.Role, what string, order map[string]any) {
	t.Helper()
	found := map[string]bool{}
	walk(order, what, func(path, key string, _ any) {
		if ownerKeys[key] {
			found[path+"."+key] = true
		}
	})
	if role != shared.RoleOwner {
		if len(found) > 0 {
			t.Errorf("%s as %s carries %v, which only the owner sees", what, role, found)
		}
		return
	}
	for _, want := range []string{what + ".cost", what + ".margin", what + ".items[0].cost"} {
		if !found[want] {
			t.Errorf("%s as owner has no %s: %v", what, want, order)
		}
	}
}

// assertQuoteVisibility checks a quote's answer: the owner gets `cost` and
// `margin`, every other role neither, and a refused role gets a problem.
func assertQuoteVisibility(t *testing.T, role shared.Role, wantStatus, status int, out map[string]any) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("quote as %s answered %d %v, want %d", role, status, out, wantStatus)
	}
	_, hasCost := out["cost"]
	_, hasMargin := out["margin"]
	switch {
	case status >= 400:
		walk(out, "quote problem", func(path, key string, _ any) {
			if ownerKeys[key] {
				t.Errorf("quote problem as %s carries %s", role, key)
			}
		})
	case role == shared.RoleOwner:
		if !hasCost || !hasMargin {
			t.Errorf("quote as owner: cost %v margin %v, want both: %v", hasCost, hasMargin, out)
		}
	case hasCost || hasMargin:
		t.Errorf("quote as %s carries cost %v margin %v, which only the owner sees", role, hasCost, hasMargin)
	}
}

// orderOf is the Order inside an answer: the body itself, or its "order".
func orderOf(out map[string]any) map[string]any {
	if o, ok := out["order"].(map[string]any); ok {
		return o
	}
	return out
}

// Every read and every write that answers an order has `cost`, `margin`
// and `items[].cost` for the owner only; Hoje has none for anyone; the
// order list has no field for them. A role refused a write gets a problem,
// which has none either (ADR 0025).
func TestOrders_CostsAreTheOwnersOnly(t *testing.T) {
	t.Parallel()

	for _, role := range []shared.Role{shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()

			r := newVisRig(t)
			allowed := func(owner, admin, staff bool) int {
				if (role == shared.RoleOwner && owner) || (role == shared.RoleAdmin && admin) || (role == shared.RoleStaff && staff) {
					return 200
				}
				return 403
			}
			check := func(what string, wantStatus int, status int, out map[string]any) {
				t.Helper()
				if status != wantStatus && (wantStatus != 200 || status != 201) {
					t.Fatalf("%s as %s answered %d %v, want %d", what, role, status, out, wantStatus)
				}
				if status >= 400 {
					walk(out, what, func(path, key string, _ any) {
						if ownerKeys[key] {
							t.Errorf("%s problem as %s carries %s", what, role, key)
						}
					})
					return
				}
				assertVisibility(t, role, what, orderOf(out))
			}

			// Reads.
			path, id, version := r.fresh()
			status, out := r.do(role, "GET", path, nil, "")
			check("get", 200, status, out)
			status, out = r.do(role, "GET", visBase, nil, "")
			if status != 200 {
				t.Fatalf("list as %s answered %d %v", role, status, out)
			}
			walk(out, "list", func(p, key string, _ any) {
				if ownerKeys[key] {
					t.Errorf("the list as %s carries %s", role, key)
				}
			})
			status, out = r.do(role, "GET", "/api/v1/admin/today", nil, "")
			if status != 200 {
				t.Fatalf("today as %s answered %d %v", role, status, out)
			}
			walk(out, "today", func(p, key string, _ any) {
				if ownerKeys[key] {
					t.Errorf("Hoje as %s carries %s at %s", role, key, p)
				}
			})

			// Create and its replay.
			write := allowed(true, true, true)
			key := r.key()
			status, out = r.do(role, "POST", visBase, r.createBody(), key)
			check("create", write, status, out)
			status, replay := r.do(role, "POST", visBase, r.createBody(), key)
			check("create replay", write, status, replay)
			if write == 200 {
				if _, tracked := replay["trackingUrl"]; tracked {
					t.Errorf("a replay carries the tracking link again")
				}
			}

			// The quote has the owner's cost and margin too, and no one else's.
			quoteBody := map[string]any{"lines": r.createBody()["lines"], "terms": r.createBody()["terms"]}
			status, out = r.do(role, "POST", visBase+"/quote", quoteBody, "")
			assertQuoteVisibility(t, role, write, status, out)

			// Change, advance, item moves, payment and its replay.
			change := map[string]any{"lines": r.createBody()["lines"], "terms": r.createBody()["terms"], "customerApproved": false, "version": version}
			status, out = r.do(role, "PUT", path, change, "")
			check("change", write, status, out)
			if write == 200 {
				version++
			}
			advance := map[string]any{"to": "awaiting_deposit", "version": version}
			status, out = r.do(role, "POST", path+"/advance", advance, "")
			check("advance", write, status, out)
			if write == 200 {
				version++
			}
			itemID := firstItem(t, r, shared.RoleOwner, path)
			// The backup supplier is for an item still to be bought.
			manage := allowed(true, true, false)
			status, out = r.do(role, "POST", path+"/items/"+itemID+"/use-backup-supplier", map[string]any{"version": version}, "")
			check("backup supplier", manage, status, out)
			if manage == 200 {
				version++
			}
			status, out = r.do(role, "POST", path+"/items/"+itemID+"/move", map[string]any{"to": "sent_to_supplier", "version": version}, "")
			check("move item", write, status, out)
			if write == 200 {
				version++
			}
			pay := map[string]any{"kind": "deposit", "amountCents": 10000, "method": "pix", "version": version}
			payKey := r.key()
			status, out = r.do(role, "POST", path+"/payments", pay, payKey)
			check("payment", write, status, out)
			status, out = r.do(role, "POST", path+"/payments", pay, payKey)
			check("payment replay", write, status, out)
			if write == 200 {
				version++
			}

			// Owner and admin writes, then the owner's alone.
			date := map[string]any{"date": "2026-12-30", "reason": "Atraso", "version": version}
			status, out = r.do(role, "PUT", path+"/delivery-date", date, "")
			check("delivery date", manage, status, out)
			if manage == 200 {
				version++
			}
			ownerOnly := allowed(true, false, false)
			paidPath, paymentID, paidVersion := r.paid()
			status, out = r.do(role, "POST", paidPath+"/payments/"+paymentID+"/void", map[string]any{"reason": "Valor errado", "version": paidVersion}, "")
			check("void payment", ownerOnly, status, out)
			status, out = r.do(role, "POST", path+"/cancel", map[string]any{"reason": "Desistiu", "version": version}, "")
			check("cancel", manage, status, out)

			// Release works on an order with no deposit.
			path2, _, version2 := r.fresh()
			status, out = r.do(role, "POST", path2+"/release", map[string]any{"version": version2}, "")
			check("release", ownerOnly, status, out)
			_ = id
		})
	}
}

func firstItem(t *testing.T, r *visRig, role shared.Role, path string) string {
	t.Helper()
	_, out := r.do(role, "GET", path, nil, "")
	return out["items"].([]any)[0].(map[string]any)["id"].(string)
}

// paid makes an order as the owner and records a deposit on it, answering
// its path, the payment's id and the version after it.
func (r *visRig) paid() (path, payment string, version int) {
	r.t.Helper()
	path, _, version = r.fresh()
	pay := map[string]any{"kind": "deposit", "amountCents": 10000, "method": "pix", "version": version}
	status, out := r.do(shared.RoleOwner, "POST", path+"/payments", pay, r.key())
	if status != 201 {
		r.t.Fatalf("payment as owner answered %d %v", status, out)
	}
	return path, out["payments"].([]any)[0].(map[string]any)["id"].(string), int(out["version"].(float64))
}

// Financeiro is the owner's: any other role is refused, and its answer has
// the costs and margins the owner is owed.
func TestFinance_IsTheOwnersOnly(t *testing.T) {
	t.Parallel()

	r := newVisRig(t)
	const path = "/api/v1/admin/finance/summary?from=2026-12-01&to=2026-12-31"
	for _, role := range []shared.Role{shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer} {
		if status, out := r.do(role, "GET", path, nil, ""); status != 403 || out["code"] != respond.CodeForbidden {
			t.Errorf("%s answered %d %v, want 403 forbidden", role, status, out)
		}
	}
	if status, out := r.do(shared.RoleOwner, "GET", path, nil, ""); status != 200 {
		t.Errorf("owner answered %d %v", status, out)
	}
}

// A request without a key is refused by the real use case, after its
// permission check: a viewer gets 403, not a hint about the header.
func TestOrders_MissingKeyIsRefusedAfterThePermission(t *testing.T) {
	t.Parallel()

	r := newVisRig(t)
	if status, out := r.do(shared.RoleOwner, "POST", visBase, r.createBody(), ""); status != 400 || out["code"] != respond.CodeIdempotencyKeyMissing {
		t.Errorf("owner without a key answered %d %v, want 400 idempotency_key_required", status, out)
	}
	if status, out := r.do(shared.RoleViewer, "POST", visBase, r.createBody(), ""); status != 403 {
		t.Errorf("viewer without a key answered %d %v, want 403", status, out)
	}
	key := r.key()
	if status, _ := r.do(shared.RoleOwner, "POST", visBase, r.createBody(), key); status != 201 {
		t.Fatalf("create answered %d", status)
	}
	other := r.createBody()
	other["sendToCustomer"] = true
	if status, out := r.do(shared.RoleOwner, "POST", visBase, other, key); status != 422 || out["code"] != respond.CodeIdempotencyKeyReused {
		t.Errorf("the key with another body answered %d %v, want 422 idempotency_key_reused", status, out)
	}
}
