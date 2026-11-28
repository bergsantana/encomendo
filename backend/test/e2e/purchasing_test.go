//go:build e2e

package e2e_test

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Milestone 5's flow against the real stack: a bought item from the deposit
// to the order moving into assembling, through Compras and Receber itens
// (docs/plans/m5-purchasing-receiving.md, step 5.10): the deposit, "A
// comprar", the send, the promised date, a receipt with one defect that
// brings the item back as "Reposição", a second send and a receipt that is
// ok.
//
// UNVERIFIED when written: it needs the Postgres purchasing repository of
// step 5.9, which was built on another branch. It was vetted with
// `go vet -tags e2e` only; run it with make stack-up and the e2e job once
// 5.9 is merged.
func TestPurchasing_FromDepositToAssembling(t *testing.T) {
	t.Parallel()

	s := newOrdersStore(t)
	const base = "/api/v1/admin/purchasing"
	token := s.owner.access
	supplier := firstSupplierID(t, s)

	created := s.create(t, unique("order"), false).Body["order"].(map[string]any)
	id := created["id"].(string)
	itemID := created["items"].([]any)[0].(map[string]any)["id"].(string)

	// Nothing to buy before the deposit.
	if n := len(must(t, call(t, "GET", base+"/to-buy", nil, token), 200, "A comprar before the deposit").Body["suppliers"].([]any)); n != 0 {
		t.Fatalf("A comprar lists %d suppliers before the deposit, want none", n)
	}

	// The deposit releases the order, and the item shows in A comprar with
	// the owner's cost.
	deposit := map[string]any{"kind": "deposit", "amountCents": cents(created, "depositDue"), "method": "pix", "version": version(created)}
	o := must(t, callWith(t, "POST", "/api/v1/admin/orders/"+id+"/payments", deposit, token, map[string]string{"Idempotency-Key": unique("deposit")}), 201, "record deposit").Body
	o = must(t, call(t, "POST", "/api/v1/admin/orders/"+id+"/advance", map[string]any{"to": "purchasing", "version": version(o)}, token), 200, "advance to purchasing").Body
	group := toBuyOf(t, s, supplier)
	row := group["items"].([]any)[0].(map[string]any)
	if row["orderItemId"] != itemID || row["replacement"] != false || cents(row, "cost") != 40000 {
		t.Fatalf("A comprar row = %v, want the item, not a replacement, at the owner's cost of R$ 400,00", row)
	}

	// Preview, send, and a retry of the send with the same key.
	pick := map[string]any{"supplierId": supplier, "orderItemIds": []any{itemID}}
	preview := must(t, call(t, "POST", base+"/supplier-orders/preview", pick, token), 200, "preview").str("text")
	sendKey := unique("send")
	sent := must(t, callWith(t, "POST", base+"/supplier-orders", pick, token, map[string]string{"Idempotency-Key": sendKey}), 201, "send")
	first := sent.Body["supplierOrder"].(map[string]any)
	if first["number"] != "C-0001" || !strings.HasPrefix(sent.str("whatsappUrl"), "https://wa.me/5511987654321?text=") || !strings.Contains(sent.str("text"), "Banco de couro") {
		t.Fatalf("send = number %v, link %q, text %q", first["number"], sent.str("whatsappUrl"), sent.str("text"))
	}
	for _, text := range []string{preview, sent.str("text")} {
		if strings.Contains(text, "João") || strings.Contains(text, "5592981234567") || strings.Contains(text, "P-0") {
			t.Errorf("a supplier text holds client data: %q", text)
		}
	}
	replay := must(t, callWith(t, "POST", base+"/supplier-orders", pick, token, map[string]string{"Idempotency-Key": sendKey}), 201, "retry the send")
	if replay.Body["supplierOrder"].(map[string]any)["id"] != first["id"] {
		t.Errorf("the retry made another pedido de compra: %v", replay.Body["supplierOrder"])
	}
	again := callWith(t, "POST", base+"/supplier-orders", pick, token, map[string]string{"Idempotency-Key": unique("send")})
	if again.Status != 409 || again.str("code") != "item_not_to_buy" {
		t.Errorf("sending an item already sent = %d %v, want 409 item_not_to_buy", again.Status, again.Body)
	}
	if st := s.order(t, id)["items"].([]any)[0].(map[string]any)["status"]; st != "sent_to_supplier" {
		t.Errorf("item status after the send = %v, want sent_to_supplier", st)
	}

	// The promised date redates the forecast, never the promise.
	before := s.order(t, id)
	date := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	dated := must(t, call(t, "PUT", base+"/supplier-orders/"+first["id"].(string)+"/promised-date",
		map[string]any{"version": first["version"], "date": date}, token), 200, "promised date").Body
	if dated["promisedDate"] != date {
		t.Errorf("promised date = %v, want %s", dated["promisedDate"], date)
	}
	after := s.order(t, id)
	if after["deliveryDate"] != before["deliveryDate"] {
		t.Errorf("the promise to the client moved from %v to %v", before["deliveryDate"], after["deliveryDate"])
	}
	if item := after["items"].([]any)[0].(map[string]any); item["expectedDate"] == before["items"].([]any)[0].(map[string]any)["expectedDate"] {
		t.Errorf("the forecast stayed at %v after a promised date of %s", item["expectedDate"], date)
	}

	// Receive it with a defect: it comes back as Reposição.
	line := toReceiveLine(t, s, 0)
	noNote := call(t, "POST", base+"/receipts", map[string]any{"entries": []any{map[string]any{"lineId": line, "check": "defective"}}}, token)
	if noNote.Status != 400 {
		t.Errorf("a defect with no note = %d %v, want 400", noNote.Status, noNote.Body)
	}
	defective := must(t, callWith(t, "POST", base+"/receipts",
		map[string]any{"entries": []any{map[string]any{"lineId": line, "check": "defective", "note": "Veio rasgado"}}},
		token, map[string]string{"Idempotency-Key": unique("receipt")}), 201, "receive with a defect")
	if back, _ := defective.Body["backToBuy"].([]any); len(back) != 1 || len(defective.Body["completedOrders"].([]any)) != 0 {
		t.Fatalf("receipt = %v, want one item back to buy and no completed order", defective.Body)
	}
	row = toBuyOf(t, s, supplier)["items"].([]any)[0].(map[string]any)
	if row["orderItemId"] != itemID || row["replacement"] != true {
		t.Fatalf("A comprar after the defect = %v, want the item as Reposição", row)
	}
	notYet := call(t, "POST", "/api/v1/admin/orders/"+id+"/advance", map[string]any{"to": "assembling", "version": version(s.order(t, id))}, token)
	if notYet.Status != 409 || notYet.str("guard") != "items_not_ready" {
		t.Errorf("advance with the item back to buy = %d %v, want 409 items_not_ready", notYet.Status, notYet.Body)
	}

	// Send it again and receive it ok.
	second := must(t, callWith(t, "POST", base+"/supplier-orders", pick, token, map[string]string{"Idempotency-Key": unique("send")}), 201, "send again")
	if second.Body["supplierOrder"].(map[string]any)["number"] != "C-0002" {
		t.Errorf("the second send = %v, want C-0002: the first number is never reused", second.Body["supplierOrder"])
	}
	line = toReceiveLine(t, s, 0)
	ok := must(t, callWith(t, "POST", base+"/receipts",
		map[string]any{"entries": []any{map[string]any{"lineId": line, "check": "ok"}}},
		token, map[string]string{"Idempotency-Key": unique("receipt")}), 201, "receive ok")
	if done, _ := ok.Body["completedOrders"].([]any); len(done) != 1 || done[0].(map[string]any)["orderId"] != id {
		t.Fatalf("receipt = %v, want the order completed", ok.Body)
	}

	// The order may now move into assembling.
	o = s.order(t, id)
	if it := o["items"].([]any)[0].(map[string]any); it["arrived"] != true || it["check"] != "ok" {
		t.Errorf("item after receiving = %v, want arrived and checked ok", it)
	}
	must(t, call(t, "POST", "/api/v1/admin/orders/"+id+"/advance", map[string]any{"to": "assembling", "version": version(o)}, token), 200, "advance to assembling")

	// What the order screen and Hoje read.
	lines := must(t, call(t, "GET", base+"/orders/"+id+"/lines", nil, token), 200, "the order's purchases").Body["items"].([]any)
	if len(lines) != 1 || lines[0].(map[string]any)["status"] != "received" || lines[0].(map[string]any)["number"] != "C-0002" || lines[0].(map[string]any)["supplierChatUrl"] == nil {
		t.Errorf("order purchases = %v, want the item received on C-0002 with a chat link", lines)
	}
	today := must(t, call(t, "GET", base+"/today", nil, token), 200, "Hoje's purchasing entries")
	if today.Body["toSend"] != float64(0) {
		t.Errorf("Hoje has %v to send, want 0", today.Body["toSend"])
	}
}

