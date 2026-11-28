package purchasing_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// photo is a valid receiving photo of tenant.
func photo(tenant shared.TenantID) purchasing.Photo {
	id := shared.PhotoID(testkit.NextID())
	return purchasing.Photo{ID: id, Key: purchasing.PhotoKey(tenant, id), ContentType: "image/jpeg", Size: 1024}
}

func photos(tenant shared.TenantID, n int) []purchasing.Photo {
	out := make([]purchasing.Photo, n)
	for i := range out {
		out[i] = photo(tenant)
	}
	return out
}

func receiptSpec(t *testing.T, entries ...purchasing.ReceiptEntry) purchasing.ReceiptSpec {
	t.Helper()
	tenant := shared.TenantID(testkit.NextID())
	return purchasing.ReceiptSpec{
		Tenant: tenant, ID: shared.ReceiptID(testkit.NextID()), By: testkit.Staff(t, tenant, shared.RoleStaff),
		Now: day1, Entries: entries,
	}
}

func entry(check purchasing.Check) purchasing.ReceiptEntry {
	e := purchasing.ReceiptEntry{Line: lineID(), Check: check}
	if check == purchasing.CheckDefective {
		e.Note = "riscado"
	}
	return e
}

func TestNewReceipt_Limits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		edit  func(*purchasing.ReceiptSpec)
		want  error
		field string
	}{
		{"no entries", func(s *purchasing.ReceiptSpec) { s.Entries = nil }, purchasing.ErrEmptyReceipt, "entries"},
		{"201 entries", func(s *purchasing.ReceiptSpec) {
			s.Entries = nil
			for range 201 {
				s.Entries = append(s.Entries, entry(purchasing.CheckOK))
			}
		}, shared.ErrInvalid, "entries"},
		{"a line twice", func(s *purchasing.ReceiptSpec) {
			s.Entries = append(s.Entries, purchasing.ReceiptEntry{Line: s.Entries[0].Line, Check: purchasing.CheckOK})
		}, purchasing.ErrLineTwice, "entries[1].lineId"},
		{"defect without a note", func(s *purchasing.ReceiptSpec) { s.Entries[0].Note = "" }, shared.ErrInvalid, "entries[0].note"},
		{"defect with a note over 500", func(s *purchasing.ReceiptSpec) { s.Entries[0].Note = strings.Repeat("x", 501) }, shared.ErrInvalid, "entries[0].note"},
		{"unknown check", func(s *purchasing.ReceiptSpec) { s.Entries[0].Check = "meh" }, shared.ErrInvalid, "entries[0].check"},
		{"5 invoice photos", func(s *purchasing.ReceiptSpec) { s.Invoice = photos(s.Tenant, 5) }, purchasing.ErrPhotoLimit, "invoicePhotoKeys"},
		{"3 photos on an entry", func(s *purchasing.ReceiptSpec) { s.Entries[0].Photos = photos(s.Tenant, 3) }, purchasing.ErrPhotoLimit, "entries[0].photoKeys"},
		{"a photo of another store", func(s *purchasing.ReceiptSpec) {
			s.Invoice = []purchasing.Photo{photo(shared.TenantID(testkit.NextID()))}
		}, purchasing.ErrPhotoKey, "invoicePhotoKeys[0]"},
		{"a photo with a foreign key", func(s *purchasing.ReceiptSpec) {
			p := photo(s.Tenant)
			p.Key += "/../x"
			s.Entries[0].Photos = []purchasing.Photo{p}
		}, purchasing.ErrPhotoKey, "entries[0].photoKeys[0]"},
		{"a photo that is a gif", func(s *purchasing.ReceiptSpec) {
			p := photo(s.Tenant)
			p.ContentType = "image/gif"
			s.Invoice = []purchasing.Photo{p}
		}, shared.ErrInvalid, "invoicePhotoKeys[0]"},
		{"an empty photo", func(s *purchasing.ReceiptSpec) {
			p := photo(s.Tenant)
			p.Size = 0
			s.Invoice = []purchasing.Photo{p}
		}, shared.ErrInvalid, "invoicePhotoKeys[0]"},
		{"a photo over 10 MB", func(s *purchasing.ReceiptSpec) {
			p := photo(s.Tenant)
			p.Size = 10<<20 + 1
			s.Invoice = []purchasing.Photo{p}
		}, shared.ErrInvalid, "invoicePhotoKeys[0]"},
		{"a photo on the invoice and on an entry", func(s *purchasing.ReceiptSpec) {
			p := photo(s.Tenant)
			s.Entries[0].Photos = []purchasing.Photo{p}
			s.Invoice = []purchasing.Photo{p}
		}, shared.ErrInvalid, "invoicePhotoKeys[0]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := receiptSpec(t, entry(purchasing.CheckDefective))
			tc.edit(&s)
			got, err := purchasing.NewReceipt(s)
			if !errors.Is(err, tc.want) || got != nil {
				t.Fatalf("NewReceipt = %v, %v; want %v", got, err, tc.want)
			}
			if f := fieldOf(t, err); f != tc.field {
				t.Errorf("field = %q, want %q", f, tc.field)
			}
		})
	}
}

