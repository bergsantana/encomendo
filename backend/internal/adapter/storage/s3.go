package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// UploadPrefix is where uploads are staged: the bucket's lifecycle rule
// expires everything under it after a day, and nothing else (ADR 0028).
const UploadPrefix = "uploads/"

// maxRetries is how many times a failed call to the store is tried again.
const maxRetries = 2

// Config is how to reach the bucket. Endpoints are host[:port].
type Config struct {
	// Endpoint is where the API reaches the store for reads, copies and
	// deletes.
	Endpoint string
	// PublicEndpoint is the host signed URLs name, which browsers and the
	// Android emulator reach (ADR 0025). Signing binds the host.
	PublicEndpoint string
	Bucket         string
	Region         string
	AccessKey      string
	SecretKey      string
	UseSSL         bool
}

// S3 implements appcatalog.PhotoStore on an S3-compatible store: MinIO
// locally, Cloudflare R2 in production. It talks to Endpoint, and signs
// URLs for PublicEndpoint with a second client that never connects:
// with the region set, signing is local. Every failure but a missing or
// replaced object matches shared.ErrUnavailable. It never logs a secret or
// a signed URL.
type S3 struct {
	api    *minio.Client
	signer *minio.Client
	bucket string
	log    *slog.Logger
}

var _ appcatalog.PhotoStore = (*S3)(nil)

// New returns the store for cfg. It does not connect.
func New(cfg Config, log *slog.Logger) (*S3, error) {
	api, err := client(cfg, cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	signer, err := client(cfg, cfg.PublicEndpoint)
	if err != nil {
		return nil, err
	}
	return &S3{api: api, signer: signer, bucket: cfg.Bucket, log: log}, nil
}

func client(cfg Config, endpoint string) (*minio.Client, error) {
	c, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		// The region is set, so no call asks the bucket for it, and the
		// path style keeps one host per endpoint, as R2 and MinIO serve.
		Region:       cfg.Region,
		BucketLookup: minio.BucketLookupPath,
		// A few retries ride out a blip; the default ten would hold a
		// request for many seconds when the store is down.
		MaxRetries: maxRetries,
	})
	if err != nil {
		// minio.New fails only on a malformed endpoint; the message names
		// the endpoint, never the credentials.
		return nil, fmt.Errorf("storage client for %s: %w", endpoint, err)
	}
	return c, nil
}

func uploadName(key string) string { return UploadPrefix + key }

// unavailable wraps a failure of the store so the API answers 503.
func unavailable(doing, key string, err error) error {
	return fmt.Errorf("storage: %s %s: %w: %w", doing, key, shared.ErrUnavailable, err)
}

// refusal maps the store's answers for a missing or a replaced object to
// the port's errors, and anything else to unavailable.
func refusal(doing, key string, err error) error {
	var resp minio.ErrorResponse
	if errors.As(err, &resp) {
		switch {
		case resp.Code == minio.NoSuchKey || resp.StatusCode == http.StatusNotFound:
			return fmt.Errorf("storage: %s %s: %w", doing, key, appcatalog.ErrObjectMissing)
		case resp.Code == minio.PreconditionFailed || resp.StatusCode == http.StatusPreconditionFailed:
			return fmt.Errorf("storage: %s %s: %w", doing, key, appcatalog.ErrObjectChanged)
		}
	}
	return unavailable(doing, key, err)
}

// SignGet implements appcatalog.PhotoURLs: a GET of the photo's object.
func (s *S3) SignGet(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	expires := time.Now().Add(ttl)
	u, err := s.signer.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", time.Time{}, unavailable("sign a read of", key, err)
	}
	return u.String(), expires, nil
}

