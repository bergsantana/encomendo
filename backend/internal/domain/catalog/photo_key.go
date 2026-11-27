package catalog

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// OwnerKind says what a photo belongs to: an item or a kit.
type OwnerKind string

// The kinds of photo owner, as the contract's PhotoUploadRequest.ownerType
// spells them.
const (
	OwnerItem OwnerKind = "item"
	OwnerKit  OwnerKind = "kit"
)

// PhotoOwner is the item or kit a photo belongs to. The tenant is not part
// of it: it always comes from the caller (ADR 0025).
type PhotoOwner struct {
	Kind OwnerKind
	ID   shared.ID
}

// ItemPhotos is the owner of an item's photos.
func ItemPhotos(id shared.ItemID) PhotoOwner { return PhotoOwner{Kind: OwnerItem, ID: shared.ID(id)} }

// KitPhotos is the owner of a kit's photos.
func KitPhotos(id shared.KitID) PhotoOwner { return PhotoOwner{Kind: OwnerKit, ID: shared.ID(id)} }

// Validate returns ErrInvalidPhotoOwner unless the kind is item or kit and
// the id is set.
func (o PhotoOwner) Validate() error {
	if o.segment() == "" || o.ID == (shared.ID{}) {
		return fmt.Errorf("photo owner %q %v: %w", o.Kind, o.ID, ErrInvalidPhotoOwner)
	}
	return nil
}

// segment is the owner's folder in a key: items or kits; "" for an unknown
// kind.
func (o PhotoOwner) segment() string {
	switch o.Kind {
	case OwnerItem:
		return "items"
	case OwnerKit:
		return "kits"
	default:
		return ""
	}
}

// PhotoKey returns the storage key of photo id of owner in tenant:
// t/{tenant}/{items|kits}/{owner}/{photo}, every id in its lowercase
// canonical form (ADR 0025). The migration 0008_catalog holds every stored
// photo row to this key. The owner must be valid.
func PhotoKey(tenant shared.TenantID, owner PhotoOwner, id shared.PhotoID) string {
	return keyPrefix(tenant, owner) + id.String()
}

func keyPrefix(tenant shared.TenantID, owner PhotoOwner) string {
	return "t/" + tenant.String() + "/" + owner.segment() + "/" + owner.ID.String() + "/"
}

// ParsePhotoKey returns the photo id of key when key is exactly the key
// PhotoKey builds for tenant and owner: the caller's own store, this owner's
// folder, a canonical photo id, and nothing after it. Any other key,
// another store's or owner's, one with a trailing segment, "..", an
// encoded slash or an upper-case id, is ErrPhotoNotUploaded, the same answer
// as a key nobody uploaded to, so the check is no oracle (ADR 0025).
// Comparing with the key rebuilt from the parsed id refuses every other
// spelling of the same photo.
func ParsePhotoKey(key string, tenant shared.TenantID, owner PhotoOwner) (shared.PhotoID, error) {
	if tenant.IsZero() || owner.Validate() != nil {
		return shared.PhotoID{}, ErrPhotoNotUploaded
	}
	rest, ok := strings.CutPrefix(key, keyPrefix(tenant, owner))
	if !ok {
		return shared.PhotoID{}, ErrPhotoNotUploaded
	}
	id, err := shared.ParsePhotoID(rest)
	if err != nil || PhotoKey(tenant, owner, id) != key {
		return shared.PhotoID{}, ErrPhotoNotUploaded
	}
	return id, nil
}

// SniffLen is how many leading bytes SniffPhotoType needs: the 512 bytes
// content sniffing reads (ADR 0024).
const SniffLen = 512

// SniffPhotoType returns the photo content type that head, the first bytes
// of a file, shows by its signature: JPEG, PNG or WebP. Any other content,
// whatever the file's name or declared type, returns "" and false.
func SniffPhotoType(head []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
		return ContentTypeJPEG, true
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		return ContentTypePNG, true
	case len(head) >= 12 && bytes.Equal(head[0:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")):
		return ContentTypeWebP, true
	default:
		return "", false
	}
}

// IsPhotoType reports whether contentType is one the store accepts.
func IsPhotoType(contentType string) bool {
	return contentType == ContentTypeJPEG || contentType == ContentTypePNG || contentType == ContentTypeWebP
}