func TestNewReceipt_AtTheLimits(t *testing.T) {
	t.Parallel()
	s := receiptSpec(t)
	for range 200 {
		s.Entries = append(s.Entries, entry(purchasing.CheckOK))
	}
	s.Entries[0] = entry(purchasing.CheckDefective)
	s.Entries[0].Note = "  trincado  "
	s.Entries[0].Photos = photos(s.Tenant, 2)
	s.Entries[1].Note = "an ok line keeps no note"
	s.Invoice = photos(s.Tenant, 4)
	r, err := purchasing.NewReceipt(s)
	if err != nil {
		t.Fatal(err)
	}
	got := r.Entries()
	if len(got) != 200 || len(r.Invoice()) != 4 || got[0].Note != "trincado" || got[1].Note != "" || len(got[0].Photos) != 2 ||
		r.Tenant() != s.Tenant || r.ID() != s.ID || r.By() != s.By.UserID() || !r.At().Equal(day1) {
		t.Errorf("receipt = %+v", r)
	}
	// Getters hand out copies.
	got[0].Photos[0].Key = "changed"
	r.Invoice()[0].Key = "changed"
	if r.Entries()[0].Photos[0].Key == "changed" || r.Invoice()[0].Key == "changed" {
		t.Error("getters share their slices with the receipt")
	}
}

func TestNewReceipt_MissingArgumentsAreBugs(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]func(*purchasing.ReceiptSpec){
		"tenant": func(s *purchasing.ReceiptSpec) { s.Tenant = shared.TenantID{} },
		"id":     func(s *purchasing.ReceiptSpec) { s.ID = shared.ReceiptID{} },
		"by":     func(s *purchasing.ReceiptSpec) { s.By = shared.Principal{} },
		"line":   func(s *purchasing.ReceiptSpec) { s.Entries[0].Line = shared.PurchaseLineID{} },
	} {
		s := receiptSpec(t, entry(purchasing.CheckOK))
		edit(&s)
		if _, err := purchasing.NewReceipt(s); err == nil || errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: NewReceipt = %v, want a caller bug", name, err)
		}
	}
}

