package inproc

import (
	"context"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// CatalogSuppliers implements appcatalog.SupplierDirectory over the
// supplier use cases, so the catalog reads supplier names without importing
// app/supplier. The dependency runs one way, catalog → supplier (ADR 0025).
type CatalogSuppliers struct{ suppliers appsupplier.UseCases }

var _ appcatalog.SupplierDirectory = (*CatalogSuppliers)(nil)

// NewCatalogSuppliers returns the catalog's supplier directory.
func NewCatalogSuppliers(u appsupplier.UseCases) *CatalogSuppliers {
	return &CatalogSuppliers{suppliers: u}
}

// Names returns the names of the caller's store's suppliers among ids; an
// unknown id, or another store's, is absent. The supplier use case
// authorizes the caller (catalog:read).
func (c *CatalogSuppliers) Names(ctx context.Context, ids []shared.SupplierID) (map[shared.SupplierID]string, error) {
	return c.suppliers.Names(ctx, ids)
}
