package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Index and constraint names of 0008_catalog that carry a domain meaning.
const (
	categoryNameIndex  = "categories_tenant_name_key"
	attributeNameIndex = "attributes_tenant_name_key"
)

// wrapName is wrap for a write that may break the unique name index of a
// table: that violation is catalog.ErrNameTaken, any other as wrap maps it.
func wrapName(doing, index string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == index {
		return fmt.Errorf("%s: %w: %w", doing, catalog.ErrNameTaken, err)
	}
	return wrap(doing, err)
}

// isForeignKey reports whether err is a foreign key violation.
func isForeignKey(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// Categories implements appcatalog.Categories on categories (tenant data:
// the store page lists them too).
type Categories struct{}

// NewCategories returns the categories repository.
func NewCategories() *Categories { return &Categories{} }

// List returns every category of the store, by position, lower(name), id.
func (*Categories) List(ctx context.Context, tenant shared.TenantID) ([]*catalog.Category, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var rows []models.Category
	err = db.Where("tenant_id = ?", tenant.String()).Order("position, lower(name), id").Find(&rows).Error
	if err != nil {
		return nil, wrap("list categories", err)
	}
	out := make([]*catalog.Category, 0, len(rows))
	for _, m := range rows {
		c, err := m.ToDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// ByID returns the category, or catalog.ErrCategoryNotFound.
func (*Categories) ByID(ctx context.Context, tenant shared.TenantID, id shared.CategoryID) (*catalog.Category, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var m models.Category
	err = db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, catalog.ErrCategoryNotFound
	}
	if err != nil {
		return nil, wrap("read category", err)
	}
	return m.ToDomain()
}

// Insert stores a new category. A taken name is catalog.ErrNameTaken; a
// taken id, of any store, an error matching shared.ErrConflict.
func (*Categories) Insert(ctx context.Context, c *catalog.Category) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.CategoryFromDomain(c)
	return wrapName("insert category", categoryNameIndex, db.Create(&m).Error)
}

// Update stores the name and position while the stored version is still
// c.Version(), and the next version. See appcatalog.Categories for errors.
func (*Categories) Update(ctx context.Context, c *catalog.Category) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.CategoryFromDomain(c)
	res := db.Model(&models.Category{}).
		Where("tenant_id = ? AND id = ? AND version = ?", m.TenantID, m.ID, c.Version()).
		Updates(map[string]any{
			"name": m.Name, "position": m.Position, "version": c.Version() + 1, "updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return wrapName("update category", categoryNameIndex, res.Error)
	}
	if res.RowsAffected > 0 {
		return nil
	}
	return missingOrStale(db, "categories", c.TenantID(), m.ID, catalog.ErrCategoryNotFound)
}

// Delete removes the category. It returns catalog.ErrCategoryNotFound, and
// catalog.ErrCategoryInUse while an item, archived or not, has it (the
// foreign key of items, 0008_catalog).
func (*Categories) Delete(ctx context.Context, tenant shared.TenantID, id shared.CategoryID) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	res := db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Delete(&models.Category{})
	if isForeignKey(res.Error) {
		return fmt.Errorf("delete category: %w: %w", catalog.ErrCategoryInUse, res.Error)
	}
	ok, err := affected(res, "delete category")
	if err != nil {
		return err
	}
	if !ok {
		return catalog.ErrCategoryNotFound
	}
	return nil
}

// Attributes implements appcatalog.Attributes on attributes and
// attribute_values.
type Attributes struct{}

// NewAttributes returns the attributes repository.
func NewAttributes() *Attributes { return &Attributes{} }

