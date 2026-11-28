package purchasing_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

var day1 = time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)

func date(t *testing.T, s string) shared.CivilDate { t.Helper(); return testkit.Date(t, s) }
func brl(t *testing.T, c int64) shared.Money       { t.Helper(); return testkit.BRL(t, c) }

func lineID() shared.PurchaseLineID { return shared.PurchaseLineID(testkit.NextID()) }

// choice is a store option choice with fresh ids.
func choice(option, label string) purchasing.Choice {
	return purchasing.Choice{
		Option: shared.OptionID(testkit.NextID()), ChoiceID: shared.ChoiceID(testkit.NextID()),
		Name: option, Label: label,
	}
}

// item is a new order item to buy, of a fresh order and catalog item.
func item(name string, qty int, choices ...purchasing.Choice) purchasing.Item {
	return purchasing.Item{
		Order: shared.OrderID(testkit.NextID()), OrderItem: shared.OrderItemID(testkit.NextID()),
		CatalogItem: shared.ItemID(testkit.NextID()), Name: name, Quantity: qty, Choices: choices,
	}
}

func spec(it purchasing.Item) purchasing.LineSpec {
	return purchasing.LineSpec{ID: lineID(), Item: it}
}

type sendFixture struct {
	tenant shared.TenantID
	by     shared.Principal
	spec   purchasing.SendSpec
}

// sendOf returns a spec to send items to a fresh supplier.
func sendOf(t *testing.T, items ...purchasing.Item) sendFixture {
	t.Helper()
	tenant := shared.TenantID(testkit.NextID())
	by := testkit.Staff(t, tenant, shared.RoleAdmin)
	lines := make([]purchasing.LineSpec, len(items))
	for i, it := range items {
		lines[i] = spec(it)
	}
	return sendFixture{tenant: tenant, by: by, spec: purchasing.SendSpec{
		Tenant: tenant, ID: shared.SupplierOrderID(testkit.NextID()), Number: 12,
		Supplier: shared.SupplierID(testkit.NextID()), SupplierName: "Moto Peças Ltda", SupplierPhone: "5592999990000",
		Lines: lines, By: by, Now: day1,
	}}
}

// sent returns a supplier order of the items, with every line sent.
func sent(t *testing.T, items ...purchasing.Item) *purchasing.SupplierOrder {
	t.Helper()
	o, err := purchasing.Send(sendOf(t, items...).spec)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	return o
}

// fieldOf returns the field of the first ValidationError in err's chain.
func fieldOf(t *testing.T, err error) string {
	t.Helper()
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || len(ve.Fields) == 0 {
		t.Fatalf("error %v is not a ValidationError", err)
	}
	return ve.Fields[0].Field
}
