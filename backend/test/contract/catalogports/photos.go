package catalogports

import (
	"context"
	"errors"
	"slices"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// PhotosHarness is the Photos repository with what its suite needs. It is
// apart from Harness so the photos suite runs without the other
// repositories: the owners are seeded however the implementation can.
type PhotosHarness struct {
	Photos appcatalog.Photos
	Tx     appshared.TxManager
	NewID  func() shared.ID
	// NewTenant returns a store rows may belong to.
	NewTenant func(t *testing.T) shared.TenantID
	// NewItem and NewKit return an item and a kit of tenant that photos
	// may belong to.
	NewItem func(t *testing.T, tenant shared.TenantID) shared.ItemID
	NewKit  func(t *testing.T, tenant shared.TenantID) shared.KitID
}

// RunPhotosContract checks the Photos repository: positions with no gap,
// single-use ids and keys checked before the 8-photo limit, owners of the
// store only, reorder and delete, rollback and tenant isolation.
func RunPhotosContract(t *testing.T, newHarness func(t *testing.T) PhotosHarness) {
	t.Run("photos are recorded last and read by position", func(t *testing.T) { photosInsert(t, newHarness(t)) })
	t.Run("an id or a key is recorded once, before the limit", func(t *testing.T) { photosTaken(t, newHarness(t)) })
	t.Run("an owner holds at most 8 photos", func(t *testing.T) { photosLimit(t, newHarness(t)) })
	t.Run("a photo belongs to an item or kit of its store", func(t *testing.T) { photosOwner(t, newHarness(t)) })
	t.Run("reorder names every photo once", func(t *testing.T) { photosReorder(t, newHarness(t)) })
	t.Run("delete closes the gap", func(t *testing.T) { photosDelete(t, newHarness(t)) })
	t.Run("a failed transaction undoes a photo write", func(t *testing.T) { photosRollback(t, newHarness(t)) })
	t.Run("another store's photos are invisible", func(t *testing.T) { photosIsolation(t, newHarness(t)) })
	t.Run("photo calls need a transaction", func(t *testing.T) { photosNeedATransaction(t, newHarness(t)) })
}

func (h PhotosHarness) tenant(t *testing.T) tenant {
	t.Helper()
	return tenant{id: h.NewTenant(t), user: shared.UserID(h.NewID())}
}

func (h PhotosHarness) in(t *testing.T, s appshared.Scope, fn func(ctx context.Context) error) {
	t.Helper()
	if err := h.try(s, fn); err != nil {
		t.Fatal(err)
	}
}

func (h PhotosHarness) try(s appshared.Scope, fn func(ctx context.Context) error) error {
	return h.Tx.WithinTx(appshared.WithScope(context.Background(), s), fn)
}

// photo returns a photo of owner in tn with a new id and its own key.
func (h PhotosHarness) photo(t *testing.T, tn tenant, owner catalog.PhotoOwner) catalog.Photo {
	t.Helper()
	id := shared.PhotoID(h.NewID())
	p, err := catalog.NewPhoto(id, catalog.PhotoKey(tn.id, owner, id), catalog.ContentTypePNG, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// add records n new photos of owner and returns them in order.
func (h PhotosHarness) add(t *testing.T, tn tenant, owner catalog.PhotoOwner, n int) []catalog.Photo {
	t.Helper()
	out := make([]catalog.Photo, 0, n)
	for range n {
		p := h.photo(t, tn, owner)
		h.in(t, tn.scope(), func(ctx context.Context) error {
			_, err := h.Photos.Insert(ctx, tn.id, owner, p)
			return err
		})
		out = append(out, p)
	}
	return out
}

func (h PhotosHarness) ids(t *testing.T, tn tenant, owner catalog.PhotoOwner) []shared.PhotoID {
	t.Helper()
	var out []shared.PhotoID
	h.in(t, tn.scope(), func(ctx context.Context) error {
		photos, err := h.Photos.ByOwner(ctx, tn.id, owner)
		for _, p := range photos {
			out = append(out, p.ID())
		}
		return err
	})
	return out
}

func idsOf(photos []catalog.Photo) []shared.PhotoID {
	out := make([]shared.PhotoID, 0, len(photos))
	for _, p := range photos {
		out = append(out, p.ID())
	}
	return out
}

func photosInsert(t *testing.T, h PhotosHarness) {
	tn := h.tenant(t)
	item, kit := catalog.ItemPhotos(h.NewItem(t, tn.id)), catalog.KitPhotos(h.NewKit(t, tn.id))
	for i := range 3 {
		p := h.photo(t, tn, item)
		h.in(t, tn.scope(), func(ctx context.Context) error {
			pos, err := h.Photos.Insert(ctx, tn.id, item, p)
			if err != nil || pos != i {
				t.Errorf("Insert = %d, %v; want position %d", pos, err, i)
			}
			return nil
		})
	}
	kitPhotos := h.add(t, tn, kit, 1)
	h.in(t, tn.storePage(), func(ctx context.Context) error {
		got, err := h.Photos.ByID(ctx, tn.id, kitPhotos[0].ID())
		if err != nil || got.Owner != kit || got.Position != 0 || got.Photo != kitPhotos[0] {
			t.Errorf("ByID in a store page scope = %+v, %v; want the kit's photo", got, err)
		}
		photos, err := h.Photos.ByOwner(ctx, tn.id, item)
		if err != nil || len(photos) != 3 || photos[0].ContentType() != catalog.ContentTypePNG || photos[0].Size() != 2048 {
			t.Errorf("ByOwner = %v, %v; want the item's 3 photos", photos, err)
		}
		return nil
	})
	if got := h.ids(t, tn, kit); !slices.Equal(got, idsOf(kitPhotos)) {
		t.Errorf("kit photos = %v, want only its own", got)
	}
}

func photosTaken(t *testing.T, h PhotosHarness) {
	tn, other := h.tenant(t), h.tenant(t)
	item := catalog.ItemPhotos(h.NewItem(t, tn.id))
	theirs := catalog.ItemPhotos(h.NewItem(t, other.id))
	p := h.add(t, tn, item, 1)[0]
	err := h.try(tn.scope(), func(ctx context.Context) error {
		_, err := h.Photos.Insert(ctx, tn.id, item, p)
		return err
	})
	if !errors.Is(err, appcatalog.ErrPhotoTaken) {
		t.Errorf("the same photo again: %v, want ErrPhotoTaken", err)
	}
	// Another store's photo with the same id: the id is still taken.
	same, err := catalog.NewPhoto(p.ID(), catalog.PhotoKey(other.id, theirs, p.ID()), catalog.ContentTypeJPEG, 10)
	if err != nil {
		t.Fatal(err)
	}
	err = h.try(other.scope(), func(ctx context.Context) error {
		_, err := h.Photos.Insert(ctx, other.id, theirs, same)
		return err
	})
	if !errors.Is(err, appcatalog.ErrPhotoTaken) {
		t.Errorf("another store's photo with the id: %v, want ErrPhotoTaken", err)
	}
	if got := h.ids(t, other, theirs); len(got) != 0 {
		t.Errorf("the other store recorded %v", got)
	}
}

func photosLimit(t *testing.T, h PhotosHarness) {
	tn := h.tenant(t)
	item := catalog.ItemPhotos(h.NewItem(t, tn.id))
	photos := h.add(t, tn, item, 8)
	insert := func(p catalog.Photo) error {
		return h.try(tn.scope(), func(ctx context.Context) error {
			_, err := h.Photos.Insert(ctx, tn.id, item, p)
			return err
		})
	}
	if err := insert(h.photo(t, tn, item)); !errors.Is(err, catalog.ErrPhotoLimit) {
		t.Errorf("a 9th photo: %v, want ErrPhotoLimit", err)
	}
	if err := insert(photos[7]); !errors.Is(err, appcatalog.ErrPhotoTaken) {
		t.Errorf("the 8th photo again: %v, want ErrPhotoTaken, checked before the limit", err)
	}
	if got := h.ids(t, tn, item); !slices.Equal(got, idsOf(photos)) {
		t.Errorf("photos = %v", got)
	}
}

func photosOwner(t *testing.T, h PhotosHarness) {
	tn, other := h.tenant(t), h.tenant(t)
	theirs := catalog.ItemPhotos(h.NewItem(t, other.id))
	mine := catalog.ItemPhotos(h.NewItem(t, tn.id))
	noItem, noKit := catalog.ItemPhotos(shared.ItemID(h.NewID())), catalog.KitPhotos(shared.KitID(h.NewID()))
	tests := map[string]struct {
		owner catalog.PhotoOwner
		photo catalog.Photo
	}{
		"another store's item": {theirs, h.photo(t, tn, theirs)},
		"an item never made":   {noItem, h.photo(t, tn, noItem)},
		"a kit never made":     {noKit, h.photo(t, tn, noKit)},
		"another owner's key":  {mine, h.photo(t, tn, noItem)},
		"the kit folder":       {mine, h.photo(t, tn, catalog.PhotoOwner{Kind: catalog.OwnerKit, ID: mine.ID})},
	}
	for name, tt := range tests {
		err := h.try(tn.scope(), func(ctx context.Context) error {
			_, err := h.Photos.Insert(ctx, tn.id, tt.owner, tt.photo)
			return err
		})
		if !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: %v, want an error matching ErrInvalid", name, err)
		}
	}
	if got := h.ids(t, other, theirs); len(got) != 0 {
		t.Errorf("the other store's item got %v", got)
	}
}

func photosReorder(t *testing.T, h PhotosHarness) {
	tn := h.tenant(t)
	item := catalog.ItemPhotos(h.NewItem(t, tn.id))
	p := idsOf(h.add(t, tn, item, 3))
	h.in(t, tn.scope(), func(ctx context.Context) error {
		return h.Photos.Reorder(ctx, tn.id, item, []shared.PhotoID{p[2], p[0], p[1]})
	})
	want := []shared.PhotoID{p[2], p[0], p[1]}
	if got := h.ids(t, tn, item); !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		got, err := h.Photos.ByID(ctx, tn.id, p[1])
		if err != nil || got.Position != 2 {
			t.Errorf("ByID = %+v, %v; want position 2", got, err)
		}
		return nil
	})
	for name, ids := range map[string][]shared.PhotoID{
		"one missing": {p[0], p[1]}, "one twice": {p[0], p[1], p[1]},
		"a stranger": {p[0], p[1], shared.PhotoID(h.NewID())}, "none": nil,
	} {
		err := h.try(tn.scope(), func(ctx context.Context) error { return h.Photos.Reorder(ctx, tn.id, item, ids) })
		var ve *shared.ValidationError
		if !errors.As(err, &ve) || len(ve.Fields) != 1 || ve.Fields[0].Field != "photoIds" {
			t.Errorf("%s: %v, want a ValidationError on photoIds", name, err)
		}
	}
	if got := h.ids(t, tn, item); !slices.Equal(got, want) {
		t.Errorf("a refused reorder changed the order to %v", got)
	}
}

