package customer

import (
	"errors"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	tenantID   = shared.TenantID(shared.NewV7(1, [10]byte{1}))
	customerID = shared.CustomerID(shared.NewV7(2, [10]byte{2}))
	homeID     = shared.AddressID(shared.NewV7(3, [10]byte{3}))
	workID     = shared.AddressID(shared.NewV7(4, [10]byte{4}))
)

const goodPhone = "5592991234567"

// goodAddress returns a valid address spec with the given id.
func goodAddress(id shared.AddressID) AddressSpec {
	return AddressSpec{
		ID: id, Label: "Casa", CEP: "69000000", Street: "Rua das Flores",
		Number: "10", Complement: "Apto 2", District: "Centro", City: "Manaus", State: "AM",
	}
}

func goodSpec() Spec {
	return Spec{
		Name: "João Pereira", Phone: goodPhone, Email: "joao@example.com", Notes: "Prefere à tarde.",
		Addresses: []AddressSpec{goodAddress(homeID)},
	}
}

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

	s := goodSpec()
	s.Name = "  João Pereira  "
	s.Notes = "  Prefere à tarde.\n"
	c, err := New(tenantID, customerID, s)
	if err != nil {
		t.Fatal(err)
	}
	if c.TenantID() != tenantID || c.ID() != customerID {
		t.Errorf("tenant %v id %v", c.TenantID(), c.ID())
	}
	if c.Name() != "João Pereira" || c.Notes() != "Prefere à tarde." {
		t.Errorf("name %q notes %q, want them trimmed", c.Name(), c.Notes())
	}
	if c.Phone() != goodPhone || c.Email().String() != "joao@example.com" {
		t.Errorf("phone %q email %q", c.Phone(), c.Email())
	}
	if c.Version() != 1 || !c.ClientAccountID().IsZero() {
		t.Errorf("version %d account %v, want 1 and unset", c.Version(), c.ClientAccountID())
	}
	want := goodAddress(homeID)
	got := c.Addresses()
	if len(got) != 1 || got[0] != Address(want) {
		t.Errorf("Addresses() = %+v, want %+v", got, want)
	}
}

func TestNew_NoOptionalParts(t *testing.T) {
	t.Parallel()

	c, err := New(tenantID, customerID, Spec{Name: "Ana", Phone: goodPhone})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Email().IsZero() || c.Notes() != "" || len(c.Addresses()) != 0 {
		t.Errorf("email %q notes %q addresses %d, want none", c.Email(), c.Notes(), len(c.Addresses()))
	}
}

func TestNew_ZeroIDs(t *testing.T) {
	t.Parallel()

	if _, err := New(shared.TenantID{}, customerID, goodSpec()); !errors.Is(err, ErrInvalidCustomer) {
		t.Errorf("zero tenant error = %v, want ErrInvalidCustomer", err)
	}
	if _, err := New(tenantID, shared.CustomerID{}, goodSpec()); !errors.Is(err, ErrInvalidCustomer) {
		t.Errorf("zero customer id error = %v, want ErrInvalidCustomer", err)
	}
	s := goodSpec()
	s.Addresses[0].ID = shared.AddressID{}
	err := func() error { _, err := New(tenantID, customerID, s); return err }()
	if !errors.Is(err, ErrInvalidCustomer) || errors.Is(err, shared.ErrInvalid) {
		t.Errorf("zero address id error = %v, want ErrInvalidCustomer and not a 400", err)
	}
}

func TestNew_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, wantMsg string // wantMsg "" means accepted
	}{
		{"one character", "A", ""},
		{"accents", "José d'Ávila", ""},
		{"120 characters", strings.Repeat("a", 120), ""},
		{"120 multibyte characters", strings.Repeat("é", 120), ""},
		{"empty", "", msgNameRequired},
		{"spaces only", "   \t ", msgNameRequired},
		{"121 characters", strings.Repeat("a", 121), msgNameTooLong},
		{"newline inside", "Ana\nMaria", msgNameControl},
		{"tab inside", "Ana\tMaria", msgNameControl},
		{"NUL", "Ana\x00", msgNameControl},
		{"invalid UTF-8", "Ana\xff", msgNameControl},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := goodSpec()
			s.Name = tt.in
			_, err := New(tenantID, customerID, s)
			assertField(t, err, "name", tt.wantMsg)
		})
	}
}