func TestCounterLine(t *testing.T) {
	t.Parallel()
	today := date(t, "2026-12-05")
	supplier := shared.SupplierID(testkit.NextID())
	cost := brl(t, 4_000)
	it := item("Retrovisor", 2)

	t.Run("received at once with no supplier order", func(t *testing.T) {
		t.Parallel()
		l, err := purchasing.CounterLine(lineID(), supplier, it, &cost, purchasing.CheckOK, "", today)
		if err != nil {
			t.Fatal(err)
		}
		if l.Status() != purchasing.LineReceived || !l.SupplierOrder().IsZero() || l.Supplier() != supplier ||
			l.ReceivedOn() != today || l.Item().OrderItem != it.OrderItem || l.Cost() == nil || *l.Cost() != cost || l.Status().Open() {
			t.Errorf("line = %+v", l.Record())
		}
		if _, err := purchasing.RestoreLine(l.Record()); err != nil {
			t.Errorf("a counter line does not restore: %v", err)
		}
	})
	t.Run("defective keeps the note", func(t *testing.T) {
		t.Parallel()
		l, err := purchasing.CounterLine(lineID(), supplier, it, nil, purchasing.CheckDefective, " quebrado ", today)
		if err != nil || l.Status() != purchasing.LineDefective || l.CheckNote() != "quebrado" || l.Cost() != nil {
			t.Errorf("CounterLine = %+v, %v", l.Record(), err)
		}
	})
	t.Run("a defect needs a note", func(t *testing.T) {
		t.Parallel()
		if _, err := purchasing.CounterLine(lineID(), supplier, it, nil, purchasing.CheckDefective, "", today); !errors.Is(err, shared.ErrInvalid) || fieldOf(t, err) != "note" {
			t.Errorf("error = %v, want invalid on note", err)
		}
	})
	t.Run("refusals", func(t *testing.T) {
		t.Parallel()
		bad := it
		bad.Quantity = 100
		if _, err := purchasing.CounterLine(lineID(), supplier, bad, nil, purchasing.CheckOK, "", today); !errors.Is(err, shared.ErrInvalid) || fieldOf(t, err) != "item.quantity" {
			t.Errorf("quantity: %v", err)
		}
		if _, err := purchasing.CounterLine(lineID(), supplier, it, nil, "meh", "", today); !errors.Is(err, shared.ErrInvalid) || fieldOf(t, err) != "check" {
			t.Errorf("check: %v", err)
		}
		neg := brl(t, -1)
		if _, err := purchasing.CounterLine(lineID(), supplier, it, &neg, purchasing.CheckOK, "", today); !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("cost: %v", err)
		}
	})
	t.Run("caller bugs", func(t *testing.T) {
		t.Parallel()
		for name, f := range map[string]func() error{
			"supplier": func() error {
				_, err := purchasing.CounterLine(lineID(), shared.SupplierID{}, it, nil, purchasing.CheckOK, "", today)
				return err
			},
			"today": func() error {
				_, err := purchasing.CounterLine(lineID(), supplier, it, nil, purchasing.CheckOK, "", shared.CivilDate{})
				return err
			},
			"id": func() error {
				_, err := purchasing.CounterLine(shared.PurchaseLineID{}, supplier, it, nil, purchasing.CheckOK, "", today)
				return err
			},
		} {
			if err := f(); err == nil || errors.Is(err, shared.ErrInvalid) {
				t.Errorf("%s: error = %v, want a caller bug", name, err)
			}
		}
	})
}

func TestPhotoKey_RoundTrip(t *testing.T) {
	t.Parallel()
	tenant := shared.TenantID(testkit.NextID())
	id := shared.PhotoID(testkit.NextID())
	key := purchasing.PhotoKey(tenant, id)
	if want := "t/" + tenant.String() + "/receipts/" + id.String(); key != want {
		t.Fatalf("PhotoKey = %q, want %q", key, want)
	}
	if got, err := purchasing.ParsePhotoKey(tenant, key); err != nil || got != id {
		t.Fatalf("ParsePhotoKey = %v, %v", got, err)
	}
	other := shared.TenantID(testkit.NextID())
	for name, bad := range map[string]string{
		"another store":  purchasing.PhotoKey(other, id),
		"empty":          "",
		"no id":          "t/" + tenant.String() + "/receipts/",
		"trailing":       key + "/x",
		"dots":           "t/" + tenant.String() + "/receipts/../" + id.String(),
		"upper case":     "t/" + tenant.String() + "/receipts/" + strings.ToUpper(id.String()),
		"catalog folder": "t/" + tenant.String() + "/items/" + id.String(),
		"not an id":      "t/" + tenant.String() + "/receipts/abc",
	} {
		if _, err := purchasing.ParsePhotoKey(tenant, bad); !errors.Is(err, purchasing.ErrPhotoKey) || !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: ParsePhotoKey = %v, want ErrPhotoKey", name, err)
		}
	}
	if _, err := purchasing.ParsePhotoKey(shared.TenantID{}, key); !errors.Is(err, purchasing.ErrPhotoKey) {
		t.Errorf("zero tenant: %v", err)
	}
}
