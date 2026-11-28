package handler_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	puPick    = fmt.Sprintf(`{"supplierId":%q,"orderItemIds":[%q]}`, puSupplier.String(), orItem.String())
	puReceive = fmt.Sprintf(`{"entries":[{"lineId":%q,"check":"ok"},{"orderItemId":%q,"check":"defective","note":"Rasgado","photoKeys":["t/x/receipts/y"]}],`+
		`"invoicePhotoKeys":["t/x/receipts/z"]}`, puLine.String(), orKitItem.String())
)

// puOp is one purchasing operation of the contract with a body that passes,
// and the status it answers.
type puOp struct {
	pattern string
	body    string
	status  int
}

func (o puOp) method() string { m, _, _ := strings.Cut(o.pattern, " "); return m }

func (o puOp) path() string { _, p, _ := strings.Cut(o.pattern, " "); return puPaths.Replace(p) }

func puOps() []puOp {
	const base = "/api/v1/admin/purchasing"
	return []puOp{
		{"GET " + base + "/to-buy", "", 200},
		{"POST " + base + "/supplier-orders/preview", puPick, 200},
		{"GET " + base + "/supplier-orders?state=all&limit=10&cursor=abc", "", 200},
		{"POST " + base + "/supplier-orders", puPick, 201},
		{"GET " + base + "/supplier-orders/{supplierOrderId}", "", 200},
		{"PUT " + base + "/supplier-orders/{supplierOrderId}/promised-date",
			fmt.Sprintf(`{"version":3,"date":"2026-12-18","lines":[{"lineId":%q,"date":"2026-12-22"}]}`, puLine.String()), 200},
		{"POST " + base + "/supplier-orders/{supplierOrderId}/in-transit", `{"version":3}`, 200},
		{"POST " + base + "/supplier-orders/{supplierOrderId}/undo", `{"version":3}`, 200},
		{"PUT " + base + "/supplier-orders/{supplierOrderId}/paid-prices", fmt.Sprintf(`{"version":3,"prices":[{"lineId":%q,"cents":3800}]}`, puLine.String()), 200},
		{"GET " + base + "/to-receive", "", 200},
		{"POST " + base + "/receipt-photo-uploads", `{"contentType":"image/jpeg"}`, 201},
		{"POST " + base + "/receipts", puReceive, 201},
		{"GET " + base + "/notices", "", 200},
		{"POST " + base + "/notices/{lineId}/resolve", "", 204},
		{"GET " + base + "/orders/{orderId}/lines", "", 200},
		{"GET " + base + "/today", "", 200},
	}
}

func TestPurchasing_ServesEveryOperation(t *testing.T) {
	t.Parallel()

	ops := puOps()
	if len(ops) != 16 {
		t.Fatalf("%d operations listed, the tag purchasing has 16", len(ops))
	}
	for _, op := range ops {
		t.Run(op.pattern, func(t *testing.T) {
			t.Parallel()

			f := newPuFixture(t)
			rec := f.call(t, op.method(), op.path(), op.body)
			if rec.Code != op.status {
				t.Fatalf("answer %d %s, want %d", rec.Code, rec.Body.String(), op.status)
			}
			if f.uc.calls != 1 {
				t.Errorf("%d use-case calls, want 1", f.uc.calls)
			}
		})
	}
}

// A path id that is not an id names nothing: 404, and no use case runs.
func TestPurchasing_MalformedPathIDIsNotFound(t *testing.T) {
	t.Parallel()

	for _, op := range puOps() {
		if !strings.Contains(op.pattern, "{") {
			continue
		}
		t.Run(op.pattern, func(t *testing.T) {
			t.Parallel()

			f := newPuFixture(t)
			path := strings.NewReplacer("{supplierOrderId}", "nope", "{lineId}", "nope", "{orderId}", "nope").Replace(strings.SplitN(op.pattern, " ", 2)[1])
			rec := f.call(t, op.method(), path, op.body)
			if rec.Code != 404 || code(t, rec) != respond.CodeNotFound || f.uc.calls != 0 {
				t.Errorf("answer %d %s after %d use-case calls, want 404 not_found and none", rec.Code, rec.Body.String(), f.uc.calls)
			}
		})
	}
}

