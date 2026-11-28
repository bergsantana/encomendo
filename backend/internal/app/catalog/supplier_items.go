package catalog

import (
	"context"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// SupplierItemUseView is one catalog item a supplier supplies, as main or
// backup: a row of the Fornecedores tab of Compras.
type SupplierItemUseView struct {
	Item     shared.ItemID
	Name     string
	Role     supplier.Role
	LeadDays int
	// Cost is this supplier's cost for the item. It is set only for a role
	// that sees costs (identity.SeesCosts, ADR 0025) and only when one is
	// recorded.
	Cost *shared.Money
}

// SupplierItems lists the items that are not archived and have the supplier
// as main or backup, by name (catalog:read). Costs are the owner's only. It
// returns an error matching shared.ErrNotFound when the store has no such
// supplier, so an unknown id is told from a supplier with no items.
//
// It reads the store's items a page at a time instead of adding a query to
// the Items port: a store's catalog is small, and the port stays as the
// adapters implement it.
func (s *Service) SupplierItems(ctx context.Context, id shared.SupplierID) ([]SupplierItemUseView, error) {
	by, err := authz.Require(ctx, identity.PermCatalogRead)
	if err != nil {
		return nil, err
	}
	names, err := s.supplierNames(ctx, []shared.SupplierID{id})
	if err != nil {
		return nil, err
	}
	if _, ok := names[id]; !ok {
		return nil, fmt.Errorf("supplier %s: %w", id, shared.ErrNotFound)
	}
	var items []*catalog.Item
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		all, err := s.allItems(ctx, by.TenantID())
		items = all
		return err
	})
	if err != nil {
		return nil, err
	}
	sees := identity.SeesCosts(by.Role())
	out := []SupplierItemUseView{}
	for _, it := range items {
		for _, get := range []func() (supplier.SupplierItem, bool){it.MainSupplier, it.BackupSupplier} {
			si, ok := get()
			if !ok || si.SupplierID() != id {
				continue
			}
			v := SupplierItemUseView{Item: it.ID(), Name: it.Name(), Role: si.Role(), LeadDays: si.LeadDays()}
			if sees {
				v.Cost = si.Cost()
			}
			out = append(out, v)
		}
	}
	return out, nil
}

// allItems reads every item that is not archived, by name, a page at a
// time.
func (s *Service) allItems(ctx context.Context, tenant shared.TenantID) ([]*catalog.Item, error) {
	var all []*catalog.Item
	cursor := ""
	for {
		page, err := shared.NewPageRequest(cursor, shared.MaxPageLimit)
		if err != nil {
			return nil, err
		}
		got, err := s.items.List(ctx, tenant, ItemFilter{}, page)
		if err != nil {
			return nil, fmt.Errorf("list items: %w", err)
		}
		all = append(all, got.Items...)
		if got.NextCursor == "" {
			return all, nil
		}
		cursor = got.NextCursor
	}
}