func TestNew_Phone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, wantMsg string
	}{
		{"mobile", "5592991234567", ""},
		{"landline", "559232341234", ""},
		{"empty", "", msgPhoneRequired},
		{"without country code", "92991234567", msgPhoneInvalid},
		{"formatted", "(92) 99123-4567", msgPhoneInvalid},
		{"with plus", "+5592991234567", msgPhoneInvalid},
		{"padded", " 5592991234567", msgPhoneInvalid},
		{"letters", "55929912345ab", msgPhoneInvalid},
		{"DDD starting with zero", "5502991234567", msgPhoneInvalid},
		{"too short", "55929912345", msgPhoneInvalid},
		{"too long", "55929912345678", msgPhoneInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := goodSpec()
			s.Phone = tt.in
			_, err := New(tenantID, customerID, s)
			assertField(t, err, "phone", tt.wantMsg)
		})
	}
}

func TestNew_Email(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, wantMsg string
		wantStored        string
	}{
		{"empty is no email", "", "", ""},
		{"blank is no email", "   ", "", ""},
		{"address", "ana@example.com", "", "ana@example.com"},
		{"trimmed", "  ana@example.com ", "", "ana@example.com"},
		{"no at sign", "ana.example.com", msgEmailInvalid, ""},
		{"no domain dot", "ana@example", msgEmailInvalid, ""},
		{"space inside", "ana maria@example.com", msgEmailInvalid, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := goodSpec()
			s.Email = tt.in
			c, err := New(tenantID, customerID, s)
			assertField(t, err, "email", tt.wantMsg)
			if err == nil && c.Email().String() != tt.wantStored {
				t.Errorf("Email() = %q, want %q", c.Email(), tt.wantStored)
			}
		})
	}
}

func TestNew_Notes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, wantMsg string
	}{
		{"empty", "", ""},
		{"line breaks and tabs", "Linha 1\r\nLinha 2\t.", ""},
		{"2000 characters", strings.Repeat("é", 2000), ""},
		{"2001 characters", strings.Repeat("é", 2001), msgNotesTooLong},
		{"padding does not count", "  " + strings.Repeat("a", 2000) + "  ", ""},
		{"NUL", "a\x00b", msgNotesInvalid},
		{"escape", "a\x1bb", msgNotesInvalid},
		{"invalid UTF-8", "a\xffb", msgNotesInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := goodSpec()
			s.Notes = tt.in
			_, err := New(tenantID, customerID, s)
			assertField(t, err, "notes", tt.wantMsg)
		})
	}
}

func TestNew_AddressCount(t *testing.T) {
	t.Parallel()

	build := func(n int) []AddressSpec {
		out := make([]AddressSpec, n)
		for i := range out {
			out[i] = goodAddress(shared.AddressID(shared.NewV7(int64(100+i), [10]byte{byte(i + 1)})))
		}
		return out
	}
	tests := []struct {
		name    string
		n       int
		wantMsg string
	}{
		{"none", 0, ""},
		{"ten", 10, ""},
		{"eleven", 11, msgTooManyAddrs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := goodSpec()
			s.Addresses = build(tt.n)
			c, err := New(tenantID, customerID, s)
			assertField(t, err, "addresses", tt.wantMsg)
			if err == nil && len(c.Addresses()) != tt.n {
				t.Errorf("got %d addresses, want %d", len(c.Addresses()), tt.n)
			}
		})
	}
}

func TestNew_RepeatedAddressID(t *testing.T) {
	t.Parallel()

	s := goodSpec()
	s.Addresses = append(s.Addresses, goodAddress(homeID))
	_, err := New(tenantID, customerID, s)
	assertField(t, err, "addresses[1].id", msgAddressRepeat)
}

func TestNew_ReportsEveryInvalidField(t *testing.T) {
	t.Parallel()

	s := Spec{Name: "", Phone: "1", Email: "x", Addresses: []AddressSpec{{ID: homeID}}}
	_, err := New(tenantID, customerID, s)
	got := fieldsOf(t, err)
	for _, f := range []string{
		"name", "phone", "email", "addresses[0].label", "addresses[0].cep",
		"addresses[0].street", "addresses[0].city", "addresses[0].state",
	} {
		if _, ok := got[f]; !ok {
			t.Errorf("field %q not reported; got %v", f, got)
		}
	}
}

