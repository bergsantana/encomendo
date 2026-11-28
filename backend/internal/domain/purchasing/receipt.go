package purchasing

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Receipt limits (ADR 0031).
const (
	maxReceiptEntries = 200
	maxInvoicePhotos  = 4
	maxEntryPhotos    = 2
	// maxReceiptPhotos caps the storage calls one receipt makes, so a
	// request cannot hold its transaction over hundreds of them.
	maxReceiptPhotos = 20
)

// CounterSpec is an item received while still in "A comprar" (bought at
// the counter), with its check. It has no cost: the repository copies it,
// as for a sent line.
type CounterSpec struct {
	ID       shared.PurchaseLineID
	Supplier shared.SupplierID
	Item     Item
	Check    Check
	Note     string
	Today    shared.CivilDate
}

// CounterLine is a line for an item received while still in "A comprar":
// received or defective at once, on s.Today, with no supplier order. It
// returns a ValidationError on "item.name", "item.quantity", "check" or
// "note" for a bad value; a defect needs a note of 1 to 500 characters.
func CounterLine(s CounterSpec) (Line, error) {
	if s.Supplier.IsZero() {
		return Line{}, fmt.Errorf("counter line %v: supplier: %w", s.ID, errMissing)
	}
	l, err := newLine(LineSpec{ID: s.ID, Item: s.Item}, s.Supplier, shared.SupplierOrderID{}, LineSent, "item")
	if err != nil {
		return Line{}, err
	}
	if s.Today.IsZero() {
		return Line{}, fmt.Errorf("counter line %v: today: %w", s.ID, errMissing)
	}
	kept, err := checkResult(s.Check, s.Note, "")
	if err != nil {
		return Line{}, err
	}
	l.receive(s.Check, kept, s.Today)
	return l, nil
}

// Photo is a stored receiving photo: its id, the object key under
// t/{tenant}/receipts/, its content type and its size in bytes.
type Photo struct {
	ID          shared.PhotoID
	Key         string
	ContentType string
	Size        int64
}

// ReceiptEntry is the check of one line in a receipt: Tudo certo or Com
// defeito with its note, and up to two photos of it.
type ReceiptEntry struct {
	Line   shared.PurchaseLineID
	Check  Check
	Note   string
	Photos []Photo
}

// ReceiptSpec is what NewReceipt needs.
type ReceiptSpec struct {
	Tenant  shared.TenantID
	ID      shared.ReceiptID
	By      shared.Principal
	Now     time.Time
	Entries []ReceiptEntry // 1..200, one line each, no line twice
	Invoice []Photo        // 0..4
}

// Receipt is one save on Receber itens: the checks of the lines that
// arrived, with the invoice photos. It is written once and never changed.
type Receipt struct {
	tenant  shared.TenantID
	id      shared.ReceiptID
	by      shared.UserID
	at      time.Time
	entries []ReceiptEntry
	invoice []Photo
}

// NewReceipt validates and returns a receipt. It returns ErrEmptyReceipt
// for no entry, ErrLineTwice for a line checked twice, ErrPhotoLimit for
// more than 4 invoice photos (field "invoicePhotoKeys"), 2 on an entry
// ("entries[i].photoKeys") or 20 in all ("invoicePhotoKeys"), ErrPhotoKey for a photo that is not under this
// store's receiving folder, and a ValidationError for a missing or too
// long defect note ("entries[i].note"), an unknown check
// ("entries[i].check"), more than 200 entries or a photo that is not a
// JPEG, PNG or WebP of 1 byte to 10 MB, or is repeated.
func NewReceipt(spec ReceiptSpec) (*Receipt, error) {
	if spec.Tenant.IsZero() || spec.ID.IsZero() || spec.By.UserID().IsZero() || spec.Now.IsZero() {
		return nil, fmt.Errorf("new receipt: %w", errMissing)
	}
	seen := map[shared.PhotoID]bool{}
	entries, err := checkEntries(spec, seen)
	if err != nil {
		return nil, err
	}
	if len(spec.Invoice) > maxInvoicePhotos {
		return nil, refuse(ErrPhotoLimit, "invoicePhotoKeys", msgInvoicePhotos)
	}
	if photoCount(entries)+len(spec.Invoice) > maxReceiptPhotos {
		return nil, refuse(ErrPhotoLimit, "invoicePhotoKeys", msgReceiptPhotos)
	}
	for i, p := range spec.Invoice {
		if err := checkPhoto(spec.Tenant, p, seen, fmt.Sprintf("invoicePhotoKeys[%d]", i)); err != nil {
			return nil, err
		}
	}
	return &Receipt{
		tenant: spec.Tenant, id: spec.ID, by: spec.By.UserID(), at: spec.Now,
		entries: entries, invoice: slices.Clone(spec.Invoice),
	}, nil
}

