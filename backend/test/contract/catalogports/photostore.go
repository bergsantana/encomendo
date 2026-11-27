package catalogports

import (
	"bytes"
	"context"
	"errors"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// PhotoStoreHarness is a PhotoStore with what its suite needs: staging an
// upload as an app does, and reading a promoted photo back.
type PhotoStoreHarness struct {
	Store appcatalog.PhotoStore
	// Upload stages data at key as an app's signed PUT does, a new version
	// each time.
	Upload func(t *testing.T, key, contentType string, data []byte)
	// Read returns the promoted photo at key and its stored content type,
	// and false when there is none.
	Read func(t *testing.T, key string) ([]byte, string, bool)
	// NewID returns a fresh id, for keys no other case uses.
	NewID func() shared.ID
}

// RunPhotoStoreContract checks a PhotoStore (ADR 0028): the staging place
// and the photo's place, ETag-bound reads and promotions, harmless repeats
// and deletes of missing objects, and caller bugs that match no sentinel.
func RunPhotoStoreContract(t *testing.T, newHarness func(t *testing.T) PhotoStoreHarness) {
	t.Run("a key nobody uploaded to is missing", func(t *testing.T) { storeMissing(t, newHarness(t)) })
	t.Run("a staged upload is read, but not yet a photo", func(t *testing.T) { storeStaged(t, newHarness(t)) })
	t.Run("an upload replaced after Stat is neither read nor promoted", func(t *testing.T) { storeReplaced(t, newHarness(t)) })
	t.Run("a promotion copies the checked upload, and repeats", func(t *testing.T) { storePromote(t, newHarness(t)) })
	t.Run("a caller's bug matches no sentinel", func(t *testing.T) { storeMisuse(t, newHarness(t)) })
}

// photoBytes is a JPEG signature followed by n bytes in all.
func photoBytes(n int, fill byte) []byte {
	b := bytes.Repeat([]byte{fill}, n)
	copy(b, []byte{0xFF, 0xD8, 0xFF, 0xE0})
	return b
}

func (h PhotoStoreHarness) key() string {
	return catalog.PhotoKey(shared.TenantID(h.NewID()), catalog.ItemPhotos(shared.ItemID(h.NewID())), shared.PhotoID(h.NewID()))
}

func storeMissing(t *testing.T, h PhotoStoreHarness) {
	ctx, key := context.Background(), h.key()
	if _, err := h.Store.Stat(ctx, key); !errors.Is(err, appcatalog.ErrObjectMissing) {
		t.Errorf("Stat = %v, want ErrObjectMissing", err)
	}
	if _, err := h.Store.Head(ctx, key, "abc", catalog.SniffLen); !errors.Is(err, appcatalog.ErrObjectMissing) {
		t.Errorf("Head = %v, want ErrObjectMissing", err)
	}
	if err := h.Store.Promote(ctx, key, "abc", catalog.ContentTypeJPEG); !errors.Is(err, appcatalog.ErrObjectMissing) {
		t.Errorf("Promote = %v, want ErrObjectMissing", err)
	}
	if err := h.Store.DeleteUpload(ctx, key); err != nil {
		t.Errorf("DeleteUpload of nothing = %v, want nil", err)
	}
	if err := h.Store.Delete(ctx, key); err != nil {
		t.Errorf("Delete of nothing = %v, want nil", err)
	}
	h.Store.ReportOrphan(ctx, key) // never fails
}

func storeStaged(t *testing.T, h PhotoStoreHarness) {
	ctx, key := context.Background(), h.key()
	data := photoBytes(2000, 0x42)
	h.Upload(t, key, catalog.ContentTypeJPEG, data)
	info, err := h.Store.Stat(ctx, key)
	if err != nil || info.Size != int64(len(data)) || info.ETag == "" {
		t.Fatalf("Stat = %+v, %v; want %d bytes and an ETag", info, err, len(data))
	}
	head, err := h.Store.Head(ctx, key, info.ETag, catalog.SniffLen)
	if err != nil || !bytes.Equal(head, data[:catalog.SniffLen]) {
		t.Errorf("Head = %d bytes, %v; want the first %d", len(head), err, catalog.SniffLen)
	}
	all, err := h.Store.Head(ctx, key, info.ETag, 1<<20)
	if err != nil || !bytes.Equal(all, data) {
		t.Errorf("Head past the end = %d bytes, %v; want all %d", len(all), err, len(data))
	}
	if _, _, ok := h.Read(t, key); ok {
		t.Error("a staged upload is already readable as a photo")
	}
	if err := h.Store.DeleteUpload(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Store.Stat(ctx, key); !errors.Is(err, appcatalog.ErrObjectMissing) {
		t.Errorf("Stat after DeleteUpload = %v, want ErrObjectMissing", err)
	}
}

func storeReplaced(t *testing.T, h PhotoStoreHarness) {
	ctx, key := context.Background(), h.key()
	h.Upload(t, key, catalog.ContentTypeJPEG, photoBytes(1000, 0x42))
	info, err := h.Store.Stat(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	h.Upload(t, key, catalog.ContentTypeJPEG, []byte("trocado depois da checagem"))
	if _, err := h.Store.Head(ctx, key, info.ETag, catalog.SniffLen); !errors.Is(err, appcatalog.ErrObjectChanged) {
		t.Errorf("Head at the old ETag = %v, want ErrObjectChanged", err)
	}
	if err := h.Store.Promote(ctx, key, info.ETag, catalog.ContentTypeJPEG); !errors.Is(err, appcatalog.ErrObjectChanged) {
		t.Errorf("Promote at the old ETag = %v, want ErrObjectChanged", err)
	}
	if _, _, ok := h.Read(t, key); ok {
		t.Error("the replaced upload was promoted")
	}
}

func storePromote(t *testing.T, h PhotoStoreHarness) {
	ctx, key := context.Background(), h.key()
	data := photoBytes(3000, 0x43)
	// Declared as PNG; the sniffed type is what the photo keeps.
	h.Upload(t, key, catalog.ContentTypePNG, data)
	info, err := h.Store.Stat(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if err := h.Store.Promote(ctx, key, info.ETag, catalog.ContentTypeJPEG); err != nil {
			t.Fatalf("Promote %d: %v", i+1, err)
		}
	}
	got, ct, ok := h.Read(t, key)
	if !ok || !bytes.Equal(got, data) || ct != catalog.ContentTypeJPEG {
		t.Errorf("photo = %v, %d bytes, %q; want the upload as %s", ok, len(got), ct, catalog.ContentTypeJPEG)
	}
	// The staged copy and the photo are apart.
	if err := h.Store.DeleteUpload(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := h.Read(t, key); !ok {
		t.Error("DeleteUpload removed the photo")
	}
	if err := h.Store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := h.Read(t, key); ok {
		t.Error("the photo outlived Delete")
	}
	if err := h.Store.Delete(ctx, key); err != nil {
		t.Errorf("a second Delete = %v, want nil", err)
	}
}

func storeMisuse(t *testing.T, h PhotoStoreHarness) {
	ctx, key := context.Background(), h.key()
	h.Upload(t, key, catalog.ContentTypeJPEG, photoBytes(100, 0x44))
	info, err := h.Store.Stat(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"Head of 0 bytes":     func() error { _, err := h.Store.Head(ctx, key, info.ETag, 0); return err },
		"Head with no ETag":   func() error { _, err := h.Store.Head(ctx, key, "", catalog.SniffLen); return err },
		"Head of minus bytes": func() error { _, err := h.Store.Head(ctx, key, info.ETag, -1); return err },
	} {
		err := call()
		if err == nil {
			t.Errorf("%s succeeded", name)
			continue
		}
		for _, sentinel := range []error{appcatalog.ErrObjectMissing, appcatalog.ErrObjectChanged, shared.ErrUnavailable, shared.ErrInvalid} {
			if errors.Is(err, sentinel) {
				t.Errorf("%s = %v, which matches %v; want a bug error", name, err, sentinel)
			}
		}
	}
}
