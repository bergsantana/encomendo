package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

var (
	orTerms = `{"handover":"pickup"}`
	orLines = fmt.Sprintf(`[{"itemId":%q,"quantity":1}]`, ctItem.String())
	// orCreateBody and orChangeBody pass the handlers' checks.
	orCreateBody = fmt.Sprintf(`{"customerId":%q,"lines":%s,"terms":%s,"sendToCustomer":true}`, orCustomer.String(), orLines, orTerms)
	orChangeBody = fmt.Sprintf(`{"lines":%s,"terms":%s,"customerApproved":true,"version":3}`, orLines, orTerms)
	orCustomerIn = fmt.Sprintf(`{"name":"João Pereira","phone":"5592981234567","email":null,"notes":"","addresses":[`+
		`{"id":%q,"label":"Casa","cep":"69057000","street":"Rua A","number":"10","complement":"","district":"Centro","city":"Manaus","state":"AM"}]}`,
		orAddress.String())
)

// orOp is one M4 operation of the contract with a body that passes, and the
// status it answers.
type orOp struct {
	pattern string
	body    string
	status  int
}

func (o orOp) method() string { m, _, _ := strings.Cut(o.pattern, " "); return m }

func (o orOp) path() string { _, p, _ := strings.Cut(o.pattern, " "); return orPaths.Replace(p) }

func (o orOp) hasBody() bool { return o.body != "" }

// orOps is every M4 operation but choosing the workflow, which is a
// tenancy route (TestTenancy_ChooseWorkflow).
func orOps() []orOp {
	const base = "/api/v1/admin/orders"
	return []orOp{
		{"GET /api/v1/admin/status-workflows", "", 200},
		{"GET /api/v1/admin/customers?q=jo&limit=10", "", 200},
		{"POST /api/v1/admin/customers", orCustomerIn, 201},
		{"GET /api/v1/admin/customers/{customerId}", "", 200},
		{"PUT /api/v1/admin/customers/{customerId}", strings.TrimSuffix(orCustomerIn, "}") + `,"version":2}`, 200},
		{"POST " + base + "/quote", fmt.Sprintf(`{"lines":%s,"terms":%s,"orderId":%q}`, orLines, orTerms, orOrder.String()), 200},
		{"GET " + base + "?status=rascunho&phase=draft&scope=all&customerId=" + orCustomer.String() + "&q=231&alert=true&limit=5", "", 200},
		{"POST " + base, orCreateBody, 201},
		{"GET " + base + "/{orderId}", "", 200},
		{"PUT " + base + "/{orderId}", orChangeBody, 200},
		{"POST " + base + "/{orderId}/advance", `{"to":"entregue","handover":"pickup","version":3}`, 200},
		{"POST " + base + "/{orderId}/release", `{"version":3}`, 200},
		{"POST " + base + "/{orderId}/items/{itemId}/move", `{"to":"received","check":"ok","version":3}`, 200},
		{"POST " + base + "/{orderId}/items/{itemId}/use-backup-supplier", `{"version":3}`, 200},
		{"POST " + base + "/{orderId}/payments", `{"kind":"deposit","amountCents":63500,"method":"pix","version":3}`, 201},
		{"POST " + base + "/{orderId}/payments/{paymentId}/void", `{"reason":"Valor errado","version":3}`, 200},
		{"PUT " + base + "/{orderId}/delivery-date", `{"date":"2026-12-28","reason":"Atraso","version":3}`, 200},
		{"POST " + base + "/{orderId}/tracking-link", "", 200},
		{"POST " + base + "/{orderId}/cancel", `{"reason":"Desistiu","version":3}`, 200},
		{"GET " + base + "/{orderId}/history?limit=5", "", 200},
		{"GET /api/v1/admin/today", "", 200},
		{"GET /api/v1/admin/finance/summary?from=2026-12-01&to=2026-12-31", "", 200},
	}
}

func TestOrders_ServeEveryOperation(t *testing.T) {
	t.Parallel()

	ops := orOps()
	if len(ops) != 22 {
		t.Fatalf("%d operations listed, the contract has 22 besides choosing the workflow", len(ops))
	}
	for _, op := range ops {
		t.Run(op.pattern, func(t *testing.T) {
			t.Parallel()

			h := newOrHandlers(t)
			rec := h.call(t, op.method(), op.path(), op.body)
			if rec.Code != op.status {
				t.Fatalf("answer %d %s, want %d", rec.Code, rec.Body.String(), op.status)
			}
			if h.uses() != 1 {
				t.Errorf("%d use-case calls, want 1", h.uses())
			}
		})
	}
}

