package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// uploadURLTTL is how long a signed upload URL works (plan 3.9): enough to
// send 10 MB over a slow phone line, short enough that a leaked URL dies.
const uploadURLTTL = 5 * time.Minute

const (
	fieldContentType = "contentType"
	// The wording of docs/product/design-document.md, "Catálogo".
	msgPhotoType = "Use uma foto JPEG, PNG ou WebP."
)

// PhotoUploadInput asks for a signed upload of one photo of an item or kit
// (the PhotoUploadRequest schema).
type PhotoUploadInput struct {
	Owner       catalog.PhotoOwner
	ContentType string
}

// PhotoUploadView is where and how to upload one photo (the PhotoUpload
// schema). Key goes back in ConfirmPhoto; Method, URL and Headers are the
// signed request; ExpiresAt is when the URL stops working.
type PhotoUploadView struct {
	Key       string
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// PhotoConfirmation is the photo ConfirmPhoto recorded. AlreadyRecorded is
// true when the key had been confirmed on this owner before and nothing
// changed: the API answers 200 then, and 201 for a new photo.
type PhotoConfirmation struct {
	Photo           PhotoView
	AlreadyRecorded bool
}

// RequestPhotoUpload signs an upload of one photo for in.Owner: a key the
// server picks, t/{store}/{items|kits}/{owner}/{new photo id}, and a PUT URL
// valid for 5 minutes with the content type bound (ADR 0024, ADR 0025). It
// needs catalog:write. It returns ErrInvalidPhotoOwner, a ValidationError
// on "contentType" for a type other than JPEG, PNG or WebP, the owner's
// not-found error, and catalog.ErrPhotoLimit when the owner has 8 photos.
// Nothing is recorded until ConfirmPhoto.
func (s *Service) RequestPhotoUpload(ctx context.Context, in PhotoUploadInput) (PhotoUploadView, error) {
	by, err := writer(ctx, false)
	if err != nil {
		return PhotoUploadView{}, err
	}
	if err := in.Owner.Validate(); err != nil {
		return PhotoUploadView{}, err
	}
	if !catalog.IsPhotoType(in.ContentType) {
		return PhotoUploadView{}, shared.NewValidationError(shared.FieldError{Field: fieldContentType, Message: msgPhotoType})
	}
	tenant := by.TenantID()
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		list, err := s.ownerPhotos(ctx, tenant, in.Owner)
		if err != nil {
			return err
		}
		if list.Full() {
			return catalog.ErrPhotoLimit
		}
		return nil
	})
	if err != nil {
		return PhotoUploadView{}, err
	}
	key := catalog.PhotoKey(tenant, in.Owner, shared.PhotoID(s.ids.NewID()))
	up, err := s.storage.PresignPut(ctx, key, in.ContentType, uploadURLTTL)
	if err != nil {
		return PhotoUploadView{}, fmt.Errorf("sign photo upload: %w", err)
	}
	return PhotoUploadView{Key: key, URL: up.URL, Method: up.Method, Headers: up.Headers, ExpiresAt: up.ExpiresAt}, nil
}

// ConfirmPhoto records the photo uploaded to key as owner's last photo. It
// needs catalog:write and runs, in this order (plan 3.9, from the tenancy
// review of 3.1; ADR 0025):
//
//  1. key must parse exactly as the caller's store's key for owner; any
//     other is catalog.ErrPhotoNotUploaded, and storage is never touched;
//  2. a photo already recorded with the key's id is returned with
//     AlreadyRecorded when it is owner's, and is ErrPhotoNotUploaded when
//     it is not; neither reads nor deletes anything, and this runs before
//     the 8-photo limit, so a retried confirm of the 8th photo is no
//     catalog.ErrPhotoLimit;
//  3. the upload must exist, or it is ErrPhotoNotUploaded;
//  4. it must be 1 byte to 10 MB and a JPEG, PNG or WebP by its first 512
//     bytes; otherwise the upload is deleted and it is catalog.ErrPhotoInvalid;
//  5. the row is recorded with the key's id as its primary key, and the
//     upload is promoted to the photo's place in the same transaction. A
//     confirm that loses a race to record the same key returns the photo
//     the other recorded, and deletes nothing.
//
// It also returns ErrInvalidPhotoOwner, the owner's not-found error and
// catalog.ErrPhotoLimit. The owner's version never changes (plan 3.6).
func (s *Service) ConfirmPhoto(ctx context.Context, owner catalog.PhotoOwner, key string) (PhotoConfirmation, error) {
	by, err := writer(ctx, false)
	if err != nil {
		return PhotoConfirmation{}, err
	}
	if err := owner.Validate(); err != nil {
		return PhotoConfirmation{}, err
	}
	tenant := by.TenantID()
	id, err := catalog.ParsePhotoKey(key, tenant, owner)
	if err != nil {
		return PhotoConfirmation{}, err
	}
	stored, found, err := s.recordedPhoto(ctx, tenant, owner, id)
	if err != nil || found {
		return s.confirmation(ctx, stored, true, err)
	}
	info, contentType, err := s.checkUpload(ctx, key)
	if err != nil {
		return PhotoConfirmation{}, err
	}
	stored, already, err := s.recordPhoto(ctx, tenant, owner, key, info, contentType)
	if err == nil && !already {
		s.invalidate(ctx, tenant)
	}
	return s.confirmation(ctx, stored, already, err)
}

