package catalog

import (
	"context"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// UseCases is the driving port of the management apps: what the catalog
// handlers call.
type UseCases interface {
	ListCategories(ctx context.Context) ([]CategoryView, error)
	CreateCategory(ctx context.Context, in CreateCategoryInput) (CategoryView, error)
	UpdateCategory(ctx context.Context, in UpdateCategoryInput) (CategoryView, error)
	DeleteCategory(ctx context.Context, id shared.CategoryID) error

	ListAttributes(ctx context.Context) ([]AttributeView, error)
	CreateAttribute(ctx context.Context, in AttributeInput) (AttributeView, error)
	ReplaceAttribute(ctx context.Context, in ReplaceAttributeInput) (AttributeView, error)
	DeleteAttribute(ctx context.Context, id shared.AttributeID) error

	ListItems(ctx context.Context, filter ItemFilter, page shared.PageRequest) (shared.Page[ItemSummaryView], error)
	GetItem(ctx context.Context, id shared.ItemID) (ItemView, error)
	CreateItem(ctx context.Context, in ItemInput) (ItemView, error)
	ReplaceItem(ctx context.Context, in ReplaceItemInput) (ItemView, error)
	SetItemPublished(ctx context.Context, in SetItemPublishedInput) (ItemView, error)
	ArchiveItem(ctx context.Context, in ItemVersionInput) (ItemView, error)
	RestoreItem(ctx context.Context, in ItemVersionInput) (ItemView, error)

	ListKits(ctx context.Context, filter KitFilter, page shared.PageRequest) (shared.Page[KitSummaryView], error)
	GetKit(ctx context.Context, id shared.KitID) (KitView, error)
	CreateKit(ctx context.Context, in KitInput) (KitView, error)
	ReplaceKit(ctx context.Context, in ReplaceKitInput) (KitView, error)
	SetKitPublished(ctx context.Context, in SetKitPublishedInput) (KitView, error)
	ArchiveKit(ctx context.Context, in KitVersionInput) (KitView, error)
	RestoreKit(ctx context.Context, in KitVersionInput) (KitView, error)
}

// StoreQueries is the driving port of the store page (milestone 7): the
// published catalog, cached, with no internal field.
type StoreQueries interface {
	PublicCatalog(ctx context.Context) (StoreCatalog, error)
	PublicKit(ctx context.Context, id shared.KitID) (StoreKit, error)
}

// The repository ports. Every catalog table is tenant-scoped under RLS
// (0008_catalog, 0009_suppliers); supplier_items and item_costs are
// staff-only (ADR 0024): a scope with a tenant and no user, as the store
// page has, reads none of their rows and writes none. A method sees only the
// rows of the tenant in the transaction's scope; with another tenant's id it
// finds nothing and writes nothing (a write is refused). The methods take
// the tenant explicitly too, so the SQL filters on it as well. Every method
// runs inside TxManager.WithinTx and returns appshared.ErrNoTransaction
// outside it. Other failures pass through unchanged.

// Categories stores the store's categories.
type Categories interface {
	// List returns every category of the store, by position, then name,
	// then id.
	List(ctx context.Context, tenant shared.TenantID) ([]*catalog.Category, error)
	// ByID returns the category, or catalog.ErrCategoryNotFound.
	ByID(ctx context.Context, tenant shared.TenantID, id shared.CategoryID) (*catalog.Category, error)
	// Insert stores a new category. It returns catalog.ErrNameTaken when
	// another category of the store has the name, ignoring case.
	Insert(ctx context.Context, c *catalog.Category) error
	// Update stores c's name and position while the stored version is still
	// c.Version(), and stores the next version. It returns
	// shared.ErrVersionConflict for a stale version,
	// catalog.ErrCategoryNotFound when it is gone, and catalog.ErrNameTaken.
	Update(ctx context.Context, c *catalog.Category) error
	// Delete removes the category. It returns catalog.ErrCategoryNotFound,
	// and catalog.ErrCategoryInUse while an item, archived or not, has it.
	Delete(ctx context.Context, tenant shared.TenantID, id shared.CategoryID) error
}

// PlacedAttribute is an item field with its place in the store's order.
type PlacedAttribute struct {
	Attribute *catalog.Attribute
	Position  int
}

// Attributes stores the store's item fields and their values.
type Attributes interface {
	// List returns every field of the store, by position, then id.
	List(ctx context.Context, tenant shared.TenantID) ([]PlacedAttribute, error)
	// ByID returns the field, or catalog.ErrAttributeNotFound.
	ByID(ctx context.Context, tenant shared.TenantID, id shared.AttributeID) (PlacedAttribute, error)
	// Insert stores a new field at position. It returns catalog.ErrNameTaken
	// when another field of the store has the name, ignoring case.
	Insert(ctx context.Context, a *catalog.Attribute, position int) error
	// Update stores a's name and values while the stored version is still
	// a.Version(), and stores the next version. The values in removed (what
	// catalog.Attribute.Replace returned) leave the field and every item and
	// kit that picked them; those items and kits keep their versions. It
	// returns shared.ErrVersionConflict, catalog.ErrAttributeNotFound and
	// catalog.ErrNameTaken.
	Update(ctx context.Context, a *catalog.Attribute, removed []shared.AttributeValueID) error
	// Delete removes the field; its values leave every item and kit. It
	// returns catalog.ErrAttributeNotFound.
	Delete(ctx context.Context, tenant shared.TenantID, id shared.AttributeID) error
	// UsageCounts returns, for each value of the store's fields that any
	// item or kit picked (archived ones included), how many of them did.
	// Values no one picked are absent.
	UsageCounts(ctx context.Context, tenant shared.TenantID) (map[shared.AttributeValueID]int, error)
}

// ItemFacts is what a kit needs to know about one item, with the item's id
// and name to show it and to name it in a refusal.
type ItemFacts struct {
	ID    shared.ItemID
	Name  string
	Facts catalog.ItemFacts
}

// PublishedItem is a published item that is not archived, as any scope of
// its store may read it: no supplier and no cost. ReadyDays is the stored
// items.ready_days, so no staff-only table is read (ADR 0025).
type PublishedItem struct {
	ID              shared.ItemID
	Name            string
	Description     string
	Category        shared.CategoryID
	Price           shared.Money
	ReadyDays       int
	AttributeValues []shared.AttributeValueID
	Options         []catalog.CustomizationOption
}

// ItemFilter narrows the item list (GET /api/v1/admin/catalog/items). The
// zero value lists every item that is not archived.
type ItemFilter struct {
	// Query keeps the items whose name contains it, ignoring case. It is
	// literal text: "%" and "_" match only themselves.
	Query string
	// Category keeps the items of this category; zero keeps all.
	Category shared.CategoryID
	// IncludeArchived lists archived items too ("Mostrar arquivados").
	IncludeArchived bool
}

// Items stores items with their sourcing: Insert and Update write the item,
// its supplier links (supplier_items) with their costs, its in-house cost
// (item_costs) and items.ready_days (Item.ReadyDays) in the same call, so
// the stored ready days always match the sourcing. They never touch photos,
// which are their own resource (step 3.9).
//
// ByID and List rebuild whole items, supplier links and costs included, so
// they need a staff scope (a tenant and a user). In a scope with no user
// they find nothing (catalog.ErrItemNotFound, an empty page) rather than
// fail; such a scope reads items through ByIDs and Published, which never
// read a staff-only table.
type Items interface {
	// Insert stores a new item at its version. It returns an error matching
	// shared.ErrConflict when an item, of any store, already has the id.
	Insert(ctx context.Context, it *catalog.Item) error
	// Update stores it while the stored version is still it.Version(), and
	// stores the next version. It returns shared.ErrVersionConflict for a
	// stale version and catalog.ErrItemNotFound when it is gone.
	Update(ctx context.Context, it *catalog.Item) error
	// ByID returns the item, or catalog.ErrItemNotFound.
	ByID(ctx context.Context, tenant shared.TenantID, id shared.ItemID) (*catalog.Item, error)
	// ByIDs returns the facts of the items with these ids that exist,
	// archived ones included, in any order; a missing id is simply absent.
	// In a scope with no user every Cost is nil, and nothing fails.
	ByIDs(ctx context.Context, tenant shared.TenantID, ids []shared.ItemID) ([]ItemFacts, error)
	// List returns one page of the store's items matching filter, ordered
	// by name and then id. The cursor is opaque and holds the id of the last
	// row of the previous page. A cursor whose id does not parse, or names
	// no item the scope sees, is a ValidationError on "cursor". The last
	// page has no NextCursor.
	List(ctx context.Context, tenant shared.TenantID, filter ItemFilter, page shared.PageRequest) (shared.Page[*catalog.Item], error)
	// ActiveKitsHolding returns the kits that are not archived and hold the
	// item as a member or as an alternative, by name then id, each once.
	ActiveKitsHolding(ctx context.Context, tenant shared.TenantID, id shared.ItemID) ([]Reference, error)
	// Published returns the store's published items that are not archived,
	// by name then id. It works in any scope of the store.
	Published(ctx context.Context, tenant shared.TenantID) ([]PublishedItem, error)
}

// KitFilter narrows the kit list (GET /api/v1/admin/catalog/kits). The zero
// value lists every kit that is not archived.
type KitFilter struct {
	// Query keeps the kits whose name contains it, ignoring case; literal.
	Query string
	// IncludeArchived lists archived kits too.
	IncludeArchived bool
}

// Kits stores kits with their items and alternatives. Kit rows hold no
// internal data, so every method works in any scope of the store. Insert
// and Update never touch photos.
type Kits interface {
	// Insert stores a new kit at its version. It returns an error matching
	// shared.ErrConflict when a kit, of any store, already has the id.
	Insert(ctx context.Context, k *catalog.Kit) error
	// Update stores k while the stored version is still k.Version(), and
	// stores the next version. It returns shared.ErrVersionConflict and
	// catalog.ErrKitNotFound.
	Update(ctx context.Context, k *catalog.Kit) error
	// ByID returns the kit, or catalog.ErrKitNotFound.
	ByID(ctx context.Context, tenant shared.TenantID, id shared.KitID) (*catalog.Kit, error)
	// List returns one page of the store's kits matching filter, ordered by
	// name and then id, with cursors as Items.List.
	List(ctx context.Context, tenant shared.TenantID, filter KitFilter, page shared.PageRequest) (shared.Page[*catalog.Kit], error)
	// Published returns the store's published kits that are not archived,
	// by name then id.
	Published(ctx context.Context, tenant shared.TenantID) ([]*catalog.Kit, error)
}

// SupplierDirectory reads the store's suppliers (app/supplier, through
// adapter/inproc; ADR 0025). The dependency runs one way: catalog reads
// suppliers, suppliers know nothing of the catalog.
type SupplierDirectory interface {
	// Names returns the name of each supplier in ids that the caller's
	// store has. An unknown id, or another store's, is absent, not an
	// error. It needs a caller with catalog:read.
	Names(ctx context.Context, ids []shared.SupplierID) (map[shared.SupplierID]string, error)
}

// Defaults is what the catalog takes from the store's settings.
type Defaults struct {
	// Currency is the currency of every price and cost of the store.
	Currency string
	// AssemblyDays and SafetyMarginDays are the kit days used when a kit
	// has no value of its own.
	AssemblyDays     int
	SafetyMarginDays int
	// StockEnabled and ProductionEnabled turn on the stock and in_house
	// sourcing modes; a mode that is off is refused on a write.
	StockEnabled      bool
	ProductionEnabled bool
}

// StoreDefaults reads the defaults of the store in the request's scope
// (app/tenancy, through adapter/inproc). It needs only a tenant in the
// scope, so the store page can call it, and returns an error matching
// shared.ErrForbidden (authz.ErrTenantRequired) without one.
type StoreDefaults interface {
	StoreDefaults(ctx context.Context) (Defaults, error)
}

// PhotoURLs signs read URLs for photos in admin views (adapter/storage, step
// 3.9).
type PhotoURLs interface {
	// SignGet returns a URL that reads the object at key until expiresAt,
	// ttl from now.
	SignGet(ctx context.Context, key string, ttl time.Duration) (url string, expiresAt time.Time, err error)
}