// A path id that is not an id names nothing: 404, and no use case runs.
func TestOrders_MalformedPathIDIsNotFound(t *testing.T) {
	t.Parallel()

	for _, op := range orOps() {
		if !strings.Contains(op.pattern, "{") {
			continue
		}
		t.Run(op.pattern, func(t *testing.T) {
			t.Parallel()

			h := newOrHandlers(t)
			path := strings.NewReplacer("{orderId}", "nope", "{itemId}", "nope", "{paymentId}", "nope", "{customerId}", "nope").
				Replace(strings.SplitN(op.pattern, " ", 2)[1])
			rec := h.call(t, op.method(), path, op.body)
			if rec.Code != 404 || code(t, rec) != respond.CodeNotFound || h.uses() != 0 {
				t.Errorf("answer %d %s after %d use-case calls, want 404 not_found and none", rec.Code, rec.Body.String(), h.uses())
			}
		})
	}
}

// An unknown field in a body is 400 invalid, and a body with nothing in it
// names what is missing.
func TestOrders_RefuseUnknownFieldsAndEmptyBodies(t *testing.T) {
	t.Parallel()

	for _, op := range orOps() {
		if !op.hasBody() {
			continue
		}
		t.Run(op.pattern, func(t *testing.T) {
			t.Parallel()

			h := newOrHandlers(t)
			rec := h.call(t, op.method(), op.path(), withUnknownField(t, op.body))
			if rec.Code != 400 || code(t, rec) != respond.CodeInvalid || h.uses() != 0 {
				t.Errorf("unknown field: answer %d %s after %d use-case calls, want 400 invalid and none", rec.Code, rec.Body.String(), h.uses())
			}
			rec = h.call(t, op.method(), op.path(), `{}`)
			var p respond.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 400 || p.Code != respond.CodeInvalid || len(p.Errors) == 0 || h.uses() != 0 {
				t.Errorf("empty body: answer %d %s after %d use-case calls, want 400 naming fields and none", rec.Code, rec.Body.String(), h.uses())
			}
		})
	}
}

