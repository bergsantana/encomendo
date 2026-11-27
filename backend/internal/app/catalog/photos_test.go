package catalog_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The leading bytes of each accepted type, padded to a few hundred bytes.
var (
	jpegBytes = pad([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00})
	pngBytes  = pad([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR"))
	webpBytes = pad([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "))
	textBytes = []byte("isto é texto, não uma foto")
)

func pad(head []byte) []byte { return append(head, bytes.Repeat([]byte{0x42}, 600)...) }

// stage asks for an upload as the owner and stages data at its key, as the
// app's PUT would. It returns the key.
func (h *harness) stage(t *testing.T, owner catalog.PhotoOwner, data []byte) string {
	t.Helper()
	up, err := h.svc.RequestPhotoUpload(h.owner(t), appcatalog.PhotoUploadInput{Owner: owner, ContentType: catalog.ContentTypeJPEG})
	if err != nil {
		t.Fatalf("RequestPhotoUpload: %v", err)
	}
	h.storage.Upload(up.Key, catalog.ContentTypeJPEG, data)
	return up.Key
}

// confirmPhoto stages data and confirms it as the owner.
func (h *harness) confirmPhoto(t *testing.T, owner catalog.PhotoOwner, data []byte) appcatalog.PhotoConfirmation {
	t.Helper()
	c, err := h.svc.ConfirmPhoto(h.owner(t), owner, h.stage(t, owner, data))
	if err != nil {
		t.Fatalf("ConfirmPhoto: %v", err)
	}
	return c
}

// warm puts a store page catalog in the cache, so a test can see a write
// drop it.
func (h *harness) warm(t *testing.T) {
	t.Helper()
	if err := h.cache.Set(context.Background(), storeKey(t, h.store1), appcatalog.StoreCatalog{}, time.Minute); err != nil {
		t.Fatal(err)
	}
}

// fillPhotos gives the item n recorded photos directly in the store.
func (h *harness) fillPhotos(t *testing.T, id shared.ItemID, n int) []catalog.Photo {
	t.Helper()
	var photos []catalog.Photo
	for range n {
		pid := shared.PhotoID(h.ids.NewID())
		p, err := catalog.NewPhoto(pid, catalog.PhotoKey(h.store1, catalog.ItemPhotos(id), pid), catalog.ContentTypeJPEG, 100)
		if err != nil {
			t.Fatal(err)
		}
		photos = append(photos, p)
	}
	h.store.SetItemPhotos(id, photos)
	return photos
}

func (h *harness) itemPhotoIDs(t *testing.T, id shared.ItemID) ([]shared.PhotoID, int) {
	t.Helper()
	v, err := h.svc.GetItem(h.owner(t), id)
	if err != nil {
		t.Fatal(err)
	}
	var ids []shared.PhotoID
	for i, p := range v.Photos {
		if p.Position != i {
			t.Errorf("photo %d has position %d", i, p.Position)
		}
		ids = append(ids, p.ID)
	}
	return ids, v.Version
}

func TestRequestPhotoUpload_SignsAKeyOfTheCallersStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	up, err := h.svc.RequestPhotoUpload(h.admin(t), appcatalog.PhotoUploadInput{Owner: catalog.ItemPhotos(it.ID), ContentType: catalog.ContentTypePNG})
	if err != nil {
		t.Fatal(err)
	}
	prefix := "t/" + h.store1.String() + "/items/" + it.ID.String() + "/"
	if _, err := catalog.ParsePhotoKey(up.Key, h.store1, catalog.ItemPhotos(it.ID)); err != nil || !strings.HasPrefix(up.Key, prefix) {
		t.Errorf("key = %q (%v), want one under %q", up.Key, err, prefix)
	}
	if up.Method != "PUT" || up.URL != "https://uploads.test/"+up.Key || up.Headers["Content-Type"] != catalog.ContentTypePNG {
		t.Errorf("upload = %+v", up)
	}
	if !up.ExpiresAt.Equal(start.Add(5 * time.Minute)) {
		t.Errorf("expiresAt = %v, want 5 minutes from now", up.ExpiresAt)
	}
	if ids, _ := h.itemPhotoIDs(t, it.ID); len(ids) != 0 {
		t.Errorf("a request recorded photos %v", ids)
	}
	k := h.createKit(t, kitOf("Kit", 9000, one(it.ID)))
	up, err = h.svc.RequestPhotoUpload(h.admin(t), appcatalog.PhotoUploadInput{Owner: catalog.KitPhotos(k.ID), ContentType: catalog.ContentTypeWebP})
	if err != nil || !strings.HasPrefix(up.Key, "t/"+h.store1.String()+"/kits/"+k.ID.String()+"/") {
		t.Errorf("kit upload = %+v, %v", up, err)
	}
}

func TestRequestPhotoUpload_Refusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	full := h.createItem(t, inHouseInput("Cheio", 10000, 5))
	h.fillPhotos(t, full.ID, 8)
	theirs := h.createItemIn(t, h.store2)
	tests := map[string]struct {
		in   appcatalog.PhotoUploadInput
		want error
	}{
		"a gif":                {appcatalog.PhotoUploadInput{Owner: catalog.ItemPhotos(it.ID), ContentType: "image/gif"}, shared.ErrInvalid},
		"no type":              {appcatalog.PhotoUploadInput{Owner: catalog.ItemPhotos(it.ID)}, shared.ErrInvalid},
		"an unknown owner":     {appcatalog.PhotoUploadInput{Owner: catalog.PhotoOwner{Kind: "user", ID: shared.ID(it.ID)}, ContentType: catalog.ContentTypeJPEG}, catalog.ErrInvalidPhotoOwner},
		"an item never made":   {appcatalog.PhotoUploadInput{Owner: catalog.ItemPhotos(shared.ItemID(h.ids.NewID())), ContentType: catalog.ContentTypeJPEG}, catalog.ErrItemNotFound},
		"another store's item": {appcatalog.PhotoUploadInput{Owner: catalog.ItemPhotos(theirs), ContentType: catalog.ContentTypeJPEG}, catalog.ErrItemNotFound},
		"a kit never made":     {appcatalog.PhotoUploadInput{Owner: catalog.KitPhotos(shared.KitID(h.ids.NewID())), ContentType: catalog.ContentTypeJPEG}, catalog.ErrKitNotFound},
		"8 photos already":     {appcatalog.PhotoUploadInput{Owner: catalog.ItemPhotos(full.ID), ContentType: catalog.ContentTypeJPEG}, catalog.ErrPhotoLimit},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := h.svc.RequestPhotoUpload(h.owner(t), tt.in); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
	if _, err := h.svc.RequestPhotoUpload(h.owner(t), tests["a gif"].in); !slices.Equal(fieldsOf(t, err), []string{"contentType"}) {
		t.Errorf("a gif: %v, want a field error on contentType", err)
	}
	if calls := h.storage.Calls(); len(calls) != 0 {
		t.Errorf("refusals reached storage: %v", calls)
	}
}

// createItemIn adds an item to tenant as its owner.
func (h *harness) createItemIn(t *testing.T, tenant shared.TenantID) shared.ItemID {
	t.Helper()
	v, err := h.svc.CreateItem(h.as(t, tenant, shared.RoleOwner), inHouseInput("Alheio", 100, 1))
	if err != nil {
		t.Fatal(err)
	}
	return v.ID
}

func TestConfirmPhoto_RecordsTheCheckedUpload(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	owner := catalog.ItemPhotos(it.ID)
	first := h.confirmPhoto(t, owner, jpegBytes)
	h.warm(t)
	// The sniffed type is recorded, whatever the upload declared.
	key := h.stage(t, owner, pngBytes)
	second, err := h.svc.ConfirmPhoto(h.admin(t), owner, key)
	if err != nil {
		t.Fatal(err)
	}
	if first.AlreadyRecorded || second.AlreadyRecorded || first.Photo.Position != 0 || second.Photo.Position != 1 {
		t.Errorf("confirmations = %+v, %+v; want two new photos at 0 and 1", first, second)
	}
	if second.Photo.URL != "https://photos.test/"+key || !second.Photo.ExpiresAt.Equal(start.Add(10*time.Minute)) {
		t.Errorf("view = %+v, want a URL signed for 10 minutes", second.Photo)
	}
	if data, ct, ok := h.storage.Promoted(key); !ok || ct != catalog.ContentTypePNG || !bytes.Equal(data, pngBytes) {
		t.Errorf("promoted = %v, %q; want the PNG", ok, ct)
	}
	if h.cached(t, h.store1) {
		t.Error("the confirm left the store's catalog cached")
	}
	ids, version := h.itemPhotoIDs(t, it.ID)
	if !slices.Equal(ids, []shared.PhotoID{first.Photo.ID, second.Photo.ID}) || version != it.Version {
		t.Errorf("item photos = %v at version %d, want both at version %d", ids, version, it.Version)
	}
}

func TestConfirmPhoto_KitPhotosKeepTheKitVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	k := h.createKit(t, kitOf("Kit", 9000, one(it.ID)))
	c := h.confirmPhoto(t, catalog.KitPhotos(k.ID), webpBytes)
	v, err := h.svc.GetKit(h.owner(t), k.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Photos) != 1 || v.Photos[0].ID != c.Photo.ID || v.Version != k.Version {
		t.Errorf("kit = %+v at version %d, want the photo at version %d", v.Photos, v.Version, k.Version)
	}
	// The kit's photos are not the item's.
	if ids, _ := h.itemPhotoIDs(t, it.ID); len(ids) != 0 {
		t.Errorf("item photos = %v", ids)
	}
}

// Every key that is not the caller's own is photo_not_uploaded and never
// reaches storage, which would answer anything (ADR 0025).
func TestConfirmPhoto_RefusesForeignKeysWithoutTouchingStorage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	other := h.createItem(t, inHouseInput("Outro", 10000, 5))
	owner := catalog.ItemPhotos(it.ID)
	// Upper case needs hex letters: take an id that has some.
	pid := shared.PhotoID(h.ids.NewID())
	for strings.ToUpper(pid.String()) == pid.String() {
		pid = shared.PhotoID(h.ids.NewID())
	}
	good := catalog.PhotoKey(h.store1, owner, pid)
	upper := strings.TrimSuffix(good, pid.String()) + strings.ToUpper(pid.String())
	keys := map[string]string{
		"another store's key":  catalog.PhotoKey(h.store2, owner, pid),
		"another item's key":   catalog.PhotoKey(h.store1, catalog.ItemPhotos(other.ID), pid),
		"the kit folder":       catalog.PhotoKey(h.store1, catalog.PhotoOwner{Kind: catalog.OwnerKit, ID: owner.ID}, pid),
		"a trailing segment":   good + "/x",
		"a dot-dot":            strings.TrimSuffix(good, pid.String()) + "../" + other.ID.String() + "/" + pid.String(),
		"an encoded slash":     strings.TrimSuffix(good, "/"+pid.String()) + "%2F" + pid.String(),
		"an upper-case id":     upper,
		"the staging prefix":   "uploads/" + good,
		"an empty key":         "",
		"a key with no photo":  strings.TrimSuffix(good, pid.String()),
		"a name, not an id":    strings.TrimSuffix(good, pid.String()) + "foto.jpg",
		"a leading slash":      "/" + good,
		"an encoded hyphen id": strings.Replace(good, "-", "%2D", 1),
	}
	for name, key := range keys {
		h.storage.Upload(key, catalog.ContentTypeJPEG, jpegBytes) // even an object there is never read
		t.Run(name, func(t *testing.T) {
			if _, err := h.svc.ConfirmPhoto(h.owner(t), owner, key); !errors.Is(err, catalog.ErrPhotoNotUploaded) {
				t.Errorf("ConfirmPhoto(%q) = %v, want ErrPhotoNotUploaded", key, err)
			}
		})
	}
	// The other store's own owner confirming this store's key.
	if _, err := h.svc.ConfirmPhoto(h.as(t, h.store2, shared.RoleOwner), owner, good); !errors.Is(err, catalog.ErrPhotoNotUploaded) {
		t.Errorf("another store confirming = %v", err)
	}
	if calls := h.storage.Calls(); len(calls) != 0 {
		t.Errorf("foreign keys reached storage: %v", calls)
	}
	if ids, _ := h.itemPhotoIDs(t, it.ID); len(ids) != 0 {
		t.Errorf("recorded %v", ids)
	}
}

func TestConfirmPhoto_ARepeatReturnsThePhotoAndTouchesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	owner := catalog.ItemPhotos(it.ID)
	h.fillPhotos(t, it.ID, 7)
	key := h.stage(t, owner, jpegBytes)
	eighth, err := h.svc.ConfirmPhoto(h.owner(t), owner, key)
	if err != nil {
		t.Fatal(err)
	}
	h.warm(t)
	before := len(h.storage.Calls())
	// A retry of the 8th photo: not photo_limit, and no storage call.
	again, err := h.svc.ConfirmPhoto(h.admin(t), owner, key)
	if err != nil {
		t.Fatalf("repeat: %v, want the recorded photo", err)
	}
	if !again.AlreadyRecorded || again.Photo.ID != eighth.Photo.ID || again.Photo.Position != 7 {
		t.Errorf("repeat = %+v, want the 8th photo, already recorded", again)
	}
	for _, c := range h.storage.Calls()[before:] {
		if !strings.HasPrefix(c, "SignGet ") {
			t.Errorf("a repeat called storage: %s", c)
		}
	}
	if !h.cached(t, h.store1) {
		t.Error("a repeat, which writes nothing, dropped the cache")
	}
}