// The undo, and who may do what, on a pedido de compra (ADR 0031).
func TestPurchasing_UndoAndRoles(t *testing.T) {
	t.Parallel()

	s := newOrdersStore(t)
	const base = "/api/v1/admin/purchasing"
	token := s.owner.access
	supplier := firstSupplierID(t, s)
	created := s.create(t, unique("order"), false).Body["order"].(map[string]any)
	id := created["id"].(string)
	itemID := created["items"].([]any)[0].(map[string]any)["id"].(string)
	deposit := map[string]any{"kind": "deposit", "amountCents": cents(created, "depositDue"), "method": "pix", "version": version(created)}
	must(t, callWith(t, "POST", "/api/v1/admin/orders/"+id+"/payments", deposit, token, map[string]string{"Idempotency-Key": unique("deposit")}), 201, "record deposit")

	pick := map[string]any{"supplierId": supplier, "orderItemIds": []any{itemID}}
	sent := must(t, callWith(t, "POST", base+"/supplier-orders", pick, token, map[string]string{"Idempotency-Key": unique("send")}), 201, "send")
	so := sent.Body["supplierOrder"].(map[string]any)

	// A staff member receives but neither buys nor sees a cost.
	email := unique("equipe") + "@teste.encomendo.local"
	must(t, call(t, "POST", "/api/v1/admin/invitations", map[string]any{"email": email, "role": "staff"}, token), 201, "invite")
	staff := &session{email: email, password: "segredo-forte-2"}
	staff.take(must(t, call(t, "POST", "/auth/invitations/accept", map[string]any{
		"token": invitationLink(t, email), "name": "Caio Reis", "password": staff.password, "refreshTransport": "body",
	}, ""), 200, "accept"))
	if a := call(t, "GET", base+"/to-buy", nil, staff.access); a.Status != 403 {
		t.Errorf("staff reading A comprar = %d %v, want 403", a.Status, a.Body)
	}
	toReceive := must(t, call(t, "GET", base+"/to-receive", nil, staff.access), 200, "staff reading Receber itens")
	if raw, err := json.Marshal(toReceive.Body); err != nil || strings.Contains(strings.ToLower(string(raw)), "cost") {
		t.Errorf("Receber itens carries a cost: %s %v", raw, err)
	}

	// Undo: the item is bought again and the number is not reused.
	undone := must(t, call(t, "POST", base+"/supplier-orders/"+so["id"].(string)+"/undo", map[string]any{"version": so["version"]}, token), 200, "undo")
	if undone.str("state") != "undone" {
		t.Errorf("state after the undo = %v, want undone", undone.Body["state"])
	}
	if row := toBuyOf(t, s, supplier)["items"].([]any); len(row) != 1 {
		t.Errorf("A comprar after the undo = %v, want the item back", row)
	}
	resent := must(t, callWith(t, "POST", base+"/supplier-orders", pick, token, map[string]string{"Idempotency-Key": unique("send")}), 201, "send after the undo")
	if resent.Body["supplierOrder"].(map[string]any)["number"] != "C-0002" {
		t.Errorf("the send after an undo = %v, want C-0002", resent.Body["supplierOrder"])
	}
	again := call(t, "POST", base+"/supplier-orders/"+so["id"].(string)+"/undo", map[string]any{"version": so["version"]}, token)
	if again.Status != 409 {
		t.Errorf("undoing twice = %d %v, want 409", again.Status, again.Body)
	}
}