func TestOrders_ValidationNamesTheField(t *testing.T) {
	t.Parallel()

	const base = "/api/v1/admin/orders"
	order := base + "/" + orOrder.String()
	tests := []struct{ name, method, path, body, field string }{
		{"not JSON", "POST", base + "/quote", `{`, "body"},
		{"two objects", "POST", base + "/quote", `{} {}`, "body"},
		{"no lines", "POST", base + "/quote", `{"terms":` + orTerms + `}`, "lines"},
		{"no terms", "POST", base + "/quote", `{"lines":` + orLines + `}`, "terms"},
		{"no handover", "POST", base + "/quote", `{"lines":` + orLines + `,"terms":{}}`, "terms.handover"},
		{"bad kit id", "POST", base + "/quote", `{"lines":[{"kitId":"x"}],"terms":` + orTerms + `}`, "lines[0].kitId"},
		{"bad swap", "POST", base + "/quote", `{"lines":[{"kitId":"` + orKit.String() + `","swaps":[{"itemId":"` + ctItem.String() + `"}]}],"terms":` + orTerms + `}`, "lines[0].swaps[0].withItemId"},
		{"bad option", "POST", base + "/quote", `{"lines":[{"itemId":"` + ctItem.String() + `","quantity":1,"options":[{"itemId":"x","optionId":"x","choiceId":"x"}]}],"terms":` + orTerms + `}`, "lines[0].options[0].optionId"},
		{"bad address", "POST", base + "/quote", `{"lines":` + orLines + `,"terms":{"handover":"delivery","addressId":"x"}}`, "terms.addressId"},
		{"bad order id", "POST", base + "/quote", `{"lines":` + orLines + `,"terms":` + orTerms + `,"orderId":"x"}`, "orderId"},
		{"bad customer id", "POST", base, `{"customerId":"x","lines":` + orLines + `,"terms":` + orTerms + `}`, "customerId"},
		{"new customer without phone", "POST", base, `{"newCustomer":{"name":"Ana"},"lines":` + orLines + `,"terms":` + orTerms + `}`, "newCustomer.phone"},
		{"no approval flag", "PUT", order, `{"lines":` + orLines + `,"terms":` + orTerms + `,"version":3}`, "customerApproved"},
		{"no version", "POST", order + "/release", `{}`, "version"},
		{"version zero", "POST", order + "/release", `{"version":0}`, "version"},
		{"advance version zero", "POST", order + "/advance", `{"to":"ready","version":0}`, "version"},
		{"move version zero", "POST", order + "/items/" + orItem.String() + "/move", `{"to":"in_transit","version":0}`, "version"},
		{"cancel version zero", "POST", order + "/cancel", `{"reason":"x","version":0}`, "version"},
		{"void version zero", "POST", order + "/payments/" + orPayment.String() + "/void", `{"reason":"x","version":0}`, "version"},
		{"date version zero", "PUT", order + "/delivery-date", `{"date":"2026-12-30","version":0}`, "version"},
		{"customer version zero", "PUT", "/api/v1/admin/customers/" + orCustomer.String(), `{"name":"Ana","phone":"5592981234567","addresses":[],"version":0}`, "version"},
		{"no target status", "POST", order + "/advance", `{"version":3}`, "to"},
		{"no payment amount", "POST", order + "/payments", `{"kind":"deposit","method":"pix","version":3}`, "amountCents"},
		{"no void reason", "POST", order + "/payments/" + orPayment.String() + "/void", `{"version":3}`, "reason"},
		{"bad date", "PUT", order + "/delivery-date", `{"date":"28/12/2026","version":3}`, "date"},
		{"no cancel reason", "POST", order + "/cancel", `{"version":3}`, "reason"},
		{"bad customer filter", "GET", base + "?customerId=x", "", "customerId"},
		{"bad alert filter", "GET", base + "?alert=maybe", "", "alert"},
		{"search too long", "GET", base + "?q=" + strings.Repeat("a", 101), "", "q"},
		{"bad limit", "GET", base + "?limit=x", "", "limit"},
		{"finance without period", "GET", "/api/v1/admin/finance/summary", "", "from"},
		{"finance bad day", "GET", "/api/v1/admin/finance/summary?from=2026-12-01&to=31-12", "", "to"},
		{"customer without phone", "POST", "/api/v1/admin/customers", `{"name":"Ana","addresses":[]}`, "phone"},
		{"customer address without street", "POST", "/api/v1/admin/customers", `{"name":"Ana","phone":"5592981234567","addresses":[{"label":"Casa","cep":"69057000","city":"Manaus","state":"AM"}]}`, "addresses[0].street"},
		{"customer address id bad", "POST", "/api/v1/admin/customers", `{"name":"Ana","phone":"5592981234567","addresses":[{"id":"x","label":"Casa","cep":"69057000","street":"R","city":"Manaus","state":"AM"}]}`, "addresses[0].id"},
		{"customer search too long", "GET", "/api/v1/admin/customers?q=" + strings.Repeat("a", 101), "", "q"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newOrHandlers(t)
			rec := h.call(t, tt.method, tt.path, tt.body)
			var p respond.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(p.Errors))
			for _, e := range p.Errors {
				got = append(got, e.Field)
			}
			if rec.Code != 400 || !contains(got, tt.field) || h.uses() != 0 {
				t.Errorf("answer %d with fields %v after %d use-case calls, want 400 naming %s and none", rec.Code, got, h.uses(), tt.field)
			}
		})
	}
}

