package handler_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/app/purchasing/fake"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// purchasingOwnerKeys are the keys only the owner sees in a purchasing
// answer, at any depth (ADR 0025, ADR 0031).
var purchasingOwnerKeys = map[string]bool{"cost": true, "paidCost": true}

// ownerKeysIn returns the owner-only keys found in the JSON body, with
// their paths.
func ownerKeysIn(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("not JSON: %s", raw)
	}
	found := map[string]bool{}
	walk(decoded, "$", func(path, key string, _ any) {
		if purchasingOwnerKeys[key] {
			found[path+"."+key] = true
		}
	})
	return found
}

// The owner's cost and price paid are written for the owner only, and the
// key is absent, not null, for admin, staff and viewer. The fake use cases
// here give a cost to every caller, as a buggy use case would, so what is
// tested is the DTO's own gate; TestPurchasingDTOs_RealUseCasesKeepCostsFromTheAdmin
// tests the use cases' side.
func TestPurchasingDTOs_CostOnlyForOwner(t *testing.T) {
	t.Parallel()

	costed := []struct{ method, path, body string }{
		{"GET", "/api/v1/admin/purchasing/to-buy", ""},
		{"POST", "/api/v1/admin/purchasing/supplier-orders", puPick},
		{"GET", puPaths.Replace("/api/v1/admin/purchasing/supplier-orders/{supplierOrderId}"), ""},
		{"PUT", puPaths.Replace("/api/v1/admin/purchasing/supplier-orders/{supplierOrderId}/promised-date"), `{"version":3,"date":"2026-12-18"}`},
		{"POST", puPaths.Replace("/api/v1/admin/purchasing/supplier-orders/{supplierOrderId}/in-transit"), `{"version":3}`},
		{"POST", puPaths.Replace("/api/v1/admin/purchasing/supplier-orders/{supplierOrderId}/undo"), `{"version":3}`},
		{"PUT", puPaths.Replace("/api/v1/admin/purchasing/supplier-orders/{supplierOrderId}/paid-prices"), fmt.Sprintf(`{"version":3,"prices":[{"lineId":%q,"cents":1}]}`, puLine.String())},
	}
	for _, role := range []shared.Role{shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer} {
		for _, c := range costed {
			t.Run(string(role)+" "+c.method+" "+c.path, func(t *testing.T) {
				t.Parallel()

				f := newPuFixture(t)
				f.uc.costs = true
				rec := f.callAs(t, role, c.method, c.path, c.body)
				if rec.Code != 200 && rec.Code != 201 {
					t.Fatalf("answer %d %s", rec.Code, rec.Body.String())
				}
				found := ownerKeysIn(t, rec.Body.Bytes())
				if role != shared.RoleOwner {
					if len(found) > 0 {
						t.Errorf("as %s the answer has %v, which only the owner sees", role, found)
					}
					return
				}
				if len(found) == 0 {
					t.Errorf("as the owner the answer has no cost: %s", rec.Body.String())
				}
			})
		}
	}
}

// A pedido de compra's owner view writes paidCost as null while nothing was
// paid, so the owner can tell "none yet" from "not yours to see".
func TestPurchasingDTOs_OwnerSeesANullPaidCost(t *testing.T) {
	t.Parallel()

	f := newPuFixture(t)
	f.uc.costs = true
	rec := f.call(t, "GET", puPaths.Replace("/api/v1/admin/purchasing/supplier-orders/{supplierOrderId}"), "")
	lines := body(t, rec)["lines"].([]any)
	second := lines[1].(map[string]any)
	if v, has := second["paidCost"]; !has || v != nil {
		t.Errorf("owner's unpaid line: paidCost = %v (present %v), want null", v, has)
	}
	first := lines[0].(map[string]any)
	if first["paidCost"] == nil || first["cost"] == nil {
		t.Errorf("owner's paid line = %v", first)
	}
}

