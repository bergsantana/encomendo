package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Photos implements appcatalog.Photos on the photos table. Its policy is
// tenant_isolation (0008_catalog), so any scope of the store reads and
// writes its photos, and every query filters on the tenant besides.
//
// Insert, Reorder and Delete first take a transaction-level advisory lock
// on the owner, so the count behind the 8-photo limit and the positions
// stay right under concurrent requests. A lock on the items or kits row
// would do too, but would also wait on an unrelated edit of the item.
type Photos struct{}

// NewPhotos returns the photos repository.
func NewPhotos() *Photos { return &Photos{} }

var _ appcatalog.Photos = (*Photos)(nil)

// photoOwnerKind is the ownerKind of a photo's owner (catalog_owned.go).
func photoOwnerKind(owner catalog.PhotoOwner) (ownerKind, error) {
	if err := owner.Validate(); err != nil {
		return 0, err
	}
	if owner.Kind == catalog.OwnerKit {
		return kitOwners, nil
	}
	return itemOwners, nil
}

// ownerWhere is the filter on owner's photos in tenant. The column comes
// from ownerKind's constants, never from input.
func ownerWhere(tenant shared.TenantID, owner catalog.PhotoOwner) (map[string]any, error) {
	kind, err := photoOwnerKind(owner)
	if err != nil {
		return nil, err
	}
	return map[string]any{"tenant_id": tenant.String(), kind.column(): owner.ID.String()}, nil
}

// lockOwner serializes the photo writes of one owner until the transaction
// ends. The key is a hash of the owner's id; a collision only makes two
// owners wait on each other.
func lockOwner(db *gorm.DB, owner catalog.PhotoOwner) error {
	err := db.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "photos:"+owner.ID.String()).Error
	return wrap("lock the owner's photos", err)
}

// ByOwner implements appcatalog.Photos.
func (*Photos) ByOwner(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner) ([]catalog.Photo, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	return ownerPhotos(db, tenant, owner)
}

func ownerPhotos(db *gorm.DB, tenant shared.TenantID, owner catalog.PhotoOwner) ([]catalog.Photo, error) {
	where, err := ownerWhere(tenant, owner)
	if err != nil {
		return nil, err
	}
	var rows []models.Photo
	if err := db.Where(where).Order("position, id").Find(&rows).Error; err != nil {
		return nil, wrap("read photos", err)
	}
	out := make([]catalog.Photo, 0, len(rows))
	for _, r := range rows {
		p, _, err := r.ToDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// ByID implements appcatalog.Photos.
func (*Photos) ByID(ctx context.Context, tenant shared.TenantID, id shared.PhotoID) (appcatalog.StoredPhoto, error) {
	db, err := conn(ctx)
	if err != nil {
		return appcatalog.StoredPhoto{}, err
	}
	var m models.Photo
	err = db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return appcatalog.StoredPhoto{}, catalog.ErrPhotoNotFound
	}
	if err != nil {
		return appcatalog.StoredPhoto{}, wrap("read photo", err)
	}
	p, owner, err := m.ToDomain()
	if err != nil {
		return appcatalog.StoredPhoto{}, err
	}
	return appcatalog.StoredPhoto{Owner: owner, Photo: p, Position: m.Position}, nil
}

// Insert implements appcatalog.Photos. The checks run in the port's order:
// the key, a photo of the store with the id or key, the limit, and then
// the insert, whose ON CONFLICT DO NOTHING also catches an id another
// store holds, which RLS hides from the first check.
func (*Photos) Insert(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner, p catalog.Photo) (int, error) {
	db, err := conn(ctx)
	if err != nil {
		return 0, err
	}
	if err := owner.Validate(); err != nil {
		return 0, err
	}
	// The table's check would refuse it too, as a 500; this is the port's
	// ErrInvalid.
	if p.Key() != catalog.PhotoKey(tenant, owner, p.ID()) {
		return 0, fmt.Errorf("photo key %q is not its owner's: %w", p.Key(), shared.ErrInvalid)
	}
	if err := lockOwner(db, owner); err != nil {
		return 0, err
	}
	var taken int64
	err = db.Model(&models.Photo{}).Where("tenant_id = ? AND (id = ? OR object_key = ?)", tenant.String(), p.ID().String(), p.Key()).
		Count(&taken).Error
	if err != nil {
		return 0, wrap("look for the photo", err)
	}
	if taken > 0 {
		return 0, appcatalog.ErrPhotoTaken
	}
	return insertLast(db, tenant, owner, p)
}

// insertLast inserts p after the owner's photos; the domain's list refuses
// a 9th.
func insertLast(db *gorm.DB, tenant shared.TenantID, owner catalog.PhotoOwner, p catalog.Photo) (int, error) {
	photos, err := ownerPhotos(db, tenant, owner)
	if err != nil {
		return 0, err
	}
	list, err := catalog.NewPhotoList(photos)
	if err != nil {
		return 0, err
	}
	if err := list.AddPhoto(p); err != nil {
		return 0, err
	}
	position := len(photos)
	m := models.PhotoFromDomain(tenant, owner, p, position)
	res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&m)
	if res.Error != nil {
		return 0, wrap("insert photo", res.Error)
	}
	if res.RowsAffected == 0 {
		return 0, appcatalog.ErrPhotoTaken
	}
	return position, nil
}

// Reorder implements appcatalog.Photos.
func (*Photos) Reorder(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner, ids []shared.PhotoID) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	if err := lockOwner(db, owner); err != nil {
		return err
	}
	photos, err := ownerPhotos(db, tenant, owner)
	if err != nil {
		return err
	}
	list, err := catalog.NewPhotoList(photos)
	if err != nil {
		return err
	}
	if err := list.ReorderPhotos(ids); err != nil {
		return err
	}
	where, err := ownerWhere(tenant, owner)
	if err != nil {
		return err
	}
	for i, id := range ids {
		// The owner too, not just the id: the update can only ever move
		// this owner's photos, whatever ids reach it.
		err := db.Model(&models.Photo{}).Where(where).Where("id = ?", id.String()).
			Updates(map[string]any{"position": i, "updated_at": gorm.Expr("now()")}).Error
		if err != nil {
			return wrap("move photo", err)
		}
	}
	return nil
}

// Delete implements appcatalog.Photos.
func (*Photos) Delete(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner, id shared.PhotoID) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	where, err := ownerWhere(tenant, owner)
	if err != nil {
		return err
	}
	if err := lockOwner(db, owner); err != nil {
		return err
	}
	var m models.Photo
	err = db.Where(where).Where("id = ?", id.String()).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return catalog.ErrPhotoNotFound
	}
	if err != nil {
		return wrap("read photo", err)
	}
	if err := db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Delete(&models.Photo{}).Error; err != nil {
		return wrap("delete photo", err)
	}
	err = db.Model(&models.Photo{}).Where(where).Where("position > ?", m.Position).
		Updates(map[string]any{"position": gorm.Expr("position - 1"), "updated_at": gorm.Expr("now()")}).Error
	return wrap("close the gap", err)
}