// What the body says reaches the use case whole: the lines, the terms with
// their defaults, and the customer.
func TestOrders_CreateReachesTheUseCaseWhole(t *testing.T) {
	t.Parallel()

	h := newOrHandlers(t)
	body := fmt.Sprintf(`{"newCustomer":{"name":"Ana","phone":"5592981234568"},"sendToCustomer":true,"lines":[`+
		`{"kitId":%q,"swaps":[{"itemId":%q,"withItemId":%q}],"options":[{"itemId":%q,"optionId":%q,"choiceId":%q}]},`+
		`{"itemId":%q,"quantity":3}],"terms":{"handover":"delivery","addressId":%q,"freightCents":1500,"discountCents":500,"depositPercent":30,"assemblyDays":4,"safetyMarginDays":1}}`,
		orKit, ctItem, ctItem, ctItem, ctOption, ctChoice, ctItem, orAddress)

	rec := h.call(t, "POST", "/api/v1/admin/orders", body)

	if rec.Code != 201 {
		t.Fatalf("answer %d %s", rec.Code, rec.Body.String())
	}
	got, ok := h.orders.got.(apporder.CreateInput)
	if !ok {
		t.Fatalf("use case got %T", h.orders.got)
	}
	discount, freight, deposit, assembly, margin := int64(500), int64(1500), 30, 4, 1
	want := apporder.CreateInput{
		New: &apporder.NewCustomer{Name: "Ana", Phone: "5592981234568"}, SendToCustomer: true,
		Lines: []apporder.LineRequest{
			{
				Kit: orKit, Swaps: []catalog.Swap{{Item: ctItem, With: ctItem}},
				Options: []apporder.OptionChoice{{Item: ctItem, Option: ctOption, Choice: ctChoice}},
			},
			{Item: ctItem, Quantity: 3},
		},
		Terms: apporder.TermsInput{
			Handover: order.HandoverDelivery, Address: orAddress, FreightCents: &freight, DiscountCents: &discount,
			DepositPercent: &deposit, AssemblyDays: &assembly, SafetyMarginDays: &margin,
		},
		Idempotency: got.Idempotency,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("use case got\n%+v\nwant\n%+v", got, want)
	}
}

// Terms left out take the defaults the use case reads from nil; a freight
// left out is zero (apporder.TermsInput).
// Terms left out reach the use case as nil, freight included, so a change
// keeps the order's own values (go-reviewer and security review of 4.14).
func TestOrders_TermsLeftOutAreNil(t *testing.T) {
	t.Parallel()

	h := newOrHandlers(t)
	rec := h.call(t, "PUT", "/api/v1/admin/orders/"+orOrder.String(), orChangeBody)

	if rec.Code != 200 {
		t.Fatalf("answer %d %s", rec.Code, rec.Body.String())
	}
	got := h.orders.got.(apporder.ChangeInput)
	if got.Terms != (apporder.TermsInput{Handover: order.HandoverPickup}) || !got.CustomerApproved || got.Version != 3 || got.Order != orOrder {
		t.Errorf("use case got %+v", got)
	}
}

func TestOrders_ListReadsTheFilter(t *testing.T) {
	t.Parallel()

	h := newOrHandlers(t)
	rec := h.call(t, "GET", "/api/v1/admin/orders?status=rascunho&phase=draft&scope=all&customerId="+orCustomer.String()+"&q=231&alert=true&limit=5", "")

	if rec.Code != 200 {
		t.Fatalf("answer %d %s", rec.Code, rec.Body.String())
	}
	args := h.orders.got.([]any)
	f := args[0].(apporder.ListFilter)
	want := apporder.ListFilter{Status: "rascunho", Phase: "draft", Scope: apporder.ScopeAll, Customer: orCustomer, Query: "231", Alert: true}
	if f != want || args[1].(shared.PageRequest).Limit() != 5 {
		t.Errorf("use case got %+v and limit %d, want %+v and 5", f, args[1].(shared.PageRequest).Limit(), want)
	}
	if got := body(t, rec); got["nextCursor"] != "next" {
		t.Errorf("answer %v, want the next cursor passed on", got)
	}
}

func TestOrders_FinanceReadsThePeriod(t *testing.T) {
	t.Parallel()

	h := newOrHandlers(t)
	rec := h.call(t, "GET", "/api/v1/admin/finance/summary?from=2026-12-01&to=2026-12-31", "")

	if rec.Code != 200 {
		t.Fatalf("answer %d %s", rec.Code, rec.Body.String())
	}
	got := h.orders.got.([]shared.CivilDate)
	if got[0] != orDate(1) || got[1] != orDate(31) {
		t.Errorf("use case got %v", got)
	}
	totals := body(t, rec)["toReceive"].(map[string]any)
	if totals["orders"] != float64(1) {
		t.Errorf("toReceive = %v, want the number of orders", totals)
	}
	if list, _ := totals["list"].([]any); len(list) != 1 || list[0].(map[string]any)["number"] == nil {
		t.Errorf("toReceive.list = %v, want the order with a balance", totals["list"])
	}
}

