package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
)

// ReceiptStore implements apppurchasing.ReceiptPhotos on the same bucket as
// catalog photos: uploads staged under uploads/ and promoted once a receipt
// records them (ADR 0028), under the keys purchasing issues,
// t/{tenant}/receipts/{photo}. It only maps the S3 adapter's types and
// errors to purchasing's, which may not import app/catalog.
type ReceiptStore struct{ s *S3 }

var _ apppurchasing.ReceiptPhotos = ReceiptStore{}

// NewReceiptStore returns the receipt photo store over s.
func NewReceiptStore(s *S3) ReceiptStore { return ReceiptStore{s: s} }

// PresignPut implements apppurchasing.ReceiptPhotos.
func (r ReceiptStore) PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (apppurchasing.SignedUpload, error) {
	u, err := r.s.PresignPut(ctx, key, contentType, ttl)
	if err != nil {
		return apppurchasing.SignedUpload{}, receiptErr(err)
	}
	return apppurchasing.SignedUpload{URL: u.URL, Method: u.Method, Headers: u.Headers, ExpiresAt: u.ExpiresAt}, nil
}

// Stat implements apppurchasing.ReceiptPhotos.
func (r ReceiptStore) Stat(ctx context.Context, key string) (apppurchasing.ObjectInfo, error) {
	info, err := r.s.Stat(ctx, key)
	if err != nil {
		return apppurchasing.ObjectInfo{}, receiptErr(err)
	}
	return apppurchasing.ObjectInfo{Size: info.Size, ETag: info.ETag}, nil
}

// Head implements apppurchasing.ReceiptPhotos.
func (r ReceiptStore) Head(ctx context.Context, key, etag string, n int) ([]byte, error) {
	b, err := r.s.Head(ctx, key, etag, n)
	return b, receiptErr(err)
}

// Promote implements apppurchasing.ReceiptPhotos.
func (r ReceiptStore) Promote(ctx context.Context, key, etag, contentType string) error {
	return receiptErr(r.s.Promote(ctx, key, etag, contentType))
}

// DeleteUpload implements apppurchasing.ReceiptPhotos.
func (r ReceiptStore) DeleteUpload(ctx context.Context, key string) error {
	return receiptErr(r.s.DeleteUpload(ctx, key))
}

// ReportOrphan implements apppurchasing.ReceiptPhotos.
func (r ReceiptStore) ReportOrphan(ctx context.Context, key string) { r.s.ReportOrphan(ctx, key) }

// receiptErr maps the S3 adapter's named refusals to purchasing's; every
// other error (shared.ErrUnavailable and the rest) passes through.
func receiptErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, appcatalog.ErrObjectMissing):
		return fmt.Errorf("%w (%w)", apppurchasing.ErrObjectMissing, err)
	case errors.Is(err, appcatalog.ErrObjectChanged):
		return fmt.Errorf("%w (%w)", apppurchasing.ErrObjectChanged, err)
	default:
		return err
	}
}