func TestConfirmPhoto_APhotoIDOfAnotherOwnerIsNotUploaded(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.createItem(t, inHouseInput("A", 10000, 5))
	b := h.createItem(t, inHouseInput("B", 10000, 5))
	recorded := h.fillPhotos(t, a.ID, 1)[0]
	// b's key with a's photo id: it parses for b, but the row is a's.
	key := catalog.PhotoKey(h.store1, catalog.ItemPhotos(b.ID), recorded.ID())
	h.storage.Upload(key, catalog.ContentTypeJPEG, jpegBytes)
	if _, err := h.svc.ConfirmPhoto(h.owner(t), catalog.ItemPhotos(b.ID), key); !errors.Is(err, catalog.ErrPhotoNotUploaded) {
		t.Errorf("err = %v, want ErrPhotoNotUploaded", err)
	}
	if calls := h.storage.Calls(); len(calls) != 0 {
		t.Errorf("storage was touched: %v", calls)
	}
}

func TestConfirmPhoto_RefusesWhatTheUploadIsNot(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		data   []byte
		stage  bool
		want   error
		erased bool
	}{
		"nothing uploaded":      {nil, false, catalog.ErrPhotoNotUploaded, false},
		"text named .jpg":       {textBytes, true, catalog.ErrPhotoInvalid, true},
		"an empty file":         {[]byte{}, true, catalog.ErrPhotoInvalid, true},
		"11 MB":                 {pad(append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, 11<<20)...)), true, catalog.ErrPhotoInvalid, true},
		"1 byte over 10 MB":     {append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, catalog.MaxPhotoBytes-2)...), true, catalog.ErrPhotoInvalid, true},
		"a gif":                 {pad([]byte("GIF89a")), true, catalog.ErrPhotoInvalid, true},
		"an svg":                {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), true, catalog.ErrPhotoInvalid, true},
		"exactly 10 MB of jpeg": {append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, catalog.MaxPhotoBytes-3)...), true, nil, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			it := h.createItem(t, inHouseInput("Banco", 10000, 5))
			owner := catalog.ItemPhotos(it.ID)
			up, err := h.svc.RequestPhotoUpload(h.owner(t), appcatalog.PhotoUploadInput{Owner: owner, ContentType: catalog.ContentTypeJPEG})
			if err != nil {
				t.Fatal(err)
			}
			if tt.stage {
				h.storage.Upload(up.Key, catalog.ContentTypeJPEG, tt.data)
			}
			_, err = h.svc.ConfirmPhoto(h.owner(t), owner, up.Key)
			if !errors.Is(err, tt.want) || (tt.want == nil) != (err == nil) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if h.storage.Uploaded(up.Key) == tt.erased && tt.stage {
				t.Errorf("upload still there = %v, want %v", h.storage.Uploaded(up.Key), !tt.erased)
			}
			ids, _ := h.itemPhotoIDs(t, it.ID)
			if (len(ids) == 1) != (tt.want == nil) {
				t.Errorf("recorded %v", ids)
			}
		})
	}
}

