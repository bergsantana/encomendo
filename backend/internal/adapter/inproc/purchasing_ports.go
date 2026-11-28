package inproc

import (
	"context"
	"fmt"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// PurchasingSuppliers implements apppurchasing.Suppliers over the supplier
// use cases: who a pedido de compra goes to. The supplier use case
// authorizes the caller (catalog:read, which every role has).
type PurchasingSuppliers struct{ suppliers appsupplier.UseCases }

var _ apppurchasing.Suppliers = (*PurchasingSuppliers)(nil)

// NewPurchasingSuppliers returns the adapter over u.
func NewPurchasingSuppliers(u appsupplier.UseCases) *PurchasingSuppliers {
	return &PurchasingSuppliers{suppliers: u}
}

// Contact implements apppurchasing.Suppliers.
func (a *PurchasingSuppliers) Contact(ctx context.Context, id shared.SupplierID) (apppurchasing.SupplierContact, error) {
	v, err := a.suppliers.Get(ctx, id)
	if err != nil {
		return apppurchasing.SupplierContact{}, err
	}
	return apppurchasing.SupplierContact{ID: v.ID, Name: v.Name, Phone: v.WhatsAppPhone}, nil
}

// storeDefaults is the part of the tenancy service the store adapters read.
type storeDefaults interface {
	StoreDefaults(ctx context.Context) (apptenancy.StoreDefaultsView, error)
}

// PurchasingStore implements apppurchasing.Store over the store's settings
// and the clock: its name for the supplier texts, and today in its time
// zone (ADR 0030).
type PurchasingStore struct {
	tenancy storeDefaults
	clock   appshared.Clock
}

var _ apppurchasing.Store = (*PurchasingStore)(nil)

// NewPurchasingStore returns the adapter over t and clock.
func NewPurchasingStore(t storeDefaults, clock appshared.Clock) *PurchasingStore {
	return &PurchasingStore{tenancy: t, clock: clock}
}

// Name implements apppurchasing.Store.
func (a *PurchasingStore) Name(ctx context.Context) (string, error) {
	v, err := a.tenancy.StoreDefaults(ctx)
	if err != nil {
		return "", err
	}
	return v.StoreName, nil
}

// Today implements apppurchasing.Store.
func (a *PurchasingStore) Today(ctx context.Context) (shared.CivilDate, error) {
	v, err := a.tenancy.StoreDefaults(ctx)
	if err != nil {
		return shared.CivilDate{}, err
	}
	loc, err := v.TimeZone.Location()
	if err != nil {
		return shared.CivilDate{}, fmt.Errorf("store time zone %q: %w", v.TimeZone, err)
	}
	return shared.DateOf(a.clock.Now(), loc), nil
}
