package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ListKits returns one page of the store's kits, ordered by name and then
// id, with their totals and days (catalog:read); cost and margin for the
// owner only. A cursor the repository did not issue is a ValidationError.
func (s *Service) ListKits(ctx context.Context, filter KitFilter, page shared.PageRequest) (shared.Page[KitSummaryView], error) {
	by, err := reader(ctx)
	if err != nil {
		return shared.Page[KitSummaryView]{}, err
	}
	d, err := s.storeDefaults(ctx)
	if err != nil {
		return shared.Page[KitSummaryView]{}, err
	}
	filter.Query = strings.TrimSpace(filter.Query)
	var got shared.Page[*catalog.Kit]
	var facts map[shared.ItemID]ItemFacts
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		listed, err := s.kits.List(ctx, by.TenantID(), filter, page)
		if err != nil {
			return fmt.Errorf("list kits: %w", err)
		}
		var items []catalog.KitItem
		for _, k := range listed.Items {
			items = append(items, k.Items()...)
		}
		read, err := s.kitFacts(ctx, by.TenantID(), items)
		if err != nil {
			return err
		}
		got, facts = listed, read
		return nil
	})
	if err != nil {
		return shared.Page[KitSummaryView]{}, err
	}
	out := shared.Page[KitSummaryView]{Items: make([]KitSummaryView, 0, len(got.Items)), NextCursor: got.NextCursor}
	for _, k := range got.Items {
		cover, err := s.cover(ctx, k.Photos())
		if err != nil {
			return shared.Page[KitSummaryView]{}, err
		}
		row, err := kitSummary(k, facts, d, cover, identity.SeesCosts(by.Role()))
		if err != nil {
			return shared.Page[KitSummaryView]{}, err
		}
		out.Items = append(out.Items, row)
	}
	return out, nil
}

// GetKit returns one kit with its items, totals and "Prazo calculado"
// (catalog:read); cost and margin for the owner only. It returns
// catalog.ErrKitNotFound.
func (s *Service) GetKit(ctx context.Context, id shared.KitID) (KitView, error) {
	by, err := reader(ctx)
	if err != nil {
		return KitView{}, err
	}
	d, err := s.storeDefaults(ctx)
	if err != nil {
		return KitView{}, err
	}
	var k *catalog.Kit
	var facts map[shared.ItemID]ItemFacts
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		loaded, err := s.kits.ByID(ctx, by.TenantID(), id)
		if err != nil {
			return fmt.Errorf("load kit %s: %w", id, err)
		}
		read, err := s.kitFacts(ctx, by.TenantID(), loaded.Items())
		if err != nil {
			return err
		}
		k, facts = loaded, read
		return nil
	})
	if err != nil {
		return KitView{}, err
	}
	return s.kitResult(ctx, by, kitState{kit: k, version: k.Version(), facts: facts}, d)
}

// CreateKit adds a kit at version 1 (catalog:write), published when
// in.Published. It returns the domain's ValidationError (catalog.NewKit,
// with an item of another store reported as unknown), a field error on
// "attributeValueIds" for an unknown value, and, once those pass, a
// *RefusalError matching catalog.ErrItemArchived that names the archived
// items the kit would hold or offer.
func (s *Service) CreateKit(ctx context.Context, in KitInput) (KitView, error) {
	by, err := writer(ctx, false)
	if err != nil {
		return KitView{}, err
	}
	d, err := s.storeDefaults(ctx)
	if err != nil {
		return KitView{}, err
	}
	var st kitState
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		valuesErr, facts, err := s.kitChecks(ctx, by.TenantID(), in)
		if err != nil {
			return err
		}
		din, err := kitDomainInput(in, d.Currency)
		if err != nil {
			return err
		}
		k, err := catalog.NewKit(by.TenantID(), shared.KitID(s.ids.NewID()), din, lookupOf(facts))
		if err := refusalOf(shared.MergeValidation(valuesErr, err), facts); err != nil {
			return err
		}
		if err := setKitPublished(k, in.Published); err != nil {
			return err
		}
		if err := s.kits.Insert(ctx, k); err != nil {
			return fmt.Errorf("insert kit: %w", err)
		}
		st = kitState{kit: k, version: k.Version(), facts: facts}
		return nil
	})
	if err != nil {
		return KitView{}, err
	}
	s.invalidate(ctx, by.TenantID())
	return s.kitResult(ctx, by, st, d)
}

