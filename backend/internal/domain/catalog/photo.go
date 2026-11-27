package catalog

import (
	"fmt"
	"slices"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

const (
	maxPhotos = 8
	// MaxPhotoBytes is the largest photo the store accepts: 10 MB (ADR 0024).
	MaxPhotoBytes = 10 << 20
)

// Photo content types the store accepts, checked by content, not by name
// (ADR 0024).
const (
	ContentTypeJPEG = "image/jpeg"
	ContentTypePNG  = "image/png"
	ContentTypeWebP = "image/webp"
)

// Photo is a photo of an item or kit that was uploaded and confirmed. It is
// an immutable value; its place in the list is the position in its owner's
// photos, and the first is the cover.
type Photo struct {
	id          shared.PhotoID
	key         string
	contentType string
	size        int64
}

// NewPhoto returns the record of a stored photo: its id, the object key in
// storage, its content type and its size in bytes. It returns
// ErrPhotoInvalid for a zero id, an empty key, a type other than JPEG, PNG
// or WebP, or a size outside 1 byte to 10 MB. The upload flow checks the
// bytes themselves before it calls this (ADR 0024).
func NewPhoto(id shared.PhotoID, key, contentType string, size int64) (Photo, error) {
	if id.IsZero() || key == "" || !IsPhotoType(contentType) || size < 1 || size > MaxPhotoBytes {
		return Photo{}, fmt.Errorf("new photo %v (%q, %d bytes): %w", id, contentType, size, ErrPhotoInvalid)
	}
	return Photo{id: id, key: key, contentType: contentType, size: size}, nil
}

// ID returns the photo's id, which is the last part of its storage key.
func (p Photo) ID() shared.PhotoID { return p.id }

// Key returns the object key in storage.
func (p Photo) Key() string { return p.key }

// ContentType returns image/jpeg, image/png or image/webp.
func (p Photo) ContentType() string { return p.contentType }

// Size returns the object's size in bytes.
func (p Photo) Size() int64 { return p.size }

// photoSet holds the ordered photos of an item or a kit. Items and kits
// embed it, so they share these methods.
type photoSet struct {
	photos []Photo
}

// restorePhotos checks the photos of a stored row: at most 8, none twice, no
// zero value.
func restorePhotos(photos []Photo) (photoSet, error) {
	ids := make([]shared.PhotoID, len(photos))
	for i, p := range photos {
		ids[i] = p.id
		if p.id.IsZero() {
			return photoSet{}, fmt.Errorf("photo %d is empty: %w", i, ErrPhotoInvalid)
		}
	}
	if len(photos) > maxPhotos || hasDuplicate(ids) {
		return photoSet{}, fmt.Errorf("%d photos, repeated or too many: %w", len(photos), ErrPhotoInvalid)
	}
	return photoSet{photos: cloneSlice(photos)}, nil
}

// PhotoList is the ordered photos of one item or kit, apart from the item or
// kit itself. The photo use cases change photos through it without loading
// the owner or changing its version: photos are their own resource (plan
// 3.6). It has the same rules as an item's or kit's photos: at most 8, none
// twice, the first is the cover.
type PhotoList struct {
	photoSet
}

// NewPhotoList returns the list of stored photos, in order. It returns
// ErrPhotoInvalid for more than 8 photos, a repeated id or a zero photo.
func NewPhotoList(photos []Photo) (*PhotoList, error) {
	set, err := restorePhotos(photos)
	if err != nil {
		return nil, err
	}
	return &PhotoList{photoSet: set}, nil
}

// Full reports whether there are already 8 photos, so no other may be
// added (ErrPhotoLimit).
func (s *photoSet) Full() bool { return len(s.photos) >= maxPhotos }

// Photos returns the photos in order, the cover first. The result is a copy.
func (s *photoSet) Photos() []Photo { return cloneSlice(s.photos) }

// Cover returns the first photo, and false when there are none.
func (s *photoSet) Cover() (Photo, bool) {
	if len(s.photos) == 0 {
		return Photo{}, false
	}
	return s.photos[0], true
}

// AddPhoto puts p last. It returns ErrPhotoLimit when there are already 8
// photos and ErrPhotoInvalid for the zero Photo or an id already there;
// nothing changes then. The first photo added is the cover.
func (s *photoSet) AddPhoto(p Photo) error {
	if p.id.IsZero() || s.indexOf(p.id) >= 0 {
		return fmt.Errorf("add photo %v: %w", p.id, ErrPhotoInvalid)
	}
	if s.Full() {
		return ErrPhotoLimit
	}
	s.photos = append(s.photos, p)
	return nil
}

// RemovePhoto takes the photo out, and the next one becomes the cover if it
// was the cover. It returns ErrPhotoNotFound when the id is not there.
func (s *photoSet) RemovePhoto(id shared.PhotoID) error {
	i := s.indexOf(id)
	if i < 0 {
		return fmt.Errorf("remove photo %v: %w", id, ErrPhotoNotFound)
	}
	s.photos = slices.Delete(s.photos, i, i+1)
	return nil
}

// ReorderPhotos puts the photos in the order of ids; the first becomes the
// cover. Every photo must be listed exactly once: it returns a
// ValidationError on "photoIds" for a missing, unknown or repeated id, and
// nothing changes then.
func (s *photoSet) ReorderPhotos(ids []shared.PhotoID) error {
	if len(ids) != len(s.photos) || hasDuplicate(ids) {
		return fieldError(fieldPhotoIDs, msgPhotoOrder)
	}
	ordered := make([]Photo, 0, len(ids))
	for _, id := range ids {
		i := s.indexOf(id)
		if i < 0 {
			return fieldError(fieldPhotoIDs, msgPhotoOrder)
		}
		ordered = append(ordered, s.photos[i])
	}
	s.photos = ordered
	return nil
}

func (s *photoSet) indexOf(id shared.PhotoID) int {
	return slices.IndexFunc(s.photos, func(p Photo) bool { return p.id == id })
}