// An unknown field in any body is 400 invalid, and no use case runs.
func TestPurchasing_RefusesUnknownFields(t *testing.T) {
	t.Parallel()

	for _, op := range puOps() {
		if op.body == "" {
			continue
		}
		t.Run(op.pattern, func(t *testing.T) {
			t.Parallel()

			f := newPuFixture(t)
			body := strings.TrimSuffix(op.body, "}") + `,"surprise":1}`
			rec := f.call(t, op.method(), op.path(), body)
			if rec.Code != 400 || code(t, rec) != respond.CodeInvalid || f.uc.calls != 0 {
				t.Errorf("answer %d %s after %d use-case calls, want 400 invalid and none", rec.Code, rec.Body.String(), f.uc.calls)
			}
		})
	}
}

// Each use case's refusals reach the client with the contract's code.
func TestPurchasing_ProblemCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		op         string // the operation, by its pattern's suffix
		err        error
		wantStatus int
		wantCode   string
	}{
		{"send: an item sent by someone else", "POST supplier-orders", apppurchasing.ErrItemNotToBuy, 409, respond.CodeItemNotToBuy},
		{"preview: an item no longer to buy", "POST supplier-orders/preview", apppurchasing.ErrItemNotToBuy, 409, respond.CodeItemNotToBuy},
		{"send: the key was used for another request", "POST supplier-orders", appshared.ErrIdempotencyKeyReused, 422, respond.CodeIdempotencyKeyReused},
		{"send: the key is in flight", "POST supplier-orders", appshared.ErrIdempotencyKeyInFlight, 409, respond.CodeConflict},
		{"undo: something was received", "POST supplier-orders/{supplierOrderId}/undo", purchasing.ErrSupplierOrderReceived, 409, respond.CodeSupplierOrderReceived},
		{"undo: already undone", "POST supplier-orders/{supplierOrderId}/undo", purchasing.ErrUndone, 409, respond.CodeLineClosed},
		{"in transit: nothing to move", "POST supplier-orders/{supplierOrderId}/in-transit", purchasing.ErrNothingOpen, 409, respond.CodeLineClosed},
		{"in transit: someone saved first", "POST supplier-orders/{supplierOrderId}/in-transit", shared.ErrVersionConflict, 409, respond.CodeVersionConflict},
		{"paid prices: a cancelled line", "PUT supplier-orders/{supplierOrderId}/paid-prices", purchasing.ErrLineClosed, 409, respond.CodeLineClosed},
		{"paid prices: not the owner", "PUT supplier-orders/{supplierOrderId}/paid-prices", shared.ErrForbidden, 403, respond.CodeForbidden},
		{"promised date: a day in the past", "PUT supplier-orders/{supplierOrderId}/promised-date", shared.NewValidationError(shared.FieldError{Field: "date", Message: "Escolha uma data a partir de hoje."}), 400, respond.CodeInvalid},
		{"promised date: an order closed", "PUT supplier-orders/{supplierOrderId}/promised-date", order.ErrOrderClosed, 409, respond.CodeOrderClosed},
		{"supplier order: not found", "GET supplier-orders/{supplierOrderId}", purchasing.ErrSupplierOrderNotFound, 404, respond.CodeNotFound},
		{"receipt: a line already received", "POST receipts", purchasing.ErrLineClosed, 409, respond.CodeLineClosed},
		{"receipt: a photo that was never uploaded", "POST receipts", fmt.Errorf("photo: %w", purchasing.ErrPhotoKey), 400, respond.CodePhotoNotUploaded},
		{"resolve: no longer a notice", "POST notices/{lineId}/resolve", purchasing.ErrLineClosed, 409, respond.CodeLineClosed},
		{"to buy: forbidden", "GET to-buy", shared.ErrForbidden, 403, respond.CodeForbidden},
		{"to buy: the store is down", "GET to-buy", shared.ErrUnavailable, 503, respond.CodeNotReady},
	}
	// An operation is named by its method and its path after the base, with
	// the path parameters as written in the contract and no query.
	ops := map[string]puOp{}
	for _, op := range puOps() {
		_, p, _ := strings.Cut(op.pattern, " ")
		p, _, _ = strings.Cut(strings.TrimPrefix(p, "/api/v1/admin/purchasing/"), "?")
		ops[op.method()+" "+p] = op
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			op, ok := ops[tt.op]
			if !ok {
				t.Fatalf("no operation %q", tt.op)
			}
			f := newPuFixture(t)
			f.uc.err = tt.err
			rec := f.call(t, op.method(), op.path(), op.body)
			if rec.Code != tt.wantStatus || code(t, rec) != tt.wantCode {
				t.Errorf("answer %d %s, want %d %s", rec.Code, rec.Body.String(), tt.wantStatus, tt.wantCode)
			}
		})
	}
}

