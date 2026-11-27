//go:build integration

package storage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/lifecycle"

	"github.com/bergsantana/encomendo/backend/internal/adapter/storage"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/app/catalog/fake"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/contract/catalogports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

var jpeg = append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0x42}, 2048)...)

func newStore(t *testing.T) *storage.S3 {
	t.Helper()
	s, err := storage.New(storage.Config{
		Endpoint: endpoint, PublicEndpoint: endpoint, Bucket: bucket, Region: "us-east-1",
		AccessKey: accessKey, SecretKey: secretKey,
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// env is the catalog use cases over fake repositories and the real store.
type env struct {
	svc    *appcatalog.Service
	s3     *storage.S3
	ctx    context.Context
	tenant shared.TenantID
	owner  catalog.PhotoOwner
}

func newEnv(t *testing.T) env {
	t.Helper()
	clock := sharedfake.NewClock(time.Now())
	ids := sharedfake.NewIDs(clock)
	repos := fake.NewStore()
	defaults := fake.NewDefaults()
	tenant := shared.TenantID(pgtest.NewID())
	defaults.Set(tenant, appcatalog.Defaults{Currency: "BRL", ProductionEnabled: true})
	s3 := newStore(t)
	svc := appcatalog.NewService(appcatalog.Deps{
		Categories: repos.Categories, Attributes: repos.Attributes, Items: repos.Items, Kits: repos.Kits,
		Suppliers: fake.NewSuppliers(), Defaults: defaults, Photos: repos.Photos, Storage: s3,
		Cache: sharedfake.NewCache(clock), Tx: sharedfake.NewTxManager(), IDs: ids,
	})
	p, err := shared.NewBackofficePrincipal(shared.UserID(pgtest.NewID()), tenant, shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authz.WithPrincipal(t.Context(), p)
	it, err := svc.CreateItem(ctx, appcatalog.ItemInput{Name: "Banco", SourcingMode: catalog.ModeInHouse, PriceCents: 100, ProductionDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	return env{svc: svc, s3: s3, ctx: ctx, tenant: tenant, owner: catalog.ItemPhotos(it.ID)}
}

// put uploads body with the signed request, as an app does: exactly the
// signed headers, no Authorization.
func put(t *testing.T, up appcatalog.SignedUpload, body []byte) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), up.Method, up.URL, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range up.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close() // the status is all the test needs
	return resp.StatusCode
}

// signed is the request an upload view tells the app to send.
func signed(v appcatalog.PhotoUploadView) appcatalog.SignedUpload {
	return appcatalog.SignedUpload{URL: v.URL, Method: v.Method, Headers: v.Headers, ExpiresAt: v.ExpiresAt}
}

func (e env) upload(t *testing.T, contentType string, body []byte) appcatalog.PhotoUploadView {
	t.Helper()
	up, err := e.svc.RequestPhotoUpload(e.ctx, appcatalog.PhotoUploadInput{Owner: e.owner, ContentType: contentType})
	if err != nil {
		t.Fatal(err)
	}
	if code := put(t, signed(up), body); code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200", code)
	}
	return up
}

func exists(t *testing.T, name string) bool {
	t.Helper()
	c, err := rawClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.StatObject(t.Context(), bucket, name, minio.StatObjectOptions{})
	if err != nil && minio.ToErrorResponse(err).Code != minio.NoSuchKey {
		t.Fatal(err)
	}
	return err == nil
}

func TestPhoto_PresignPutConfirmAndRead(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	up := e.upload(t, catalog.ContentTypeJPEG, jpeg)
	c, err := e.svc.ConfirmPhoto(e.ctx, e.owner, up.Key)
	if err != nil || c.AlreadyRecorded {
		t.Fatalf("ConfirmPhoto = %+v, %v; want a new photo", c, err)
	}
	if !exists(t, up.Key) {
		t.Fatal("the photo was not promoted to its key")
	}
	resp, err := http.Get(c.Photo.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK || !bytes.Equal(got, jpeg) || resp.Header.Get("Content-Type") != catalog.ContentTypeJPEG {
		t.Errorf("signed GET = %d %q, %d bytes (%v)", resp.StatusCode, resp.Header.Get("Content-Type"), len(got), err)
	}
	// The upload URL, reused within its 5 minutes, cannot replace the photo.
	if code := put(t, signed(up), []byte("outra coisa")); code != http.StatusOK {
		t.Fatalf("reused PUT = %d", code)
	}
	again, err := http.Get(c.Photo.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = again.Body.Close() }()
	if got, _ := io.ReadAll(again.Body); !bytes.Equal(got, jpeg) {
		t.Error("the reused upload URL replaced the confirmed photo")
	}
	// A repeat confirm returns the photo and touches nothing.
	if r, err := e.svc.ConfirmPhoto(e.ctx, e.owner, up.Key); err != nil || !r.AlreadyRecorded || r.Photo.ID != c.Photo.ID {
		t.Errorf("repeat = %+v, %v", r, err)
	}
}

func TestPhoto_An11MBObjectIsRefusedAndDeleted(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	big := append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, 11<<20)...)
	up := e.upload(t, catalog.ContentTypeJPEG, big)
	if _, err := e.svc.ConfirmPhoto(e.ctx, e.owner, up.Key); !errors.Is(err, catalog.ErrPhotoInvalid) {
		t.Fatalf("err = %v, want ErrPhotoInvalid", err)
	}
	if exists(t, storage.UploadPrefix+up.Key) || exists(t, up.Key) {
		t.Error("the refused upload is still stored")
	}
}

func TestPhoto_TextNamedJPGIsRefused(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	up := e.upload(t, catalog.ContentTypeJPEG, []byte("isto é texto com nome de foto.jpg"))
	if _, err := e.svc.ConfirmPhoto(e.ctx, e.owner, up.Key); !errors.Is(err, catalog.ErrPhotoInvalid) {
		t.Fatalf("err = %v, want ErrPhotoInvalid", err)
	}
	if exists(t, storage.UploadPrefix+up.Key) {
		t.Error("the refused upload is still stored")
	}
}

func TestPhoto_TheSignedContentTypeIsBound(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	up, err := e.svc.RequestPhotoUpload(e.ctx, appcatalog.PhotoUploadInput{Owner: e.owner, ContentType: catalog.ContentTypePNG})
	if err != nil {
		t.Fatal(err)
	}
	up.Headers["Content-Type"] = "text/html"
	if code := put(t, signed(up), jpeg); code != http.StatusForbidden {
		t.Errorf("PUT with another content type = %d, want 403", code)
	}
}

func TestPhoto_AnExpiredURLIsRefused(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	key := catalog.PhotoKey(shared.TenantID(pgtest.NewID()), catalog.ItemPhotos(shared.ItemID(pgtest.NewID())), shared.PhotoID(pgtest.NewID()))
	up, err := s.PresignPut(t.Context(), key, catalog.ContentTypeJPEG, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// Poll until the second has passed, with a deadline.
	deadline := time.Now().Add(15 * time.Second)
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for time.Now().Before(up.ExpiresAt.Add(time.Second)) && time.Now().Before(deadline) {
		<-tick.C
	}
	if code := put(t, up, jpeg); code != http.StatusForbidden {
		t.Errorf("PUT after expiry = %d, want 403", code)
	}
	if exists(t, storage.UploadPrefix+key) {
		t.Error("an expired URL stored an object")
	}
}

// The PhotoStore contract, the same suite the fake passes in
// test/contract: missing keys, staged uploads, ETag-bound reads and copies,
// repeated promotions and deletes, and caller bugs.
func TestS3_Contract(t *testing.T) {
	catalogports.RunPhotoStoreContract(t, func(t *testing.T) catalogports.PhotoStoreHarness {
		s := newStore(t)
		return catalogports.PhotoStoreHarness{
			Store: s,
			Upload: func(t *testing.T, key, contentType string, data []byte) {
				t.Helper()
				up, err := s.PresignPut(t.Context(), key, contentType, time.Minute)
				if err != nil {
					t.Fatal(err)
				}
				if code := put(t, up, data); code != http.StatusOK {
					t.Fatalf("PUT = %d, want 200", code)
				}
			},
			Read:  read,
			NewID: pgtest.NewID,
		}
	})
}

// read returns the object at name and its content type, through a plain
// client.
func read(t *testing.T, name string) ([]byte, string, bool) {
	t.Helper()
	if !exists(t, name) {
		return nil, "", false
	}
	c, err := rawClient()
	if err != nil {
		t.Fatal(err)
	}
	obj, err := c.GetObject(t.Context(), bucket, name, minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = obj.Close() }() // a read-only body
	info, err := obj.Stat()
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(obj)
	if err != nil {
		t.Fatal(err)
	}
	return data, info.ContentType, true
}

func TestPhoto_AConfirmOfAKeyNobodyUploadedTo(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	missing := catalog.PhotoKey(e.tenant, e.owner, shared.PhotoID(pgtest.NewID()))
	if _, err := e.svc.ConfirmPhoto(e.ctx, e.owner, missing); !errors.Is(err, catalog.ErrPhotoNotUploaded) {
		t.Errorf("ConfirmPhoto of a key nobody uploaded to = %v", err)
	}
}

// A bucket that is not there is a broken store, not a missing upload: the
// server names it NoSuchBucket, even on a HEAD.
func TestS3_AMissingBucketIsUnavailable(t *testing.T) {
	t.Parallel()
	s, err := storage.New(storage.Config{
		Endpoint: endpoint, PublicEndpoint: endpoint, Bucket: "no-such-bucket", Region: "us-east-1",
		AccessKey: accessKey, SecretKey: secretKey,
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	key := catalog.PhotoKey(shared.TenantID(pgtest.NewID()), catalog.ItemPhotos(shared.ItemID(pgtest.NewID())), shared.PhotoID(pgtest.NewID()))
	if _, err := s.Stat(t.Context(), key); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("Stat = %v, want ErrUnavailable", err)
	}
	if err := s.Promote(t.Context(), key, "abc", catalog.ContentTypeJPEG); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("Promote = %v, want ErrUnavailable", err)
	}
}

// The bucket-init job's lifecycle rule, as the stack sets it, applies to
// uploads/ only: a confirmed photo never expires. This checks that the
// server takes the rule; the compose job that sets it (minio-init in
// deploy/docker-compose.yml) is checked by hand, from its log, when the
// stack changes.
func TestPhoto_LifecycleRuleCoversUploadsOnly(t *testing.T) {
	t.Parallel()
	c, err := rawClient()
	if err != nil {
		t.Fatal(err)
	}
	cfg := lifecycle.NewConfiguration()
	cfg.Rules = []lifecycle.Rule{{
		ID: "expire-unconfirmed-uploads", Status: "Enabled",
		RuleFilter: lifecycle.Filter{Prefix: storage.UploadPrefix}, Expiration: lifecycle.Expiration{Days: 1},
	}}
	if err := c.SetBucketLifecycle(t.Context(), bucket, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetBucketLifecycle(t.Context(), bucket)
	if err != nil || len(got.Rules) != 1 || got.Rules[0].RuleFilter.Prefix != storage.UploadPrefix || got.Rules[0].Expiration.Days != 1 {
		t.Errorf("lifecycle = %+v, %v", got, err)
	}
}

func TestPhoto_DeleteRemovesTheObject(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	up := e.upload(t, catalog.ContentTypeJPEG, jpeg)
	c, err := e.svc.ConfirmPhoto(e.ctx, e.owner, up.Key)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeletePhoto(e.ctx, e.owner, c.Photo.ID); err != nil {
		t.Fatal(err)
	}
	if exists(t, up.Key) || exists(t, storage.UploadPrefix+up.Key) {
		t.Error("the photo's object or its staged upload is still stored")
	}
	// The key is single-use (ADR 0025): confirming it again finds nothing.
	if _, err := e.svc.ConfirmPhoto(e.ctx, e.owner, up.Key); !errors.Is(err, catalog.ErrPhotoNotUploaded) {
		t.Errorf("a confirm after the delete = %v, want ErrPhotoNotUploaded", err)
	}
}