// checkEntries validates the entries of a receipt and returns copies with
// the notes the lines will keep.
func checkEntries(spec ReceiptSpec, photos map[shared.PhotoID]bool) ([]ReceiptEntry, error) {
	switch n := len(spec.Entries); {
	case n == 0:
		return nil, refuse(ErrEmptyReceipt, "entries", msgNoEntries)
	case n > maxReceiptEntries:
		return nil, fieldError("entries", msgTooManyLines)
	}
	lines := make(map[shared.PurchaseLineID]bool, len(spec.Entries))
	out := make([]ReceiptEntry, len(spec.Entries))
	for i, e := range spec.Entries {
		field := fmt.Sprintf("entries[%d]", i)
		if e.Line.IsZero() {
			return nil, fmt.Errorf("%s: line id: %w", field, errMissing)
		}
		if lines[e.Line] {
			return nil, refuse(ErrLineTwice, field+".lineId", msgLineTwice)
		}
		lines[e.Line] = true
		note, err := checkResult(e.Check, e.Note, field+".")
		if err != nil {
			return nil, err
		}
		if len(e.Photos) > maxEntryPhotos {
			return nil, refuse(ErrPhotoLimit, field+".photoKeys", msgEntryPhotos)
		}
		for j, p := range e.Photos {
			if err := checkPhoto(spec.Tenant, p, photos, fmt.Sprintf("%s.photoKeys[%d]", field, j)); err != nil {
				return nil, err
			}
		}
		out[i] = ReceiptEntry{Line: e.Line, Check: e.Check, Note: note, Photos: slices.Clone(e.Photos)}
	}
	return out, nil
}

// checkPhoto validates one photo and marks its id as seen, refusing one
// seen before. Its key must be the one PhotoKey builds for the store, so a
// key into another store's folder never gets in.
func checkPhoto(tenant shared.TenantID, p Photo, seen map[shared.PhotoID]bool, field string) error {
	switch {
	case p.ID.IsZero() || p.Key != PhotoKey(tenant, p.ID):
		return refuse(ErrPhotoKey, field, msgPhoto)
	case !catalog.IsPhotoType(p.ContentType) || p.Size < 1 || p.Size > catalog.MaxPhotoBytes || seen[p.ID]:
		return fieldError(field, msgPhoto)
	}
	seen[p.ID] = true
	return nil
}

// PhotoKey returns a receiving photo's object key: t/{tenant}/receipts/{photo},
// each id in its lowercase canonical form (ADR 0028, ADR 0031).
func PhotoKey(tenant shared.TenantID, photo shared.PhotoID) string {
	return keyPrefix(tenant) + photo.String()
}

func keyPrefix(tenant shared.TenantID) string {
	return "t/" + tenant.String() + "/receipts/"
}

// ParsePhotoKey returns the photo id of key when key is exactly the key
// PhotoKey builds for tenant. It returns ErrPhotoKey for anything else:
// another store's folder, a trailing segment, "..", an upper-case id. It
// says nothing about whether the object exists.
func ParsePhotoKey(tenant shared.TenantID, key string) (shared.PhotoID, error) {
	if tenant.IsZero() {
		return shared.PhotoID{}, ErrPhotoKey
	}
	rest, ok := strings.CutPrefix(key, keyPrefix(tenant))
	if !ok {
		return shared.PhotoID{}, ErrPhotoKey
	}
	id, err := shared.ParsePhotoID(rest)
	if err != nil || PhotoKey(tenant, id) != key {
		return shared.PhotoID{}, ErrPhotoKey
	}
	return id, nil
}

// Tenant returns the store the receipt belongs to.
func (r *Receipt) Tenant() shared.TenantID { return r.tenant }

// ID returns the receipt's id.
func (r *Receipt) ID() shared.ReceiptID { return r.id }

// By returns who saved the receipt.
func (r *Receipt) By() shared.UserID { return r.by }

// At returns when the receipt was saved.
func (r *Receipt) At() time.Time { return r.at }

// Entries returns the checks, in the order given. The result is a copy.
func (r *Receipt) Entries() []ReceiptEntry {
	out := make([]ReceiptEntry, len(r.entries))
	for i, e := range r.entries {
		e.Photos = slices.Clone(e.Photos)
		out[i] = e
	}
	return out
}

// Invoice returns the invoice photos. The result is a copy.
func (r *Receipt) Invoice() []Photo { return slices.Clone(r.invoice) }

// photoCount returns the photos of entries.
func photoCount(entries []ReceiptEntry) int {
	n := 0
	for _, e := range entries {
		n += len(e.Photos)
	}
	return n
}