// Receber itens and the order's purchases carry no cost, for any role, even
// when the fake gives one.
func TestPurchasingDTOs_NoCostWhereTheContractHasNone(t *testing.T) {
	t.Parallel()

	for _, role := range []shared.Role{shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff} {
		for _, c := range []struct{ method, path, body string }{
			{"GET", "/api/v1/admin/purchasing/to-receive", ""},
			{"POST", "/api/v1/admin/purchasing/receipts", puReceive},
			{"GET", "/api/v1/admin/purchasing/notices", ""},
			{"GET", puPaths.Replace("/api/v1/admin/purchasing/orders/{orderId}/lines"), ""},
			{"GET", "/api/v1/admin/purchasing/today", ""},
			{"GET", "/api/v1/admin/purchasing/supplier-orders", ""},
		} {
			t.Run(string(role)+" "+c.method+" "+c.path, func(t *testing.T) {
				t.Parallel()

				f := newPuFixture(t)
				f.uc.costs = true
				rec := f.callAs(t, role, c.method, c.path, c.body)
				if found := ownerKeysIn(t, rec.Body.Bytes()); len(found) > 0 {
					t.Errorf("as %s the answer has %v", role, found)
				}
			})
		}
	}
}

// puRealRig serves the purchasing handlers over the real purchasing use
// cases and their in-memory fakes, so who sees a cost is decided by the app
// layer as in production, and the supplier texts are the real ones.
type puRealRig struct {
	t        *testing.T
	p        *handler.Purchasing
	items    *fake.OrderItems
	ids      *sharedfake.IDs
	today    shared.CivilDate
	supplier apppurchasing.SupplierContact
	catalog  map[string]shared.ItemID
	keys     int
}

// clientFields are everything a customer of the fake orders has. None of it
// may reach a supplier text.
type clientFields struct{ Name, Phone, Address, Email string }

var puMaria = clientFields{Name: "Maria Aparecida Souza", Phone: "5592981234567", Address: "Rua das Flores, 123", Email: "maria@example.com"}

func (c clientFields) all() []string {
	return []string{c.Name, "Maria", "Aparecida", "Souza", c.Phone, "92981234567", c.Address, "Rua das Flores", c.Email}
}

func newPuRealRig(t *testing.T) *puRealRig {
	t.Helper()
	clock := sharedfake.NewClock(time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC))
	ids := sharedfake.NewIDs(clock)
	zone := time.FixedZone("BRT", -3*3600)
	today := shared.DateOf(clock.Now(), zone)
	items, repo, suppliers := fake.NewOrderItems(today), fake.NewRepository(), fake.NewSuppliers()
	repo.LinkCosts(items)
	repo.NameUser(userID, "Dona Rita")
	contact := apppurchasing.SupplierContact{ID: shared.SupplierID(ids.NewID()), Name: "Couros Paulista", Phone: "5511987654321"}
	suppliers.Add(contact)
	svc, err := apppurchasing.NewService(apppurchasing.Deps{
		Repo: repo, Orders: items, Suppliers: suppliers, Store: fake.NewStore("Alta RPM", "BRL", zone, clock),
		Links: fake.SupplierLinks{}, Photos: fake.NewReceiptPhotos(clock), Idempotency: sharedfake.NewIdempotency(clock),
		Tx: sharedfake.NewTxManager(), Clock: clock, IDs: ids,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := handler.NewPurchasing(handler.PurchasingDeps{UseCases: svc, Limiter: &limiter{refuse: map[string]bool{}}, Log: discardLog()})
	if err != nil {
		t.Fatal(err)
	}
	return &puRealRig{t: t, p: p, items: items, ids: ids, today: today, supplier: contact, catalog: map[string]shared.ItemID{}}
}

// order adds a released order of the supplier whose customer is puMaria and
// whose one bought item costs costCents. It returns the order and item ids.
func (r *puRealRig) order(number int, name string, costCents int64) (shared.OrderID, shared.OrderItemID) {
	r.t.Helper()
	id, item := shared.OrderID(r.ids.NewID()), shared.OrderItemID(r.ids.NewID())
	if _, ok := r.catalog[name]; !ok {
		r.catalog[name] = shared.ItemID(r.ids.NewID())
	}
	cost := testkit.BRL(r.t, costCents)
	r.items.AddOrder(
		apppurchasing.OrderFacts{Order: id, Number: order.Number(number).String(), ArrivalLimit: r.today.AddDays(17)}, true,
		fake.Item{CatalogItem: r.catalog[name], Cost: &cost, Facts: apppurchasing.OrderItemFacts{
			Item: item, Bought: true, Status: order.ItemToBuy, Name: name, Quantity: 2, Expected: r.today.AddDays(15),
			Choices:  []purchasing.Choice{{Option: shared.OptionID(r.ids.NewID()), ChoiceID: shared.ChoiceID(r.ids.NewID()), Name: "Cor", Label: "Preto"}},
			Supplier: r.supplier.ID, SupplierName: r.supplier.Name,
		}},
	)
	return id, item
}

// do sends the request as role, with a fresh key for a creating call, and
// checks the answer against the contract. It returns the status and the
// raw body.
func (r *puRealRig) do(role shared.Role, method, path string, body any) (int, []byte) {
	r.t.Helper()
	payload := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			r.t.Fatal(err)
		}
		payload = string(b)
	}
	r.keys++
	var headers map[string]string
	if method == "POST" && (strings.HasSuffix(path, "/supplier-orders") || strings.HasSuffix(path, "/receipts")) {
		headers = map[string]string{"Idempotency-Key": fmt.Sprintf("real-%d", r.keys)}
	}
	rec := serveWith(r.t, puRoutes(r.p), puRequest{caller: staffIn(r.t, role), method: method, path: path, body: payload, headers: headers})
	return rec.Code, rec.Body.Bytes()
}

