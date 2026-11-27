//go:build integration

package postgres_test

import (
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/contract/catalogports"
	"github.com/bergsantana/encomendo/backend/test/contract/supplierports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// The catalog and supplier port contracts, the same suites the fakes pass in
// test/contract. Tenants and suppliers are seeded as the superuser, so the
// suites run as encomendo_app against real rows and real RLS.

func TestSupplierRepository_Contract(t *testing.T) {
	tx := postgres.NewTxManager(openApp(t))
	s := env.Seeder(t)
	supplierports.RunRepositoryContract(t, func(*testing.T) supplierports.Harness {
		return supplierports.Harness{
			Suppliers: postgres.NewSuppliers(),
			Tx:        tx,
			NewID:     pgtest.NewID,
			NewTenant: func(*testing.T) shared.TenantID { return s.Tenant(s.User()) },
		}
	})
}

func TestCatalogRepositories_Contract(t *testing.T) {
	tx := postgres.NewTxManager(openApp(t))
	s := env.Seeder(t)
	catalogports.RunRepositoryContract(t, func(*testing.T) catalogports.Harness {
		return catalogports.Harness{
			Categories: postgres.NewCategories(), Attributes: postgres.NewAttributes(),
			Items: postgres.NewItems(), Kits: postgres.NewKits(),
			Tx:          tx,
			NewID:       pgtest.NewID,
			NewTenant:   func(*testing.T) shared.TenantID { return s.Tenant(s.User()) },
			NewSupplier: func(_ *testing.T, tenant shared.TenantID) shared.SupplierID { return s.Supplier(tenant) },
		}
	})
}