// recordedPhoto finds the photo id already recorded on owner (step 2 of
// ConfirmPhoto), after checking that owner is the store's.
func (s *Service) recordedPhoto(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner, id shared.PhotoID) (StoredPhoto, bool, error) {
	var stored StoredPhoto
	var found bool
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.ownerExists(ctx, tenant, owner); err != nil {
			return err
		}
		var err error
		stored, found, err = s.photoOn(ctx, tenant, owner, id)
		return err
	})
	return stored, found, err
}

// photoOn returns the photo id when it is recorded on owner. A photo of
// another owner is catalog.ErrPhotoNotUploaded, like any key that is not
// this owner's.
func (s *Service) photoOn(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner, id shared.PhotoID) (StoredPhoto, bool, error) {
	stored, err := s.photos.ByID(ctx, tenant, id)
	switch {
	case errors.Is(err, catalog.ErrPhotoNotFound):
		return StoredPhoto{}, false, nil
	case err != nil:
		return StoredPhoto{}, false, fmt.Errorf("read photo %s: %w", id, err)
	case stored.Owner != owner:
		return StoredPhoto{}, false, catalog.ErrPhotoNotUploaded
	}
	return stored, true, nil
}

// checkUpload checks the staged upload at key (steps 3 and 4 of
// ConfirmPhoto) and returns it with its sniffed content type. Only an
// upload that passed the key check and has no photo row gets here, so only
// such an upload is ever deleted (ADR 0025).
func (s *Service) checkUpload(ctx context.Context, key string) (ObjectInfo, string, error) {
	info, err := s.storage.Stat(ctx, key)
	if err != nil {
		return ObjectInfo{}, "", storageError("read the upload", err)
	}
	if info.Size < 1 || info.Size > catalog.MaxPhotoBytes {
		return ObjectInfo{}, "", s.refuseUpload(ctx, key, fmt.Errorf("upload of %d bytes: %w", info.Size, catalog.ErrPhotoInvalid))
	}
	head, err := s.storage.Head(ctx, key, info.ETag, catalog.SniffLen)
	if err != nil {
		return ObjectInfo{}, "", storageError("read the upload's first bytes", err)
	}
	contentType, ok := catalog.SniffPhotoType(head)
	if !ok {
		return ObjectInfo{}, "", s.refuseUpload(ctx, key, fmt.Errorf("upload is not JPEG, PNG or WebP: %w", catalog.ErrPhotoInvalid))
	}
	return info, contentType, nil
}

// refuseUpload deletes a refused upload and returns why it was refused.
func (s *Service) refuseUpload(ctx context.Context, key string, why error) error {
	// A failed delete must not turn the refusal into a 503: the bucket's
	// lifecycle rule removes the upload within a day anyway (ADR 0024), and
	// the adapter logs the failure.
	_ = s.storage.DeleteUpload(ctx, key)
	return why
}

// recordPhoto records the checked upload as owner's last photo and promotes
// it (step 5 of ConfirmPhoto). already is true when a racing confirm
// recorded the key first; then nothing is promoted or deleted.
func (s *Service) recordPhoto(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner, key string, info ObjectInfo, contentType string) (StoredPhoto, bool, error) {
	id, err := catalog.ParsePhotoKey(key, tenant, owner)
	if err != nil {
		return StoredPhoto{}, false, err
	}
	photo, err := catalog.NewPhoto(id, key, contentType, info.Size)
	if err != nil {
		return StoredPhoto{}, false, err
	}
	var stored StoredPhoto
	var already bool
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		pos, err := s.photos.Insert(ctx, tenant, owner, photo)
		if errors.Is(err, ErrPhotoTaken) {
			stored, already, err = s.photoOn(ctx, tenant, owner, id)
			if err == nil && !already {
				err = catalog.ErrPhotoNotUploaded // another store's id, which RLS hides
			}
			return err
		}
		if err != nil {
			return fmt.Errorf("record photo %s: %w", id, err)
		}
		stored = StoredPhoto{Owner: owner, Photo: photo, Position: pos}
		// Inside the transaction: if the copy fails, no row points at a
		// missing object.
		return storageError("promote the upload", s.storage.Promote(ctx, key, info.ETag, contentType))
	})
	return stored, already, err
}