// List returns every field of the store with its values, by position, id.
func (*Attributes) List(ctx context.Context, tenant shared.TenantID) ([]appcatalog.PlacedAttribute, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var attrs []models.Attribute
	if err := db.Where("tenant_id = ?", tenant.String()).Order("position, id").Find(&attrs).Error; err != nil {
		return nil, wrap("list attributes", err)
	}
	var values []models.AttributeValue
	if err := db.Where("tenant_id = ?", tenant.String()).Order("position, id").Find(&values).Error; err != nil {
		return nil, wrap("list attribute values", err)
	}
	byAttr := map[string][]models.AttributeValue{}
	for _, v := range values {
		byAttr[v.AttributeID] = append(byAttr[v.AttributeID], v)
	}
	out := make([]appcatalog.PlacedAttribute, 0, len(attrs))
	for _, a := range attrs {
		p, err := placedOf(models.AttributeRows{Attribute: a, Values: byAttr[a.ID]})
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func placedOf(rows models.AttributeRows) (appcatalog.PlacedAttribute, error) {
	a, err := rows.ToDomain()
	if err != nil {
		return appcatalog.PlacedAttribute{}, err
	}
	return appcatalog.PlacedAttribute{Attribute: a, Position: rows.Attribute.Position}, nil
}

// ByID returns the field with its values, or catalog.ErrAttributeNotFound.
func (*Attributes) ByID(ctx context.Context, tenant shared.TenantID, id shared.AttributeID) (appcatalog.PlacedAttribute, error) {
	db, err := conn(ctx)
	if err != nil {
		return appcatalog.PlacedAttribute{}, err
	}
	var a models.Attribute
	err = db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Take(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return appcatalog.PlacedAttribute{}, catalog.ErrAttributeNotFound
	}
	if err != nil {
		return appcatalog.PlacedAttribute{}, wrap("read attribute", err)
	}
	var values []models.AttributeValue
	err = db.Where("tenant_id = ? AND attribute_id = ?", a.TenantID, a.ID).Order("position, id").Find(&values).Error
	if err != nil {
		return appcatalog.PlacedAttribute{}, wrap("read attribute values", err)
	}
	return placedOf(models.AttributeRows{Attribute: a, Values: values})
}

// Insert stores a new field at position with its values. A taken name is
// catalog.ErrNameTaken; a taken field or value id, of any store, an error
// matching shared.ErrConflict.
func (*Attributes) Insert(ctx context.Context, a *catalog.Attribute, position int) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	rows := models.AttributeFromDomain(a, position)
	if err := db.Create(&rows.Attribute).Error; err != nil {
		return wrapName("insert attribute", attributeNameIndex, err)
	}
	if len(rows.Values) == 0 {
		return nil
	}
	return wrap("insert attribute values", db.Create(&rows.Values).Error)
}

// Update stores the name and values while the stored version is still
// a.Version(), and the next version. The values in removed leave the field,
// and the foreign keys of item_attribute_values and kit_attribute_values
// cascade, so every item and kit that picked them loses them with no change
// of its own version (ADR 0025). Kept values keep their ids and take the new
// label and position; new ones are inserted.
func (*Attributes) Update(ctx context.Context, a *catalog.Attribute, removed []shared.AttributeValueID) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	rows := models.AttributeFromDomain(a, 0)
	res := db.Model(&models.Attribute{}).
		Where("tenant_id = ? AND id = ? AND version = ?", rows.Attribute.TenantID, rows.Attribute.ID, a.Version()).
		Updates(map[string]any{"name": rows.Attribute.Name, "version": a.Version() + 1, "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return wrapName("update attribute", attributeNameIndex, res.Error)
	}
	if res.RowsAffected == 0 {
		return missingOrStale(db, "attributes", a.TenantID(), rows.Attribute.ID, catalog.ErrAttributeNotFound)
	}
	return saveValues(db, rows, removed)
}

// saveValues deletes the removed values of the field, then updates the
// values the field already has and inserts the new ones.
func saveValues(db *gorm.DB, rows models.AttributeRows, removed []shared.AttributeValueID) error {
	if len(removed) > 0 {
		ids := make([]string, 0, len(removed))
		for _, id := range removed {
			ids = append(ids, id.String())
		}
		err := db.Where("tenant_id = ? AND attribute_id = ? AND id IN ?", rows.Attribute.TenantID, rows.Attribute.ID, ids).
			Delete(&models.AttributeValue{}).Error
		if err != nil {
			return wrap("remove attribute values", err)
		}
	}
	var existing []string
	err := db.Model(&models.AttributeValue{}).
		Where("tenant_id = ? AND attribute_id = ?", rows.Attribute.TenantID, rows.Attribute.ID).
		Pluck("id", &existing).Error
	if err != nil {
		return wrap("read attribute values", err)
	}
	have := make(map[string]bool, len(existing))
	for _, id := range existing {
		have[id] = true
	}
	var fresh []models.AttributeValue
	for _, v := range rows.Values {
		if !have[v.ID] {
			fresh = append(fresh, v)
			continue
		}
		err := db.Model(&models.AttributeValue{}).
			Where("tenant_id = ? AND attribute_id = ? AND id = ?", v.TenantID, v.AttributeID, v.ID).
			Updates(map[string]any{"label": v.Label, "position": v.Position, "updated_at": gorm.Expr("now()")}).Error
		if err != nil {
			return wrap("update attribute value", err)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	return wrap("insert attribute values", db.Create(&fresh).Error)
}

// Delete removes the field; its values, and their use by items and kits,
// go with it through the foreign keys. It returns
// catalog.ErrAttributeNotFound.
func (*Attributes) Delete(ctx context.Context, tenant shared.TenantID, id shared.AttributeID) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	res := db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Delete(&models.Attribute{})
	ok, err := affected(res, "delete attribute")
	if err != nil {
		return err
	}
	if !ok {
		return catalog.ErrAttributeNotFound
	}
	return nil
}

// usageRow is one line of UsageCounts.
type usageRow struct {
	ValueID string
	Uses    int
}

// UsageCounts returns how many items and kits, archived ones included,
// picked each value that anyone picked.
func (*Attributes) UsageCounts(ctx context.Context, tenant shared.TenantID) (map[shared.AttributeValueID]int, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var rows []usageRow
	err = db.Raw(`SELECT value_id::text AS value_id, count(*) AS uses FROM (
			SELECT value_id FROM item_attribute_values WHERE tenant_id = ?
			UNION ALL
			SELECT value_id FROM kit_attribute_values WHERE tenant_id = ?
		) picked GROUP BY value_id`, tenant.String(), tenant.String()).Scan(&rows).Error
	if err != nil {
		return nil, wrap("count attribute value usage", err)
	}
	out := make(map[shared.AttributeValueID]int, len(rows))
	for _, r := range rows {
		id, err := shared.ParseID(r.ValueID)
		if err != nil {
			return nil, fmt.Errorf("stored value id: %w", err)
		}
		out[shared.AttributeValueID(id)] = r.Uses
	}
	return out, nil
}
