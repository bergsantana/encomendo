//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	"github.com/bergsantana/encomendo/backend/internal/adapter/system"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
	"github.com/bergsantana/encomendo/backend/test/contract/purchasingports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// purchasingWorld seeds what purchasing lines point at: stores with a staff
// member, suppliers, and orders with one bought item each, written through
// the order repository so the sourcing, and its cost, is what an order
// really stores.
type purchasingWorld struct {
	seed      *pgtest.Seeder
	tx        appshared.TxManager
	orders    *postgres.Orders
	wf        workflow.Workflow
	mu        sync.Mutex
	staff     map[shared.TenantID]shared.UserID
	customers map[shared.TenantID]shared.CustomerID
}

func (w *purchasingWorld) newStore(t *testing.T) purchasingports.Store {
	t.Helper()
	user := seedNamedUser(t, w.seed, "Ana Souza")
	tenant := w.seed.Tenant(user)
	customer := seedCustomer(t, w.seed, tenant, "Cliente de teste")
	w.mu.Lock()
	defer w.mu.Unlock()
	w.staff[tenant], w.customers[tenant] = user, customer
	return purchasingports.Store{Tenant: tenant, User: user}
}

// newItem stores an order of the store with one bought item, whose unit cost
// is cost cents when not nil.
func (w *purchasingWorld) newItem(t *testing.T, tenant shared.TenantID, cost *int64) purchasingports.Bought {
	t.Helper()
	w.mu.Lock()
	user, customer := w.staff[tenant], w.customers[tenant]
	w.mu.Unlock()
	by, err := shared.NewBackofficePrincipal(user, tenant, shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	var o *order.Order
	scope := appshared.Scope{TenantID: tenant, UserID: user}
	err = w.tx.WithinTx(appshared.WithScope(context.Background(), scope), func(ctx context.Context) error {
		n, err := w.orders.NextNumber(ctx, tenant)
		if err != nil {
			return err
		}
		spec := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 5, Cost: cost}).Spec(t)
		spec.Tenant, spec.Customer, spec.Workflow, spec.Number = tenant, customer, w.wf, n
		spec.Tracking = shared.SecretHash(pgtest.RandomHash())
		spec.By = by
		if o, err = order.New(spec); err != nil {
			return err
		}
		return w.orders.Insert(ctx, o)
	})
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
	it := o.Items()[0]
	return purchasingports.Bought{Order: o.ID(), Item: it.ID(), Catalog: it.Snapshot().CatalogItem}
}

// The purchasing port contract, the same suite the fake passes in
// test/contract. It runs as encomendo_app against real rows, real RLS and
// the real column grants.
func TestPurchasingRepository_Contract(t *testing.T) {
	tx := postgres.NewTxManager(openApp(t))
	s := env.Seeder(t)
	w := &purchasingWorld{
		seed: s, tx: tx, orders: postgres.NewOrders(system.NewIDs()), wf: defaultWorkflow(t, tx),
		staff: map[shared.TenantID]shared.UserID{}, customers: map[shared.TenantID]shared.CustomerID{},
	}
	repo := postgres.NewPurchasing()
	purchasingports.RunRepositoryContract(t, func(*testing.T) purchasingports.RepositoryHarness {
		return purchasingports.RepositoryHarness{
			Repo: repo, Tx: tx, NewID: pgtest.NewID, NewStore: w.newStore, NewItem: w.newItem,
			NewSupplier: func(_ *testing.T, tenant shared.TenantID) shared.SupplierID { return s.Supplier(tenant) },
			RemoveItem: func(t *testing.T, tenant shared.TenantID, b purchasingports.Bought) {
				// As the superuser, so the case does not depend on the order
				// repository's own removal: the foreign key's ON DELETE SET
				// NULL is what keeps the line.
				_, err := s.DB().ExecContext(t.Context(), `DELETE FROM order_items WHERE tenant_id = $1 AND id = $2`,
					tenant.String(), b.Item.String())
				if err != nil {
					t.Fatalf("remove order item: %v", err)
				}
			},
		}
	})
}
