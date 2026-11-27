package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Items implements appcatalog.Items on items, supplier_items, item_costs,
// item_attribute_values and customization_options. Reads that rebuild whole
// items and every write need a staff scope (a tenant and a user), because
// supplier_items and item_costs are staff-only; see appcatalog.Items.
type Items struct{}

// NewItems returns the items repository.
func NewItems() *Items { return &Items{} }

// Insert stores a new item with its supplier links, in-house cost, field
// values, options and ready days, in the caller's transaction. Photos are
// never written here. Errors are those of appcatalog.Items.
func (*Items) Insert(ctx context.Context, it *catalog.Item) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	if !hasStaff(ctx) {
		return fmt.Errorf("insert item %v: %w", it.ID(), errStaffScope)
	}
	rows, err := models.ItemFromDomain(it, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := db.Create(&rows.Item).Error; err != nil {
		return wrap("insert item", err)
	}
	return writeItemChildren(db, rows)
}

// Update stores the item while the stored version is still it.Version(),
// and the next version, and replaces its children. Photos are untouched.
// Errors are those of appcatalog.Items.
func (*Items) Update(ctx context.Context, it *catalog.Item) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	if !hasStaff(ctx) {
		return fmt.Errorf("update item %v: %w", it.ID(), errStaffScope)
	}
	rows, err := models.ItemFromDomain(it, time.Now().UTC())
	if err != nil {
		return err
	}
	m := rows.Item
	res := db.Model(&models.Item{}).
		Where("tenant_id = ? AND id = ? AND version = ?", m.TenantID, m.ID, it.Version()).
		Updates(map[string]any{
			"name": m.Name, "description": m.Description, "category_id": m.CategoryID,
			"sourcing_mode": m.SourcingMode, "price_cents": m.PriceCents, "production_days": m.ProductionDays,
			"ready_days": m.ReadyDays, "published": m.Published,
			// An item that stays archived keeps the date it was archived.
			"archived_at": gorm.Expr("CASE WHEN ? THEN COALESCE(archived_at, now()) END", it.Archived()),
			"version":     it.Version() + 1, "updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return wrap("update item", res.Error)
	}
	if res.RowsAffected == 0 {
		return missingOrStale(db, "items", it.TenantID(), m.ID, catalog.ErrItemNotFound)
	}
	if err := deleteItemChildren(db, m.TenantID, m.ID); err != nil {
		return err
	}
	return writeItemChildren(db, rows)
}

// deleteItemChildren drops the rows an item's save replaces. The tables
// grant DELETE for this (0008_catalog, 0009_suppliers).
func deleteItemChildren(db *gorm.DB, tenant, item string) error {
	for _, table := range []any{&models.SupplierItem{}, &models.ItemCost{}, &models.ItemAttributeValue{}, &models.Option{}} {
		if err := db.Where("tenant_id = ? AND item_id = ?", tenant, item).Delete(table).Error; err != nil {
			return wrap(fmt.Sprintf("replace the rows of %T", table), err)
		}
	}
	return nil
}

// writeItemChildren inserts the rows that hang on an item. The supplier
// links go first, so a supplier of another store fails on its own foreign
// key.
func writeItemChildren(db *gorm.DB, rows models.ItemRows) error {
	if len(rows.Links) > 0 {
		if err := db.Create(&rows.Links).Error; err != nil {
			return wrap("insert supplier items", err)
		}
	}
	if rows.Cost != nil {
		if err := db.Create(rows.Cost).Error; err != nil {
			return wrap("insert item cost", err)
		}
	}
	if len(rows.Values) > 0 {
		values := make([]models.ItemAttributeValue, 0, len(rows.Values))
		for _, v := range rows.Values {
			values = append(values, models.ItemAttributeValue{TenantID: rows.Item.TenantID, ItemID: rows.Item.ID, ValueID: v})
		}
		if err := db.Create(&values).Error; err != nil {
			return wrap("insert item attribute values", err)
		}
	}
	if len(rows.Options) > 0 {
		if err := db.Create(&rows.Options).Error; err != nil {
			return wrap("insert customization options", err)
		}
	}
	return nil
}

