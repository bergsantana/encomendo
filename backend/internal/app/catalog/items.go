package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// Field names of the contract and pt-BR messages the app layer adds to the
// domain's: the "Catálogo" section of docs/product/design-document.md where
// it has the words, and new wording (category and field values) for the
// user to review.
const (
	fieldSourcingMode    = "sourcingMode"
	fieldCategoryID      = "categoryId"
	fieldAttributeValues = "attributeValueIds"
	fieldMainSupplierID  = "mainSupplier.supplierId"
	fieldBackupSupplier  = "backupSupplier.supplierId"

	msgSourcingMode   = "Escolha a origem do item."
	msgMainSupplier   = "Escolha o fornecedor principal."
	msgBackupSupplier = "Escolha o fornecedor reserva."
	msgCategory       = "Escolha uma das categorias da loja."
	msgValues         = "Escolha valores dos campos da loja."
)

func fieldError(field, message string) error {
	return shared.NewValidationError(shared.FieldError{Field: field, Message: message})
}

// ListItems returns one page of the store's items, ordered by name and then
// id (catalog:read). Cost and margin are in each row for the owner only. A
// cursor the repository did not issue is a ValidationError.
func (s *Service) ListItems(ctx context.Context, filter ItemFilter, page shared.PageRequest) (shared.Page[ItemSummaryView], error) {
	by, err := reader(ctx)
	if err != nil {
		return shared.Page[ItemSummaryView]{}, err
	}
	filter.Query = strings.TrimSpace(filter.Query)
	var got shared.Page[*catalog.Item]
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		got, err = s.items.List(ctx, by.TenantID(), filter, page)
		if err != nil {
			return fmt.Errorf("list items: %w", err)
		}
		return nil
	})
	if err != nil {
		return shared.Page[ItemSummaryView]{}, err
	}
	var ids []shared.SupplierID
	for _, it := range got.Items {
		if m, ok := it.MainSupplier(); ok {
			ids = append(ids, m.SupplierID())
		}
	}
	names, err := s.supplierNames(ctx, ids)
	if err != nil {
		return shared.Page[ItemSummaryView]{}, err
	}
	out := shared.Page[ItemSummaryView]{Items: make([]ItemSummaryView, 0, len(got.Items)), NextCursor: got.NextCursor}
	for _, it := range got.Items {
		cover, err := s.cover(ctx, it.Photos())
		if err != nil {
			return shared.Page[ItemSummaryView]{}, err
		}
		out.Items = append(out.Items, itemSummary(it, names, cover, identity.SeesCosts(by.Role())))
	}
	return out, nil
}

// GetItem returns one item with its suppliers, options and photos
// (catalog:read); costs and margin for the owner only. It returns
// catalog.ErrItemNotFound for an unknown item, and for another store's.
func (s *Service) GetItem(ctx context.Context, id shared.ItemID) (ItemView, error) {
	by, err := reader(ctx)
	if err != nil {
		return ItemView{}, err
	}
	var it *catalog.Item
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		it, err = s.items.ByID(ctx, by.TenantID(), id)
		if err != nil {
			return fmt.Errorf("load item %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return ItemView{}, err
	}
	return s.itemResult(ctx, by, it, it.Version(), nil)
}

// CreateItem adds an item at version 1 (catalog:write), published when
// in.Published. A cost field from a role that does not see costs is
// ErrCostForbidden. It returns a ValidationError listing every invalid
// field: the domain's (catalog.NewItem), a sourcing mode the store turned
// off ("sourcingMode"), a supplier the store does not have
// ("mainSupplier.supplierId", "backupSupplier.supplierId"), an unknown
// category ("categoryId") or field value ("attributeValueIds").
func (s *Service) CreateItem(ctx context.Context, in ItemInput) (ItemView, error) {
	by, err := writer(ctx, in.sendsCosts())
	if err != nil {
		return ItemView{}, err
	}
	d, err := s.storeDefaults(ctx)
	if err != nil {
		return ItemView{}, err
	}
	names, srcErr, err := s.checkSourcing(ctx, in, d)
	if err != nil {
		return ItemView{}, err
	}
	var it *catalog.Item
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		refErr, err := s.checkRefs(ctx, by.TenantID(), in)
		if err != nil {
			return err
		}
		din, err := s.itemDomainInput(in, d.Currency, nil)
		if err != nil {
			return err
		}
		it, err = catalog.NewItem(by.TenantID(), shared.ItemID(s.ids.NewID()), din)
		if err := shared.MergeValidation(srcErr, refErr, err); err != nil {
			return err
		}
		if err := setItemPublished(it, in.Published); err != nil {
			return err
		}
		if err := s.items.Insert(ctx, it); err != nil {
			return fmt.Errorf("insert item: %w", err)
		}
		return nil
	})
	if err != nil {
		return ItemView{}, err
	}
	s.invalidate(ctx, by.TenantID())
	return s.itemResult(ctx, by, it, it.Version(), names)
}