func photosDelete(t *testing.T, h PhotosHarness) {
	tn := h.tenant(t)
	item, other := catalog.ItemPhotos(h.NewItem(t, tn.id)), catalog.ItemPhotos(h.NewItem(t, tn.id))
	p := idsOf(h.add(t, tn, item, 3))
	h.in(t, tn.scope(), func(ctx context.Context) error { return h.Photos.Delete(ctx, tn.id, item, p[0]) })
	if got := h.ids(t, tn, item); !slices.Equal(got, p[1:]) {
		t.Errorf("after deleting the cover: %v, want %v", got, p[1:])
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		for i, id := range p[1:] {
			got, err := h.Photos.ByID(ctx, tn.id, id)
			if err != nil || got.Position != i {
				t.Errorf("ByID(%v) = %+v, %v; want position %d", id, got, err, i)
			}
		}
		return nil
	})
	// A new photo goes after the others, with no gap.
	h.in(t, tn.scope(), func(ctx context.Context) error {
		pos, err := h.Photos.Insert(ctx, tn.id, item, h.photo(t, tn, item))
		if err != nil || pos != 2 {
			t.Errorf("Insert after a delete = %d, %v; want position 2", pos, err)
		}
		return nil
	})
	for name, call := range map[string]func(ctx context.Context) error{
		"a deleted photo":      func(ctx context.Context) error { return h.Photos.Delete(ctx, tn.id, item, p[0]) },
		"another item's photo": func(ctx context.Context) error { return h.Photos.Delete(ctx, tn.id, other, p[1]) },
	} {
		if err := h.try(tn.scope(), call); !errors.Is(err, catalog.ErrPhotoNotFound) {
			t.Errorf("%s: %v, want ErrPhotoNotFound", name, err)
		}
	}
}

