package catalog_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// A deleted photo's key is not used again: the staged upload goes with the
// row, so a later confirm finds nothing (ADR 0025).
func TestDeletePhoto_TheKeyStaysSingleUse(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	owner := catalog.ItemPhotos(it.ID)
	key := h.stage(t, owner, jpegBytes)
	c, err := h.svc.ConfirmPhoto(h.owner(t), owner, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.svc.DeletePhoto(h.owner(t), owner, c.Photo.ID); err != nil {
		t.Fatal(err)
	}
	if h.storage.Uploaded(key) {
		t.Error("the staged upload outlived its photo")
	}
	if _, err := h.svc.ConfirmPhoto(h.owner(t), owner, key); !errors.Is(err, catalog.ErrPhotoNotUploaded) {
		t.Errorf("a confirm after the delete = %v, want ErrPhotoNotUploaded", err)
	}
	if ids, _ := h.itemPhotoIDs(t, it.ID); len(ids) != 0 {
		t.Errorf("the key was recorded again: %v", ids)
	}
}

// When the staged upload cannot be deleted, the row stays: deleting it
// would let the key be confirmed again.
func TestDeletePhoto_KeepsTheRowWhenTheUploadStays(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	owner := catalog.ItemPhotos(it.ID)
	c := h.confirmPhoto(t, owner, jpegBytes)
	h.storage.Fail("DeleteUpload", errBoom)
	if err := h.svc.DeletePhoto(h.owner(t), owner, c.Photo.ID); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want the outage", err)
	}
	if ids, _ := h.itemPhotoIDs(t, it.ID); !slices.Equal(ids, []shared.PhotoID{c.Photo.ID}) {
		t.Errorf("photos = %v, want the photo kept", ids)
	}
	if _, _, ok := h.storage.Promoted(catalog.PhotoKey(h.store1, owner, c.Photo.ID)); !ok {
		t.Error("the photo's object was deleted while its row stayed")
	}
}

// A promotion whose transaction failed may leave an object with no row,
// which the store is told about; a refusal before the copy leaves none.
func TestConfirmPhoto_ReportsAPossibleOrphan(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	owner := catalog.ItemPhotos(it.ID)
	key := h.stage(t, owner, jpegBytes)
	h.storage.Fail("Promote", errBoom)
	if _, err := h.svc.ConfirmPhoto(h.owner(t), owner, key); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
	if got := h.storage.Orphans(); !slices.Equal(got, []string{key}) {
		t.Errorf("orphans = %v, want %v", got, []string{key})
	}
	h.storage.Fail("Promote", nil)
	replaced := h.stage(t, owner, jpegBytes)
	h.storage.Before("Promote", func() { h.storage.Upload(replaced, catalog.ContentTypeJPEG, textBytes) })
	if _, err := h.svc.ConfirmPhoto(h.owner(t), owner, replaced); !errors.Is(err, catalog.ErrPhotoNotUploaded) {
		t.Fatalf("err = %v", err)
	}
	if got := h.storage.Orphans(); len(got) != 1 {
		t.Errorf("a refused copy was reported: %v", got)
	}
}