func TestConfirmPhoto_TheLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	owner := catalog.ItemPhotos(it.ID)
	key := h.stage(t, owner, jpegBytes) // asked for at 0 photos
	h.fillPhotos(t, it.ID, 8)
	if _, err := h.svc.ConfirmPhoto(h.owner(t), owner, key); !errors.Is(err, catalog.ErrPhotoLimit) {
		t.Errorf("err = %v, want ErrPhotoLimit", err)
	}
	if _, _, promoted := h.storage.Promoted(key); promoted {
		t.Error("a refused photo was promoted")
	}
}

// A confirm that loses the race to record the same key returns the photo
// the other one recorded, and is never a delete.
func TestConfirmPhoto_ALostRaceReturnsTheOtherPhoto(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	owner := catalog.ItemPhotos(it.ID)
	key := h.stage(t, owner, jpegBytes)
	id, err := catalog.ParsePhotoKey(key, h.store1, owner)
	if err != nil {
		t.Fatal(err)
	}
	// The other confirm records the key while this one reads storage.
	h.storage.Before("Head", func() {
		p, err := catalog.NewPhoto(id, key, catalog.ContentTypeJPEG, int64(len(jpegBytes)))
		if err != nil {
			t.Error(err)
		}
		h.store.SetItemPhotos(it.ID, []catalog.Photo{p})
	})
	c, err := h.svc.ConfirmPhoto(h.owner(t), owner, key)
	if err != nil || !c.AlreadyRecorded || c.Photo.ID != id {
		t.Fatalf("ConfirmPhoto = %+v, %v; want the photo the other recorded", c, err)
	}
	for _, call := range h.storage.Calls() {
		if strings.HasPrefix(call, "Delete") || strings.HasPrefix(call, "Promote") {
			t.Errorf("the loser called %s", call)
		}
	}
}

