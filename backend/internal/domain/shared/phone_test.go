package shared

import (
	"regexp"
	"testing"
)

func TestValidWhatsAppPhone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in string
		want     bool
	}{
		{"mobile with the ninth digit", "5592991234567", true},
		{"landline", "551133334444", true},
		{"landline in the Northeast", "558199991234", true},
		{"empty", "", false},
		{"no country code", "92991234567", false},
		{"DDD starts with 0", "5502991234567", false},
		{"DDD second digit is 0", "5590991234567", false},
		{"too short", "55929912345", false},
		{"too long", "55929912345678", false},
		{"plus sign", "+5592991234567", false},
		{"punctuation", "(92) 99123-4567", false},
		{"spaces", "55 92 99123-4567", false},
		{"surrounding space", " 5592991234567", false},
		{"letter in the right length", "559299123x567", false},
		{"wrong country", "5192991234567", false},
		{"Arabic-Indic digits", "55٩٢٩٩١٢٣٤٥٦٧", false},
		{"newline at the end", "5592991234567\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ValidWhatsAppPhone(tt.in); got != tt.want {
				t.Errorf("ValidWhatsAppPhone(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func FuzzValidWhatsAppPhone(f *testing.F) {
	pattern := regexp.MustCompile(`^55[1-9]{2}[0-9]{8,9}$`)
	for _, seed := range []string{"5592991234567", "551133334444", "", "5502991234567", "+5592991234567", "55929912345678"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		// The function and the contract's pattern must agree on every input.
		if got, want := ValidWhatsAppPhone(in), pattern.MatchString(in); got != want {
			t.Errorf("ValidWhatsAppPhone(%q) = %v, pattern says %v", in, got, want)
		}
	})
}