// storageError maps the storage port's refusals to the contract's answer:
// an upload that is missing, or was replaced after it was checked, is
// catalog.ErrPhotoNotUploaded ("A foto não foi enviada. Tente de novo.").
// Every other failure is wrapped as it is, so an outage stays a 503.
func storageError(doing string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrObjectMissing), errors.Is(err, ErrObjectChanged):
		return fmt.Errorf("%s: %w", doing, catalog.ErrPhotoNotUploaded)
	default:
		return fmt.Errorf("%s: %w", doing, err)
	}
}

// confirmation signs the view of a confirmed photo; err passes through.
func (s *Service) confirmation(ctx context.Context, stored StoredPhoto, already bool, err error) (PhotoConfirmation, error) {
	if err != nil {
		return PhotoConfirmation{}, err
	}
	v, err := s.photoView(ctx, stored.Photo, stored.Position)
	if err != nil {
		return PhotoConfirmation{}, err
	}
	return PhotoConfirmation{Photo: v, AlreadyRecorded: already}, nil
}

// OrderPhotos puts owner's photos in the order of ids; the first becomes
// the cover. It needs catalog:write. Every photo must be listed exactly
// once: otherwise it is a ValidationError on "photoIds". It also returns
// ErrInvalidPhotoOwner and the owner's not-found error. The owner's
// version never changes (plan 3.6).
func (s *Service) OrderPhotos(ctx context.Context, owner catalog.PhotoOwner, ids []shared.PhotoID) ([]PhotoView, error) {
	by, err := writer(ctx, false)
	if err != nil {
		return nil, err
	}
	if err := owner.Validate(); err != nil {
		return nil, err
	}
	tenant := by.TenantID()
	var ordered []catalog.Photo
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		list, err := s.ownerPhotos(ctx, tenant, owner)
		if err != nil {
			return err
		}
		if err := list.ReorderPhotos(ids); err != nil {
			return err
		}
		if err := s.photos.Reorder(ctx, tenant, owner, ids); err != nil {
			return fmt.Errorf("reorder photos: %w", err)
		}
		ordered = list.Photos()
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.invalidate(ctx, tenant)
	return s.photoViews(ctx, ordered)
}

// DeletePhoto removes owner's photo id: the row, then its object. It needs
// catalog:write and returns ErrInvalidPhotoOwner, and catalog.ErrPhotoNotFound
// when owner has no such photo. Once the row is gone the photo is gone for
// every reader, so an object that storage fails to delete is no error: the
// adapter logs its key for an operator. The owner's version never changes
// (plan 3.6).
func (s *Service) DeletePhoto(ctx context.Context, owner catalog.PhotoOwner, id shared.PhotoID) error {
	by, err := writer(ctx, false)
	if err != nil {
		return err
	}
	if err := owner.Validate(); err != nil {
		return err
	}
	tenant := by.TenantID()
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.photos.Delete(ctx, tenant, owner, id); err != nil {
			return fmt.Errorf("delete photo %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.invalidate(ctx, tenant)
	// The key is rebuilt, not read: a row of this owner has exactly this
	// key (0008_catalog's check).
	_ = s.storage.Delete(ctx, catalog.PhotoKey(tenant, owner, id)) // logged by the adapter; see above
	return nil
}

// ownerPhotos checks that owner is the store's and returns its photos.
func (s *Service) ownerPhotos(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner) (*catalog.PhotoList, error) {
	if err := s.ownerExists(ctx, tenant, owner); err != nil {
		return nil, err
	}
	photos, err := s.photos.ByOwner(ctx, tenant, owner)
	if err != nil {
		return nil, fmt.Errorf("read photos: %w", err)
	}
	return catalog.NewPhotoList(photos)
}

// ownerExists returns catalog.ErrItemNotFound or catalog.ErrKitNotFound
// unless owner is an item or kit of the store, archived or not.
func (s *Service) ownerExists(ctx context.Context, tenant shared.TenantID, owner catalog.PhotoOwner) error {
	if owner.Kind == catalog.OwnerKit {
		if _, err := s.kits.ByID(ctx, tenant, shared.KitID(owner.ID)); err != nil {
			return fmt.Errorf("load kit %s: %w", owner.ID, err)
		}
		return nil
	}
	facts, err := s.items.ByIDs(ctx, tenant, []shared.ItemID{shared.ItemID(owner.ID)})
	if err != nil {
		return fmt.Errorf("load item %s: %w", owner.ID, err)
	}
	if len(facts) == 0 {
		return catalog.ErrItemNotFound
	}
	return nil
}
