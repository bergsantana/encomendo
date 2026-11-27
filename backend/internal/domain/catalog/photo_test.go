package catalog

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func newPhoto(t testing.TB, n byte) Photo {
	t.Helper()
	p, err := NewPhoto(photoID(n), "t/tenant/items/item/"+photoID(n).String(), ContentTypeJPEG, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func photoIDs(s interface{ Photos() []Photo }) []shared.PhotoID {
	var out []shared.PhotoID
	for _, p := range s.Photos() {
		out = append(out, p.ID())
	}
	return out
}

func TestNewPhoto(t *testing.T) {
	t.Parallel()

	p, err := NewPhoto(photoID(1), "t/x/items/y/z", ContentTypeWebP, MaxPhotoBytes)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID() != photoID(1) || p.Key() != "t/x/items/y/z" || p.ContentType() != ContentTypeWebP || p.Size() != MaxPhotoBytes {
		t.Errorf("photo = %+v", p)
	}
	for _, ct := range []string{ContentTypeJPEG, ContentTypePNG, ContentTypeWebP} {
		if _, err := NewPhoto(photoID(1), "k", ct, 1); err != nil {
			t.Errorf("content type %s refused: %v", ct, err)
		}
	}
}

func TestNewPhoto_Refusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   shared.PhotoID
		key  string
		ct   string
		size int64
	}{
		{"zero id", shared.PhotoID{}, "k", ContentTypeJPEG, 10},
		{"empty key", photoID(1), "", ContentTypeJPEG, 10},
		{"gif", photoID(1), "k", "image/gif", 10},
		{"no type", photoID(1), "k", "", 10},
		{"empty file", photoID(1), "k", ContentTypePNG, 0},
		{"negative size", photoID(1), "k", ContentTypePNG, -1},
		{"over 10 MB", photoID(1), "k", ContentTypePNG, MaxPhotoBytes + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewPhoto(tt.id, tt.key, tt.ct, tt.size)
			if !errors.Is(err, ErrPhotoInvalid) || !errors.Is(err, shared.ErrInvalid) {
				t.Errorf("error = %v, want ErrPhotoInvalid (400)", err)
			}
		})
	}
}

func TestPhotos_AddRemoveReorder(t *testing.T) {
	t.Parallel()

	i := newItem(t, 1, toOrderInput(t))
	for _, n := range []byte{1, 2, 3} {
		if err := i.AddPhoto(newPhoto(t, n)); err != nil {
			t.Fatal(err)
		}
	}
	if got := photoIDs(i); len(got) != 3 || got[0] != photoID(1) || got[2] != photoID(3) {
		t.Fatalf("photos = %v, want 1 2 3 in order", got)
	}

	if err := i.ReorderPhotos([]shared.PhotoID{photoID(3), photoID(1), photoID(2)}); err != nil {
		t.Fatal(err)
	}
	if cover, _ := i.Cover(); cover.ID() != photoID(3) {
		t.Errorf("cover = %v, want 3 after the reorder", cover.ID())
	}

	if err := i.RemovePhoto(photoID(3)); err != nil {
		t.Fatal(err)
	}
	if cover, _ := i.Cover(); cover.ID() != photoID(1) {
		t.Errorf("cover = %v, want 1 after the cover was removed", cover.ID())
	}
	if err := i.RemovePhoto(photoID(3)); !errors.Is(err, ErrPhotoNotFound) || !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("removing a missing photo = %v, want ErrPhotoNotFound", err)
	}
}

func TestPhotos_AddRefusals(t *testing.T) {
	t.Parallel()

	k := newKit(t, 1, validKitInput(t), validLookup(t))
	if err := k.AddPhoto(newPhoto(t, 1)); err != nil {
		t.Fatal(err)
	}
	if err := k.AddPhoto(newPhoto(t, 1)); !errors.Is(err, ErrPhotoInvalid) {
		t.Errorf("the same photo twice = %v, want ErrPhotoInvalid", err)
	}
	if err := k.AddPhoto(Photo{}); !errors.Is(err, ErrPhotoInvalid) {
		t.Errorf("the zero photo = %v, want ErrPhotoInvalid", err)
	}
	if len(k.Photos()) != 1 {
		t.Errorf("photos = %d, want 1", len(k.Photos()))
	}
	for n := byte(2); n <= 8; n++ {
		if err := k.AddPhoto(newPhoto(t, n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := k.AddPhoto(newPhoto(t, 9)); !errors.Is(err, ErrPhotoLimit) {
		t.Errorf("ninth photo = %v, want ErrPhotoLimit", err)
	}
}

func TestPhotos_ReorderMustListEveryPhotoOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ids  []shared.PhotoID
	}{
		{"a photo left out", []shared.PhotoID{photoID(1), photoID(2)}},
		{"a photo listed twice", []shared.PhotoID{photoID(1), photoID(1), photoID(2)}},
		{"an unknown photo", []shared.PhotoID{photoID(1), photoID(2), photoID(9)}},
		{"an extra photo", []shared.PhotoID{photoID(1), photoID(2), photoID(3), photoID(9)}},
		{"nothing", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			i := newItem(t, 1, toOrderInput(t))
			for _, n := range []byte{1, 2, 3} {
				if err := i.AddPhoto(newPhoto(t, n)); err != nil {
					t.Fatal(err)
				}
			}
			wantFields(t, i.ReorderPhotos(tt.ids), map[string]string{"photoIds": msgPhotoOrder})
			if got := photoIDs(i); got[0] != photoID(1) || got[1] != photoID(2) || got[2] != photoID(3) {
				t.Errorf("a refused reorder changed the order: %v", got)
			}
		})
	}
}

func TestPhotos_ReturnedSliceIsACopy(t *testing.T) {
	t.Parallel()

	i := newItem(t, 1, toOrderInput(t))
	if err := i.AddPhoto(newPhoto(t, 1)); err != nil {
		t.Fatal(err)
	}
	i.Photos()[0] = Photo{}
	if cover, _ := i.Cover(); cover.ID() != photoID(1) {
		t.Error("changing the returned photos changed the item")
	}
}
