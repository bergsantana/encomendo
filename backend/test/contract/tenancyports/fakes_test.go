package tenancyports

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/app/tenancy/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

func TestFakeRepository_Contract(t *testing.T) {
	RunRepositoryContract(t, func(*testing.T) Harness {
		start := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
		ids := sharedfake.NewIDs(sharedfake.NewClock(start))
		store := fake.NewStore()
		var n atomic.Int64
		return Harness{
			Tenants: store, Members: store, Invitations: store,
			Tx:      sharedfake.NewTxManager(),
			NewID:   ids.NewID,
			NewUser: func(*testing.T) shared.UserID { return shared.UserID(ids.NewID()) },
			NewSlug: func(*testing.T) string { return fmt.Sprintf("loja-%d", n.Add(1)) },
			Workflow: tenancy.WorkflowRef{
				ID: shared.WorkflowID(ids.NewID()), Version: 1, Name: "Encomenda com montagem",
			},
			OtherWorkflow: tenancy.WorkflowRef{
				ID: shared.WorkflowID(ids.NewID()), Version: 2, Name: "Orçamento primeiro",
			},
			Now:     start,
			Suspend: func(_ *testing.T, id shared.TenantID) { store.Suspend(id) },
		}
	})
}
