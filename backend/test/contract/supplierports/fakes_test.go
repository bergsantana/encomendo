package supplierports

import (
	"testing"
	"time"

	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/app/supplier/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestFakeRepository_Contract(t *testing.T) {
	RunRepositoryContract(t, func(*testing.T) Harness {
		ids := sharedfake.NewIDs(sharedfake.NewClock(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)))
		return Harness{
			Suppliers: fake.NewStore(),
			Tx:        sharedfake.NewTxManager(),
			NewID:     ids.NewID,
			NewTenant: func(*testing.T) shared.TenantID { return shared.TenantID(ids.NewID()) },
		}
	})
}
