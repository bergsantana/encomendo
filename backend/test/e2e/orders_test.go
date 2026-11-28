//go:build e2e

package e2e_test

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// ordersStore is a store that sells one bought item with a 15-day supplier
// and one item made in-house in 15 days, on the encomenda_montagem
// workflow, with one customer.
type ordersStore struct {
	owner    *session
	item     string // bought: moves through purchasing (ADR 0031)
	made     string // in-house: moves by hand until milestone 6
	customer string
	email    string
}

// newOrdersStore signs an owner up, opens a store, chooses the
// encomenda_montagem workflow, and makes a supplier, an item and a customer.
func newOrdersStore(t *testing.T) *ordersStore {
	t.Helper()
	owner := signUp(t, "Ana Souza")
	owner.newStore(t, "Alta RPM")

	presets := must(t, call(t, "GET", "/api/v1/admin/status-workflows", nil, owner.access), 200, "list workflows")
	var workflowID string
	var workflowVersion float64
	for _, it := range presets.Body["items"].([]any) {
		w := it.(map[string]any)
		if w["key"] == "encomenda_montagem" {
			workflowID, workflowVersion = w["id"].(string), w["version"].(float64)
		}
	}
	if workflowID == "" {
		t.Fatalf("no encomenda_montagem preset in %v", presets.Body)
	}
	settings := must(t, call(t, "GET", "/api/v1/admin/settings", nil, owner.access), 200, "read settings")
	chosen := must(t, call(t, "PUT", "/api/v1/admin/settings/status-workflow", map[string]any{
		"workflowId": workflowID, "version": workflowVersion, "expectedVersion": settings.Body["version"],
	}, owner.access), 200, "choose the workflow")
	if wf, _ := chosen.Body["statusWorkflow"].(map[string]any); wf["id"] != workflowID {
		t.Fatalf("the store's workflow = %v, want %s", chosen.Body["statusWorkflow"], workflowID)
	}

	supplier := must(t, call(t, "POST", "/api/v1/admin/suppliers", map[string]any{
		"name": "Couros Paulista", "whatsappPhone": "5511987654321",
	}, owner.access), 201, "create supplier")
	item := must(t, call(t, "POST", "/api/v1/admin/catalog/items", map[string]any{
		"name": "Banco de couro", "description": "", "categoryId": nil, "sourcingMode": "to_order", "priceCents": 100000,
		"productionDays": nil, "mainSupplier": map[string]any{"supplierId": supplier.str("id"), "leadDays": 15, "costCents": 40000},
		"backupSupplier": nil, "attributeValueIds": []any{}, "options": []any{}, "published": true,
	}, owner.access), 201, "create item")

	// Produção on, for the in-house item.
	settings = must(t, call(t, "GET", "/api/v1/admin/settings", nil, owner.access), 200, "read settings again")
	edit := map[string]any{"productionEnabled": true, "version": settings.Body["version"]}
	for _, k := range []string{"stockEnabled", "purchaseBeforeDeposit", "depositPercent", "assemblyDays", "safetyMarginDays", "whatsappPhone", "timeZone"} {
		edit[k] = settings.Body[k]
	}
	must(t, call(t, "PUT", "/api/v1/admin/settings", edit, owner.access), 200, "turn Produção on")
	made := must(t, call(t, "POST", "/api/v1/admin/catalog/items", map[string]any{
		"name": "Banco de couro sob medida", "description": "", "categoryId": nil, "sourcingMode": "in_house", "priceCents": 100000,
		"productionDays": 15, "productionCostCents": 40000, "mainSupplier": nil, "backupSupplier": nil,
		"attributeValueIds": []any{}, "options": []any{}, "published": true,
	}, owner.access), 201, "create the in-house item")

	email := unique("cliente") + "@teste.encomendo.local"
	customer := must(t, call(t, "POST", "/api/v1/admin/customers", map[string]any{
		"name": "João Pereira", "phone": "5592981234567", "email": email, "notes": "", "addresses": []any{},
	}, owner.access), 201, "create customer")
	return &ordersStore{owner: owner, item: item.str("id"), made: made.str("id"), customer: customer.str("id"), email: email}
}

// create makes an order of the bought item and returns the answer.
func (s *ordersStore) create(t *testing.T, key string, send bool) answer {
	t.Helper()
	return s.createOf(t, s.item, key, send)
}

// createOf makes an order of one item and returns the answer.
func (s *ordersStore) createOf(t *testing.T, item, key string, send bool) answer {
	t.Helper()
	return must(t, callWith(t, "POST", "/api/v1/admin/orders", map[string]any{
		"customerId": s.customer, "sendToCustomer": send,
		"lines": []any{map[string]any{"itemId": item, "quantity": 1}},
		"terms": map[string]any{"handover": "pickup"},
	}, s.owner.access, map[string]string{"Idempotency-Key": key}), 201, "create order")
}

