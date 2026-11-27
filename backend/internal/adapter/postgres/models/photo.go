package models

import (
	"errors"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// errStoredPhoto means a photos row breaks a rule the table should hold. It
// matches no sentinel, so it is a 500.
var errStoredPhoto = errors.New("stored photo breaks a rule")

// Photo maps photos, for the photo repository, which writes them, and for
// Items.ByID and Kits.ByID, which read them with their owner. Exactly one
// of ItemID and KitID is set (0008_catalog). created_at and updated_at are
// left to their defaults.
type Photo struct {
	TenantID    string  `gorm:"column:tenant_id"`
	ID          string  `gorm:"column:id;primaryKey"`
	ItemID      *string `gorm:"column:item_id"`
	KitID       *string `gorm:"column:kit_id"`
	ObjectKey   string  `gorm:"column:object_key"`
	ContentType string  `gorm:"column:content_type"`
	SizeBytes   int64   `gorm:"column:size_bytes"`
	Position    int     `gorm:"column:position"`
}

// TableName returns photos.
func (Photo) TableName() string { return "photos" }

// PhotoFromDomain returns the row of p, a photo of owner in tenant at
// position.
func PhotoFromDomain(tenant shared.TenantID, owner catalog.PhotoOwner, p catalog.Photo, position int) Photo {
	m := Photo{
		TenantID: tenant.String(), ID: p.ID().String(), ObjectKey: p.Key(),
		ContentType: p.ContentType(), SizeBytes: p.Size(), Position: position,
	}
	id := owner.ID.String()
	if owner.Kind == catalog.OwnerKit {
		m.KitID = &id
	} else {
		m.ItemID = &id
	}
	return m
}

// ToDomain rebuilds the photo and its owner.
func (m Photo) ToDomain() (catalog.Photo, catalog.PhotoOwner, error) {
	id, err := parseID[shared.PhotoID](m.ID)
	if err != nil {
		return catalog.Photo{}, catalog.PhotoOwner{}, err
	}
	var owner catalog.PhotoOwner
	switch {
	case m.ItemID != nil:
		owner.Kind = catalog.OwnerItem
		owner.ID, err = parseID[shared.ID](*m.ItemID)
	case m.KitID != nil:
		owner.Kind = catalog.OwnerKit
		owner.ID, err = parseID[shared.ID](*m.KitID)
	default:
		err = fmt.Errorf("photo %s has no owner", m.ID)
	}
	if err != nil {
		return catalog.Photo{}, catalog.PhotoOwner{}, err
	}
	p, err := catalog.NewPhoto(id, m.ObjectKey, m.ContentType, m.SizeBytes)
	if err != nil {
		// Not wrapped: a stored row that breaks a rule is a bug (500), not
		// the caller's invalid photo (400).
		return catalog.Photo{}, catalog.PhotoOwner{}, fmt.Errorf("%w: %s: %s", errStoredPhoto, m.ID, err.Error())
	}
	return p, owner, nil
}