func TestOrders_MapErrorsToProblems(t *testing.T) {
	t.Parallel()

	const base = "/api/v1/admin/orders"
	o := base + "/" + orOrder.String()
	advance := orOp{"POST " + base + "/{orderId}/advance", `{"to":"entregue","version":3}`, 0}
	guard := func(g order.Guard) error { return &order.GuardError{Guard: g} }
	tests := []struct {
		name         string
		method, path string
		body         string
		err          error
		status       int
		code         string
	}{
		{"invalid transition", "POST", o + "/advance", advance.body, order.ErrInvalidTransition, 409, respond.CodeInvalidTransition},
		{"deposit unpaid", "POST", o + "/advance", advance.body, guard(order.GuardDepositUnpaid), 409, respond.CodeGuardFailed},
		{"items not ready", "POST", o + "/advance", advance.body, fmt.Errorf("advance: %w", guard(order.GuardItemsNotReady)), 409, respond.CodeGuardFailed},
		{"balance unpaid", "POST", o + "/advance", advance.body, guard(order.GuardBalanceUnpaid), 409, respond.CodeGuardFailed},
		{"not editable", "PUT", o, orChangeBody, order.ErrNotEditable, 409, respond.CodeOrderNotEditable},
		{"closed", "POST", o + "/cancel", `{"reason":"x","version":3}`, order.ErrOrderClosed, 409, respond.CodeOrderClosed},
		{"approval required", "PUT", o, orChangeBody, order.ErrApprovalRequired, 400, respond.CodeApprovalRequired},
		{"payment exceeds due", "POST", o + "/payments", `{"kind":"balance","amountCents":1,"method":"pix","version":3}`, order.ErrPaymentExceedsDue, 400, respond.CodePaymentExceedsDue},
		{"invalid item move", "POST", o + "/items/" + orItem.String() + "/move", `{"to":"installed","version":3}`, order.ErrInvalidItemMove, 409, respond.CodeInvalidItemMove},
		{"no backup supplier", "POST", o + "/items/" + orItem.String() + "/use-backup-supplier", `{"version":3}`, order.ErrNoBackupSupplier, 409, respond.CodeNoBackupSupplier},
		{"item archived on create", "POST", base, orCreateBody, refusal(catalog.ErrItemArchived), 409, respond.CodeItemUnavailable},
		{"item archived on change", "PUT", o, orChangeBody, refusal(catalog.ErrItemArchived), 409, respond.CodeItemUnavailable},
		{"item archived on quote", "POST", base + "/quote", `{"lines":` + orLines + `,"terms":` + orTerms + `}`, refusal(catalog.ErrItemArchived), 409, respond.CodeItemUnavailable},
		{"stale version", "POST", o + "/release", `{"version":3}`, shared.ErrVersionConflict, 409, respond.CodeVersionConflict},
		{"key reused", "POST", base, orCreateBody, appshared.ErrIdempotencyKeyReused, 422, respond.CodeIdempotencyKeyReused},
		{"key reused on a payment", "POST", o + "/payments", `{"kind":"deposit","amountCents":1,"method":"pix","version":3}`, appshared.ErrIdempotencyKeyReused, 422, respond.CodeIdempotencyKeyReused},
		{"key in flight", "POST", base, orCreateBody, appshared.ErrIdempotencyKeyInFlight, 409, respond.CodeConflict},
		{"key missing", "POST", base, orCreateBody, apporder.ErrIdempotencyKeyRequired, 400, respond.CodeIdempotencyKeyMissing},
		{"order gone", "GET", o, "", apporder.ErrNotFound, 404, respond.CodeNotFound},
		{"past date", "PUT", o + "/delivery-date", `{"date":"2020-01-01","version":3}`, shared.NewValidationError(shared.FieldError{Field: "date", Message: "Escolha uma data a partir de hoje."}), 400, respond.CodeInvalid},
		{"handover missing", "POST", o + "/advance", advance.body, shared.NewValidationError(shared.FieldError{Field: "handover", Message: "x"}), 400, respond.CodeInvalid},
		{"no permission", "POST", o + "/cancel", `{"reason":"x","version":3}`, shared.ErrForbidden, 403, respond.CodeForbidden},
		{"a bug", "GET", base, "", fmt.Errorf("boom"), 500, respond.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newOrHandlers(t)
			h.fail(tt.err)
			rec := h.call(t, tt.method, tt.path, tt.body)
			if rec.Code != tt.status || code(t, rec) != tt.code {
				t.Errorf("answer %d %s, want %d %s", rec.Code, rec.Body.String(), tt.status, tt.code)
			}
			var p respond.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if tt.code == respond.CodeGuardFailed && p.Guard == "" {
				t.Errorf("guard_failed names no guard: %s", rec.Body.String())
			}
			if tt.code == respond.CodeItemUnavailable && len(p.References) != 1 {
				t.Errorf("item_unavailable references = %v, want the archived row", p.References)
			}
		})
	}
}