// ReplaceItem replaces everything about an item but its photos, at the
// version read (catalog:write). A cost field left out keeps the cost of the
// same supplier, wherever it sits now (ADR 0025); a cost field sent by a role
// that does not see costs is ErrCostForbidden. It returns
// catalog.ErrItemNotFound, shared.ErrVersionConflict (checked before the new
// values), catalog.ErrPublishArchived for publishing an archived item, and
// the ValidationError of CreateItem.
func (s *Service) ReplaceItem(ctx context.Context, in ReplaceItemInput) (ItemView, error) {
	by, err := writer(ctx, in.Item.sendsCosts())
	if err != nil {
		return ItemView{}, err
	}
	d, err := s.storeDefaults(ctx)
	if err != nil {
		return ItemView{}, err
	}
	names, srcErr, err := s.checkSourcing(ctx, in.Item, d)
	if err != nil {
		return ItemView{}, err
	}
	var it *catalog.Item
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if it, err = s.loadItem(ctx, by.TenantID(), in.ID, in.Version); err != nil {
			return err
		}
		refErr, err := s.checkRefs(ctx, by.TenantID(), in.Item)
		if err != nil {
			return err
		}
		din, err := s.itemDomainInput(in.Item, it.Price().Currency(), it.Options())
		if err != nil {
			return err
		}
		if err := shared.MergeValidation(srcErr, refErr, it.Replace(din)); err != nil {
			return err
		}
		if err := setItemPublished(it, in.Item.Published); err != nil {
			return err
		}
		return s.saveItem(ctx, it)
	})
	if err != nil {
		return ItemView{}, err
	}
	s.invalidate(ctx, by.TenantID())
	return s.itemResult(ctx, by, it, it.Version()+1, names) // the repository stored the next version
}

// SetItemPublished shows or hides an item on the store page, at the version
// read (catalog:write). It returns catalog.ErrItemNotFound,
// shared.ErrVersionConflict and catalog.ErrPublishArchived.
func (s *Service) SetItemPublished(ctx context.Context, in SetItemPublishedInput) (ItemView, error) {
	return s.changeItem(ctx, in.ID, in.Version, func(_ context.Context, it *catalog.Item) error {
		return setItemPublished(it, in.Published)
	})
}

// ArchiveItem archives an item, which also unpublishes it, at the version
// read (catalog:write). While kits that are not archived hold the item, as a
// member or an alternative, it returns a *RefusalError matching
// catalog.ErrItemInKit that names them (ADR 0025). It also returns
// catalog.ErrItemNotFound and shared.ErrVersionConflict.
func (s *Service) ArchiveItem(ctx context.Context, in ItemVersionInput) (ItemView, error) {
	return s.changeItem(ctx, in.ID, in.Version, func(ctx context.Context, it *catalog.Item) error {
		kits, err := s.items.ActiveKitsHolding(ctx, it.TenantID(), it.ID())
		if err != nil {
			return fmt.Errorf("kits holding item %s: %w", it.ID(), err)
		}
		if len(kits) > 0 {
			return &RefusalError{Err: catalog.ErrItemInKit, References: kits}
		}
		it.Archive()
		return nil
	})
}

// RestoreItem brings an archived item back, unpublished, at the version read
// (catalog:write). It returns catalog.ErrItemNotFound and
// shared.ErrVersionConflict.
func (s *Service) RestoreItem(ctx context.Context, in ItemVersionInput) (ItemView, error) {
	return s.changeItem(ctx, in.ID, in.Version, func(_ context.Context, it *catalog.Item) error {
		it.Restore()
		return nil
	})
}

// changeItem runs one small write on an item: load it at version, let
// change decide, save, invalidate the cache and return the view.
func (s *Service) changeItem(ctx context.Context, id shared.ItemID, version int, change func(context.Context, *catalog.Item) error) (ItemView, error) {
	by, err := writer(ctx, false)
	if err != nil {
		return ItemView{}, err
	}
	var it *catalog.Item
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if it, err = s.loadItem(ctx, by.TenantID(), id, version); err != nil {
			return err
		}
		if err := change(ctx, it); err != nil {
			return err
		}
		return s.saveItem(ctx, it)
	})
	if err != nil {
		return ItemView{}, err
	}
	s.invalidate(ctx, by.TenantID())
	return s.itemResult(ctx, by, it, it.Version()+1, nil)
}

