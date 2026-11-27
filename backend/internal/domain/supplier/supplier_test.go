package supplier

import (
	"errors"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	tenantID   = shared.TenantID(shared.NewV7(1, [10]byte{1}))
	supplierID = shared.SupplierID(shared.NewV7(2, [10]byte{2}))
)

const goodPhone = "5511987654321"

// fieldsOf returns the field names and messages of a ValidationError, or
// fails the test when err is not one.
func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %v, want a ValidationError", err)
	}
	if !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("error %v does not match shared.ErrInvalid", err)
	}
	out := make(map[string]string, len(ve.Fields))
	for _, f := range ve.Fields {
		out[f.Field] = f.Message
	}
	return out
}

func TestNew(t *testing.T) {
	t.Parallel()

	s, err := New(tenantID, supplierID, "  Couros Paulista  ", goodPhone)
	if err != nil {
		t.Fatal(err)
	}
	if s.TenantID() != tenantID || s.ID() != supplierID {
		t.Errorf("tenant %v id %v", s.TenantID(), s.ID())
	}
	if s.Name() != "Couros Paulista" {
		t.Errorf("Name() = %q, want it trimmed", s.Name())
	}
	if s.WhatsAppPhone() != goodPhone || s.Version() != 1 {
		t.Errorf("phone %q version %d", s.WhatsAppPhone(), s.Version())
	}
}

func TestNew_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, wantMsg string // wantMsg "" means accepted
	}{
		{"one character", "A", ""},
		{"accented", "Peças Ágil", ""},
		{"120 characters", strings.Repeat("a", 120), ""},
		{"120 multi-byte characters", strings.Repeat("ç", 120), ""},
		{"empty", "", "Informe o nome do fornecedor."},
		{"blank", " \t ", "Informe o nome do fornecedor."},
		{"121 characters", strings.Repeat("a", 121), "Use um nome de até 120 caracteres."},
		{"control character", "Couros\nPaulista", "Use um nome sem quebras de linha ou caracteres de controle."},
		{"invalid UTF-8", "Couros\xff", "Use um nome sem quebras de linha ou caracteres de controle."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := New(tenantID, supplierID, tt.in, goodPhone)
			if tt.wantMsg == "" {
				if err != nil {
					t.Errorf("New(%q) error = %v", tt.in, err)
				}
				return
			}
			if got := fieldsOf(t, err)["name"]; got != tt.wantMsg {
				t.Errorf("New(%q) name message = %q, want %q", tt.in, got, tt.wantMsg)
			}
		})
	}
}

func TestNew_Phone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, wantMsg string // wantMsg "" means accepted
	}{
		{"mobile", "5592991234567", ""},
		{"landline", "551133334444", ""},
		{"empty", "", "Informe o WhatsApp do fornecedor."},
		{"formatted", "(11) 98765-4321", "Confira o número: use DDD e 8 ou 9 dígitos."},
		{"without country code", "11987654321", "Confira o número: use DDD e 8 ou 9 dígitos."},
		{"plus sign", "+5511987654321", "Confira o número: use DDD e 8 ou 9 dígitos."},
		{"DDD with a zero", "5501987654321", "Confira o número: use DDD e 8 ou 9 dígitos."},
		{"too long", "55119876543210", "Confira o número: use DDD e 8 ou 9 dígitos."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := New(tenantID, supplierID, "Couros", tt.in)
			if tt.wantMsg == "" {
				if err != nil {
					t.Errorf("New(%q) error = %v", tt.in, err)
				}
				return
			}
			if got := fieldsOf(t, err)["whatsappPhone"]; got != tt.wantMsg {
				t.Errorf("New(%q) phone message = %q, want %q", tt.in, got, tt.wantMsg)
			}
		})
	}
}

func TestNew_ReportsEveryInvalidField(t *testing.T) {
	t.Parallel()

	_, err := New(tenantID, supplierID, "", "")
	got := fieldsOf(t, err)
	if len(got) != 2 || got["name"] == "" || got["whatsappPhone"] == "" {
		t.Errorf("fields = %v, want name and whatsappPhone", got)
	}
}

func TestNew_RejectsZeroIDs(t *testing.T) {
	t.Parallel()

	if _, err := New(shared.TenantID{}, supplierID, "Couros", goodPhone); !errors.Is(err, ErrInvalidSupplier) {
		t.Errorf("zero tenant: %v", err)
	}
	if _, err := New(tenantID, shared.SupplierID{}, "Couros", goodPhone); !errors.Is(err, ErrInvalidSupplier) {
		t.Errorf("zero id: %v", err)
	}
}

