package handler_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// refuseList is every word a key of the client's view may never contain,
// matched as a case-insensitive substring so costCents, supplierId and
// safetyMarginDays fail too (docs/product/design-document.md, System rules;
// backend/CLAUDE.md). The legitimate keys (store, number, steps, label,
// current, cancelled, deliveryDate, items, name, quantity, options, arrived,
// total, paid, balanceDue, handover, cents, currency) contain none of them;
// if a new one does, rename it or this list is the place to argue.
var refuseList = []string{
	"cost", "margin", "supplier", "lead", "atrisk", "alert", "recordedby", "history", "expected", "token", "phone", "reason",
	"by", "actor", "user", "staff", "sourcing", "check", "defective", "safety", "assembly",
}

// trackingOrder is an order with a supplier, a cost, an alert-worthy date
// and a chosen option: everything a client must not be shown.
func trackingOrder(t *testing.T) *order.Order {
	t.Helper()
	cost := int64(40000)
	return testkit.NewOrder().WithItem(testkit.ItemOpts{
		Name: "Banco de couro", Mode: catalog.ModeToOrder, LeadDays: 15, Cost: &cost,
		Options: []order.ChosenOption{{
			Option: shared.OptionID(testkit.NextID()), Choice: shared.ChoiceID(testkit.NextID()),
			Name: "Cor da costura", Label: "Vermelha", ExtraPrice: testkit.BRL(t, 5000), ExtraDays: 2,
		}},
	}).WithItems(1).Build(t)
}

// The client's view of an order has no key that names a supplier, a cost, a
// margin, an alert, a staff name, the history or a forecast date, at any
// depth, and it fits the contract's closed TrackingOrder schema.
func TestTrackingOrder_HasNoInternalField(t *testing.T) {
	t.Parallel()

	view := apporder.NewTrackingView(trackingOrder(t), "Alta RPM")
	raw, err := json.Marshal(dto.NewTrackingOrder(view))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded["items"].([]any)) == 0 || len(decoded["steps"].([]any)) == 0 {
		t.Fatalf("the view is empty, so the walk proves nothing: %s", raw)
	}
	keys := 0
	walk(decoded, "tracking", func(path, key string, _ any) {
		keys++
		for _, refused := range refuseList {
			if strings.Contains(strings.ToLower(key), refused) {
				t.Errorf("TrackingOrder has %q at %s", key, path)
			}
		}
	})
	if keys < 10 {
		t.Errorf("walked %d keys, want the whole view", keys)
	}
	doc, err := openapi3.NewLoader().LoadFromFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	schema := doc.Components.Schemas["TrackingOrder"]
	if schema == nil {
		t.Fatal("the contract has no TrackingOrder schema")
	}
	if err := schema.Value.VisitJSON(decoded); err != nil {
		t.Errorf("the client's view breaks the contract's closed schema: %v\n%s", err, raw)
	}
}
