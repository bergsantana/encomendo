package inproc

import (
	"context"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
)

// TenancyDefaults is what CatalogDefaults needs of app/tenancy:
// *apptenancy.Service.StoreDefaults.
type TenancyDefaults interface {
	StoreDefaults(ctx context.Context) (apptenancy.StoreDefaultsView, error)
}

// CatalogDefaults implements appcatalog.StoreDefaults over tenancy's
// StoreDefaults query, which needs only a tenant in the scope, so the store
// page can read the defaults too.
type CatalogDefaults struct{ tenancy TenancyDefaults }

var _ appcatalog.StoreDefaults = (*CatalogDefaults)(nil)

// NewCatalogDefaults returns the catalog's store defaults port.
func NewCatalogDefaults(t TenancyDefaults) *CatalogDefaults { return &CatalogDefaults{tenancy: t} }

// StoreDefaults returns the defaults of the store in the scope, with the
// errors of apptenancy.Service.StoreDefaults.
func (c *CatalogDefaults) StoreDefaults(ctx context.Context) (appcatalog.Defaults, error) {
	v, err := c.tenancy.StoreDefaults(ctx)
	if err != nil {
		return appcatalog.Defaults{}, err
	}
	return appcatalog.Defaults{
		Currency: v.Currency, AssemblyDays: v.AssemblyDays, SafetyMarginDays: v.SafetyMarginDays,
		StockEnabled: v.StockEnabled, ProductionEnabled: v.ProductionEnabled,
	}, nil
}
