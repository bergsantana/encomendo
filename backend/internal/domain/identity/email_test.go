package identity

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestParseEmail_AcceptsAndTrims(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"dono@altarpm.com.br":               "dono@altarpm.com.br",
		"  Dono@AltaRPM.com.br \n":          "Dono@AltaRPM.com.br",
		"a.b+loja@sub.dominio.co":           "a.b+loja@sub.dominio.co",
		"joão@exemplo.com.br":               "joão@exemplo.com.br",
		"x@y.io":                            "x@y.io",
		strings.Repeat("a", 64) + "@ex.com": strings.Repeat("a", 64) + "@ex.com",
	} {
		got, err := ParseEmail(in)
		if err != nil {
			t.Errorf("ParseEmail(%q): %v", in, err)
			continue
		}
		if got.String() != want {
			t.Errorf("ParseEmail(%q) = %q, want %q", in, got.String(), want)
		}
	}
}

func TestParseEmail_RefusesWithAFieldError(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 64) + "@" + strings.Repeat("b", 186) + ".com" // 255 characters
	for name, in := range map[string]string{
		"empty":               "",
		"blank":               "   ",
		"no at":               "dono.altarpm.com.br",
		"two ats":             "dono@loja@altarpm.com.br",
		"no local part":       "@altarpm.com.br",
		"no domain":           "dono@",
		"domain without dot":  "dono@localhost",
		"empty label":         "dono@altarpm..com",
		"leading dot":         "dono@.altarpm.com",
		"trailing dot":        "dono@altarpm.com.",
		"space inside":        "dono silva@altarpm.com",
		"display name":        "Dono <dono@altarpm.com>",
		"control character":   "dono\x00@altarpm.com",
		"local part too long": strings.Repeat("a", 65) + "@ex.com",
		"too long":            long,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseEmail(in)
			var ve *shared.ValidationError
			if !errors.As(err, &ve) || len(ve.Fields) != 1 || ve.Fields[0].Field != "email" {
				t.Fatalf("ParseEmail(%q) = %v, want a ValidationError on email", in, err)
			}
			if ve.Fields[0].Message == "" {
				t.Error("the field error has no pt-BR message")
			}
		})
	}
}

func TestEmail_KeyIgnoresCase(t *testing.T) {
	t.Parallel()

	// The database compares emails as citext, and every per-email limit
	// keys on the trimmed, lower-cased form (ADR 0010).
	a, _ := ParseEmail(" Dono@AltaRPM.com.br")
	b, _ := ParseEmail("dono@altarpm.com.br")
	if a.Key() != b.Key() || a.Key() != "dono@altarpm.com.br" {
		t.Errorf("keys %q and %q, want both dono@altarpm.com.br", a.Key(), b.Key())
	}
	if !a.Equal(b) {
		t.Error("Equal is false for emails that differ only in case")
	}
}

func TestEmail_ZeroValue(t *testing.T) {
	t.Parallel()

	var e Email
	if !e.IsZero() || e.String() != "" {
		t.Errorf("zero Email: IsZero %v, String %q", e.IsZero(), e.String())
	}
	if parsed, _ := ParseEmail("a@b.co"); parsed.IsZero() {
		t.Error("a parsed email reads as zero")
	}
}

func FuzzParseEmail(f *testing.F) {
	f.Add("dono@altarpm.com.br")
	f.Add(" A@B.CO ")
	f.Add("a@@b")
	f.Fuzz(func(t *testing.T, in string) {
		e, err := ParseEmail(in)
		if err != nil {
			return
		}
		again, err := ParseEmail(e.String())
		if err != nil || again != e {
			t.Errorf("ParseEmail(%q) = %q, which does not parse to itself (%v)", in, e.String(), err)
		}
		if n := utf8.RuneCountInString(e.String()); n < 3 || n > MaxEmailLength {
			t.Errorf("accepted an email of %d characters", n)
		}
		if e.Key() != strings.ToLower(e.String()) {
			t.Errorf("Key() = %q for %q", e.Key(), e.String())
		}
	})
}

func TestParsePersonName(t *testing.T) {
	t.Parallel()

	got, err := ParsePersonName("  Ana Souza ")
	if err != nil || got.String() != "Ana Souza" {
		t.Errorf("ParsePersonName = %q, %v; want Ana Souza", got.String(), err)
	}
	if _, err := ParsePersonName(strings.Repeat("é", 100)); err != nil {
		t.Errorf("100 characters (200 bytes) refused: %v", err)
	}
	for name, in := range map[string]string{
		"empty":     "",
		"blank":     " \t ",
		"too long":  strings.Repeat("a", 101),
		"control":   "Ana\nSouza",
		"invisible": "Ana\u200bSouza",
	} {
		_, err := ParsePersonName(in)
		var ve *shared.ValidationError
		if !errors.As(err, &ve) || ve.Fields[0].Field != "name" {
			t.Errorf("%s: ParsePersonName(%q) = %v, want a ValidationError on name", name, in, err)
		}
	}
}

func TestCheckNewPassword(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{"12345678", strings.Repeat("ç", 128), "frase com espaços"} {
		if err := CheckNewPassword(ok); err != nil {
			t.Errorf("CheckNewPassword(%d characters): %v", len([]rune(ok)), err)
		}
	}
	for _, bad := range []string{"", "1234567", strings.Repeat("a", 129), strings.Repeat("ç", 129)} {
		err := CheckNewPassword(bad)
		var ve *shared.ValidationError
		if !errors.As(err, &ve) || ve.Fields[0].Field != "password" {
			t.Errorf("CheckNewPassword(%d characters) = %v, want a ValidationError on password", len([]rune(bad)), err)
		}
	}
}

func TestCheckPresentedPassword_OnlyBoundsTheLength(t *testing.T) {
	t.Parallel()

	// Login never tells which rule a stored password broke; it only refuses
	// input longer than any password could be (docs/api/openapi.yaml).
	for _, ok := range []string{"", "1", strings.Repeat("a", 128)} {
		if err := CheckPresentedPassword(ok); err != nil {
			t.Errorf("CheckPresentedPassword(%d characters): %v", len(ok), err)
		}
	}
	if err := CheckPresentedPassword(strings.Repeat("a", 129)); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("129 characters: %v, want ErrInvalid", err)
	}
}