func TestConfirmPhoto_AnUploadReplacedAfterItsCheck(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"Head", "Promote"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			it := h.createItem(t, inHouseInput("Banco", 10000, 5))
			owner := catalog.ItemPhotos(it.ID)
			key := h.stage(t, owner, jpegBytes)
			// The still-valid signed URL is used again, with other bytes.
			h.storage.Before(step, func() { h.storage.Upload(key, catalog.ContentTypeJPEG, textBytes) })
			if _, err := h.svc.ConfirmPhoto(h.owner(t), owner, key); !errors.Is(err, catalog.ErrPhotoNotUploaded) {
				t.Errorf("err = %v, want ErrPhotoNotUploaded", err)
			}
			if ids, _ := h.itemPhotoIDs(t, it.ID); len(ids) != 0 {
				t.Errorf("recorded %v", ids)
			}
			if _, _, promoted := h.storage.Promoted(key); promoted {
				t.Error("the replaced upload was promoted")
			}
		})
	}
}

func TestConfirmPhoto_StorageOutagesAreNoRefusal(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"Stat", "Head", "Promote", "SignGet"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			it := h.createItem(t, inHouseInput("Banco", 10000, 5))
			owner := catalog.ItemPhotos(it.ID)
			key := h.stage(t, owner, jpegBytes)
			h.storage.Fail(step, errBoom)
			_, err := h.svc.ConfirmPhoto(h.owner(t), owner, key)
			if !errors.Is(err, errBoom) || errors.Is(err, shared.ErrInvalid) {
				t.Errorf("err = %v, want the outage and no refusal", err)
			}
			if !h.storage.Uploaded(key) {
				t.Error("an outage deleted the upload")
			}
			h.storage.Fail("SignGet", nil)
			ids, _ := h.itemPhotoIDs(t, it.ID)
			// Promote fails inside the transaction, so nothing is recorded;
			// a signing failure comes after the commit.
			if want := step == "SignGet"; (len(ids) == 1) != want {
				t.Errorf("recorded %v", ids)
			}
		})
	}
}