const puBase = "/api/v1/admin/purchasing"

func (r *puRealRig) pick(items ...shared.OrderItemID) map[string]any {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.String()
	}
	return map[string]any{"supplierId": r.supplier.ID.String(), "orderItemIds": ids}
}

// A line sent by an admin, who never sees a cost, still has the owner's
// cost, and the owner alone reads it back; a price paid is the owner's to
// record and to read.
func TestPurchasingDTOs_RealUseCasesKeepCostsFromTheAdmin(t *testing.T) {
	t.Parallel()

	r := newPuRealRig(t)
	_, item := r.order(231, "Banco de couro", 4000)

	code, raw := r.do(shared.RoleAdmin, "GET", puBase+"/to-buy", nil)
	if code != 200 || len(ownerKeysIn(t, raw)) > 0 {
		t.Fatalf("admin's to-buy: %d %s", code, raw)
	}
	code, raw = r.do(shared.RoleOwner, "GET", puBase+"/to-buy", nil)
	if code != 200 || len(ownerKeysIn(t, raw)) == 0 {
		t.Fatalf("owner's to-buy has no cost: %d %s", code, raw)
	}
	code, raw = r.do(shared.RoleAdmin, "POST", puBase+"/supplier-orders", r.pick(item))
	if code != 201 || len(ownerKeysIn(t, raw)) > 0 {
		t.Fatalf("admin's send: %d %s", code, raw)
	}
	var sent struct {
		SupplierOrder struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
			Lines   []struct {
				ID string `json:"id"`
			} `json:"lines"`
		} `json:"supplierOrder"`
	}
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatal(err)
	}
	path := puBase + "/supplier-orders/" + sent.SupplierOrder.ID

	code, raw = r.do(shared.RoleAdmin, "GET", path, nil)
	if code != 200 || len(ownerKeysIn(t, raw)) > 0 {
		t.Errorf("admin's read of the supplier order: %d %s", code, raw)
	}
	code, raw = r.do(shared.RoleOwner, "GET", path, nil)
	if code != 200 || !strings.Contains(string(raw), `"cost":{"cents":4000,"currency":"BRL"}`) || !strings.Contains(string(raw), `"paidCost":null`) {
		t.Errorf("owner's read keeps the cost of 40,00 and an empty paid price: %d %s", code, raw)
	}

	prices := map[string]any{"version": sent.SupplierOrder.Version, "prices": []any{map[string]any{"lineId": sent.SupplierOrder.Lines[0].ID, "cents": 3800}}}
	if code, raw = r.do(shared.RoleAdmin, "PUT", path+"/paid-prices", prices); code != 403 {
		t.Errorf("admin recording a price paid: %d %s, want 403", code, raw)
	}
	if code, raw = r.do(shared.RoleOwner, "PUT", path+"/paid-prices", prices); code != 200 || !strings.Contains(string(raw), `"paidCost":{"cents":3800`) {
		t.Errorf("owner recording a price paid: %d %s", code, raw)
	}
	code, raw = r.do(shared.RoleAdmin, "GET", path, nil)
	if code != 200 || len(ownerKeysIn(t, raw)) > 0 {
		t.Errorf("admin reads the price paid: %d %s", code, raw)
	}
	for _, role := range []shared.Role{shared.RoleStaff, shared.RoleViewer} {
		if code, _ := r.do(role, "GET", path, nil); code != 403 {
			t.Errorf("%s reading a supplier order: %d, want 403", role, code)
		}
		if code, _ := r.do(role, "GET", puBase+"/to-buy", nil); code != 403 {
			t.Errorf("%s reading A comprar: %d, want 403", role, code)
		}
	}
}

