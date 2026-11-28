package orderports

import (
	"context"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/app/order/fake"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

func fakeHarness(t *testing.T) Harness {
	t.Helper()
	ids := sharedfake.NewIDs(sharedfake.NewClock(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)))
	wf := testkit.Preset(t, testkit.PresetEncomendaMontagem)
	orders := fake.NewOrders()
	customers := fake.NewCustomers(ids.NewID)
	orders.Link(fake.NewWorkflows(wf), customers)
	return Harness{
		Orders:    orders,
		Tx:        sharedfake.NewTxManager(),
		NewID:     ids.NewID,
		NewTenant: func(*testing.T) shared.TenantID { return shared.TenantID(ids.NewID()) },
		Workflow:  wf,
		NewCustomer: func(_ *testing.T, _ shared.TenantID, name string) shared.CustomerID {
			id := shared.CustomerID(ids.NewID())
			customers.Add(apporder.CustomerFacts{ID: id, Name: name})
			return id
		},
		NewUser: func(_ *testing.T, name string) shared.UserID {
			id := shared.UserID(ids.NewID())
			orders.NameUser(id, name)
			return id
		},
	}
}

func TestFakeRepository_Contract(t *testing.T) {
	RunRepositoryContract(t, fakeHarness)
}

// The cap on hand-overs is a constant of the port: one more than the cap
// stored, and the query still returns the cap, oldest first. Postgres runs
// the same check over rows seeded in bulk (test/integration/postgres).
func TestFakeRepository_HandedOverStopsAtTheCap(t *testing.T) {
	h := fakeHarness(t)
	s := h.newStore(t)
	for i := range apporder.HandedOverCap + 1 {
		h.insertAs(t, s, handOver(h, epoch.Add(time.Duration(i)*time.Minute)))
	}
	var got []order.Record
	h.in(t, s, func(ctx context.Context) error {
		var err error
		got, err = h.Orders.HandedOver(ctx, s.id, epoch, epoch.Add(24*365*time.Hour))
		return err
	})
	if len(got) != apporder.HandedOverCap {
		t.Fatalf("handed over = %d orders, want the cap, %d", len(got), apporder.HandedOverCap)
	}
	if !got[0].HandedOverAt.Equal(epoch) || !got[len(got)-1].HandedOverAt.Equal(epoch.Add(time.Duration(apporder.HandedOverCap-1)*time.Minute)) {
		t.Errorf("the cap kept the wrong orders: first at %v, last at %v", got[0].HandedOverAt, got[len(got)-1].HandedOverAt)
	}
}