func TestConfirmPhoto_ARefusedUploadThatCannotBeDeletedIsStillRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	owner := catalog.ItemPhotos(it.ID)
	key := h.stage(t, owner, textBytes)
	h.storage.Fail("DeleteUpload", errBoom)
	if _, err := h.svc.ConfirmPhoto(h.owner(t), owner, key); !errors.Is(err, catalog.ErrPhotoInvalid) {
		t.Errorf("err = %v, want ErrPhotoInvalid", err)
	}
}

func TestConfirmPhoto_OwnerChecks(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	gone := catalog.ItemPhotos(shared.ItemID(h.ids.NewID()))
	key := catalog.PhotoKey(h.store1, gone, shared.PhotoID(h.ids.NewID()))
	if _, err := h.svc.ConfirmPhoto(h.owner(t), gone, key); !errors.Is(err, catalog.ErrItemNotFound) {
		t.Errorf("unknown item: %v", err)
	}
	if _, err := h.svc.ConfirmPhoto(h.owner(t), catalog.PhotoOwner{Kind: "user", ID: gone.ID}, key); !errors.Is(err, catalog.ErrInvalidPhotoOwner) {
		t.Errorf("unknown kind: %v", err)
	}
	if calls := h.storage.Calls(); len(calls) != 0 {
		t.Errorf("storage was touched: %v", calls)
	}
}