// A send and a receipt need an Idempotency-Key: without one the use case
// answers 400 idempotency_key_required, and a key that is not 1 to 128
// printable characters is 400 invalid before any use case runs.
func TestPurchasing_IdempotencyKey(t *testing.T) {
	t.Parallel()

	for _, op := range []puOp{puOps()[3], puOps()[11]} {
		t.Run(op.pattern, func(t *testing.T) {
			t.Parallel()

			f := newPuFixture(t)
			rec := serveWith(t, puRoutes(f.p), puRequest{caller: staffIn(t, shared.RoleOwner), method: op.method(), path: op.path(), body: op.body})
			if rec.Code != 400 || code(t, rec) != respond.CodeIdempotencyKeyMissing || f.uc.calls != 1 {
				t.Errorf("without a key: answer %d %s after %d calls, want 400 idempotency_key_required from the use case", rec.Code, rec.Body.String(), f.uc.calls)
			}

			f = newPuFixture(t)
			rec = serveWith(t, puRoutes(f.p), puRequest{
				caller: staffIn(t, shared.RoleOwner), method: op.method(), path: op.path(), body: op.body, headers: map[string]string{"Idempotency-Key": "has space"},
			})
			if rec.Code != 400 || code(t, rec) != respond.CodeInvalid || f.uc.calls != 0 {
				t.Errorf("with a bad key: answer %d %s after %d calls, want 400 invalid and none", rec.Code, rec.Body.String(), f.uc.calls)
			}

			f = newPuFixture(t)
			if rec := f.call(t, op.method(), op.path(), op.body); rec.Code != 201 {
				t.Fatalf("with a key: answer %d %s", rec.Code, rec.Body.String())
			}
			var key string
			switch in := f.uc.got.(type) {
			case apppurchasing.SendInput:
				key = in.Idempotency.Key
			case apppurchasing.ReceiptInput:
				key = in.Idempotency.Key
			}
			if key != "key-1" {
				t.Errorf("the use case got key %q, want key-1", key)
			}
		})
	}
}

// The two keyed calls hash who, the path and the body, so the same key with
// another body is a different request.
func TestPurchasing_KeyHashCoversTheBody(t *testing.T) {
	t.Parallel()

	hash := func(body string) [32]byte {
		f := newPuFixture(t)
		f.call(t, "POST", "/api/v1/admin/purchasing/supplier-orders", body)
		return f.uc.got.(apppurchasing.SendInput).Idempotency.RequestHash
	}
	other := fmt.Sprintf(`{"supplierId":%q,"orderItemIds":[%q]}`, puSupplier.String(), orKitItem.String())
	first, same := hash(puPick), hash(puPick)
	if first == hash(other) || first != same {
		t.Error("the request hash does not follow the body")
	}
}