// ReplaceKit replaces everything about a kit but its photos, at the version
// read (catalog:write). It returns catalog.ErrKitNotFound,
// shared.ErrVersionConflict (checked before the new values),
// catalog.ErrPublishArchived, and the errors of CreateKit; so a kit that
// holds an item archived since it was saved can be saved only once the item
// is replaced.
func (s *Service) ReplaceKit(ctx context.Context, in ReplaceKitInput) (KitView, error) {
	by, err := writer(ctx, false)
	if err != nil {
		return KitView{}, err
	}
	d, err := s.storeDefaults(ctx)
	if err != nil {
		return KitView{}, err
	}
	var st kitState
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		k, err := s.loadKit(ctx, by.TenantID(), in.ID, in.Version)
		if err != nil {
			return err
		}
		valuesErr, facts, err := s.kitChecks(ctx, by.TenantID(), in.Kit)
		if err != nil {
			return err
		}
		din, err := kitDomainInput(in.Kit, k.Price().Currency())
		if err != nil {
			return err
		}
		if err := refusalOf(shared.MergeValidation(valuesErr, k.Replace(din, lookupOf(facts))), facts); err != nil {
			return err
		}
		if err := setKitPublished(k, in.Kit.Published); err != nil {
			return err
		}
		st = kitState{kit: k, version: k.Version() + 1, facts: facts} // the repository stores the next version
		return s.saveKit(ctx, k)
	})
	if err != nil {
		return KitView{}, err
	}
	s.invalidate(ctx, by.TenantID())
	return s.kitResult(ctx, by, st, d)
}

// SetKitPublished shows or hides a kit on the store page, at the version
// read (catalog:write). It returns catalog.ErrKitNotFound,
// shared.ErrVersionConflict and catalog.ErrPublishArchived.
func (s *Service) SetKitPublished(ctx context.Context, in SetKitPublishedInput) (KitView, error) {
	return s.changeKit(ctx, in.ID, in.Version, func(k *catalog.Kit, _ map[shared.ItemID]ItemFacts) error {
		return setKitPublished(k, in.Published)
	})
}

// ArchiveKit archives a kit, which also unpublishes it and stops it holding
// its items, at the version read (catalog:write). It returns
// catalog.ErrKitNotFound and shared.ErrVersionConflict.
func (s *Service) ArchiveKit(ctx context.Context, in KitVersionInput) (KitView, error) {
	return s.changeKit(ctx, in.ID, in.Version, func(k *catalog.Kit, _ map[shared.ItemID]ItemFacts) error {
		k.Archive()
		return nil
	})
}

// RestoreKit brings an archived kit back, unpublished, at the version read
// (catalog:write). While an item of the kit, or an alternative, is archived
// it returns a *RefusalError matching catalog.ErrItemArchived that names
// them. It also returns catalog.ErrKitNotFound and
// shared.ErrVersionConflict.
func (s *Service) RestoreKit(ctx context.Context, in KitVersionInput) (KitView, error) {
	return s.changeKit(ctx, in.ID, in.Version, func(k *catalog.Kit, facts map[shared.ItemID]ItemFacts) error {
		return refusalOf(k.Restore(lookupOf(facts)), facts)
	})
}

// kitState is a kit as a use case leaves it: the version to show and the
// facts of the items it refers to.
type kitState struct {
	kit     *catalog.Kit
	version int
	facts   map[shared.ItemID]ItemFacts
}

// changeKit runs one small write on a kit: load it at version with its
// items' facts, let change decide, save, invalidate and return the view.
func (s *Service) changeKit(ctx context.Context, id shared.KitID, version int, change func(*catalog.Kit, map[shared.ItemID]ItemFacts) error) (KitView, error) {
	by, err := writer(ctx, false)
	if err != nil {
		return KitView{}, err
	}
	d, err := s.storeDefaults(ctx)
	if err != nil {
		return KitView{}, err
	}
	var st kitState
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		k, err := s.loadKit(ctx, by.TenantID(), id, version)
		if err != nil {
			return err
		}
		facts, err := s.kitFacts(ctx, by.TenantID(), k.Items())
		if err != nil {
			return err
		}
		if err := change(k, facts); err != nil {
			return err
		}
		st = kitState{kit: k, version: k.Version() + 1, facts: facts}
		return s.saveKit(ctx, k)
	})
	if err != nil {
		return KitView{}, err
	}
	s.invalidate(ctx, by.TenantID())
	return s.kitResult(ctx, by, st, d)
}

