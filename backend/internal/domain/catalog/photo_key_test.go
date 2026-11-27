package catalog

import (
	"errors"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestPhotoKey_Format(t *testing.T) {
	t.Parallel()

	item, kit := ItemPhotos(itemID(2)), KitPhotos(kitID(3))
	want := "t/" + tenantID.String() + "/items/" + itemID(2).String() + "/" + photoID(4).String()
	if got := PhotoKey(tenantID, item, photoID(4)); got != want {
		t.Errorf("item key = %q, want %q", got, want)
	}
	want = "t/" + tenantID.String() + "/kits/" + kitID(3).String() + "/" + photoID(4).String()
	if got := PhotoKey(tenantID, kit, photoID(4)); got != want {
		t.Errorf("kit key = %q, want %q", got, want)
	}
}

func TestParsePhotoKey_RoundTrip(t *testing.T) {
	t.Parallel()

	for _, owner := range []PhotoOwner{ItemPhotos(itemID(2)), KitPhotos(kitID(3))} {
		id, err := ParsePhotoKey(PhotoKey(tenantID, owner, photoID(4)), tenantID, owner)
		if err != nil || id != photoID(4) {
			t.Errorf("%s: ParsePhotoKey = %v, %v", owner.Kind, id, err)
		}
	}
}

// Every key that is not exactly the caller's own is refused the same way,
// before storage is touched (ADR 0025).
func TestParsePhotoKey_RefusesEveryOtherKey(t *testing.T) {
	t.Parallel()

	owner := ItemPhotos(itemID(2))
	good := PhotoKey(tenantID, owner, photoID(4))
	other := shared.TenantID(newID(9))
	tests := map[string]string{
		"empty":                 "",
		"another store":         PhotoKey(other, owner, photoID(4)),
		"another item":          PhotoKey(tenantID, ItemPhotos(itemID(5)), photoID(4)),
		"the kit folder":        PhotoKey(tenantID, PhotoOwner{Kind: OwnerKit, ID: owner.ID}, photoID(4)),
		"a trailing segment":    good + "/x",
		"a trailing slash":      good + "/",
		"a dot-dot":             "t/" + tenantID.String() + "/items/" + itemID(2).String() + "/../" + itemID(5).String() + "/" + photoID(4).String(),
		"a dot-dot as the id":   "t/" + tenantID.String() + "/items/" + itemID(2).String() + "/..",
		"an encoded slash":      strings.Replace(good, "/"+photoID(4).String(), "%2F"+photoID(4).String(), 1),
		"an encoded id":         strings.Replace(good, "-", "%2D", 1),
		"a leading slash":       "/" + good,
		"no prefix":             strings.TrimPrefix(good, "t/"),
		"the nil id":            strings.TrimSuffix(good, photoID(4).String()) + "00000000-0000-0000-0000-000000000000",
		"not a uuid":            strings.TrimSuffix(good, photoID(4).String()) + "foto.jpg",
		"a uuid with braces":    strings.TrimSuffix(good, photoID(4).String()) + "{" + photoID(4).String() + "}",
		"a space after":         good + " ",
		"a newline after":       good + "\n",
		"a query":               good + "?x=1",
		"the uploads staging":   "uploads/" + good,
		"a doubled slash":       strings.Replace(good, "/items/", "//items/", 1),
		"a backslash":           strings.Replace(good, "/items/", "\\items/", 1),
		"an id with no hyphens": strings.TrimSuffix(good, photoID(4).String()) + strings.ReplaceAll(photoID(4).String(), "-", ""),
	}
	for name, key := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParsePhotoKey(key, tenantID, owner); !errors.Is(err, ErrPhotoNotUploaded) {
				t.Errorf("ParsePhotoKey(%q) = %v, want ErrPhotoNotUploaded", key, err)
			}
		})
	}
}