func TestPurchasing_RefusesBadBodies(t *testing.T) {
	t.Parallel()

	const base = "/api/v1/admin/purchasing"
	line, item := puLine.String(), orItem.String()
	tests := []struct {
		name, method, path, body string
		wantField                string
	}{
		{"send without a supplier", "POST", base + "/supplier-orders", fmt.Sprintf(`{"orderItemIds":[%q]}`, item), "supplierId"},
		{"send without items", "POST", base + "/supplier-orders", fmt.Sprintf(`{"supplierId":%q}`, puSupplier.String()), "orderItemIds"},
		{"send with a bad item id", "POST", base + "/supplier-orders", fmt.Sprintf(`{"supplierId":%q,"orderItemIds":["x"]}`, puSupplier.String()), "orderItemIds[0]"},
		{"preview with a bad supplier id", "POST", base + "/supplier-orders/preview", fmt.Sprintf(`{"supplierId":"x","orderItemIds":[%q]}`, item), "supplierId"},
		{"promised date without a date", "PUT", puPaths.Replace(base + "/supplier-orders/{supplierOrderId}/promised-date"), `{"version":1}`, "date"},
		{"promised date with a bad date", "PUT", puPaths.Replace(base + "/supplier-orders/{supplierOrderId}/promised-date"), `{"version":1,"date":"18/12"}`, "date"},
		{"promised date without a version", "PUT", puPaths.Replace(base + "/supplier-orders/{supplierOrderId}/promised-date"), `{"date":"2026-12-18"}`, "version"},
		{"promised line with no line id", "PUT", puPaths.Replace(base + "/supplier-orders/{supplierOrderId}/promised-date"), `{"version":1,"date":"2026-12-18","lines":[{"date":"2026-12-19"}]}`, "lines[0].lineId"},
		{"undo without a version", "POST", puPaths.Replace(base + "/supplier-orders/{supplierOrderId}/undo"), `{}`, "version"},
		{"in transit with version zero", "POST", puPaths.Replace(base + "/supplier-orders/{supplierOrderId}/in-transit"), `{"version":0}`, "version"},
		{"paid prices with no prices", "PUT", puPaths.Replace(base + "/supplier-orders/{supplierOrderId}/paid-prices"), `{"version":1,"prices":[]}`, "prices"},
		{"paid prices without cents", "PUT", puPaths.Replace(base + "/supplier-orders/{supplierOrderId}/paid-prices"), fmt.Sprintf(`{"version":1,"prices":[{"lineId":%q}]}`, line), "prices[0].cents"},
		{"photo upload without a content type", "POST", base + "/receipt-photo-uploads", `{}`, "contentType"},
		{"receipt without entries", "POST", base + "/receipts", `{}`, "entries"},
		{"receipt entry with both targets", "POST", base + "/receipts", fmt.Sprintf(`{"entries":[{"lineId":%q,"orderItemId":%q,"check":"ok"}]}`, line, item), "entries[0]"},
		{"receipt entry with no target", "POST", base + "/receipts", `{"entries":[{"check":"ok"}]}`, "entries[0]"},
		{"receipt entry without a check", "POST", base + "/receipts", fmt.Sprintf(`{"entries":[{"lineId":%q}]}`, line), "entries[0].check"},
		{"receipt entry with a bad line id", "POST", base + "/receipts", `{"entries":[{"lineId":"x","check":"ok"}]}`, "entries[0].lineId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newPuFixture(t)
			rec := f.call(t, tt.method, tt.path, tt.body)
			if rec.Code != 400 || code(t, rec) != respond.CodeInvalid || f.uc.calls != 0 {
				t.Fatalf("answer %d %s after %d use-case calls, want 400 invalid and none", rec.Code, rec.Body.String(), f.uc.calls)
			}
			var p respond.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if len(p.Errors) == 0 || p.Errors[0].Field != tt.wantField {
				t.Errorf("errors = %+v, want the field %q first", p.Errors, tt.wantField)
			}
		})
	}
}