// firstSupplierID returns the store's only supplier.
func firstSupplierID(t *testing.T, s *ordersStore) string {
	t.Helper()
	list := must(t, call(t, "GET", "/api/v1/admin/suppliers?q="+url.QueryEscape("Couros"), nil, s.owner.access), 200, "list suppliers")
	return list.Body["items"].([]any)[0].(map[string]any)["id"].(string)
}

// toBuyOf returns the supplier's group in A comprar.
func toBuyOf(t *testing.T, s *ordersStore, supplier string) map[string]any {
	t.Helper()
	got := must(t, call(t, "GET", "/api/v1/admin/purchasing/to-buy", nil, s.owner.access), 200, "A comprar")
	for _, g := range got.Body["suppliers"].([]any) {
		if g.(map[string]any)["supplierId"] == supplier {
			return g.(map[string]any)
		}
	}
	t.Fatalf("A comprar has no group of supplier %s: %v", supplier, got.Body)
	return nil
}

// toReceiveLine returns the id of the n-th open line of Receber itens.
func toReceiveLine(t *testing.T, s *ordersStore, n int) string {
	t.Helper()
	got := must(t, call(t, "GET", "/api/v1/admin/purchasing/to-receive", nil, s.owner.access), 200, "Receber itens")
	groups := got.Body["groups"].([]any)
	if len(groups) == 0 {
		t.Fatalf("Receber itens has no open pedido de compra: %v", got.Body)
	}
	return groups[0].(map[string]any)["lines"].([]any)[n].(map[string]any)["lineId"].(string)
}
