package catalog

import (
	"context"
	"fmt"
	"slices"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// storeCatalogTTL is how long the store page catalog stays cached. Writes
// drop it at once; the TTL only bounds what a missed invalidation (a cache
// down during the write) can leave behind (backend/CLAUDE.md, Redis).
const storeCatalogTTL = 5 * time.Minute

// The store page types. They are their own types, not admin views with
// fields left empty, and none of them has a supplier, cost, margin or
// lead-time-by-supplier field, at any depth (docs/product/design-document.md,
// System rules; ADR 0025). They go through the cache as JSON, hence the
// tags. Photos join them in milestone 7, with the public read path ADR 0024
// plans; admin photo URLs are signed per caller and never cached.

// StoreCatalog is everything the store page shows: the store's categories
// and item fields, and its published items and kits that are not archived.
type StoreCatalog struct {
	Categories []StoreCategory  `json:"categories"`
	Attributes []StoreAttribute `json:"attributes"`
	Items      []StoreItem      `json:"items"`
	Kits       []StoreKit       `json:"kits"`
}

// StoreCategory is a category, for the store page's filter.
type StoreCategory struct {
	ID       shared.CategoryID `json:"id"`
	Name     string            `json:"name"`
	Position int               `json:"position"`
}

// StoreAttribute is an item field and its values, for the store page's
// filter ("Serve em").
type StoreAttribute struct {
	ID       shared.AttributeID    `json:"id"`
	Name     string                `json:"name"`
	Position int                   `json:"position"`
	Values   []StoreAttributeValue `json:"values"`
}

// StoreAttributeValue is one value of an item field.
type StoreAttributeValue struct {
	ID    shared.AttributeValueID `json:"id"`
	Label string                  `json:"label"`
}

// StoreChoice is one choice of a customization option.
type StoreChoice struct {
	ID         shared.ChoiceID `json:"id"`
	Label      string          `json:"label"`
	ExtraPrice shared.Money    `json:"extraPrice"`
	ExtraDays  int             `json:"extraDays"`
}

// StoreOption is a customization option the client picks from.
type StoreOption struct {
	ID       shared.OptionID `json:"id"`
	Name     string          `json:"name"`
	Required bool            `json:"required"`
	Choices  []StoreChoice   `json:"choices"`
}

// StoreItem is a published item. ReadyDays is the days it needs before it is
// ready, whatever its sourcing, so the client learns no lead time of a
// supplier. CategoryID is the zero id ("") for "Sem categoria".
type StoreItem struct {
	ID                shared.ItemID             `json:"id"`
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	CategoryID        shared.CategoryID         `json:"categoryId"`
	Price             shared.Money              `json:"price"`
	ReadyDays         int                       `json:"readyDays"`
	AttributeValueIDs []shared.AttributeValueID `json:"attributeValueIds"`
	Options           []StoreOption             `json:"options"`
}

// StoreKitAlternative is an item the client may take in place of a kit
// item, at its single-item price.
type StoreKitAlternative struct {
	ItemID    shared.ItemID `json:"itemId"`
	ItemName  string        `json:"itemName"`
	UnitPrice shared.Money  `json:"unitPrice"`
	ReadyDays int           `json:"readyDays"`
}

// StoreKitItem is one item of a published kit. Alternatives lists only items
// that are not archived: a client never swaps to one.
type StoreKitItem struct {
	ItemID       shared.ItemID         `json:"itemId"`
	ItemName     string                `json:"itemName"`
	Quantity     int                   `json:"quantity"`
	UnitPrice    shared.Money          `json:"unitPrice"`
	ReadyDays    int                   `json:"readyDays"`
	Swappable    bool                  `json:"swappable"`
	Alternatives []StoreKitAlternative `json:"alternatives"`
}

// StoreKit is a published kit. TotalDays is its "Prazo calculado" (slowest
// item + assembly + safety margin), given as a total only: how much of it
// is the store's safety margin is the store's business.
type StoreKit struct {
	ID                shared.KitID              `json:"id"`
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	Price             shared.Money              `json:"price"`
	SingleItemsTotal  shared.Money              `json:"singleItemsTotal"`
	Discount          shared.Money              `json:"discount"`
	SwapDiscount      catalog.SwapMode          `json:"swapDiscount"`
	TotalDays         int                       `json:"totalDays"`
	Items             []StoreKitItem            `json:"items"`
	AttributeValueIDs []shared.AttributeValueID `json:"attributeValueIds"`
}

// PublicCatalog returns the store page catalog of the store in the scope:
// categories, fields, and the published items and kits that are not
// archived. It needs only a tenant in the scope, with no principal (the
// store page middleware sets it from the subdomain), and returns
// authz.ErrTenantRequired without one. It is cached under
// t:{tenant}:catalog:store for 5 minutes; a cache that fails is a miss, and
// the catalog is read from the repositories.
func (s *Service) PublicCatalog(ctx context.Context) (StoreCatalog, error) {
	tenant := appshared.ScopeFrom(ctx).TenantID
	if tenant.IsZero() {
		return StoreCatalog{}, authz.ErrTenantRequired
	}
	key, err := catalogKey(tenant, "store")
	if err != nil {
		return StoreCatalog{}, err
	}
	var cached StoreCatalog
	// A cache error is a miss: the adapter logs it and the catalog is read
	// from Postgres (backend/CLAUDE.md, Redis).
	if hit, err := s.cache.Get(ctx, key, &cached); err == nil && hit {
		return cached, nil
	}
	out, err := s.readStoreCatalog(ctx, tenant)
	if err != nil {
		return StoreCatalog{}, err
	}
	// A failed Set only costs the next request a read; it never fails this
	// one.
	_ = s.cache.Set(ctx, key, out, storeCatalogTTL)
	return out, nil
}

// PublicKit returns one kit of the store page catalog, through the same
// cache. It returns catalog.ErrKitNotFound for a kit that is not published,
// archived, or another store's, and authz.ErrTenantRequired without a
// tenant in the scope.
func (s *Service) PublicKit(ctx context.Context, id shared.KitID) (StoreKit, error) {
	c, err := s.PublicCatalog(ctx)
	if err != nil {
		return StoreKit{}, err
	}
	i := slices.IndexFunc(c.Kits, func(k StoreKit) bool { return k.ID == id })
	if i < 0 {
		return StoreKit{}, fmt.Errorf("store kit %s: %w", id, catalog.ErrKitNotFound)
	}
	return c.Kits[i], nil
}

// storeRows is what one read of the store page catalog loads.
type storeRows struct {
	categories []*catalog.Category
	attributes []PlacedAttribute
	items      []PublishedItem
	kits       []*catalog.Kit
	facts      map[shared.ItemID]ItemFacts
}

// readStoreCatalog loads the catalog in one transaction, through methods
// that never read a staff-only table, and builds the store page types.
func (s *Service) readStoreCatalog(ctx context.Context, tenant shared.TenantID) (StoreCatalog, error) {
	d, err := s.storeDefaults(ctx)
	if err != nil {
		return StoreCatalog{}, err
	}
	var rows storeRows
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if rows.categories, err = s.categories.List(ctx, tenant); err != nil {
			return fmt.Errorf("list categories: %w", err)
		}
		if rows.attributes, err = s.attributes.List(ctx, tenant); err != nil {
			return fmt.Errorf("list attributes: %w", err)
		}
		if rows.items, err = s.items.Published(ctx, tenant); err != nil {
			return fmt.Errorf("list published items: %w", err)
		}
		if rows.kits, err = s.kits.Published(ctx, tenant); err != nil {
			return fmt.Errorf("list published kits: %w", err)
		}
		var members []catalog.KitItem
		for _, k := range rows.kits {
			members = append(members, k.Items()...)
		}
		rows.facts, err = s.kitFacts(ctx, tenant, members)
		return err
	})
	if err != nil {
		return StoreCatalog{}, err
	}
	return storeCatalogOf(rows, d)
}

