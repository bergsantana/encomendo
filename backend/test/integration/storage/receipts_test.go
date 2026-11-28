//go:build integration

package storage_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/storage"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// TestReceiptStore_StageAndPromote: a receiving photo goes up to its staged
// place through the signed request, is checked there, and is promoted to
// t/{tenant}/receipts/{photo}; a key with nothing uploaded is purchasing's
// ErrObjectMissing (ADR 0028, ADR 0031).
func TestReceiptStore_StageAndPromote(t *testing.T) {
	t.Parallel()
	store := storage.NewReceiptStore(newStore(t))
	ctx := t.Context()
	key := purchasing.PhotoKey(shared.TenantID(pgtest.NewID()), shared.PhotoID(pgtest.NewID()))

	if _, err := store.Stat(ctx, key); !errors.Is(err, apppurchasing.ErrObjectMissing) {
		t.Fatalf("Stat before the upload: err = %v, want ErrObjectMissing", err)
	}
	up, err := store.PresignPut(ctx, key, "image/jpeg", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if code := put(t, appcatalog.SignedUpload{URL: up.URL, Method: up.Method, Headers: up.Headers, ExpiresAt: up.ExpiresAt}, jpeg); code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200", code)
	}
	info, err := store.Stat(ctx, key)
	if err != nil || info.Size != int64(len(jpeg)) || info.ETag == "" {
		t.Fatalf("Stat = %+v, %v", info, err)
	}
	head, err := store.Head(ctx, key, info.ETag, 4)
	if err != nil || len(head) != 4 || head[0] != 0xFF {
		t.Fatalf("Head = %x, %v", head, err)
	}
	if _, err := store.Head(ctx, key, "\"not-this-version\"", 4); !errors.Is(err, apppurchasing.ErrObjectChanged) {
		t.Errorf("Head of another version: err = %v, want ErrObjectChanged", err)
	}
	if err := store.Promote(ctx, key, info.ETag, "image/jpeg"); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if err := store.Promote(ctx, key, info.ETag, "image/jpeg"); err != nil {
		t.Errorf("Promote again: %v, want harmless", err)
	}
	if err := store.DeleteUpload(ctx, key); err != nil {
		t.Errorf("DeleteUpload: %v", err)
	}
}