func TestOrderPhotos(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	owner := catalog.ItemPhotos(it.ID)
	a, b, c := h.confirmPhoto(t, owner, jpegBytes).Photo.ID, h.confirmPhoto(t, owner, pngBytes).Photo.ID, h.confirmPhoto(t, owner, webpBytes).Photo.ID
	h.warm(t)
	views, err := h.svc.OrderPhotos(h.admin(t), owner, []shared.PhotoID{c, a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 3 || views[0].ID != c || views[0].Position != 0 || views[2].ID != b || views[2].Position != 2 || views[0].URL == "" {
		t.Errorf("views = %+v", views)
	}
	ids, version := h.itemPhotoIDs(t, it.ID)
	if !slices.Equal(ids, []shared.PhotoID{c, a, b}) || version != it.Version {
		t.Errorf("stored order = %v at version %d", ids, version)
	}
	if h.cached(t, h.store1) {
		t.Error("the order left the cache")
	}
	stranger := shared.PhotoID(h.ids.NewID())
	for name, order := range map[string][]shared.PhotoID{
		"one missing": {c, a}, "one twice": {c, a, a}, "an unknown one": {c, a, stranger}, "none": nil,
	} {
		if _, err := h.svc.OrderPhotos(h.owner(t), owner, order); !slices.Equal(fieldsOf(t, err), []string{"photoIds"}) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if ids, _ := h.itemPhotoIDs(t, it.ID); !slices.Equal(ids, []shared.PhotoID{c, a, b}) {
		t.Errorf("a refused order changed it to %v", ids)
	}
	if _, err := h.svc.OrderPhotos(h.owner(t), catalog.ItemPhotos(shared.ItemID(h.ids.NewID())), nil); !errors.Is(err, catalog.ErrItemNotFound) {
		t.Errorf("unknown item: %v", err)
	}
}

func TestDeletePhoto(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	other := h.createItem(t, inHouseInput("Outro", 10000, 5))
	owner := catalog.ItemPhotos(it.ID)
	cover := h.confirmPhoto(t, owner, jpegBytes).Photo.ID
	next := h.confirmPhoto(t, owner, pngBytes).Photo.ID
	h.warm(t)
	if err := h.svc.DeletePhoto(h.admin(t), owner, cover); err != nil {
		t.Fatal(err)
	}
	ids, version := h.itemPhotoIDs(t, it.ID)
	if !slices.Equal(ids, []shared.PhotoID{next}) || version != it.Version {
		t.Errorf("photos = %v at version %d; want the next one as the cover", ids, version)
	}
	if _, _, ok := h.storage.Promoted(catalog.PhotoKey(h.store1, owner, cover)); ok {
		t.Error("the object is still stored")
	}
	if h.cached(t, h.store1) {
		t.Error("the delete left the cache")
	}
	for name, call := range map[string]func() error{
		"a deleted photo":       func() error { return h.svc.DeletePhoto(h.owner(t), owner, cover) },
		"another item's photo":  func() error { return h.svc.DeletePhoto(h.owner(t), catalog.ItemPhotos(other.ID), next) },
		"another store's photo": func() error { return h.svc.DeletePhoto(h.as(t, h.store2, shared.RoleOwner), owner, next) },
	} {
		if err := call(); !errors.Is(err, catalog.ErrPhotoNotFound) {
			t.Errorf("%s: %v, want ErrPhotoNotFound", name, err)
		}
	}
	// Once the row is gone, a storage failure is the adapter's to log.
	h.storage.Fail("Delete", errBoom)
	if err := h.svc.DeletePhoto(h.owner(t), owner, next); err != nil {
		t.Errorf("with storage down: %v, want the row deleted anyway", err)
	}
	if ids, _ := h.itemPhotoIDs(t, it.ID); len(ids) != 0 {
		t.Errorf("photos = %v", ids)
	}
}