// linkText returns the text a wa.me link carries, decoded.
func linkText(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil || u.Host != "wa.me" {
		t.Fatalf("%q is not a wa.me link: %v", raw, err)
	}
	return u.Query().Get("text")
}

// No text the supplier gets, as the API answers it, holds a field of the
// customer behind the order, or an order number of the store: the preview,
// the send, the stored link, a notice and "Falar com o fornecedor"
// (docs/product/design-document.md, System rules).
func TestSupplierLinks_HaveNoClientData(t *testing.T) {
	t.Parallel()

	r := newPuRealRig(t)
	orderID, item := r.order(231, "Banco de couro", 4000)
	_, item2 := r.order(232, "Banco de couro", 4000)
	_, item3 := r.order(233, "Capa de banco", 1500)

	var texts []string
	add := func(what, text string) {
		t.Helper()
		if text == "" {
			t.Errorf("%s has no text", what)
		}
		texts = append(texts, text)
	}

	code, raw := r.do(shared.RoleAdmin, "POST", puBase+"/supplier-orders/preview", r.pick(item, item2, item3))
	if code != 200 {
		t.Fatalf("preview: %d %s", code, raw)
	}
	var preview struct{ Text string }
	_ = json.Unmarshal(raw, &preview)
	add("the preview", preview.Text)

	code, raw = r.do(shared.RoleAdmin, "POST", puBase+"/supplier-orders", r.pick(item, item2))
	if code != 201 {
		t.Fatalf("send: %d %s", code, raw)
	}
	var sent struct {
		SupplierOrder struct {
			WhatsAppURL string `json:"whatsappUrl"`
		} `json:"supplierOrder"`
		WhatsAppURL string `json:"whatsappUrl"`
		Text        string `json:"text"`
	}
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatal(err)
	}
	add("the sent text", sent.Text)
	add("the sent link", linkText(t, sent.WhatsAppURL))
	if sent.Text != linkText(t, sent.WhatsAppURL) {
		t.Errorf("the link carries %q, not the message %q", linkText(t, sent.WhatsAppURL), sent.Text)
	}
	if got := sent.SupplierOrder.WhatsAppURL; got != "https://wa.me/"+r.supplier.Phone {
		t.Errorf("the supplier order's own link = %q, want the chat with no text", got)
	}

	r.items.Cancel(orderID)
	code, raw = r.do(shared.RoleAdmin, "GET", puBase+"/notices", nil)
	var notices struct {
		Items []struct {
			WhatsAppURL string `json:"whatsappUrl"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &notices); err != nil || code != 200 || len(notices.Items) != 1 {
		t.Fatalf("notices: %d %s: %v", code, raw, err)
	}
	add("the notice", linkText(t, notices.Items[0].WhatsAppURL))

	code, raw = r.do(shared.RoleAdmin, "GET", puBase+"/orders/"+orderID.String()+"/lines", nil)
	var lines struct {
		Items []struct {
			SupplierChatURL string `json:"supplierChatUrl"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &lines); err != nil || code != 200 || len(lines.Items) == 0 || lines.Items[0].SupplierChatURL == "" {
		t.Fatalf("order lines: %d %s: %v", code, raw, err)
	}
	add("Falar com o fornecedor", linkText(t, lines.Items[0].SupplierChatURL))

	// Staff and viewers read the order's purchases but get no chat link.
	code, raw = r.do(shared.RoleStaff, "GET", puBase+"/orders/"+orderID.String()+"/lines", nil)
	if code != 200 || strings.Contains(string(raw), "wa.me") {
		t.Errorf("staff's order purchases: %d %s, want no link", code, raw)
	}

	for _, text := range texts {
		for _, field := range puMaria.all() {
			if strings.Contains(strings.ToLower(text), strings.ToLower(field)) {
				t.Errorf("a supplier text holds the client's %q: %q", field, text)
			}
		}
		for _, number := range []string{"P-0231", "P-0232", "P-0233", "P-"} {
			if strings.Contains(text, number) {
				t.Errorf("a supplier text holds the order number %q: %q", number, text)
			}
		}
	}
	if len(texts) != 5 {
		t.Errorf("checked %d texts, want 5", len(texts))
	}
}

