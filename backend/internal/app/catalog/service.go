package catalog

import (
	"context"
	"fmt"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Deps are the Service's ports. Every one is required.
type Deps struct {
	Categories Categories
	Attributes Attributes
	Items      Items
	Kits       Kits
	Suppliers  SupplierDirectory
	Defaults   StoreDefaults
	Photos     PhotoURLs
	Cache      appshared.Cache
	Tx         appshared.TxManager
	IDs        appshared.IDGenerator
}

// Service implements UseCases and StoreQueries: the store's catalog.
type Service struct {
	categories Categories
	attributes Attributes
	items      Items
	kits       Kits
	suppliers  SupplierDirectory
	defaults   StoreDefaults
	photos     PhotoURLs
	cache      appshared.Cache
	tx         appshared.TxManager
	ids        appshared.IDGenerator
}

// NewService returns a Service over d.
func NewService(d Deps) *Service {
	return &Service{
		categories: d.Categories, attributes: d.Attributes, items: d.Items, kits: d.Kits,
		suppliers: d.Suppliers, defaults: d.Defaults, photos: d.Photos,
		cache: d.Cache, tx: d.Tx, ids: d.IDs,
	}
}

// reader authorizes a read: catalog:read, which every role has.
func reader(ctx context.Context) (shared.Principal, error) {
	return authz.Require(ctx, identity.PermCatalogRead)
}

// writer authorizes a write: catalog:write and, when the input sends any
// cost field, a role that sees costs (ADR 0025). Both run before any
// transaction starts.
func writer(ctx context.Context, sendsCosts bool) (shared.Principal, error) {
	by, err := authz.Require(ctx, identity.PermCatalogWrite)
	if err != nil {
		return shared.Principal{}, err
	}
	// Product rule: costs are the owner's; any other role sending one, even
	// null, is refused (docs/api/openapi.yaml, "Costs and margins").
	if sendsCosts && !identity.SeesCosts(by.Role()) {
		return shared.Principal{}, fmt.Errorf("role %s: %w", by.Role(), ErrCostForbidden)
	}
	return by, nil
}

// catalogKey is the prefix of every cached catalog read of the store.
func catalogKey(tenant shared.TenantID, parts ...string) (appshared.Key, error) {
	return appshared.TenantKey(tenant, append([]string{"catalog"}, parts...)...)
}

// invalidate drops the store's cached catalog reads after a write commits.
func (s *Service) invalidate(ctx context.Context, tenant shared.TenantID) {
	key, err := catalogKey(tenant)
	if err != nil {
		return // the tenant comes from a verified principal; a bad key is unreachable
	}
	// A cache that is down must not fail a write that committed; the entries
	// expire in 5 minutes anyway (backend/CLAUDE.md, Redis), and the adapter
	// logs the failure.
	_ = s.cache.DeleteByPrefix(ctx, key)
}

// supplierNames reads the names of the suppliers in ids; none for no ids.
func (s *Service) supplierNames(ctx context.Context, ids []shared.SupplierID) (map[shared.SupplierID]string, error) {
	if len(ids) == 0 {
		return map[shared.SupplierID]string{}, nil
	}
	names, err := s.suppliers.Names(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("read supplier names: %w", err)
	}
	return names, nil
}

// storeDefaults reads the store's defaults.
func (s *Service) storeDefaults(ctx context.Context) (Defaults, error) {
	d, err := s.defaults.StoreDefaults(ctx)
	if err != nil {
		return Defaults{}, fmt.Errorf("read store defaults: %w", err)
	}
	return d, nil
}
