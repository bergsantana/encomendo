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

// refuseList is every key a client may never be sent (docs/product/
// design-document.md, System rules; backend/CLAUDE.md).
var refuseList = []string{
	"cost", "margin", "supplier", "supplierName", "leadDays", "atRisk", "alerts", "recordedBy", "history", "expected",
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
			if strings.EqualFold(key, refused) {
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
