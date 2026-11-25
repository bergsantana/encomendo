package shared

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSecret_StringRoundTrips(t *testing.T) {
	t.Parallel()

	var s Secret
	for i := range s {
		s[i] = byte(i * 7)
	}
	text := s.Reveal()
	if len(text) != 43 || strings.ContainsAny(text, "+/=") {
		t.Fatalf("String() = %q, want 43 URL-safe characters without padding", text)
	}
	back, err := ParseSecret(text)
	if err != nil {
		t.Fatalf("ParseSecret(%q): %v", text, err)
	}
	if back != s {
		t.Error("ParseSecret(String()) changed the secret")
	}
}

func TestSecret_HashIsSHA256OfTheBytes(t *testing.T) {
	t.Parallel()

	s := Secret{1, 2, 3}
	if got, want := s.Hash(), SecretHash(sha256.Sum256(s[:])); got != want {
		t.Errorf("Hash() = %x, want %x", got, want)
	}
	if (Secret{1, 2, 4}).Hash() == s.Hash() {
		t.Error("two secrets hash the same")
	}
}

func TestParseSecret_RefusesAnythingElse(t *testing.T) {
	t.Parallel()

	valid := Secret{9}.Reveal()
	for name, in := range map[string]string{
		"empty":             "",
		"too short":         valid[:42],
		"too long":          valid + "A",
		"padded":            valid + "=",
		"standard base64":   strings.Repeat("+", 43),
		"spaces":            " " + valid[1:],
		"non-canonical end": valid[:42] + "B", // the last character carries 2 unused bits
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseSecret(in); !errors.Is(err, ErrMalformedSecret) {
				t.Errorf("ParseSecret(%q) = %v, want ErrMalformedSecret", in, err)
			}
		})
	}
}

func TestSecret_NeverPrintsItsValue(t *testing.T) {
	t.Parallel()

	s := Secret{1, 2, 3}
	type carrier struct{ Token Secret }
	for _, out := range []string{
		fmt.Sprint(s), fmt.Sprintf("%s %v %+v %#v %q %x", s, s, s, s, s, s),
		fmt.Sprintf("%v %+v %#v", carrier{s}, carrier{s}, carrier{s}),
	} {
		if strings.Contains(out, s.Reveal()) || strings.Contains(out, "1 2 3") || !strings.Contains(out, "redacted") {
			t.Errorf("printed %q", out)
		}
	}
}

func TestErrMalformedSecret_MatchesNoSentinel(t *testing.T) {
	t.Parallel()

	// Each caller decides what a malformed secret means: a 401 for a
	// refresh token, a 404 for an invitation link.
	for _, sentinel := range []error{ErrNotFound, ErrConflict, ErrForbidden, ErrInvalid, ErrUnauthenticated} {
		if errors.Is(ErrMalformedSecret, sentinel) {
			t.Errorf("ErrMalformedSecret matches %v", sentinel)
		}
	}
}

func FuzzParseSecret(f *testing.F) {
	f.Add(Secret{}.Reveal())
	f.Add(Secret{0xff, 0xee}.Reveal())
	f.Add("not a secret")
	f.Fuzz(func(t *testing.T, in string) {
		s, err := ParseSecret(in)
		if err != nil {
			return
		}
		if s.Reveal() != in {
			t.Errorf("ParseSecret accepted %q, which does not round-trip (%q)", in, s.Reveal())
		}
	})
}
