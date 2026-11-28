//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	"github.com/bergsantana/encomendo/backend/internal/adapter/system"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/test/contract/orderports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

var customerPhones atomic.Int64

// defaultWorkflow reads the workflow new stores start on, the one the
// migrations seed, as the app does.
func defaultWorkflow(t *testing.T, tx appshared.TxManager) workflow.Workflow {
	t.Helper()
	var wf workflow.Workflow
	err := tx.WithinTx(context.Background(), func(ctx context.Context) error {
		var err error
		wf, err = postgres.NewWorkflows().Default(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("read the default workflow: %v", err)
	}
	return wf
}

// seedCustomer inserts, as the superuser, a customer of the store with a
// number nobody else in the run has.
func seedCustomer(t testing.TB, s *pgtest.Seeder, tenant shared.TenantID, name string) shared.CustomerID {
	t.Helper()
	id := pgtest.NewID()
	phone := fmt.Sprintf("559288%07d", customerPhones.Add(1))
	_, err := s.DB().ExecContext(t.Context(),
		`INSERT INTO customers (tenant_id, id, name, whatsapp_phone) VALUES ($1, $2, $3, $4)`,
		tenant.String(), id.String(), name, phone)
	if err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	return shared.CustomerID(id)
}

func seedNamedUser(t testing.TB, s *pgtest.Seeder, name string) shared.UserID {
	t.Helper()
	user := s.User()
	if _, err := s.DB().ExecContext(t.Context(), `UPDATE users SET name = $2 WHERE id = $1`, user.String(), name); err != nil {
		t.Fatalf("name user: %v", err)
	}
	return user
}

// The order port contract, the same suite the fake passes in
// test/contract. Stores, customers and users are seeded as the superuser, so
// the suite runs as encomendo_app against real rows, real RLS and the real
// column grants.
func TestOrderRepository_Contract(t *testing.T) {
	tx := postgres.NewTxManager(openApp(t))
	s := env.Seeder(t)
	wf := defaultWorkflow(t, tx)
	orders := postgres.NewOrders(system.NewIDs())
	orderports.RunRepositoryContract(t, func(*testing.T) orderports.Harness {
		return orderports.Harness{
			Orders:    orders,
			Tx:        tx,
			NewID:     pgtest.NewID,
			NewTenant: func(*testing.T) shared.TenantID { return s.Tenant(s.User()) },
			Workflow:  wf,
			NewCustomer: func(t *testing.T, tenant shared.TenantID, name string) shared.CustomerID {
				return seedCustomer(t, s, tenant, name)
			},
			NewUser: func(t *testing.T, name string) shared.UserID { return seedNamedUser(t, s, name) },
		}
	})
}