// PresignPut implements appcatalog.PhotoStore: a PUT to the staging place,
// with Content-Type among the signed headers, so an upload of another type
// fails its signature.
func (s *S3) PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (appcatalog.SignedUpload, error) {
	expires := time.Now().Add(ttl)
	headers := http.Header{"Content-Type": []string{contentType}}
	u, err := s.signer.PresignHeader(ctx, http.MethodPut, s.bucket, uploadName(key), ttl, nil, headers)
	if err != nil {
		return appcatalog.SignedUpload{}, unavailable("sign an upload of", key, err)
	}
	return appcatalog.SignedUpload{
		URL: u.String(), Method: http.MethodPut, Headers: map[string]string{"Content-Type": contentType}, ExpiresAt: expires,
	}, nil
}

// Stat implements appcatalog.PhotoStore.
func (s *S3) Stat(ctx context.Context, key string) (appcatalog.ObjectInfo, error) {
	info, err := s.api.StatObject(ctx, s.bucket, uploadName(key), minio.StatObjectOptions{})
	if err != nil {
		return appcatalog.ObjectInfo{}, refusal("stat the upload", key, err)
	}
	return appcatalog.ObjectInfo{Size: info.Size, ETag: info.ETag}, nil
}

// Head implements appcatalog.PhotoStore with a ranged GET bound to etag.
func (s *S3) Head(ctx context.Context, key, etag string, n int) ([]byte, error) {
	if n < 1 || etag == "" {
		// A caller's bug, not the client's input: it matches no sentinel.
		return nil, fmt.Errorf("storage: read %d bytes of %s at etag %q", n, key, etag)
	}
	var opts minio.GetObjectOptions
	if err := opts.SetMatchETag(etag); err != nil {
		return nil, unavailable("read the upload", key, err)
	}
	if err := opts.SetRange(0, int64(n)-1); err != nil {
		return nil, unavailable("read the upload", key, err)
	}
	obj, err := s.api.GetObject(ctx, s.bucket, uploadName(key), opts)
	if err != nil {
		return nil, refusal("read the upload", key, err)
	}
	defer func() { _ = obj.Close() }() // a read-only body; nothing to flush
	head, err := io.ReadAll(io.LimitReader(obj, int64(n)))
	if err != nil {
		return nil, refusal("read the upload", key, err)
	}
	return head, nil
}

// DeleteUpload implements appcatalog.PhotoStore. The caller goes on
// without the error, so a failure is logged here; the lifecycle rule
// removes the upload within a day anyway.
func (s *S3) DeleteUpload(ctx context.Context, key string) error {
	if err := s.api.RemoveObject(ctx, s.bucket, uploadName(key), minio.RemoveObjectOptions{}); err != nil {
		s.log.WarnContext(ctx, "storage: a refused upload was not deleted; the lifecycle rule will", "key", key, "err", err)
		return unavailable("delete the upload", key, err)
	}
	return nil
}

// Promote implements appcatalog.PhotoStore: a server-side copy from the
// staging place, only while the upload is still the version that was
// checked, stored with the sniffed content type. The upload stays until
// the lifecycle rule removes it, so promoting it again is harmless.
func (s *S3) Promote(ctx context.Context, key, etag, contentType string) error {
	src := minio.CopySrcOptions{Bucket: s.bucket, Object: uploadName(key), MatchETag: `"` + etag + `"`}
	dst := minio.CopyDestOptions{Bucket: s.bucket, Object: key, ReplaceMetadata: true, ContentType: contentType}
	if _, err := s.api.CopyObject(ctx, dst, src); err != nil {
		return refusal("promote the upload", key, err)
	}
	return nil
}

// Delete implements appcatalog.PhotoStore. The photo's row is already gone
// when it runs, and the caller goes on without the error, so a failure is
// logged here with the key, for an operator to remove: the lifecycle rule
// covers uploads only (ADR 0028).
func (s *S3) Delete(ctx context.Context, key string) error {
	if err := s.api.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		s.log.ErrorContext(ctx, "storage: a deleted photo's object is left in the bucket", "key", key, "err", err)
		return unavailable("delete the photo", key, err)
	}
	return nil
}
