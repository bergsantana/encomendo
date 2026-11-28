package purchasing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// uploadURLTTL is how long a signed upload URL works (as catalog photos:
// enough to send 10 MB over a slow phone line, short enough that a leaked
// URL dies).
const uploadURLTTL = 5 * time.Minute

// RequestPhotoUpload signs the upload of one receiving photo (receiving:write):
// a key the server picks, t/{store}/receipts/{new photo id}, and a PUT URL
// valid for 5 minutes with the content type bound (ADR 0028, ADR 0031). It
// returns a ValidationError on "contentType" for a type other than JPEG, PNG
// or WebP. Nothing is recorded until the key comes back in a receipt.
func (s *Service) RequestPhotoUpload(ctx context.Context, contentType string) (PhotoUploadView, error) {
	by, err := authz.Require(ctx, identity.PermReceivingWrite)
	if err != nil {
		return PhotoUploadView{}, err
	}
	if !catalog.IsPhotoType(contentType) {
		return PhotoUploadView{}, invalid("contentType", msgPhotoType)
	}
	key := purchasing.PhotoKey(by.TenantID(), shared.PhotoID(s.d.IDs.NewID()))
	up, err := s.d.Photos.PresignPut(ctx, key, contentType, uploadURLTTL)
	if err != nil {
		return PhotoUploadView{}, fmt.Errorf("sign receipt photo upload: %w", err)
	}
	return PhotoUploadView{Key: key, URL: up.URL, Method: up.Method, Headers: up.Headers, ExpiresAt: up.ExpiresAt}, nil
}

// staged is a photo whose upload was checked: the record a receipt keeps
// and the version Promote must copy.
type staged struct {
	photo purchasing.Photo
	etag  string
}

// checkUpload checks the staged upload at key, as a catalog photo's
// confirmation does (ADR 0025, ADR 0028): the key must be this store's
// receiving key (purchasing.ErrPhotoKey otherwise), the upload must exist
// (catalog.ErrPhotoNotUploaded), be 1 byte to 10 MB and a JPEG, PNG or WebP
// by its first bytes, or it is deleted and the answer is
// catalog.ErrPhotoInvalid. It runs outside any transaction, so storage never
// holds one.
func (s *Service) checkUpload(ctx context.Context, tenant shared.TenantID, key string) (staged, error) {
	id, err := purchasing.ParsePhotoKey(tenant, key)
	if err != nil {
		return staged{}, err
	}
	info, err := s.d.Photos.Stat(ctx, key)
	if err != nil {
		return staged{}, storageError("read the upload", err)
	}
	if info.Size < 1 || info.Size > catalog.MaxPhotoBytes {
		return staged{}, s.refuseUpload(ctx, key, fmt.Errorf("upload of %d bytes: %w", info.Size, catalog.ErrPhotoInvalid))
	}
	head, err := s.d.Photos.Head(ctx, key, info.ETag, catalog.SniffLen)
	if err != nil {
		return staged{}, storageError("read the upload's first bytes", err)
	}
	contentType, ok := catalog.SniffPhotoType(head)
	if !ok {
		return staged{}, s.refuseUpload(ctx, key, fmt.Errorf("upload is not JPEG, PNG or WebP: %w", catalog.ErrPhotoInvalid))
	}
	return staged{photo: purchasing.Photo{ID: id, Key: key, ContentType: contentType, Size: info.Size}, etag: info.ETag}, nil
}

// refuseUpload deletes a refused upload and returns why it was refused.
func (s *Service) refuseUpload(ctx context.Context, key string, why error) error {
	// A failed delete must not turn the refusal into a 503: the bucket's
	// lifecycle rule removes the upload within a day anyway (ADR 0024).
	_ = s.d.Photos.DeleteUpload(ctx, key)
	return why
}

// storageError maps the storage port's refusals to the contract's answer:
// an upload that is missing, or was replaced after it was checked, is
// catalog.ErrPhotoNotUploaded. Every other failure is wrapped as it is, so
// an outage stays a 503.
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

// mayHaveCopied reports whether a Promote that returned err may have left
// an object at the photo's key: always but for a refusal before the copy.
func mayHaveCopied(err error) bool {
	return !errors.Is(err, ErrObjectMissing) && !errors.Is(err, ErrObjectChanged)
}