func TestPurchasing_PassesTheInputsOn(t *testing.T) {
	t.Parallel()

	t.Run("send", func(t *testing.T) {
		t.Parallel()
		f := newPuFixture(t)
		f.call(t, "POST", "/api/v1/admin/purchasing/supplier-orders", puPick)
		in := f.uc.got.(apppurchasing.SendInput)
		if in.Supplier != puSupplier || len(in.OrderItems) != 1 || in.OrderItems[0] != orItem {
			t.Errorf("input = %+v", in)
		}
	})
	t.Run("promised date", func(t *testing.T) {
		t.Parallel()
		f := newPuFixture(t)
		op := puOps()[5]
		f.call(t, op.method(), op.path(), op.body)
		in := f.uc.got.(apppurchasing.PromisedDateInput)
		if in.ID != puSupplierOrder || in.Version != 3 || in.Date != orDate(18) || len(in.Lines) != 1 || in.Lines[0].Line != puLine || in.Lines[0].Date != orDate(22) {
			t.Errorf("input = %+v", in)
		}
	})
	t.Run("paid prices", func(t *testing.T) {
		t.Parallel()
		f := newPuFixture(t)
		op := puOps()[8]
		f.call(t, op.method(), op.path(), op.body)
		in := f.uc.got.(apppurchasing.PaidPricesInput)
		if in.ID != puSupplierOrder || in.Version != 3 || len(in.Prices) != 1 || in.Prices[0].Line != puLine || in.Prices[0].Cents != 3800 {
			t.Errorf("input = %+v", in)
		}
	})
	t.Run("receipt", func(t *testing.T) {
		t.Parallel()
		f := newPuFixture(t)
		f.call(t, "POST", "/api/v1/admin/purchasing/receipts", puReceive)
		in := f.uc.got.(apppurchasing.ReceiptInput)
		if len(in.Entries) != 2 || in.Entries[0].Line != puLine || in.Entries[0].Check != purchasing.CheckOK ||
			in.Entries[1].OrderItem != orKitItem || in.Entries[1].Check != purchasing.CheckDefective || in.Entries[1].Note != "Rasgado" ||
			len(in.Entries[1].PhotoKeys) != 1 || len(in.InvoicePhotoKeys) != 1 {
			t.Errorf("input = %+v", in)
		}
	})
	t.Run("the state filter", func(t *testing.T) {
		t.Parallel()
		for query, wantOpen := range map[string]bool{"": true, "?state=open": true, "?state=all": false} {
			f := newPuFixture(t)
			f.call(t, "GET", "/api/v1/admin/purchasing/supplier-orders"+query, "")
			if got := f.uc.got.([]any)[0].(bool); got != wantOpen {
				t.Errorf("state %q: open = %v, want %v", query, got, wantOpen)
			}
		}
	})
}

func TestPurchasing_RefusesABadQuery(t *testing.T) {
	t.Parallel()

	for _, query := range []string{"?state=maybe", "?limit=abc", "?limit=1000"} {
		t.Run(query, func(t *testing.T) {
			t.Parallel()

			f := newPuFixture(t)
			rec := f.call(t, "GET", "/api/v1/admin/purchasing/supplier-orders"+query, "")
			if rec.Code != 400 || f.uc.calls != 0 {
				t.Errorf("answer %d %s after %d calls, want 400 and none", rec.Code, rec.Body.String(), f.uc.calls)
			}
		})
	}
}

// A line received at the counter has no pedido de compra: its entry in the
// order's purchases has no supplierOrderId and a null number.
func TestPurchasing_OrderPurchasesFollowTheirStatus(t *testing.T) {
	t.Parallel()

	f := newPuFixture(t)
	rec := f.call(t, "GET", puPaths.Replace("/api/v1/admin/purchasing/orders/{orderId}/lines"), "")
	items := body(t, rec)["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items = %v", items)
	}
	toBuy, withOrder, counter := items[0].(map[string]any), items[1].(map[string]any), items[2].(map[string]any)
	if toBuy["status"] != "to_buy" || toBuy["lineId"] != nil || toBuy["supplierChatUrl"] == nil {
		t.Errorf("to buy = %v", toBuy)
	}
	if withOrder["number"] != "C-0012" || withOrder["supplierOrderId"] != puSupplierOrder.String() || withOrder["promisedDate"] != "2026-12-18" {
		t.Errorf("with a pedido de compra = %v", withOrder)
	}
	if _, has := counter["supplierOrderId"]; has || counter["number"] != nil || counter["promisedDate"] != nil {
		t.Errorf("at the counter = %v", counter)
	}
	if _, has := counter["supplierChatUrl"]; has {
		t.Errorf("a caller with no chat link got one: %v", counter)
	}
}

