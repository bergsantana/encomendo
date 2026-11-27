package postgres

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Kits implements appcatalog.Kits on kits, kit_items,
// kit_item_alternatives and kit_attribute_values. Kit rows hold nothing
// staff-only, so every method works in any scope of the store.
type Kits struct{}

// NewKits returns the kits repository.
func NewKits() *Kits { return &Kits{} }

// Insert stores a new kit with its items, alternatives and field values.
// Photos are never written here. Errors are those of appcatalog.Kits.
func (*Kits) Insert(ctx context.Context, k *catalog.Kit) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	rows := models.KitFromDomain(k, time.Now().UTC())
	if err := db.Create(&rows.Kit).Error; err != nil {
		return wrap("insert kit", err)
	}
	return writeKitChildren(db, rows)
}

// Update stores the kit while the stored version is still k.Version(), and
// the next version, and replaces its items, alternatives and field values.
// Photos are untouched. Errors are those of appcatalog.Kits.
func (*Kits) Update(ctx context.Context, k *catalog.Kit) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	rows := models.KitFromDomain(k, time.Now().UTC())
	m := rows.Kit
	res := db.Model(&models.Kit{}).
		Where("tenant_id = ? AND id = ? AND version = ?", m.TenantID, m.ID, k.Version()).
		Updates(map[string]any{
			"name": m.Name, "description": m.Description, "price_cents": m.PriceCents,
			"swap_discount": m.SwapDiscount, "assembly_days": m.AssemblyDays, "safety_margin_days": m.SafetyMarginDays,
			"published": m.Published,
			// A kit that stays archived keeps the date it was archived.
			"archived_at": gorm.Expr("CASE WHEN ? THEN COALESCE(archived_at, now()) END", k.Archived()),
			"version":     k.Version() + 1, "updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return wrap("update kit", res.Error)
	}
	if res.RowsAffected == 0 {
		return missingOrStale(db, "kits", k.TenantID(), m.ID, catalog.ErrKitNotFound)
	}
	// Deleting the items cascades to their alternatives.
	for _, table := range []any{&models.KitItem{}, &models.KitAttributeValue{}} {
		if err := db.Where("tenant_id = ? AND kit_id = ?", m.TenantID, m.ID).Delete(table).Error; err != nil {
			return wrap("replace the rows of a kit", err)
		}
	}
	return writeKitChildren(db, rows)
}

// writeKitChildren inserts the items before their alternatives, which
// reference them.
func writeKitChildren(db *gorm.DB, rows models.KitRows) error {
	if len(rows.Items) > 0 {
		if err := db.Create(&rows.Items).Error; err != nil {
			return wrap("insert kit items", err)
		}
	}
	if len(rows.Alternatives) > 0 {
		if err := db.Create(&rows.Alternatives).Error; err != nil {
			return wrap("insert kit alternatives", err)
		}
	}
	if len(rows.Values) > 0 {
		values := make([]models.KitAttributeValue, 0, len(rows.Values))
		for _, v := range rows.Values {
			values = append(values, models.KitAttributeValue{TenantID: rows.Kit.TenantID, KitID: rows.Kit.ID, ValueID: v})
		}
		if err := db.Create(&values).Error; err != nil {
			return wrap("insert kit attribute values", err)
		}
	}
	return nil
}

// ByID returns the kit with its photos, or catalog.ErrKitNotFound.
func (*Kits) ByID(ctx context.Context, tenant shared.TenantID, id shared.KitID) (*catalog.Kit, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var m models.Kit
	err = db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, catalog.ErrKitNotFound
	}
	if err != nil {
		return nil, wrap("read kit", err)
	}
	kits, err := loadKits(db, tenant, []models.Kit{m})
	if err != nil {
		return nil, err
	}
	return kits[0], nil
}

// List returns one page of the store's kits by lower(name), then id.
func (*Kits) List(ctx context.Context, tenant shared.TenantID, filter appcatalog.KitFilter, page shared.PageRequest) (shared.Page[*catalog.Kit], error) {
	db, err := conn(ctx)
	if err != nil {
		return shared.Page[*catalog.Kit]{}, err
	}
	rows, next, err := namePageRows(db, namePage{
		table: "kits", tenant: tenant, req: page,
		narrow: func(q *gorm.DB) *gorm.DB {
			if filter.Query != "" {
				q = q.Where("name ILIKE ?", containsPattern(filter.Query))
			}
			if !filter.IncludeArchived {
				q = q.Where("archived_at IS NULL")
			}
			return q
		},
	}, func(m models.Kit) string { return m.ID })
	if err != nil {
		return shared.Page[*catalog.Kit]{}, err
	}
	kits, err := loadKits(db, tenant, rows)
	if err != nil {
		return shared.Page[*catalog.Kit]{}, err
	}
	return shared.Page[*catalog.Kit]{Items: kits, NextCursor: next}, nil
}

// Published returns the published kits that are not archived, by
// lower(name) then id.
func (*Kits) Published(ctx context.Context, tenant shared.TenantID) ([]*catalog.Kit, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var rows []models.Kit
	err = db.Where("tenant_id = ? AND published AND archived_at IS NULL", tenant.String()).
		Order("lower(name), id").Find(&rows).Error
	if err != nil {
		return nil, wrap("read published kits", err)
	}
	return loadKits(db, tenant, rows)
}

// loadKits reads the children of the kit rows, a query per table for the
// whole page, and rebuilds the kits in the order of rows.
func loadKits(db *gorm.DB, tenant shared.TenantID, rows []models.Kit) ([]*catalog.Kit, error) {
	if len(rows) == 0 {
		return []*catalog.Kit{}, nil
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	t := tenant.String()
	var (
		items []models.KitItem
		alts  []models.KitAlternative
	)
	if err := db.Where("tenant_id = ? AND kit_id IN ?", t, ids).Order("position, item_id").Find(&items).Error; err != nil {
		return nil, wrap("read kit items", err)
	}
	if err := db.Where("tenant_id = ? AND kit_id IN ?", t, ids).Order("alternative_item_id").Find(&alts).Error; err != nil {
		return nil, wrap("read kit alternatives", err)
	}
	values, err := valuesOf(db, "kit_attribute_values", "kit_id", t, ids)
	if err != nil {
		return nil, err
	}
	photos, err := photosOfOwners(db, "kit_id", t, ids)
	if err != nil {
		return nil, err
	}
	rowsOf := make(map[string]*models.KitRows, len(rows))
	for _, r := range rows {
		rowsOf[r.ID] = &models.KitRows{Kit: r, Values: values[r.ID], Photos: photos[r.ID]}
	}
	for _, i := range items {
		rowsOf[i.KitID].Items = append(rowsOf[i.KitID].Items, i)
	}
	for _, a := range alts {
		rowsOf[a.KitID].Alternatives = append(rowsOf[a.KitID].Alternatives, a)
	}
	out := make([]*catalog.Kit, 0, len(rows))
	for _, r := range rows {
		k, err := rowsOf[r.ID].ToDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}