func (s *ordersStore) order(t *testing.T, id string) map[string]any {
	t.Helper()
	return must(t, call(t, "GET", "/api/v1/admin/orders/"+id, nil, s.owner.access), 200, "get order").Body
}

func version(o map[string]any) float64 { v, _ := o["version"].(float64); return v }

func cents(o map[string]any, key string) float64 {
	m, _ := o[key].(map[string]any)
	c, _ := m["cents"].(float64)
	return c
}

// Milestone 4's flow against the real stack: from a chosen workflow to an
// order handed over, with the deposit, the items made, every guard and the
// balance (docs/plans/m4-customers-orders-deadlines.md, step 4.14). The
// item is made in-house: bought items move through purchasing, whose flow
// is TestPurchasing_* (step 5.10; ADR 0031).
func TestOrders_FromQuoteToHandover(t *testing.T) {
	t.Parallel()

	s := newOrdersStore(t)
	created := s.createOf(t, s.made, unique("order"), true)
	o := created.Body["order"].(map[string]any)
	id := o["id"].(string)
	if created.str("trackingUrl") == "" || !strings.HasPrefix(created.str("whatsappUrl"), "https://wa.me/5592981234567?text=") {
		t.Errorf("created = tracking %q, whatsapp %q; want both links", created.str("trackingUrl"), created.str("whatsappUrl"))
	}
	if st, _ := o["status"].(map[string]any); st["key"] != "awaiting_deposit" || o["depositPercent"] != float64(50) {
		t.Fatalf("status %v, deposit %v; want awaiting_deposit at 50%%", o["status"], o["depositPercent"])
	}
	if cents(o, "depositDue")*2 != cents(o, "total") || o["cost"] == nil || cents(o, "total") != 100000 {
		t.Errorf("total %v, deposit due %v, cost %v; want R$ 1.000,00, half of it, and the owner's cost", o["total"], o["depositDue"], o["cost"])
	}

	// The guard: sourcing waits for the deposit.
	blocked := call(t, "POST", "/api/v1/admin/orders/"+id+"/advance", map[string]any{"to": "purchasing", "version": version(o)}, s.owner.access)
	if blocked.Status != 409 || blocked.str("code") != "guard_failed" || blocked.str("guard") != "deposit_unpaid" {
		t.Fatalf("advance without the deposit = %d %v, want 409 guard_failed deposit_unpaid", blocked.Status, blocked.Body)
	}

	// The deposit, and its retry with the same key.
	payKey := unique("deposit")
	deposit := map[string]any{"kind": "deposit", "amountCents": cents(o, "depositDue"), "method": "pix", "version": version(o)}
	paid := must(t, callWith(t, "POST", "/api/v1/admin/orders/"+id+"/payments", deposit, s.owner.access, map[string]string{"Idempotency-Key": payKey}), 201, "record deposit")
	replay := must(t, callWith(t, "POST", "/api/v1/admin/orders/"+id+"/payments", deposit, s.owner.access, map[string]string{"Idempotency-Key": payKey}), 201, "retry the deposit")
	if n := len(replay.Body["payments"].([]any)); n != 1 || paid.Body["depositPaid"] != true || paid.Body["releaseDate"] == nil {
		t.Errorf("after the retry: %d payments, deposit paid %v, release date %v; want one payment, paid and released", n, paid.Body["depositPaid"], paid.Body["releaseDate"])
	}
	other := map[string]any{"kind": "deposit", "amountCents": 1, "method": "pix", "version": version(replay.Body)}
	if a := callWith(t, "POST", "/api/v1/admin/orders/"+id+"/payments", other, s.owner.access, map[string]string{"Idempotency-Key": payKey}); a.Status != 422 || a.str("code") != "idempotency_key_reused" {
		t.Errorf("the key with another body = %d %v, want 422 idempotency_key_reused", a.Status, a.Body)
	}
	if a := call(t, "POST", "/api/v1/admin/orders/"+id+"/payments", other, s.owner.access); a.Status != 400 || a.str("code") != "idempotency_key_required" {
		t.Errorf("a payment without a key = %d %v, want 400 idempotency_key_required", a.Status, a.Body)
	}

	// Sourcing, then the item arrives and is checked.
	o = s.order(t, id)
	o = must(t, call(t, "POST", "/api/v1/admin/orders/"+id+"/advance", map[string]any{"to": "purchasing", "version": version(o)}, s.owner.access), 200, "advance to purchasing").Body
	early := call(t, "POST", "/api/v1/admin/orders/"+id+"/advance", map[string]any{"to": "assembling", "version": version(o)}, s.owner.access)
	if early.Status != 409 || early.str("guard") != "items_not_ready" {
		t.Fatalf("advance before the items arrive = %d %v, want 409 guard_failed items_not_ready", early.Status, early.Body)
	}
	itemID := o["items"].([]any)[0].(map[string]any)["id"].(string)
	o = must(t, call(t, "POST", "/api/v1/admin/orders/"+id+"/items/"+itemID+"/move", map[string]any{"to": "in_production", "version": version(o)}, s.owner.access), 200, "start making the item").Body
	o = must(t, call(t, "POST", "/api/v1/admin/orders/"+id+"/items/"+itemID+"/move", map[string]any{"to": "ready", "check": "ok", "version": version(o)}, s.owner.access), 200, "the item is made").Body
	if it := o["items"].([]any)[0].(map[string]any); it["arrived"] != true || it["check"] != "ok" {
		t.Errorf("item after making it = %v, want arrived and checked ok", it)
	}

	// Em montagem, Pronto, the balance, Entregue (pickup).
	o = must(t, call(t, "POST", "/api/v1/admin/orders/"+id+"/advance", map[string]any{"to": "assembling", "version": version(o)}, s.owner.access), 200, "advance to assembling").Body
	o = must(t, call(t, "POST", "/api/v1/admin/orders/"+id+"/advance", map[string]any{"to": "ready", "version": version(o)}, s.owner.access), 200, "advance to ready").Body
	unpaid := call(t, "POST", "/api/v1/admin/orders/"+id+"/advance", map[string]any{"to": "delivered", "handover": "pickup", "version": version(o)}, s.owner.access)
	if unpaid.Status != 409 || unpaid.str("guard") != "balance_unpaid" {
		t.Fatalf("advance with the balance unpaid = %d %v, want 409 guard_failed balance_unpaid", unpaid.Status, unpaid.Body)
	}
	balance := map[string]any{"kind": "balance", "amountCents": cents(o, "balanceDue"), "method": "dinheiro", "version": version(o)}
	o = must(t, callWith(t, "POST", "/api/v1/admin/orders/"+id+"/payments", balance, s.owner.access, map[string]string{"Idempotency-Key": unique("balance")}), 201, "record balance").Body
	if cents(o, "balanceDue") != 0 {
		t.Errorf("balance due after paying it = %v, want 0", o["balanceDue"])
	}
	o = must(t, call(t, "POST", "/api/v1/admin/orders/"+id+"/advance", map[string]any{"to": "delivered", "handover": "pickup", "version": version(o)}, s.owner.access), 200, "hand over").Body
	if st, _ := o["status"].(map[string]any); st["phase"] != "handed_over" || o["handover"] != "pickup" {
		t.Errorf("final status = %v, handover %v; want handed_over by pickup", o["status"], o["handover"])
	}

	// A closed order takes no change, and it leaves Hoje.
	closed := call(t, "POST", "/api/v1/admin/orders/"+id+"/cancel", map[string]any{"reason": "x", "version": version(o)}, s.owner.access)
	if closed.Status != 409 || closed.str("code") != "order_closed" {
		t.Errorf("cancel after the hand-over = %d %v, want 409 order_closed", closed.Status, closed.Body)
	}
	today := must(t, call(t, "GET", "/api/v1/admin/today", nil, s.owner.access), 200, "Hoje")
	if counts, _ := today.Body["counts"].(map[string]any); counts["active"] != float64(0) {
		t.Errorf("Hoje counts %v after the only order was handed over, want 0 active", counts)
	}
	finance := must(t, call(t, "GET", "/api/v1/admin/finance/summary?from="+time.Now().AddDate(0, 0, -1).Format("2006-01-02")+"&to="+time.Now().AddDate(0, 0, 1).Format("2006-01-02"), nil, s.owner.access), 200, "Financeiro")
	if received, _ := finance.Body["received"].(map[string]any); cents(received, "total") != 100000 {
		t.Errorf("Financeiro received %v, want R$ 1.000,00", received)
	}
}

