package supplier

import (
	"context"
	"fmt"
	"strings"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// Deps are the Service's ports. Every one is required.
type Deps struct {
	Suppliers Repository
	Tx        appshared.TxManager
	IDs       appshared.IDGenerator
}

// Service implements UseCases: the store's suppliers.
type Service struct {
	suppliers Repository
	tx        appshared.TxManager
	ids       appshared.IDGenerator
}

// NewService returns a Service over d.
func NewService(d Deps) *Service {
	return &Service{suppliers: d.Suppliers, tx: d.Tx, ids: d.IDs}
}

// Create adds a supplier to the caller's store at version 1 (catalog:write).
// It returns a ValidationError listing every invalid field ("name",
// "whatsappPhone").
func (s *Service) Create(ctx context.Context, in CreateInput) (SupplierView, error) {
	by, err := authz.Require(ctx, identity.PermCatalogWrite)
	if err != nil {
		return SupplierView{}, err
	}
	sup, err := supplier.New(by.TenantID(), shared.SupplierID(s.ids.NewID()), in.Name, in.WhatsAppPhone)
	if err != nil {
		return SupplierView{}, err
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.suppliers.Insert(ctx, sup); err != nil {
			return fmt.Errorf("insert supplier %s: %w", sup.ID(), err)
		}
		return nil
	})
	if err != nil {
		return SupplierView{}, err
	}
	return supplierView(sup, sup.Version()), nil
}

// Replace gives a supplier a new name and number (catalog:write). It returns
// supplier.ErrNotFound for an unknown supplier, shared.ErrVersionConflict
// when in.Version is not the stored version, and a ValidationError listing
// every invalid field. A stale version wins over invalid values: the client
// has to reload first.
func (s *Service) Replace(ctx context.Context, in ReplaceInput) (SupplierView, error) {
	by, err := authz.Require(ctx, identity.PermCatalogWrite)
	if err != nil {
		return SupplierView{}, err
	}
	var sup *supplier.Supplier
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		sup, err = s.suppliers.ByID(ctx, by.TenantID(), in.ID)
		if err != nil {
			return fmt.Errorf("load supplier %s: %w", in.ID, err)
		}
		if err := sup.CheckVersion(in.Version); err != nil {
			return err
		}
		if err := shared.MergeValidation(sup.Rename(in.Name), sup.ChangePhone(in.WhatsAppPhone)); err != nil {
			return err
		}
		if err := s.suppliers.Update(ctx, sup); err != nil { // optimistic lock on version
			return fmt.Errorf("save supplier %s: %w", in.ID, err)
		}
		return nil
	})
	if err != nil {
		return SupplierView{}, err
	}
	return supplierView(sup, sup.Version()+1), nil // the repository stored the next version
}

// Get returns one supplier (catalog:read, every role). It returns
// supplier.ErrNotFound for an unknown supplier, and for another store's.
func (s *Service) Get(ctx context.Context, id shared.SupplierID) (SupplierView, error) {
	by, err := authz.Require(ctx, identity.PermCatalogRead)
	if err != nil {
		return SupplierView{}, err
	}
	var sup *supplier.Supplier
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		sup, err = s.suppliers.ByID(ctx, by.TenantID(), id)
		if err != nil {
			return fmt.Errorf("load supplier %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return SupplierView{}, err
	}
	return supplierView(sup, sup.Version()), nil
}

// List returns one page of the store's suppliers, ordered by name and then
// id, narrowed by filter (catalog:read, every role). A cursor the repository
// did not issue is a ValidationError.
func (s *Service) List(ctx context.Context, filter ListFilter, page shared.PageRequest) (shared.Page[SupplierView], error) {
	by, err := authz.Require(ctx, identity.PermCatalogRead)
	if err != nil {
		return shared.Page[SupplierView]{}, err
	}
	filter.Query = strings.TrimSpace(filter.Query)
	var got shared.Page[*supplier.Supplier]
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		got, err = s.suppliers.List(ctx, by.TenantID(), filter, page)
		if err != nil {
			return fmt.Errorf("list suppliers: %w", err)
		}
		return nil
	})
	if err != nil {
		return shared.Page[SupplierView]{}, err
	}
	out := shared.Page[SupplierView]{Items: make([]SupplierView, 0, len(got.Items)), NextCursor: got.NextCursor}
	for _, sup := range got.Items {
		out.Items = append(out.Items, supplierView(sup, sup.Version()))
	}
	return out, nil
}

// Names returns the name of each supplier in ids that the caller's store
// has (catalog:read). An id with no supplier, or another store's, is absent
// from the map and is not an error: the catalog shows such an item without
// a supplier name.
func (s *Service) Names(ctx context.Context, ids []shared.SupplierID) (map[shared.SupplierID]string, error) {
	by, err := authz.Require(ctx, identity.PermCatalogRead)
	if err != nil {
		return nil, err
	}
	names := make(map[shared.SupplierID]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	var found []*supplier.Supplier
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		found, err = s.suppliers.ByIDs(ctx, by.TenantID(), ids)
		if err != nil {
			return fmt.Errorf("load supplier names: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, sup := range found {
		names[sup.ID()] = sup.Name()
	}
	return names, nil
}