// ParseID reads either case, so the upper-case spelling of a real key must be
// refused by the exact comparison. The ids here have hex letters.
func TestParsePhotoKey_RefusesUpperCase(t *testing.T) {
	t.Parallel()

	parse := func(s string) shared.ID {
		id, err := shared.ParseID(s)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	tenant := shared.TenantID(parse("0192f3a4-b5c6-7d8e-9fab-cdef01234567"))
	owner := ItemPhotos(shared.ItemID(parse("0192f3a4-b5c6-7d8e-afab-cdef01234568")))
	photo := shared.PhotoID(parse("0192f3a4-b5c6-7d8e-bfab-cdef01234569"))
	good := PhotoKey(tenant, owner, photo)
	if _, err := ParsePhotoKey(good, tenant, owner); err != nil {
		t.Fatalf("the canonical key: %v", err)
	}
	for name, key := range map[string]string{
		"an upper-case photo id": strings.TrimSuffix(good, photo.String()) + strings.ToUpper(photo.String()),
		"an upper-case store":    strings.Replace(good, tenant.String(), strings.ToUpper(tenant.String()), 1),
		"an upper-case owner":    strings.Replace(good, owner.ID.String(), strings.ToUpper(owner.ID.String()), 1),
		"upper-case everything":  strings.ToUpper(good),
	} {
		if _, err := ParsePhotoKey(key, tenant, owner); !errors.Is(err, ErrPhotoNotUploaded) {
			t.Errorf("%s: %q = %v, want ErrPhotoNotUploaded", name, key, err)
		}
	}
}

func TestParsePhotoKey_RefusesAnInvalidCaller(t *testing.T) {
	t.Parallel()

	owner := ItemPhotos(itemID(2))
	key := PhotoKey(tenantID, owner, photoID(4))
	for name, call := range map[string]func() error{
		"no tenant": func() error { _, err := ParsePhotoKey(key, shared.TenantID{}, owner); return err },
		"unknown kind": func() error {
			_, err := ParsePhotoKey(key, tenantID, PhotoOwner{Kind: "user", ID: owner.ID})
			return err
		},
		"no owner id":   func() error { _, err := ParsePhotoKey(key, tenantID, PhotoOwner{Kind: OwnerItem}); return err },
		"the zero kind": func() error { _, err := ParsePhotoKey(key, tenantID, PhotoOwner{ID: owner.ID}); return err },
	} {
		if err := call(); !errors.Is(err, ErrPhotoNotUploaded) {
			t.Errorf("%s: %v, want ErrPhotoNotUploaded", name, err)
		}
	}
}

func FuzzParsePhotoKey(f *testing.F) {
	owner := ItemPhotos(itemID(2))
	good := PhotoKey(tenantID, owner, photoID(4))
	for _, seed := range []string{good, good + "/x", "uploads/" + good, strings.ToUpper(good), ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, key string) {
		id, err := ParsePhotoKey(key, tenantID, owner)
		if err != nil {
			if !errors.Is(err, ErrPhotoNotUploaded) {
				t.Fatalf("error %v is not ErrPhotoNotUploaded", err)
			}
			return
		}
		// Whatever parses is exactly the key the store would issue.
		if PhotoKey(tenantID, owner, id) != key {
			t.Fatalf("%q parsed to %v, which is another key", key, id)
		}
	})
}

func TestPhotoOwner_Validate(t *testing.T) {
	t.Parallel()

	if err := ItemPhotos(itemID(1)).Validate(); err != nil {
		t.Errorf("item: %v", err)
	}
	if err := KitPhotos(kitID(1)).Validate(); err != nil {
		t.Errorf("kit: %v", err)
	}
	for name, o := range map[string]PhotoOwner{
		"unknown kind": {Kind: "user", ID: newID(1)},
		"no id":        {Kind: OwnerKit},
	} {
		if err := o.Validate(); !errors.Is(err, ErrInvalidPhotoOwner) || !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalidPhotoOwner", name, err)
		}
	}
}

func TestSniffPhotoType(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		head []byte
		want string
		ok   bool
	}{
		"jpeg":            {[]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F'}, ContentTypeJPEG, true},
		"png":             {[]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR"), ContentTypePNG, true},
		"webp":            {[]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), ContentTypeWebP, true},
		"text":            {[]byte("isto não é uma foto"), "", false},
		"html":            {[]byte("<html><script>alert(1)</script>"), "", false},
		"gif":             {[]byte("GIF89a\x01\x00\x01\x00"), "", false},
		"svg":             {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "", false},
		"a riff wave":     {[]byte("RIFF\x24\x00\x00\x00WAVEfmt "), "", false},
		"a short riff":    {[]byte("RIFF\x24\x00"), "", false},
		"half a jpeg":     {[]byte{0xFF, 0xD8}, "", false},
		"half a png":      {[]byte("\x89PNG"), "", false},
		"empty":           {nil, "", false},
		"jpeg late start": {append([]byte{0}, 0xFF, 0xD8, 0xFF), "", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, ok := SniffPhotoType(tt.head)
			if got != tt.want || ok != tt.ok {
				t.Errorf("SniffPhotoType = %q, %v, want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPhotoList(t *testing.T) {
	t.Parallel()

	var photos []Photo
	for n := byte(1); n <= 7; n++ {
		photos = append(photos, newPhoto(t, n))
	}
	l, err := NewPhotoList(photos)
	if err != nil {
		t.Fatal(err)
	}
	if l.Full() {
		t.Error("7 photos are full")
	}
	if err := l.AddPhoto(newPhoto(t, 8)); err != nil {
		t.Fatal(err)
	}
	if !l.Full() || !errors.Is(l.AddPhoto(newPhoto(t, 9)), ErrPhotoLimit) {
		t.Error("an 8th photo does not fill the list")
	}
	if _, err := NewPhotoList(append(photos, photos[0])); !errors.Is(err, ErrPhotoInvalid) {
		t.Errorf("repeated photo: %v", err)
	}
	if _, err := NewPhotoList(append(photos, newPhoto(t, 8), newPhoto(t, 9))); !errors.Is(err, ErrPhotoInvalid) {
		t.Errorf("9 photos: %v", err)
	}
}
