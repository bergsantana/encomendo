package identity

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Length limits shared with the contract (docs/api/openapi.yaml, Email and
// PersonName) and the CHECK constraints of 0002_identity.
const (
	MaxEmailLength      = 254
	maxEmailLocalLength = 64
	MaxPersonNameLength = 100
)

// Email is an email address as a person typed it, trimmed. Addresses are
// compared without regard to case, as the database compares them (citext);
// Key gives that form. The zero Email is "no email".
type Email struct {
	s string
}

// ParseEmail trims s and checks that it looks like one deliverable address:
// one "@", a local part of at most 64 bytes, a domain with at least two
// non-empty labels, no spaces or invisible characters, and 3 to 254
// characters in all. It returns a ValidationError on the field "email".
// Whether the mailbox exists is never checked here.
func ParseEmail(s string) (Email, error) {
	s = strings.TrimSpace(s)
	if !looksLikeEmail(s) {
		return Email{}, shared.NewValidationError(shared.FieldError{Field: "email", Message: "Informe um e-mail válido."})
	}
	return Email{s: s}, nil
}

func looksLikeEmail(s string) bool {
	if n := utf8.RuneCountInString(s); n < 3 || n > MaxEmailLength || !utf8.ValidString(s) {
		return false
	}
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || len(local) > maxEmailLocalLength || strings.Contains(domain, "@") {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) || !unicode.IsPrint(r) || strings.ContainsRune(`<>()[]\,;:"`, r) {
			return false
		}
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" {
			return false
		}
	}
	return true
}

// String returns the address as typed, trimmed.
func (e Email) String() string { return e.s }

// Key returns the address lower-cased: the form lookups, limits and
// comparisons use, so changing the case never makes a second account or buys
// more attempts (ADR 0010).
func (e Email) Key() string { return strings.ToLower(e.s) }

// Equal reports whether e and other are the same address, ignoring case.
func (e Email) Equal(other Email) bool { return e.Key() == other.Key() }

// IsZero reports whether e is the zero Email.
func (e Email) IsZero() bool { return e.s == "" }

// PersonName is a staff member's or client's name, trimmed, 1 to 100
// characters with no control or invisible characters, so it is safe to show
// and to put in an email. The zero PersonName is "no name".
type PersonName struct {
	s string
}

// ParsePersonName trims s and checks it. It returns a ValidationError on the
// field "name".
func ParsePersonName(s string) (PersonName, error) {
	s = strings.TrimSpace(s)
	if !printable(s, MaxPersonNameLength) {
		return PersonName{}, shared.NewValidationError(shared.FieldError{
			Field:   "name",
			Message: "Informe um nome de 1 a 100 caracteres.",
		})
	}
	return PersonName{s: s}, nil
}

// printable reports whether s has 1 to max characters, all of them visible
// letters, marks, numbers, punctuation, symbols or the ASCII space.
func printable(s string, max int) bool {
	if s == "" || utf8.RuneCountInString(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// String returns the name.
func (n PersonName) String() string { return n.s }

// IsZero reports whether n is the zero PersonName.
func (n PersonName) IsZero() bool { return n.s == "" }