func storeCatalogOf(rows storeRows, d Defaults) (StoreCatalog, error) {
	out := StoreCatalog{
		Categories: make([]StoreCategory, 0, len(rows.categories)),
		Attributes: make([]StoreAttribute, 0, len(rows.attributes)),
		Items:      make([]StoreItem, 0, len(rows.items)),
		Kits:       make([]StoreKit, 0, len(rows.kits)),
	}
	for _, c := range rows.categories {
		out.Categories = append(out.Categories, StoreCategory{ID: c.ID(), Name: c.Name(), Position: c.Position()})
	}
	for _, p := range rows.attributes {
		a := StoreAttribute{ID: p.Attribute.ID(), Name: p.Attribute.Name(), Position: p.Position, Values: []StoreAttributeValue{}}
		for _, v := range p.Attribute.Values() {
			a.Values = append(a.Values, StoreAttributeValue{ID: v.ID(), Label: v.Label()})
		}
		out.Attributes = append(out.Attributes, a)
	}
	for _, it := range rows.items {
		out.Items = append(out.Items, storeItem(it))
	}
	for _, k := range rows.kits {
		sk, err := storeKit(k, rows.facts, d)
		if err != nil {
			return StoreCatalog{}, err
		}
		out.Kits = append(out.Kits, sk)
	}
	return out, nil
}

