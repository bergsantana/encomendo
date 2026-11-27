package catalogports

import (
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/app/catalog/fake"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestFakeRepositories_Contract(t *testing.T) {
	RunRepositoryContract(t, func(*testing.T) Harness {
		ids := sharedfake.NewIDs(sharedfake.NewClock(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)))
		store := fake.NewStore()
		return Harness{
			Categories: store.Categories, Attributes: store.Attributes, Items: store.Items, Kits: store.Kits,
			Tx:        sharedfake.NewTxManager(),
			NewID:     ids.NewID,
			NewTenant: func(*testing.T) shared.TenantID { return shared.TenantID(ids.NewID()) },
			NewSupplier: func(_ *testing.T, tenant shared.TenantID) shared.SupplierID {
				id := shared.SupplierID(ids.NewID())
				store.AddSupplier(tenant, id)
				return id
			},
		}
	})
}