func TestSupplier_Rename(t *testing.T) {
	t.Parallel()

	s, _ := New(tenantID, supplierID, "Couros", goodPhone)
	if err := s.Rename("  Couros Paulista "); err != nil || s.Name() != "Couros Paulista" {
		t.Errorf("Rename = %v, name %q", err, s.Name())
	}
	for _, bad := range []string{"", "   ", strings.Repeat("a", 121)} {
		err := s.Rename(bad)
		if fieldsOf(t, err)["name"] == "" {
			t.Errorf("Rename(%q) = %v, want a name error", bad, err)
		}
		if s.Name() != "Couros Paulista" {
			t.Errorf("a refused Rename changed the name to %q", s.Name())
		}
	}
}

func TestSupplier_ChangePhone(t *testing.T) {
	t.Parallel()

	s, _ := New(tenantID, supplierID, "Couros", goodPhone)
	if err := s.ChangePhone("551133334444"); err != nil || s.WhatsAppPhone() != "551133334444" {
		t.Errorf("ChangePhone = %v, phone %q", err, s.WhatsAppPhone())
	}
	for _, bad := range []string{"", "11987654321", "5511 987654321"} {
		if err := s.ChangePhone(bad); fieldsOf(t, err)["whatsappPhone"] == "" {
			t.Errorf("ChangePhone(%q) = %v, want a phone error", bad, err)
		}
		if s.WhatsAppPhone() != "551133334444" {
			t.Errorf("a refused ChangePhone changed the phone to %q", s.WhatsAppPhone())
		}
	}
}

func TestSupplier_CheckVersion(t *testing.T) {
	t.Parallel()

	s, _ := Restore(Record{Tenant: tenantID, ID: supplierID, Name: "Couros", Phone: goodPhone, Version: 3})
	if err := s.CheckVersion(3); err != nil {
		t.Errorf("CheckVersion(3) = %v", err)
	}
	for _, stale := range []int{0, 2, 4} {
		err := s.CheckVersion(stale)
		if !errors.Is(err, shared.ErrVersionConflict) || !errors.Is(err, shared.ErrConflict) {
			t.Errorf("CheckVersion(%d) = %v, want ErrVersionConflict", stale, err)
		}
	}
}

func TestRestore(t *testing.T) {
	t.Parallel()

	good := Record{Tenant: tenantID, ID: supplierID, Name: "Couros Paulista", Phone: goodPhone, Version: 5}
	s, err := Restore(good)
	if err != nil {
		t.Fatal(err)
	}
	if s.Version() != 5 || s.Name() != "Couros Paulista" || s.WhatsAppPhone() != goodPhone {
		t.Errorf("restored = %q %q v%d", s.Name(), s.WhatsAppPhone(), s.Version())
	}

	tests := []struct {
		name   string
		mutate func(*Record)
	}{
		{"version below 1", func(r *Record) { r.Version = 0 }},
		{"zero tenant", func(r *Record) { r.Tenant = shared.TenantID{} }},
		{"zero id", func(r *Record) { r.ID = shared.SupplierID{} }},
		{"blank name", func(r *Record) { r.Name = "" }},
		{"untrimmed name", func(r *Record) { r.Name = " Couros" }},
		{"bad phone", func(r *Record) { r.Phone = "11987654321" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := good
			tt.mutate(&r)
			if _, err := Restore(r); !errors.Is(err, ErrInvalidSupplier) {
				t.Errorf("Restore = %v, want ErrInvalidSupplier", err)
			} else if errors.Is(err, shared.ErrInvalid) {
				t.Errorf("a bad stored row matched shared.ErrInvalid, so the API would answer 400: %v", err)
			}
		})
	}
}

func TestErrNotFound_MatchesSharedNotFound(t *testing.T) {
	t.Parallel()

	if !errors.Is(ErrNotFound, shared.ErrNotFound) {
		t.Error("ErrNotFound does not match shared.ErrNotFound")
	}
}

func FuzzNew(f *testing.F) {
	for _, seed := range [][2]string{
		{"Couros Paulista", goodPhone},
		{"", ""},
		{"  ", "5511"},
		{"a\x00b", "551133334444"},
	} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, name, phone string) {
		s, err := New(tenantID, supplierID, name, phone)
		if err != nil {
			return
		}
		// Whatever is accepted fits the contract and the database.
		if n := len([]rune(s.Name())); n < 1 || n > 120 || s.Name() != strings.TrimSpace(s.Name()) {
			t.Errorf("accepted name %q", s.Name())
		}
		if !shared.ValidWhatsAppPhone(s.WhatsAppPhone()) {
			t.Errorf("accepted phone %q", s.WhatsAppPhone())
		}
		if _, err := Restore(Record{Tenant: tenantID, ID: supplierID, Name: s.Name(), Phone: s.WhatsAppPhone(), Version: 1}); err != nil {
			t.Errorf("Restore refused what New built: %v", err)
		}
	})
}