// Each guard answers its own pt-BR title and names itself in `guard`.
func TestOrders_GuardFailedNamesTheGuard(t *testing.T) {
	t.Parallel()

	for g, title := range map[order.Guard]string{
		order.GuardDepositUnpaid: "O sinal ainda não foi pago. Registre o sinal ou libere o pedido sem ele.",
		order.GuardItemsNotReady: "Nem todos os itens chegaram e foram conferidos.",
		order.GuardBalanceUnpaid: "Falta receber o saldo deste pedido.",
	} {
		t.Run(string(g), func(t *testing.T) {
			t.Parallel()

			h := newOrHandlers(t)
			h.fail(&order.GuardError{Guard: g})
			rec := h.call(t, "POST", "/api/v1/admin/orders/"+orOrder.String()+"/advance", `{"to":"entregue","version":3}`)
			var p respond.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if p.Guard != string(g) || p.Title != title || p.Status != 409 {
				t.Errorf("problem = %+v, want guard %s titled %q", p, g, title)
			}
		})
	}
}

// The catalog keeps item_archived for its own routes: only an order route
// turns the refusal into item_unavailable (the catalog tests pin the
// other half).
func TestOrders_PhoneTakenAnswersItsOwnCode(t *testing.T) {
	t.Parallel()

	h := newOrHandlers(t)
	h.fail(appcustomer.ErrPhoneTaken)
	rec := h.call(t, "POST", "/api/v1/admin/customers", orCustomerIn)
	if rec.Code != 409 || code(t, rec) != respond.CodePhoneTaken {
		t.Errorf("answer %d %s, want 409 phone_taken", rec.Code, rec.Body.String())
	}
}