func TestPurchasing_TodayKeepsEachNeedsFields(t *testing.T) {
	t.Parallel()

	f := newPuFixture(t)
	needs := body(t, f.call(t, "GET", "/api/v1/admin/purchasing/today", ""))["needs"].([]any)
	if len(needs) != 3 {
		t.Fatalf("needs = %v", needs)
	}
	want := []string{"notify_supplier", "no_promised_date", "to_send"}
	for i, n := range needs {
		if n.(map[string]any)["kind"] != want[i] {
			t.Errorf("need %d = %v, want kind %s", i, n, want[i])
		}
	}
}

// Receiving photo uploads take the catalog upload limit, per user then per
// store (ADR 0021).
func TestPurchasing_RateLimitsReceiptPhotoUploads(t *testing.T) {
	t.Parallel()

	userKey, storeKey := ratelimit.PhotoUploadByUser.Key(userID.String()), ratelimit.PhotoUploadByStore.Key(tenantID.String())
	tests := []struct {
		name     string
		refuse   ratelimit.Rule
		wantKeys []string
	}{
		{"user budget used up", ratelimit.PhotoUploadByUser, []string{userKey}},
		{"store budget used up", ratelimit.PhotoUploadByStore, []string{userKey, storeKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newPuFixture(t)
			f.limiter.refuse[tt.refuse.Name+":"+tt.refuse.Kind] = true
			rec := f.call(t, "POST", "/api/v1/admin/purchasing/receipt-photo-uploads", `{"contentType":"image/png"}`)
			if rec.Code != 429 || code(t, rec) != respond.CodeRateLimited || rec.Header().Get("Retry-After") != "90" || f.uc.calls != 0 {
				t.Errorf("answer %d %s (Retry-After %q) after %d calls, want 429 and none", rec.Code, rec.Body.String(), rec.Header().Get("Retry-After"), f.uc.calls)
			}
			if fmt.Sprint(f.limiter.keys) != fmt.Sprint(tt.wantKeys) {
				t.Errorf("counted %v, want %v", f.limiter.keys, tt.wantKeys)
			}
		})
	}
}

// A role that may not receive is refused by the use case and never counted
// against the store's budget; no other purchasing call is counted.
func TestPurchasing_OnlyUploadsOfTheAllowedAreCounted(t *testing.T) {
	t.Parallel()

	f := newPuFixture(t)
	f.callAs(t, shared.RoleViewer, "POST", "/api/v1/admin/purchasing/receipt-photo-uploads", `{"contentType":"image/png"}`)
	if len(f.limiter.keys) != 0 {
		t.Errorf("a viewer's request was counted: %v", f.limiter.keys)
	}
	for _, op := range puOps() {
		if !strings.HasSuffix(op.pattern, "/receipt-photo-uploads") {
			f.call(t, op.method(), op.path(), op.body)
		}
	}
	if len(f.limiter.keys) != 0 {
		t.Errorf("counted %v", f.limiter.keys)
	}
	f.call(t, "POST", "/api/v1/admin/purchasing/receipt-photo-uploads", `{"contentType":"image/png"}`)
	if len(f.limiter.keys) != 2 {
		t.Errorf("an allowed upload counted %v, want the user and the store", f.limiter.keys)
	}
}

func TestNewPurchasing_NeedsItsDependencies(t *testing.T) {
	t.Parallel()

	log := discardLog()
	if _, err := handler.NewPurchasing(handler.PurchasingDeps{Limiter: &limiter{}, Log: log}); err == nil {
		t.Error("accepted no use cases")
	}
	if _, err := handler.NewPurchasing(handler.PurchasingDeps{UseCases: &purchasingCases{}, Log: log}); err == nil {
		t.Error("accepted no limiter")
	}
	if _, err := handler.NewPurchasing(handler.PurchasingDeps{UseCases: &purchasingCases{}, Limiter: &limiter{}}); err == nil {
		t.Error("accepted no logger")
	}
}
