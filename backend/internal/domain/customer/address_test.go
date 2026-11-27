package customer

import (
	"strings"
	"testing"
)

// addressSpecWith returns a valid customer spec whose only address has been
// changed by mutate.
func addressSpecWith(mutate func(*AddressSpec)) Spec {
	s := goodSpec()
	mutate(&s.Addresses[0])
	return s
}

func TestNew_AddressText(t *testing.T) {
	t.Parallel()

	long := func(n int) string { return strings.Repeat("é", n) }
	tests := []struct {
		name    string
		mutate  func(*AddressSpec)
		field   string
		wantMsg string
	}{
		{"label 1", func(a *AddressSpec) { a.Label = "C" }, "label", ""},
		{"label 40", func(a *AddressSpec) { a.Label = long(40) }, "label", ""},
		{"label empty", func(a *AddressSpec) { a.Label = "" }, "label", msgLabelRequired},
		{"label blank", func(a *AddressSpec) { a.Label = "  " }, "label", msgLabelRequired},
		{"label 41", func(a *AddressSpec) { a.Label = long(41) }, "label", msgLabelTooLong},
		{"label control", func(a *AddressSpec) { a.Label = "a\nb" }, "label", msgAddressTextControl},

		{"street 200", func(a *AddressSpec) { a.Street = long(200) }, "street", ""},
		{"street empty", func(a *AddressSpec) { a.Street = "" }, "street", msgStreetRequired},
		{"street 201", func(a *AddressSpec) { a.Street = long(201) }, "street", msgStreetTooLong},
		{"street invalid UTF-8", func(a *AddressSpec) { a.Street = "a\xff" }, "street", msgAddressTextControl},

		{"number empty", func(a *AddressSpec) { a.Number = "" }, "number", ""},
		{"number 20", func(a *AddressSpec) { a.Number = long(20) }, "number", ""},
		{"number 21", func(a *AddressSpec) { a.Number = long(21) }, "number", msgNumberTooLong},

		{"complement empty", func(a *AddressSpec) { a.Complement = "" }, "complement", ""},
		{"complement 100", func(a *AddressSpec) { a.Complement = long(100) }, "complement", ""},
		{"complement 101", func(a *AddressSpec) { a.Complement = long(101) }, "complement", msgComplementTooLong},

		{"district empty", func(a *AddressSpec) { a.District = "" }, "district", ""},
		{"district 100", func(a *AddressSpec) { a.District = long(100) }, "district", ""},
		{"district 101", func(a *AddressSpec) { a.District = long(101) }, "district", msgDistrictTooLong},

		{"city 100", func(a *AddressSpec) { a.City = long(100) }, "city", ""},
		{"city empty", func(a *AddressSpec) { a.City = "" }, "city", msgCityRequired},
		{"city 101", func(a *AddressSpec) { a.City = long(101) }, "city", msgCityTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tenantID, customerID, addressSpecWith(tt.mutate))
			assertField(t, err, "addresses[0]."+tt.field, tt.wantMsg)
		})
	}
}

func TestNew_AddressIsTrimmed(t *testing.T) {
	t.Parallel()

	s := addressSpecWith(func(a *AddressSpec) {
		a.Label, a.Street, a.City, a.CEP, a.State = " Casa ", " Rua A ", " Manaus ", " 69000000 ", " AM "
	})
	c, err := New(tenantID, customerID, s)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := c.Address(homeID)
	if a.Label != "Casa" || a.Street != "Rua A" || a.City != "Manaus" || a.CEP != "69000000" || a.State != "AM" {
		t.Errorf("address = %+v, want trimmed", a)
	}
}

func TestNew_CEP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, wantMsg string
	}{
		{"eight digits", "69000000", ""},
		{"leading zeros", "01001000", ""},
		{"empty", "", msgCEPInvalid},
		{"with hyphen", "69000-000", msgCEPInvalid},
		{"seven digits", "6900000", msgCEPInvalid},
		{"nine digits", "690000000", msgCEPInvalid},
		{"letters", "6900000a", msgCEPInvalid},
		{"Arabic-Indic digits", "٦٩٠٠٠٠٠٠", msgCEPInvalid},
		{"inner space", "6900 000", msgCEPInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tenantID, customerID, addressSpecWith(func(a *AddressSpec) { a.CEP = tt.in }))
			assertField(t, err, "addresses[0].cep", tt.wantMsg)
		})
	}
}

func TestNew_State(t *testing.T) {
	t.Parallel()

	for uf := range ufs {
		_, err := New(tenantID, customerID, addressSpecWith(func(a *AddressSpec) { a.State = uf }))
		if err != nil {
			t.Errorf("UF %s refused: %v", uf, err)
		}
	}
	if len(ufs) != 27 {
		t.Errorf("%d UF codes, want 27", len(ufs))
	}
	for _, bad := range []string{"", "am", "XX", "AMA", "A", "Amazonas"} {
		_, err := New(tenantID, customerID, addressSpecWith(func(a *AddressSpec) { a.State = bad }))
		assertField(t, err, "addresses[0].state", msgStateInvalid)
	}
}

func TestNew_SecondAddressErrorNamesItsIndex(t *testing.T) {
	t.Parallel()

	s := goodSpec()
	bad := goodAddress(workID)
	bad.CEP = "1"
	s.Addresses = append(s.Addresses, bad)
	_, err := New(tenantID, customerID, s)
	assertField(t, err, "addresses[1].cep", msgCEPInvalid)
}

// FuzzCEP checks that an address is accepted only when its CEP is exactly 8
// ASCII digits after trimming, and that a refusal is a ValidationError on
// the cep field.
func FuzzCEP(f *testing.F) {
	for _, seed := range []string{"69000000", "", "69000-000", " 69000000 ", "٦٩٠٠٠٠٠٠", "6900000\x00", "123456789"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, cep string) {
		c, err := New(tenantID, customerID, addressSpecWith(func(a *AddressSpec) { a.CEP = cep }))
		if err != nil {
			if _, ok := fieldsOf(t, err)["addresses[0].cep"]; !ok {
				t.Fatalf("New with cep %q = %v, want an error on the cep", cep, err)
			}
			return
		}
		a, _ := c.Address(homeID)
		if len(a.CEP) != 8 || strings.Trim(a.CEP, "0123456789") != "" || a.CEP != strings.TrimSpace(cep) {
			t.Fatalf("cep %q accepted as %q", cep, a.CEP)
		}
	})
}
