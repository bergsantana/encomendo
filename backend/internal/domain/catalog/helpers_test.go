package catalog

import (
	"errors"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var tenantID = shared.TenantID(shared.NewV7(1, [10]byte{1}))

// newID returns a distinct, valid id for each n.
func newID(n byte) shared.ID { return shared.NewV7(int64(n), [10]byte{n}) }

func itemID(n byte) shared.ItemID                    { return shared.ItemID(newID(n)) }
func supplierID(n byte) shared.SupplierID            { return shared.SupplierID(newID(n)) }
func kitID(n byte) shared.KitID                      { return shared.KitID(newID(n)) }
func photoID(n byte) shared.PhotoID                  { return shared.PhotoID(newID(n)) }
func attrValueID(n byte) shared.AttributeValueID     { return shared.AttributeValueID(newID(n)) }
func optionID(n byte) shared.OptionID                { return shared.OptionID(newID(n)) }
func choiceID(n byte) shared.ChoiceID                { return shared.ChoiceID(newID(n)) }
func categoryID(n byte) shared.CategoryID            { return shared.CategoryID(newID(n)) }
func attributeID(n byte) shared.AttributeID          { return shared.AttributeID(newID(n)) }
func moneyOf(t testing.TB, cents int64) shared.Money { return mustMoney(t, cents, "BRL") }

func mustMoney(t testing.TB, cents int64, currency string) shared.Money {
	t.Helper()
	m, err := shared.NewMoney(cents, currency)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// fieldsOf returns the field names and messages of a ValidationError, or
// fails the test when err is not one.
func fieldsOf(t testing.TB, err error) map[string]string {
	t.Helper()
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %v, want a ValidationError", err)
	}
	if !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("error %v does not match shared.ErrInvalid", err)
	}
	out := make(map[string]string, len(ve.Fields))
	for _, f := range ve.Fields {
		out[f.Field] = f.Message
	}
	return out
}

// wantFields fails the test unless err is a ValidationError on exactly the
// fields of want, with those messages.
func wantFields(t testing.TB, err error, want map[string]string) {
	t.Helper()
	got := fieldsOf(t, err)
	if len(got) != len(want) {
		t.Errorf("fields = %v, want %v", got, want)
		return
	}
	for field, msg := range want {
		if got[field] != msg {
			t.Errorf("field %q = %q, want %q", field, got[field], msg)
		}
	}
}

func costPtr(t testing.TB, cents int64) *shared.Money {
	m := moneyOf(t, cents)
	return &m
}

func link(id byte, lead int, cost CostChange) *SupplierLink {
	return &SupplierLink{Supplier: supplierID(id), LeadDays: lead, Cost: cost}
}

// toOrderInput returns a valid to_order item priced R$ 400,00 with supplier
// 1 as main (15 days, cost R$ 250,00).
func toOrderInput(t testing.TB) ItemInput {
	return ItemInput{
		ItemDetails: ItemDetails{Name: "Banco de couro Scrambler", Price: moneyOf(t, 40000)},
		Sourcing: SourcingInput{
			Mode: ModeToOrder,
			Main: link(1, 15, SetCost(moneyOf(t, 25000))),
		},
	}
}

func inHouseInput(t testing.TB) ItemInput {
	return ItemInput{
		ItemDetails: ItemDetails{Name: "Bolo de festa", Price: moneyOf(t, 15000)},
		Sourcing:    SourcingInput{Mode: ModeInHouse, ProductionDays: 3, ProductionCost: SetCost(moneyOf(t, 6000))},
	}
}

func newItem(t testing.TB, id byte, in ItemInput) *Item {
	t.Helper()
	i, err := NewItem(tenantID, itemID(id), in)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func repeat(s string, n int) string { return strings.Repeat(s, n) }
