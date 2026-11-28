package purchasingports

import (
	"testing"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/app/purchasing/fake"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// The in-memory repository the use case tests rely on passes the same suite
// as Postgres (test/integration/postgres/purchasing_contract_test.go).
func TestFakeRepository_Contract(t *testing.T) {
	RunRepositoryContract(t, func(t *testing.T) RepositoryHarness {
		ids := sharedfake.NewIDs(sharedfake.NewClock(epoch))
		repo := fake.NewRepository()
		items := fake.NewOrderItems(testkit.DateOf(epoch))
		repo.LinkCosts(items)
		return RepositoryHarness{
			Repo:  repo,
			Tx:    sharedfake.NewTxManager(),
			NewID: ids.NewID,
			NewStore: func(*testing.T) Store {
				s := Store{Tenant: shared.TenantID(ids.NewID()), User: shared.UserID(ids.NewID())}
				repo.NameUser(s.Tenant, s.User, "Ana Souza")
				return s
			},
			NewSupplier: func(*testing.T, shared.TenantID) shared.SupplierID { return shared.SupplierID(ids.NewID()) },
			NewItem: func(t *testing.T, tenant shared.TenantID, cost *int64) Bought {
				b := Bought{
					Order: shared.OrderID(ids.NewID()), Item: shared.OrderItemID(ids.NewID()), Catalog: shared.ItemID(ids.NewID()),
				}
				it := fake.Item{CatalogItem: b.Catalog, Facts: apppurchasing.OrderItemFacts{
					Item: b.Item, Bought: true, Status: order.ItemToBuy, Name: "Banco de couro", Quantity: 1,
				}}
				if cost != nil {
					it.Cost = ptr(brl(t, *cost))
				}
				items.AddOrder(apppurchasing.OrderFacts{Order: b.Order}, true, it)
				repo.KnowItem(tenant, b.Item)
				return b
			},
			RemoveItem: func(_ *testing.T, _ shared.TenantID, b Bought) {
				items.RemoveItem(b.Order, b.Item)
				repo.ForgetItem(b.Item)
			},
		}
	})
}
