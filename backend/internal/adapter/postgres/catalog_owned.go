package postgres

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
)

// ownerKind says whose rows are read: the field values and photos of items
// or of kits. It picks table and column names from constants, so no name
// from outside reaches a query, and GORM quotes the identifiers.
type ownerKind int

const (
	itemOwners ownerKind = iota
	kitOwners
)

// valueTable is the table of the field values the owners picked.
func (k ownerKind) valueTable() string {
	if k == kitOwners {
		return "kit_attribute_values"
	}
	return "item_attribute_values"
}

// column is the column of valueTable and photos that names the owner.
func (k ownerKind) column() string {
	if k == kitOwners {
		return "kit_id"
	}
	return "item_id"
}

// valueRow is a pick of a field value by an item or a kit.
type valueRow struct {
	Owner   string
	ValueID string
}

// valuesOf returns, per owner, the field values it picked, in the store's
// field order (the field's position, then the value's position): the join
// tables keep no order of their own, so the order is the store's, not the
// order the values were sent in (see appcatalog.Items and Kits).
func valuesOf(db *gorm.DB, kind ownerKind, tenant string, owners []string) (map[string][]string, error) {
	owner := clause.Column{Table: "p", Name: kind.column()}
	var rows []valueRow
	err := db.Table("? AS p", clause.Table{Name: kind.valueTable()}).
		Select("?::text AS owner, p.value_id::text AS value_id", owner).
		Joins("JOIN attribute_values v ON v.tenant_id = p.tenant_id AND v.id = p.value_id").
		Joins("JOIN attributes a ON a.tenant_id = v.tenant_id AND a.id = v.attribute_id").
		Where("p.tenant_id = ? AND ? IN ?", tenant, owner, owners).
		Order("a.position, a.id, v.position, v.id").Scan(&rows).Error
	if err != nil {
		return nil, wrap("read "+kind.valueTable(), err)
	}
	out := map[string][]string{}
	for _, r := range rows {
		out[r.Owner] = append(out[r.Owner], r.ValueID)
	}
	return out, nil
}

// photosOfOwners returns the photos of the items or kits by position.
// Writing photos is the photo repository's job (step 3.9).
func photosOfOwners(db *gorm.DB, kind ownerKind, tenant string, owners []string) (map[string][]models.Photo, error) {
	var rows []models.Photo
	err := db.Where("tenant_id = ? AND ? IN ?", tenant, clause.Column{Name: kind.column()}, owners).
		Order("position, id").Find(&rows).Error
	if err != nil {
		return nil, wrap("read photos", err)
	}
	out := map[string][]models.Photo{}
	for _, p := range rows {
		owner := p.ItemID
		if kind == kitOwners {
			owner = p.KitID
		}
		if owner != nil {
			out[*owner] = append(out[*owner], p)
		}
	}
	return out, nil
}