func photosRollback(t *testing.T, h PhotosHarness) {
	tn := h.tenant(t)
	item := catalog.ItemPhotos(h.NewItem(t, tn.id))
	kept := idsOf(h.add(t, tn, item, 2))
	err := h.try(tn.scope(), func(ctx context.Context) error {
		added := h.photo(t, tn, item)
		if _, err := h.Photos.Insert(ctx, tn.id, item, added); err != nil {
			return err
		}
		if err := h.Photos.Reorder(ctx, tn.id, item, []shared.PhotoID{added.ID(), kept[1], kept[0]}); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
	if got := h.ids(t, tn, item); !slices.Equal(got, kept) {
		t.Errorf("after a rollback: %v, want %v", got, kept)
	}
	err = h.try(tn.scope(), func(ctx context.Context) error {
		if err := h.Photos.Delete(ctx, tn.id, item, kept[0]); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
	if got := h.ids(t, tn, item); !slices.Equal(got, kept) {
		t.Errorf("after a rolled back delete: %v, want %v", got, kept)
	}
}

func photosIsolation(t *testing.T, h PhotosHarness) {
	a, b := h.tenant(t), h.tenant(t)
	item := catalog.ItemPhotos(h.NewItem(t, a.id))
	p := idsOf(h.add(t, a, item, 2))
	h.in(t, b.scope(), func(ctx context.Context) error {
		for _, tenant := range []shared.TenantID{a.id, b.id} {
			if _, err := h.Photos.ByID(ctx, tenant, p[0]); !errors.Is(err, catalog.ErrPhotoNotFound) {
				t.Errorf("ByID of the other store's photo (tenant %v): %v", tenant, err)
			}
			if photos, err := h.Photos.ByOwner(ctx, tenant, item); err != nil || len(photos) != 0 {
				t.Errorf("ByOwner of the other store's item = %v, %v", photos, err)
			}
		}
		return nil
	})
	for name, call := range map[string]func(ctx context.Context) error{
		"delete": func(ctx context.Context) error { return h.Photos.Delete(ctx, b.id, item, p[0]) },
		"reorder": func(ctx context.Context) error {
			return h.Photos.Reorder(ctx, b.id, item, []shared.PhotoID{p[1], p[0]})
		},
	} {
		if err := h.try(b.scope(), call); err == nil {
			t.Errorf("%s of another store's photos succeeded", name)
		}
	}
	// No tenant at all sees nothing.
	h.in(t, appshared.Scope{}, func(ctx context.Context) error {
		if photos, err := h.Photos.ByOwner(ctx, a.id, item); err != nil || len(photos) != 0 {
			t.Errorf("ByOwner with no tenant = %v, %v", photos, err)
		}
		return nil
	})
	if got := h.ids(t, a, item); !slices.Equal(got, p) {
		t.Errorf("the store's photos = %v, want %v", got, p)
	}
}

func photosNeedATransaction(t *testing.T, h PhotosHarness) {
	tn := h.tenant(t)
	item := catalog.ItemPhotos(h.NewItem(t, tn.id))
	ctx := appshared.WithScope(context.Background(), tn.scope())
	if _, err := h.Photos.ByOwner(ctx, tn.id, item); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("ByOwner: %v", err)
	}
	if _, err := h.Photos.Insert(ctx, tn.id, item, h.photo(t, tn, item)); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("Insert: %v", err)
	}
}