// reachable collects the named struct types of the dto package that t can
// reach through its fields, slices, maps and pointers.
func reachable(t reflect.Type, seen map[reflect.Type]bool) {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		reachable(t.Elem(), seen)
	case reflect.Map:
		reachable(t.Elem(), seen)
	case reflect.Struct:
		if seen[t] {
			return
		}
		seen[t] = true
		for i := range t.NumField() {
			reachable(t.Field(i).Type, seen)
		}
	default:
	}
}

// No purchasing DTO is reachable from a type a client gets: the tracking
// page's view holds no supplier order, purchase line, notice, receipt or
// cost (docs/product/design-document.md, System rules).
func TestPurchasingDTOs_NotInClientTypes(t *testing.T) {
	t.Parallel()

	purchasingTypes := []any{
		dto.ToBuy{}, dto.ToBuySupplier{}, dto.ToBuyItem{}, dto.SupplierMessagePreview{}, dto.PurchaseLine{}, dto.SupplierOrderSummary{},
		dto.SupplierOrder{}, dto.SentSupplierOrder{}, dto.ToReceiveLine{}, dto.ToReceiveGroup{}, dto.PurchaseToReceive{}, dto.OrderRef{},
		dto.ReceiptEntry{}, dto.BackToBuy{}, dto.Receipt{}, dto.Notice{}, dto.OrderPurchaseToBuy{}, dto.OrderPurchaseLine{},
		dto.ToSendNeed{}, dto.NoPromisedDateNeed{}, dto.NotifySupplierNeed{}, dto.PurchasingToday{}, dto.SupplierItemUse{}, dto.PurchaseChoice{},
	}
	clientTypes := []any{dto.TrackingOrder{}}
	for _, c := range clientTypes {
		seen := map[reflect.Type]bool{}
		reachable(reflect.TypeOf(c), seen)
		if !seen[reflect.TypeOf(dto.TrackingItem{})] {
			t.Fatalf("the walk from %T did not reach its items, so it proves nothing: %v", c, seen)
		}
		for _, p := range purchasingTypes {
			if seen[reflect.TypeOf(p)] {
				t.Errorf("%T reaches the purchasing type %T", c, p)
			}
		}
		for typ := range seen {
			if typ.PkgPath() == "" || !strings.HasSuffix(typ.PkgPath(), "/handler/dto") {
				continue
			}
			for i := range typ.NumField() {
				if purchasingOwnerKeys[strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]] {
					t.Errorf("%s has the owner-only key %q", typ.Name(), typ.Field(i).Tag.Get("json"))
				}
			}
		}
	}
}