// ByID returns the whole item, or catalog.ErrItemNotFound. In a scope with
// no user it finds nothing.
func (*Items) ByID(ctx context.Context, tenant shared.TenantID, id shared.ItemID) (*catalog.Item, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	if !hasStaff(ctx) {
		return nil, catalog.ErrItemNotFound
	}
	var m models.Item
	err = db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, catalog.ErrItemNotFound
	}
	if err != nil {
		return nil, wrap("read item", err)
	}
	items, err := loadItems(db, tenant, []models.Item{m})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

// loadItems reads the children of the item rows, a query per table for the
// whole page, and rebuilds the items in the order of rows. Call it in a
// staff scope only.
func loadItems(db *gorm.DB, tenant shared.TenantID, rows []models.Item) ([]*catalog.Item, error) {
	if len(rows) == 0 {
		return []*catalog.Item{}, nil
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	t := tenant.String()
	var (
		links   []models.SupplierItem
		costs   []models.ItemCost
		options []models.Option
	)
	if err := db.Where("tenant_id = ? AND item_id IN ?", t, ids).Order("role").Find(&links).Error; err != nil {
		return nil, wrap("read supplier items", err)
	}
	if err := db.Where("tenant_id = ? AND item_id IN ?", t, ids).Find(&costs).Error; err != nil {
		return nil, wrap("read item costs", err)
	}
	if err := db.Where("tenant_id = ? AND item_id IN ?", t, ids).Order("position, id").Find(&options).Error; err != nil {
		return nil, wrap("read customization options", err)
	}
	values, err := valuesOf(db, "item_attribute_values", "item_id", t, ids)
	if err != nil {
		return nil, err
	}
	photos, err := photosOfOwners(db, "item_id", t, ids)
	if err != nil {
		return nil, err
	}
	rowsOf := make(map[string]*models.ItemRows, len(rows))
	for _, r := range rows {
		rowsOf[r.ID] = &models.ItemRows{Item: r, Values: values[r.ID], Photos: photos[r.ID]}
	}
	for _, l := range links {
		rowsOf[l.ItemID].Links = append(rowsOf[l.ItemID].Links, l)
	}
	for _, c := range costs {
		rowsOf[c.ItemID].Cost = &c
	}
	for _, o := range options {
		rowsOf[o.ItemID].Options = append(rowsOf[o.ItemID].Options, o)
	}
	out := make([]*catalog.Item, 0, len(rows))
	for _, r := range rows {
		it, err := rowsOf[r.ID].ToDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, nil
}

// valueRow is a pick of a field value by an item or a kit.
type valueRow struct {
	Owner   string
	ValueID string
}

// valuesOf returns, per owner, the field values it picked, in the store's
// field order (field position, then value position): the join tables keep
// no order of their own. table is item_attribute_values or
// kit_attribute_values and ownerColumn its owner column; both are constants
// of the callers.
func valuesOf(db *gorm.DB, table, ownerColumn, tenant string, owners []string) (map[string][]string, error) {
	var rows []valueRow
	err := db.Table(table+" AS p").
		Select("p."+ownerColumn+"::text AS owner, p.value_id::text AS value_id").
		Joins("JOIN attribute_values v ON v.tenant_id = p.tenant_id AND v.id = p.value_id").
		Joins("JOIN attributes a ON a.tenant_id = v.tenant_id AND a.id = v.attribute_id").
		Where("p.tenant_id = ? AND p."+ownerColumn+" IN ?", tenant, owners).
		Order("a.position, a.id, v.position, v.id").Scan(&rows).Error
	if err != nil {
		return nil, wrap("read "+table, err)
	}
	out := map[string][]string{}
	for _, r := range rows {
		out[r.Owner] = append(out[r.Owner], r.ValueID)
	}
	return out, nil
}

// photosOfOwners returns the photos of the items or kits (ownerColumn is
// item_id or kit_id) by position. Writing photos is step 3.9's resource.
func photosOfOwners(db *gorm.DB, ownerColumn, tenant string, owners []string) (map[string][]models.PhotoRead, error) {
	var rows []models.PhotoRead
	err := db.Where("tenant_id = ? AND "+ownerColumn+" IN ?", tenant, owners).Order("position, id").Find(&rows).Error
	if err != nil {
		return nil, wrap("read photos", err)
	}
	out := map[string][]models.PhotoRead{}
	for _, p := range rows {
		owner := p.ItemID
		if ownerColumn == "kit_id" {
			owner = p.KitID
		}
		if owner != nil {
			out[*owner] = append(out[*owner], p)
		}
	}
	return out, nil
}

// factsRow is one item with the cost columns of its main supplier and of
// item_costs; the cost columns are null when the scope cannot read them.
type factsRow struct {
	ID           string
	Name         string
	Published    bool
	PriceCents   int64
	Currency     string
	ReadyDays    int
	Archived     bool
	SourcingMode string
	MainCents    *int64
	MainCurrency *string
	OwnCents     *int64
	OwnCurrency  *string
}

// ByIDs returns the facts of the items that exist, archived ones included.
// The cost comes from the staff-only tables, so it is nil in a scope with no
// user; nothing fails there.
func (*Items) ByIDs(ctx context.Context, tenant shared.TenantID, ids []shared.ItemID) ([]appcatalog.ItemFacts, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []appcatalog.ItemFacts{}, nil
	}
	strs := make([]string, 0, len(ids))
	for _, id := range ids {
		strs = append(strs, id.String())
	}
	var rows []factsRow
	err = db.Table("items AS i").
		Select(`i.id::text AS id, i.name, i.published, i.price_cents, i.currency, i.ready_days,
			i.archived_at IS NOT NULL AS archived, i.sourcing_mode,
			m.cost_cents AS main_cents, m.currency AS main_currency,
			c.cost_cents AS own_cents, c.currency AS own_currency`).
		Joins("LEFT JOIN supplier_items m ON m.tenant_id = i.tenant_id AND m.item_id = i.id AND m.role = 'main'").
		Joins("LEFT JOIN item_costs c ON c.tenant_id = i.tenant_id AND c.item_id = i.id").
		Where("i.tenant_id = ? AND i.id IN ?", tenant.String(), strs).Scan(&rows).Error
	if err != nil {
		return nil, wrap("read item facts", err)
	}
	out := make([]appcatalog.ItemFacts, 0, len(rows))
	for _, r := range rows {
		f, err := r.toFacts()
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func (r factsRow) toFacts() (appcatalog.ItemFacts, error) {
	id, err := shared.ParseID(r.ID)
	if err != nil {
		return appcatalog.ItemFacts{}, fmt.Errorf("stored id: %w", err)
	}
	price, err := shared.NewMoney(r.PriceCents, r.Currency)
	if err != nil {
		return appcatalog.ItemFacts{}, err
	}
	cents, currency := r.MainCents, r.MainCurrency
	if r.SourcingMode == string(catalog.ModeInHouse) {
		cents, currency = r.OwnCents, r.OwnCurrency
	}
	facts := catalog.ItemFacts{Price: price, ReadyDays: r.ReadyDays, Archived: r.Archived}
	if cents != nil && currency != nil {
		cost, err := shared.NewMoney(*cents, *currency)
		if err != nil {
			return appcatalog.ItemFacts{}, err
		}
		facts.Cost = &cost
	}
	return appcatalog.ItemFacts{ID: shared.ItemID(id), Name: r.Name, Published: r.Published, Facts: facts}, nil
}

// List returns one page of the store's items by lower(name), then id. In a
// scope with no user it is empty.
func (*Items) List(ctx context.Context, tenant shared.TenantID, filter appcatalog.ItemFilter, page shared.PageRequest) (shared.Page[*catalog.Item], error) {
	db, err := conn(ctx)
	if err != nil {
		return shared.Page[*catalog.Item]{}, err
	}
	if !hasStaff(ctx) {
		return shared.Page[*catalog.Item]{Items: []*catalog.Item{}}, nil
	}
	rows, next, err := namePageRows(db, namePage{
		table: "items", tenant: tenant, req: page,
		narrow: func(q *gorm.DB) *gorm.DB { return narrowItems(q, filter) },
	}, func(m models.Item) string { return m.ID })
	if err != nil {
		return shared.Page[*catalog.Item]{}, err
	}
	items, err := loadItems(db, tenant, rows)
	if err != nil {
		return shared.Page[*catalog.Item]{}, err
	}
	return shared.Page[*catalog.Item]{Items: items, NextCursor: next}, nil
}

func narrowItems(q *gorm.DB, f appcatalog.ItemFilter) *gorm.DB {
	if f.Query != "" {
		q = q.Where("name ILIKE ?", containsPattern(f.Query))
	}
	if !f.Category.IsZero() {
		q = q.Where("category_id = ?", f.Category.String())
	}
	if !f.IncludeArchived {
		q = q.Where("archived_at IS NULL")
	}
	return q
}

// ActiveKitsHolding returns the kits that are not archived and hold the item
// as a member or as an alternative, by lower(name) then id, each once
// (ADR 0027).
func (*Items) ActiveKitsHolding(ctx context.Context, tenant shared.TenantID, id shared.ItemID) ([]appcatalog.Reference, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var rows []models.Kit
	err = db.Where("tenant_id = ? AND archived_at IS NULL", tenant.String()).
		Where(`(EXISTS (SELECT 1 FROM kit_items ki WHERE ki.tenant_id = kits.tenant_id AND ki.kit_id = kits.id AND ki.item_id = ?)
			OR EXISTS (SELECT 1 FROM kit_item_alternatives ka WHERE ka.tenant_id = kits.tenant_id AND ka.kit_id = kits.id AND ka.alternative_item_id = ?))`,
			id.String(), id.String()).
		Order("lower(name), id").Find(&rows).Error
	if err != nil {
		return nil, wrap("read kits holding an item", err)
	}
	out := make([]appcatalog.Reference, 0, len(rows))
	for _, k := range rows {
		kid, err := shared.ParseID(k.ID)
		if err != nil {
			return nil, fmt.Errorf("stored id: %w", err)
		}
		out = append(out, appcatalog.Reference{ID: kid, Name: k.Name})
	}
	return out, nil
}

// Published returns the published items that are not archived, by
// lower(name) then id, in any scope of the store. It reads only items,
// item_attribute_values and customization_options, never a staff-only table
// (ADR 0025).
func (*Items) Published(ctx context.Context, tenant shared.TenantID) ([]appcatalog.PublishedItem, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var rows []models.Item
	err = db.Where("tenant_id = ? AND published AND archived_at IS NULL", tenant.String()).
		Order("lower(name), id").Find(&rows).Error
	if err != nil {
		return nil, wrap("read published items", err)
	}
	if len(rows) == 0 {
		return []appcatalog.PublishedItem{}, nil
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	var options []models.Option
	if err := db.Where("tenant_id = ? AND item_id IN ?", tenant.String(), ids).Order("position, id").Find(&options).Error; err != nil {
		return nil, wrap("read customization options", err)
	}
	values, err := valuesOf(db, "item_attribute_values", "item_id", tenant.String(), ids)
	if err != nil {
		return nil, err
	}
	optionsOf := map[string][]models.Option{}
	for _, o := range options {
		optionsOf[o.ItemID] = append(optionsOf[o.ItemID], o)
	}
	out := make([]appcatalog.PublishedItem, 0, len(rows))
	for _, r := range rows {
		parts, err := models.PublishedItemRows{Item: r, Values: values[r.ID], Options: optionsOf[r.ID]}.ToDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, appcatalog.PublishedItem{
			ID: parts.ID, Name: r.Name, Description: r.Description, Category: parts.Category, Price: parts.Price,
			ReadyDays: r.ReadyDays, AttributeValues: parts.Values, Options: parts.Options,
		})
	}
	return out, nil
}