// loadItem reads the item and checks the version the client read.
func (s *Service) loadItem(ctx context.Context, tenant shared.TenantID, id shared.ItemID, version int) (*catalog.Item, error) {
	it, err := s.items.ByID(ctx, tenant, id)
	if err != nil {
		return nil, fmt.Errorf("load item %s: %w", id, err)
	}
	if err := it.CheckVersion(version); err != nil {
		return nil, err
	}
	return it, nil
}

func (s *Service) saveItem(ctx context.Context, it *catalog.Item) error {
	if err := s.items.Update(ctx, it); err != nil { // optimistic lock on version
		return fmt.Errorf("save item %s: %w", it.ID(), err)
	}
	return nil
}

func setItemPublished(it *catalog.Item, published bool) error {
	if !published {
		it.Unpublish()
		return nil
	}
	return it.Publish()
}

// itemResult builds the caller's view of it at version. names may already
// hold the suppliers' names; nil reads them.
func (s *Service) itemResult(ctx context.Context, by shared.Principal, it *catalog.Item, version int, names map[shared.SupplierID]string) (ItemView, error) {
	if names == nil {
		var ids []shared.SupplierID
		for _, get := range []func() (supplier.SupplierItem, bool){it.MainSupplier, it.BackupSupplier} {
			if si, ok := get(); ok {
				ids = append(ids, si.SupplierID())
			}
		}
		read, err := s.supplierNames(ctx, ids)
		if err != nil {
			return ItemView{}, err
		}
		names = read
	}
	photos, err := s.photoViews(ctx, it.Photos())
	if err != nil {
		return ItemView{}, err
	}
	return itemView(it, version, names, photos, identity.SeesCosts(by.Role())), nil
}

// checkSourcing checks what the domain cannot know: that the sourcing mode
// is turned on in the store's settings, and that each supplier is the
// store's. It returns the suppliers' names, the field errors (a
// ValidationError, or nil) and, apart, a failure to read.
func (s *Service) checkSourcing(ctx context.Context, in ItemInput, d Defaults) (map[shared.SupplierID]string, error, error) {
	var errs []error
	off := (in.SourcingMode == catalog.ModeStock && !d.StockEnabled) ||
		(in.SourcingMode == catalog.ModeInHouse && !d.ProductionEnabled)
	if off {
		errs = append(errs, fieldError(fieldSourcingMode, msgSourcingMode))
	}
	slots := []struct {
		in      *SupplierInput
		field   string
		message string
	}{{in.MainSupplier, fieldMainSupplierID, msgMainSupplier}, {in.BackupSupplier, fieldBackupSupplier, msgBackupSupplier}}
	var ids []shared.SupplierID
	for _, sl := range slots {
		if sl.in != nil && !sl.in.SupplierID.IsZero() {
			ids = append(ids, sl.in.SupplierID)
		}
	}
	names, err := s.supplierNames(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	for _, sl := range slots {
		if sl.in == nil || sl.in.SupplierID.IsZero() {
			continue // the domain reports a missing supplier
		}
		if _, ok := names[sl.in.SupplierID]; !ok {
			errs = append(errs, fieldError(sl.field, sl.message))
		}
	}
	return names, shared.MergeValidation(errs...), nil
}

// checkRefs checks, inside the transaction, that the item's category and
// field values are the store's. It returns the field errors (or nil) and,
// apart, a failure to read.
func (s *Service) checkRefs(ctx context.Context, tenant shared.TenantID, in ItemInput) (error, error) {
	var errs []error
	if !in.CategoryID.IsZero() {
		_, err := s.categories.ByID(ctx, tenant, in.CategoryID)
		switch {
		case errors.Is(err, catalog.ErrCategoryNotFound):
			errs = append(errs, fieldError(fieldCategoryID, msgCategory))
		case err != nil:
			return nil, fmt.Errorf("load category %s: %w", in.CategoryID, err)
		}
	}
	verr, err := s.checkValues(ctx, tenant, in.AttributeValueIDs)
	if err != nil {
		return nil, err
	}
	return shared.MergeValidation(append(errs, verr)...), nil
}

// checkValues returns a field error on "attributeValueIds" when a value is
// not one of the store's fields' values.
func (s *Service) checkValues(ctx context.Context, tenant shared.TenantID, ids []shared.AttributeValueID) (error, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := s.attributes.List(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("list attributes: %w", err)
	}
	known := map[shared.AttributeValueID]bool{}
	for _, p := range list {
		for _, v := range p.Attribute.Values() {
			known[v.ID()] = true
		}
	}
	for _, id := range ids {
		if !known[id] {
			return fieldError(fieldAttributeValues, msgValues), nil
		}
	}
	return nil, nil
}
