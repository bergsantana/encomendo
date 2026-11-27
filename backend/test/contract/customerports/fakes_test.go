package customerports

import (
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/app/customer/fake"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestFakeRepository_Contract(t *testing.T) {
	RunRepositoryContract(t, func(*testing.T) Harness {
		ids := sharedfake.NewIDs(sharedfake.NewClock(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)))
		store := fake.NewStore()
		return Harness{
			Customers: store,
			Tx:        sharedfake.NewTxManager(),
			NewID:     ids.NewID,
			NewTenant: func(*testing.T) shared.TenantID { return shared.TenantID(ids.NewID()) },
			AddOrder: func(_ *testing.T, _ shared.TenantID, id shared.CustomerID, at time.Time) {
				store.AddOrder(id, at)
			},
		}
	})
}