func storeItem(it PublishedItem) StoreItem {
	values := it.AttributeValues
	if values == nil {
		values = []shared.AttributeValueID{}
	}
	options := make([]StoreOption, 0, len(it.Options))
	for _, o := range optionViews(it.Options) {
		so := StoreOption{ID: o.ID, Name: o.Name, Required: o.Required, Choices: make([]StoreChoice, 0, len(o.Choices))}
		for _, c := range o.Choices {
			so.Choices = append(so.Choices, StoreChoice(c))
		}
		options = append(options, so)
	}
	return StoreItem{
		ID: it.ID, Name: it.Name, Description: it.Description, CategoryID: it.Category, Price: it.Price,
		ReadyDays: it.ReadyDays, AttributeValueIDs: values, Options: options,
	}
}

// storeKit builds the store page kit. Its totals never use a cost: the
// facts may carry one in a staff scope, and nothing here reads it.
func storeKit(k *catalog.Kit, facts map[shared.ItemID]ItemFacts, d Defaults) (StoreKit, error) {
	t, err := totalsOf(k, lookupOf(facts), d, false)
	if err != nil {
		return StoreKit{}, err
	}
	values := k.AttributeValueIDs()
	if values == nil {
		values = []shared.AttributeValueID{}
	}
	out := StoreKit{
		ID: k.ID(), Name: k.Name(), Description: k.Description(), Price: k.Price(),
		SingleItemsTotal: t.single, Discount: t.discount, SwapDiscount: k.SwapMode(), TotalDays: t.days.Total,
		Items: make([]StoreKitItem, 0, len(k.Items())), AttributeValueIDs: values,
	}
	for _, ki := range k.Items() {
		f := facts[ki.Item]
		item := StoreKitItem{
			ItemID: ki.Item, ItemName: f.Name, Quantity: ki.Quantity, UnitPrice: f.Facts.Price,
			ReadyDays: f.Facts.ReadyDays, Swappable: ki.Swappable, Alternatives: []StoreKitAlternative{},
		}
		for _, alt := range ki.Alternatives {
			af := facts[alt]
			if af.Facts.Archived {
				continue
			}
			item.Alternatives = append(item.Alternatives, StoreKitAlternative{
				ItemID: alt, ItemName: af.Name, UnitPrice: af.Facts.Price, ReadyDays: af.Facts.ReadyDays,
			})
		}
		// A kit item whose alternatives were all archived cannot be swapped.
		item.Swappable = item.Swappable && len(item.Alternatives) > 0
		out.Items = append(out.Items, item)
	}
	return out, nil
}