func (s *Service) loadKit(ctx context.Context, tenant shared.TenantID, id shared.KitID, version int) (*catalog.Kit, error) {
	k, err := s.kits.ByID(ctx, tenant, id)
	if err != nil {
		return nil, fmt.Errorf("load kit %s: %w", id, err)
	}
	if err := k.CheckVersion(version); err != nil {
		return nil, err
	}
	return k, nil
}

func (s *Service) saveKit(ctx context.Context, k *catalog.Kit) error {
	if err := s.kits.Update(ctx, k); err != nil { // optimistic lock on version
		return fmt.Errorf("save kit %s: %w", k.ID(), err)
	}
	return nil
}

func setKitPublished(k *catalog.Kit, published bool) error {
	if !published {
		k.Unpublish()
		return nil
	}
	return k.Publish()
}

// kitChecks reads, inside the transaction, what a kit write needs: the field
// error for unknown values (or nil) and the facts of the items in.Items
// names. A failure to read comes apart.
func (s *Service) kitChecks(ctx context.Context, tenant shared.TenantID, in KitInput) (error, map[shared.ItemID]ItemFacts, error) {
	valuesErr, err := s.checkValues(ctx, tenant, in.AttributeValueIDs)
	if err != nil {
		return nil, nil, err
	}
	facts, err := s.kitFacts(ctx, tenant, in.Items)
	if err != nil {
		return nil, nil, err
	}
	return valuesErr, facts, nil
}

// kitFacts reads the facts of every member and alternative of items, costs
// included for a staff scope: the admin kit views use it.
func (s *Service) kitFacts(ctx context.Context, tenant shared.TenantID, items []catalog.KitItem) (map[shared.ItemID]ItemFacts, error) {
	return readFacts(ctx, tenant, items, s.items.ByIDs)
}

// publicKitFacts is kitFacts for the store page: it reads Items.PublicFacts,
// which never touches a staff-only table, so no scope can pull a cost into
// the store page's data (ADR 0024).
func (s *Service) publicKitFacts(ctx context.Context, tenant shared.TenantID, items []catalog.KitItem) (map[shared.ItemID]ItemFacts, error) {
	return readFacts(ctx, tenant, items, s.items.PublicFacts)
}

// readFacts asks read for the facts of every member and alternative of
// items, each item once.
func readFacts(
	ctx context.Context, tenant shared.TenantID, items []catalog.KitItem,
	read func(context.Context, shared.TenantID, []shared.ItemID) ([]ItemFacts, error),
) (map[shared.ItemID]ItemFacts, error) {
	seen := map[shared.ItemID]bool{}
	var ids []shared.ItemID
	for _, ki := range items {
		for _, id := range append([]shared.ItemID{ki.Item}, ki.Alternatives...) {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	out := make(map[shared.ItemID]ItemFacts, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	list, err := read(ctx, tenant, ids)
	if err != nil {
		return nil, fmt.Errorf("read kit items: %w", err)
	}
	for _, f := range list {
		out[f.ID] = f
	}
	return out, nil
}

// refusalOf turns the domain's archived-items error into a RefusalError
// naming the items; other errors come back unchanged.
func refusalOf(err error, facts map[shared.ItemID]ItemFacts) error {
	var ae *catalog.ArchivedItemsError
	if errors.As(err, &ae) {
		return archivedRefusal(ae, facts)
	}
	return err
}

func kitDomainInput(in KitInput, currency string) (catalog.KitInput, error) {
	price, err := shared.NewMoney(in.PriceCents, currency)
	if err != nil {
		return catalog.KitInput{}, err
	}
	return catalog.KitInput{
		Name: in.Name, Description: in.Description, Price: price, SwapMode: in.SwapDiscount,
		AssemblyDays: in.AssemblyDays, SafetyMarginDays: in.SafetyMarginDays, Items: in.Items,
		AttributeValues: in.AttributeValueIDs,
	}, nil
}

// kitResult builds the caller's view of the kit.
func (s *Service) kitResult(ctx context.Context, by shared.Principal, st kitState, d Defaults) (KitView, error) {
	photos, err := s.photoViews(ctx, st.kit.Photos())
	if err != nil {
		return KitView{}, err
	}
	return kitView(st.kit, st.version, st.facts, d, photos, identity.SeesCosts(by.Role()))
}