// requests that need a key and send none reach the use case without one,
// which refuses them after checking the permission (ADR 0030).
func TestOrders_MissingKeyIsLeftToTheUseCase(t *testing.T) {
	t.Parallel()

	const base = "/api/v1/admin/orders"
	tests := []struct{ name, path, body string }{
		{"create", base, orCreateBody},
		{"payment", base + "/" + orOrder.String() + "/payments", `{"kind":"deposit","amountCents":1,"method":"pix","version":3}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newOrHandlers(t)
			rec := h.serve(t, orRequest{caller: staffIn(t, shared.RoleOwner), method: "POST", path: tt.path, body: tt.body})
			if rec.Code != 400 || code(t, rec) != respond.CodeIdempotencyKeyMissing || h.orders.calls != 1 {
				t.Errorf("answer %d %s after %d use-case calls, want 400 idempotency_key_required from the use case", rec.Code, rec.Body.String(), h.orders.calls)
			}
		})
	}
}

func TestOrders_RefuseAMalformedKey(t *testing.T) {
	t.Parallel()

	for name, key := range map[string]string{
		"a space":   "has space",
		"too long":  strings.Repeat("k", 129),
		"non-ASCII": "chave-ç",
		"a control": "a\tb",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newOrHandlers(t)
			rec := h.serve(t, orRequest{
				caller: staffIn(t, shared.RoleOwner), method: "POST", path: "/api/v1/admin/orders", body: orCreateBody,
				headers: map[string]string{"Idempotency-Key": key},
			})
			if rec.Code != 400 || code(t, rec) != respond.CodeInvalid || h.uses() != 0 {
				t.Errorf("answer %d %s after %d use-case calls, want 400 invalid and none", rec.Code, rec.Body.String(), h.uses())
			}
		})
	}
}

// keyOf sends a creating request and returns the key the use case got.
func keyOf(t *testing.T, h *orHandlers, caller shared.Principal, path, body, key string) appshared.IdempotencyKey {
	t.Helper()
	rec := h.serve(t, orRequest{
		caller: caller, method: "POST", path: path, body: body, headers: map[string]string{"Idempotency-Key": key},
	})
	if rec.Code != 201 {
		t.Fatalf("answer %d %s", rec.Code, rec.Body.String())
	}
	switch in := h.orders.got.(type) {
	case apporder.CreateInput:
		return in.Idempotency
	case apporder.PaymentInput:
		return in.Idempotency
	}
	t.Fatalf("use case got %T", h.orders.got)
	return appshared.IdempotencyKey{}
}

// A retry (same key, user, path and body) hashes the same; any difference
// changes the hash, so the use case refuses the key with another request,
// and two staff members never replay each other's order (security review
// of 4.12 and 4.13).
func TestOrders_KeyHashCoversUserMethodPathAndBody(t *testing.T) {
	t.Parallel()

	const orders = "/api/v1/admin/orders"
	h := newOrHandlers(t)
	owner := staffIn(t, shared.RoleOwner)
	other, err := shared.NewBackofficePrincipal(shared.UserID(ctID(999)), tenantID, shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	first := keyOf(t, h, owner, orders, orCreateBody, "key-1")
	replay := keyOf(t, h, owner, orders, orCreateBody, "key-1")
	if first.Key != "key-1" || first != replay {
		t.Errorf("a retry hashed %x then %x, want the same", first.RequestHash, replay.RequestHash)
	}
	differs := map[string]appshared.IdempotencyKey{
		"another body":  keyOf(t, h, owner, orders, strings.Replace(orCreateBody, `"sendToCustomer":true`, `"sendToCustomer":false`, 1), "key-1"),
		"another user":  keyOf(t, h, other, orders, orCreateBody, "key-1"),
		"a payment":     keyOf(t, h, owner, orders+"/"+orOrder.String()+"/payments", `{"kind":"deposit","amountCents":1,"method":"pix","version":3}`, "key-1"),
		"another order": keyOf(t, h, owner, orders+"/"+ctItem.String()+"/payments", `{"kind":"deposit","amountCents":1,"method":"pix","version":3}`, "key-1"),
	}
	for name, k := range differs {
		if k.Key != "key-1" || k.RequestHash == first.RequestHash {
			t.Errorf("%s hashed like the first request", name)
		}
	}
	if differs["another user"].RequestHash == keyOf(t, h, other, orders, orCreateBody, "key-1").RequestHash {
		return
	}
	t.Error("the same user's retry hashed differently")
}

// Whitespace in the JSON is part of the body: a retry sends the same bytes.
func TestOrders_PaymentRetryKeepsItsKey(t *testing.T) {
	t.Parallel()

	h := newOrHandlers(t)
	owner := staffIn(t, shared.RoleOwner)
	path, body := "/api/v1/admin/orders/"+orOrder.String()+"/payments", `{"kind":"deposit","amountCents":63500,"method":"pix","version":3}`
	a, b := keyOf(t, h, owner, path, body, "pay-1"), keyOf(t, h, owner, path, body, "pay-1")
	if a != b || a.IsZero() {
		t.Errorf("keys %+v and %+v, want one key", a, b)
	}
}

func TestTenancy_ChooseWorkflow(t *testing.T) {
	t.Parallel()

	body := fmt.Sprintf(`{"workflowId":%q,"version":1,"expectedVersion":3}`, orWorkflow.String())
	tests := []struct {
		name   string
		body   string
		err    error
		status int
		code   string
	}{
		{"chosen", body, nil, 200, ""},
		{"unknown preset", body, tenancy.ErrWorkflowNotFound, 404, respond.CodeWorkflowNotFound},
		{"stale settings", body, shared.ErrVersionConflict, 409, respond.CodeVersionConflict},
		{"not the owner", body, shared.ErrForbidden, 403, respond.CodeForbidden},
		{"nothing sent", `{}`, nil, 400, respond.CodeInvalid},
		{"bad workflow id", `{"workflowId":"x","version":1,"expectedVersion":1}`, nil, 400, respond.CodeInvalid},
		{"unknown field", strings.TrimSuffix(body, "}") + `,"bogus":1}`, nil, 400, respond.CodeInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newTenancyFixture(t)
			f.uc.err = tt.err
			rec := f.call(t, staffIn(t, shared.RoleOwner), "PUT", "/api/v1/admin/settings/status-workflow", tt.body)
			if rec.Code != tt.status || (tt.code != "" && code(t, rec) != tt.code) {
				t.Errorf("answer %d %s, want %d %s", rec.Code, rec.Body.String(), tt.status, tt.code)
			}
			if tt.status == http.StatusOK {
				want := apptenancy.ChooseWorkflowInput{Workflow: orWorkflow, Version: 1, ExpectedVersion: 3}
				if f.uc.got != want {
					t.Errorf("use case got %+v, want %+v", f.uc.got, want)
				}
			}
		})
	}
}