// assertField checks that err is nil when wantMsg is "", and otherwise a
// ValidationError that carries wantMsg on field.
func assertField(t *testing.T, err error, field, wantMsg string) {
	t.Helper()
	if wantMsg == "" {
		if err != nil {
			t.Errorf("error = %v, want none", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("no error, want %q on %q", wantMsg, field)
	}
	if got := fieldsOf(t, err)[field]; got != wantMsg {
		t.Errorf("message on %q = %q, want %q", field, got, wantMsg)
	}
}

func TestReplace(t *testing.T) {
	t.Parallel()

	c, err := New(tenantID, customerID, goodSpec())
	if err != nil {
		t.Fatal(err)
	}
	next := Spec{
		Name: "Maria", Phone: "559232341234", Notes: "Nova",
		Addresses: []AddressSpec{goodAddress(workID)},
	}
	if err := c.Replace(next, 1); err != nil {
		t.Fatal(err)
	}
	if c.Name() != "Maria" || c.Phone() != "559232341234" || c.Notes() != "Nova" || !c.Email().IsZero() {
		t.Errorf("after Replace: %q %q %q %q", c.Name(), c.Phone(), c.Notes(), c.Email())
	}
	if _, ok := c.Address(homeID); ok {
		t.Error("the address left out of Replace is still there")
	}
	if _, ok := c.Address(workID); !ok {
		t.Error("the new address is missing")
	}
	if c.Version() != 1 {
		t.Errorf("Version() = %d, want 1: the repository raises it", c.Version())
	}
}

func TestReplace_StaleVersion(t *testing.T) {
	t.Parallel()

	c, err := New(tenantID, customerID, goodSpec())
	if err != nil {
		t.Fatal(err)
	}
	err = c.Replace(Spec{Name: "Maria", Phone: goodPhone}, 2)
	if !errors.Is(err, shared.ErrVersionConflict) || !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error = %v, want ErrVersionConflict", err)
	}
	if c.Name() != "João Pereira" {
		t.Errorf("Name() = %q, a refused Replace must change nothing", c.Name())
	}
}

func TestReplace_InvalidChangesNothing(t *testing.T) {
	t.Parallel()

	c, err := New(tenantID, customerID, goodSpec())
	if err != nil {
		t.Fatal(err)
	}
	err = c.Replace(Spec{Name: "Maria", Phone: "123"}, 1)
	assertField(t, err, "phone", msgPhoneInvalid)
	if c.Name() != "João Pereira" || c.Phone() != goodPhone || len(c.Addresses()) != 1 {
		t.Errorf("a refused Replace changed the customer: %q %q %d", c.Name(), c.Phone(), len(c.Addresses()))
	}
}

func TestReplace_KeepsAddressByID(t *testing.T) {
	t.Parallel()

	c, err := New(tenantID, customerID, goodSpec())
	if err != nil {
		t.Fatal(err)
	}
	s := goodSpec()
	s.Addresses[0].Street = "Avenida Nova"
	if err := c.Replace(s, 1); err != nil {
		t.Fatal(err)
	}
	a, ok := c.Address(homeID)
	if !ok || a.Street != "Avenida Nova" {
		t.Errorf("Address(home) = %+v, %v; want the street updated under the same id", a, ok)
	}
}

func TestAddress(t *testing.T) {
	t.Parallel()

	s := goodSpec()
	s.Addresses = append(s.Addresses, goodAddress(workID))
	s.Addresses[1].Label = "Trabalho"
	c, err := New(tenantID, customerID, s)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := c.Address(workID)
	if !ok || a.Label != "Trabalho" {
		t.Errorf("Address(work) = %+v, %v", a, ok)
	}
	other := shared.AddressID(shared.NewV7(9, [10]byte{9}))
	if a, ok := c.Address(other); ok || a != (Address{}) {
		t.Errorf("Address(unknown) = %+v, %v; want zero and false", a, ok)
	}
}

func TestAddresses_ReturnsACopy(t *testing.T) {
	t.Parallel()

	c, err := New(tenantID, customerID, goodSpec())
	if err != nil {
		t.Fatal(err)
	}
	c.Addresses()[0].Street = "Changed"
	if a, _ := c.Address(homeID); a.Street != "Rua das Flores" {
		t.Errorf("Street = %q: editing the returned slice changed the customer", a.Street)
	}
}

func TestRestore_RoundTrip(t *testing.T) {
	t.Parallel()

	account := shared.ClientAccountID(shared.NewV7(7, [10]byte{7}))
	c, err := New(tenantID, customerID, goodSpec())
	if err != nil {
		t.Fatal(err)
	}
	rec := Record{
		Tenant: c.TenantID(), ID: c.ID(), Name: c.Name(), Phone: c.Phone(), Email: c.Email().String(),
		Notes: c.Notes(), Addresses: c.Addresses(), ClientAccountID: account, Version: 5,
	}
	got, err := Restore(rec)
	if err != nil {
		t.Fatal(err)
	}
	if got.TenantID() != tenantID || got.ID() != customerID || got.Name() != c.Name() ||
		got.Phone() != c.Phone() || got.Email().String() != "joao@example.com" || got.Notes() != c.Notes() {
		t.Errorf("Restore() = %+v", got)
	}
	if got.Version() != 5 || got.ClientAccountID() != account {
		t.Errorf("version %d account %v, want 5 and %v", got.Version(), got.ClientAccountID(), account)
	}
	if a, ok := got.Address(homeID); !ok || a != c.Addresses()[0] {
		t.Errorf("Address(home) = %+v, %v", a, ok)
	}
}

func TestRestore_RefusesBrokenRecords(t *testing.T) {
	t.Parallel()

	valid := Record{Tenant: tenantID, ID: customerID, Name: "Ana", Phone: goodPhone, Version: 1}
	tests := []struct {
		name   string
		mutate func(*Record)
	}{
		{"version zero", func(r *Record) { r.Version = 0 }},
		{"zero tenant", func(r *Record) { r.Tenant = shared.TenantID{} }},
		{"bad name", func(r *Record) { r.Name = "" }},
		{"bad phone", func(r *Record) { r.Phone = "1" }},
		{"bad address", func(r *Record) { r.Addresses = []Address{{ID: homeID}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := valid
			tt.mutate(&r)
			_, err := Restore(r)
			if !errors.Is(err, ErrInvalidCustomer) {
				t.Errorf("error = %v, want ErrInvalidCustomer", err)
			}
			if errors.Is(err, shared.ErrInvalid) {
				t.Errorf("error %v matches ErrInvalid: a bad stored row must not answer 400", err)
			}
		})
	}
}

func TestFirstName(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{"João Pereira", "João"},
		{"  Maria  da Silva", "Maria"},
		{"Ana", "Ana"},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		if got := FirstName(tt.in); got != tt.want {
			t.Errorf("FirstName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestErrNotFound(t *testing.T) {
	t.Parallel()

	if !errors.Is(ErrNotFound, shared.ErrNotFound) {
		t.Error("ErrNotFound does not match shared.ErrNotFound")
	}
}

// FuzzName checks that New accepts a name only when it is a trimmed,
// 1 to 120 character, valid UTF-8 string with no control character, and
// that a refusal is always a ValidationError on "name".
func FuzzName(f *testing.F) {
	for _, seed := range []string{
		"João Pereira", "", "   ", "a\nb", "a\x00", "\xff", strings.Repeat("é", 121), strings.Repeat("é", 120),
		"\u200bzero width", "tab\there", "  padded  ",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		s := goodSpec()
		s.Name = name
		c, err := New(tenantID, customerID, s)
		if err != nil {
			if _, ok := fieldsOf(t, err)["name"]; !ok {
				t.Fatalf("New(%q) = %v, want an error on name", name, err)
			}
			return
		}
		got := c.Name()
		if got != strings.TrimSpace(name) || got == "" || !utf8.ValidString(got) ||
			utf8.RuneCountInString(got) > 120 || strings.ContainsFunc(got, unicode.IsControl) {
			t.Fatalf("New(%q) accepted the name %q", name, got)
		}
		if first := FirstName(got); first == "" || strings.Contains(first, " ") {
			t.Fatalf("FirstName(%q) = %q", got, first)
		}
	})
}
