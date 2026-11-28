package orderports

import (
	"reflect"
	"testing"
)

// sameRecord fails t unless got is want, field for field.
func sameRecord(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("differs\n got: %+v\nwant: %+v", got, want)
	}
}

// contractCase is one behavior of the port. Each runs on a harness of its
// own, with its own stores.
type contractCase struct {
	name string
	run  func(*testing.T, Harness)
}

// RunRepositoryContract checks the order repository: what is stored reads
// back whole, optimistic updates, lines and items replaced as sets, payments
// and voids, the history, the number counter, and every query the Hoje,
// Pedidos and Financeiro screens read.
func RunRepositoryContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Helper()
	groups := []struct {
		name  string
		cases []contractCase
	}{
		{"round trip", roundTripCases},
		{"update", updateCases},
		{"history", historyCases},
		{"numbers", numberCases},
		{"list", listCases},
		{"active", activeCases},
		{"payments", paymentCases},
		{"handed over", handedOverCases},
		{"messages and transactions", messageCases},
		{"isolation", isolationCases},
	}
	for _, g := range groups {
		t.Run(g.name, func(t *testing.T) {
			for _, c := range g.cases {
				t.Run(c.name, func(t *testing.T) { c.run(t, newHarness(t)) })
			}
		})
	}
}