// Changing the date gives a wa.me link and sends nothing: the customer's
// mailbox, the only channel the stack has, stays empty (docs/architecture.md
// section 4.2; rule 9).
func TestOrders_ChangingTheDateOnlyMakesALink(t *testing.T) {
	t.Parallel()

	s := newOrdersStore(t)
	o := s.create(t, unique("order"), false).Body["order"].(map[string]any)
	id := o["id"].(string)
	date := time.Now().AddDate(0, 0, 90).Format("2006-01-02")

	changed := must(t, call(t, "PUT", "/api/v1/admin/orders/"+id+"/delivery-date", map[string]any{
		"date": date, "reason": "Atraso do fornecedor", "version": version(o),
	}, s.owner.access), 200, "change the date")

	if !strings.HasPrefix(changed.str("whatsappUrl"), "https://wa.me/5592981234567?text=") || strings.Contains(changed.str("whatsappUrl"), "/p/") {
		t.Errorf("whatsappUrl = %q, want a wa.me link with no tracking link", changed.str("whatsappUrl"))
	}
	order := changed.Body["order"].(map[string]any)
	if order["deliveryDate"] != date || order["deliveryDateChanged"] != true {
		t.Errorf("order after the change: date %v changed %v, want %s by hand", order["deliveryDate"], order["deliveryDateChanged"], date)
	}
	history := must(t, call(t, "GET", "/api/v1/admin/orders/"+id+"/history", nil, s.owner.access), 200, "history")
	var sawDate bool
	for _, e := range history.Body["items"].([]any) {
		if e.(map[string]any)["kind"] == "date" {
			sawDate = true
		}
	}
	if !sawDate {
		t.Errorf("history %v has no date change", history.Body["items"])
	}
	var mail struct {
		Messages []struct{ ID string } `json:"messages"`
	}
	getJSON(t, mailpitURL+"/api/v1/search?query="+url.QueryEscape("to:"+s.email), &mail)
	if len(mail.Messages) != 0 {
		t.Errorf("the customer got %d emails, want none: nothing is sent automatically", len(mail.Messages))
	}
}

