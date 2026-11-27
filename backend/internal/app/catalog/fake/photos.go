package fake

import (
	"context"
	"fmt"
	"slices"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Photos is the appcatalog.Photos of a Store. The photos live in the item
// and kit records, so Items.ByID and Kits.ByID return them, as the Postgres
// repositories read the photos table with their rows. tenant_isolation is
// emulated: any scope of the store sees and writes its photos.
type Photos struct{ *Store }

var _ appcatalog.Photos = (*Photos)(nil)

// ownerPhotos returns the owner's photos and whether the owner is a row of
// tenant that scope sees. Call it with s.mu held.
func (s *Store) ownerPhotos(scope appshared.Scope, tenant shared.TenantID, owner catalog.PhotoOwner) ([]catalog.Photo, bool) {
	if !sees(scope, tenant) {
		return nil, false
	}
	switch owner.Kind {
	case catalog.OwnerItem:
		if row, ok := s.items[shared.ItemID(owner.ID)]; ok && row.rec.Tenant == tenant {
			return row.rec.Photos, true
		}
	case catalog.OwnerKit:
		if r, ok := s.kits[shared.KitID(owner.ID)]; ok && r.Tenant == tenant {
			return r.Photos, true
		}
	}
	return nil, false
}

// setOwnerPhotos stores the owner's photos, undone if the transaction
// fails. The owner's version does not change. Call it with s.mu held, for
// an owner ownerPhotos found.
func (s *Store) setOwnerPhotos(ctx context.Context, owner catalog.PhotoOwner, photos []catalog.Photo) {
	photos = slices.Clone(photos)
	if owner.Kind == catalog.OwnerKit {
		id := shared.KitID(owner.ID)
		sharedfake.KeepOnRollback(ctx, &s.mu, s.kits, id)
		r := s.kits[id]
		r.Photos = photos
		s.kits[id] = r
		return
	}
	id := shared.ItemID(owner.ID)
	sharedfake.KeepOnRollback(ctx, &s.mu, s.items, id)
	row := s.items[id]
	row.rec.Photos = photos
	s.items[id] = row
}

// find returns the photo with id or key, of any store and owner. Call it
// with s.mu held.
func (s *Store) find(match func(catalog.Photo) bool) (shared.TenantID, catalog.PhotoOwner, int, catalog.Photo, bool) {
	for id, row := range s.items {
		if i := slices.IndexFunc(row.rec.Photos, match); i >= 0 {
			return row.rec.Tenant, catalog.ItemPhotos(id), i, row.rec.Photos[i], true
		}
	}
	for id, r := range s.kits {
		if i := slices.IndexFunc(r.Photos, match); i >= 0 {
			return r.Tenant, catalog.KitPhotos(id), i, r.Photos[i], true
		}
	}
	return shared.TenantID{}, catalog.PhotoOwner{}, 0, catalog.Photo{}, false
}

// ByOwner implements appcatalog.Photos ("PhotosByOwner").
func (s *Photos) ByOwner(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner) ([]catalog.Photo, error) {
	scope, err := s.enter(ctx, "PhotosByOwner")
	if err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	photos, _ := s.ownerPhotos(scope, tenant, owner)
	return slices.Clone(photos), nil
}

// ByID implements appcatalog.Photos ("PhotosByID").
func (s *Photos) ByID(ctx context.Context, tenant shared.TenantID, id shared.PhotoID) (appcatalog.StoredPhoto, error) {
	scope, err := s.enter(ctx, "PhotosByID")
	if err != nil {
		return appcatalog.StoredPhoto{}, err
	}
	defer s.mu.Unlock()
	owned, owner, pos, p, ok := s.find(func(p catalog.Photo) bool { return p.ID() == id })
	if !ok || owned != tenant || !sees(scope, tenant) {
		return appcatalog.StoredPhoto{}, catalog.ErrPhotoNotFound
	}
	return appcatalog.StoredPhoto{Owner: owner, Photo: p, Position: pos}, nil
}

// Insert implements appcatalog.Photos ("PhotosInsert"), in the order the
// port gives: a taken id or key, the foreign key and the key check, then
// the limit.
func (s *Photos) Insert(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner, p catalog.Photo) (int, error) {
	scope, err := s.enter(ctx, "PhotosInsert")
	if err != nil {
		return 0, err
	}
	defer s.mu.Unlock()
	if !sees(scope, tenant) {
		return 0, ErrRowSecurity
	}
	if _, _, _, _, taken := s.find(func(q catalog.Photo) bool { return q.ID() == p.ID() || q.Key() == p.Key() }); taken {
		return 0, fmt.Errorf("photos_pkey: %w", appcatalog.ErrPhotoTaken)
	}
	photos, ok := s.ownerPhotos(scope, tenant, owner)
	if !ok {
		return 0, fmt.Errorf("photos owner: %w", errForeignKey)
	}
	if p.Key() != catalog.PhotoKey(tenant, owner, p.ID()) {
		return 0, fmt.Errorf("photos object_key check: %w", shared.ErrInvalid)
	}
	list, err := catalog.NewPhotoList(photos)
	if err != nil {
		return 0, err
	}
	if err := list.AddPhoto(p); err != nil {
		return 0, err
	}
	s.setOwnerPhotos(ctx, owner, list.Photos())
	return len(photos), nil
}

// Reorder implements appcatalog.Photos ("PhotosReorder").
func (s *Photos) Reorder(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner, ids []shared.PhotoID) error {
	scope, err := s.enter(ctx, "PhotosReorder")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	photos, _ := s.ownerPhotos(scope, tenant, owner)
	list, err := catalog.NewPhotoList(photos)
	if err != nil {
		return err
	}
	if err := list.ReorderPhotos(ids); err != nil {
		return err
	}
	if len(photos) > 0 {
		s.setOwnerPhotos(ctx, owner, list.Photos())
	}
	return nil
}

// Delete implements appcatalog.Photos ("PhotosDelete").
func (s *Photos) Delete(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner, id shared.PhotoID) error {
	scope, err := s.enter(ctx, "PhotosDelete")
	if err != nil {
		return err
	}
	defer s.mu.Unlock()
	photos, _ := s.ownerPhotos(scope, tenant, owner)
	list, err := catalog.NewPhotoList(photos)
	if err != nil {
		return err
	}
	if err := list.RemovePhoto(id); err != nil {
		return err
	}
	s.setOwnerPhotos(ctx, owner, list.Photos())
	return nil
}