// A second staff member never replays another's order with the same key.
func TestOrders_AKeyIsPerPerson(t *testing.T) {
	t.Parallel()

	s := newOrdersStore(t)
	key := unique("shared-key")
	first := s.create(t, key, false).Body["order"].(map[string]any)

	email := unique("equipe") + "@teste.encomendo.local"
	must(t, call(t, "POST", "/api/v1/admin/invitations", map[string]any{"email": email, "role": "staff"}, s.owner.access), 201, "invite")
	staff := &session{email: email, password: "segredo-forte-2"}
	staff.take(must(t, call(t, "POST", "/auth/invitations/accept", map[string]any{
		"token": invitationLink(t, email), "name": "Caio Reis", "password": staff.password, "refreshTransport": "body",
	}, ""), 200, "accept"))

	body := map[string]any{
		"customerId": s.customer, "lines": []any{map[string]any{"itemId": s.item, "quantity": 1}},
		"terms": map[string]any{"handover": "pickup"},
	}
	// The same key and body from another person is not the first person's
	// retry: it is a key in use, never a replay of the first order.
	clash := callWith(t, "POST", "/api/v1/admin/orders", body, staff.access, map[string]string{"Idempotency-Key": key})
	if clash.Status != 422 || clash.str("code") != "idempotency_key_reused" || clash.Body["order"] != nil {
		t.Errorf("a second person with the same key = %d %v, want 422 idempotency_key_reused and no order", clash.Status, clash.Body)
	}
	second := must(t, callWith(t, "POST", "/api/v1/admin/orders", body, staff.access, map[string]string{"Idempotency-Key": unique("own-key")}), 201, "create as staff")
	if second.Body["order"].(map[string]any)["id"] == first["id"] {
		t.Errorf("a second person's order is the first person's")
	}
	if _, ok := second.Body["order"].(map[string]any)["cost"]; ok {
		t.Errorf("staff got a cost: %v", second.Body["order"])
	}
}

// A bought item is not moved on the order: Compras and Receber itens move
// it, and the order offers no move until it is received (ADR 0031).
func TestOrders_BoughtItemsMoveOnlyThroughPurchasing(t *testing.T) {
	t.Parallel()

	s := newOrdersStore(t)
	o := s.create(t, unique("order"), false).Body["order"].(map[string]any)
	id := o["id"].(string)
	it := o["items"].([]any)[0].(map[string]any)
	if moves, _ := it["nextMoves"].([]any); len(moves) != 0 {
		t.Errorf("a bought item to buy offers %v, want no move on the order", moves)
	}
	got := call(t, "POST", "/api/v1/admin/orders/"+id+"/items/"+it["id"].(string)+"/move",
		map[string]any{"to": "received", "check": "ok", "version": version(o)}, s.owner.access)
	if got.Status != 409 || got.str("code") != "item_moved_by_purchasing" {
		t.Errorf("receive a bought item on the order = %d %v, want 409 item_moved_by_purchasing", got.Status, got.Body)
	}
}
