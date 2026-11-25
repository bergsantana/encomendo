package tenancy

import (
	"fmt"
	"slices"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

const (
	minSlugLength = 3
	maxSlugLength = 40
)

// reservedSlugs are refused as store addresses whatever the configuration.
// web/apps/store/src/site.ts holds the same list, and a test keeps the two
// in step (ADR 0011). 0004_tenancy refuses them in SQL too.
var reservedSlugs = []string{"www", "api", "admin", "app", "conta", "mail", "static"}

// ReservedSlugs returns the fixed reserved labels, as a new slice.
func ReservedSlugs() []string { return slices.Clone(reservedSlugs) }

// Slug is a store's address label: {slug}.encomendo.com.br. The zero Slug
// is "no address".
type Slug struct {
	s string
}

// String returns the label.
func (s Slug) String() string { return s.s }

// IsZero reports whether s is the zero Slug.
func (s Slug) IsZero() bool { return s.s == "" }

// SlugPolicy checks new slugs against the fixed reserved list plus the
// labels of the configured admin and API hosts, which only the configuration
// knows.
type SlugPolicy struct {
	reserved map[string]bool
}

// NewSlugPolicy returns a policy that also reserves hostLabels, compared in
// lower case. Blank labels are ignored.
func NewSlugPolicy(hostLabels ...string) SlugPolicy {
	p := SlugPolicy{reserved: map[string]bool{}}
	for _, l := range append(ReservedSlugs(), hostLabels...) {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			p.reserved[l] = true
		}
	}
	return p
}

// Parse checks a slug chosen for a new store, exactly as typed. It returns a
// ValidationError on the field "slug" when the form is wrong or the label is
// reserved. Whether another store has it is the repository's answer
// (ErrSlugTaken).
func (p SlugPolicy) Parse(s string) (Slug, error) {
	if !wellFormedSlug(s) {
		return Slug{}, slugError("Use de 3 a 40 letras minúsculas, números e hífens, sem hífen no começo ou no fim.")
	}
	if p.reserved[s] {
		return Slug{}, slugError("Este endereço é reservado. Escolha outro.")
	}
	return Slug{s: s}, nil
}

func slugError(msg string) error {
	return shared.NewValidationError(shared.FieldError{Field: "slug", Message: msg})
}

// RestoreSlug rebuilds a stored slug, checking only its form: a store keeps
// its address when the configured hosts change. It returns ErrInvalidTenant
// for a malformed one.
func RestoreSlug(s string) (Slug, error) {
	if !wellFormedSlug(s) {
		return Slug{}, fmt.Errorf("restore slug %q: %w", s, ErrInvalidTenant)
	}
	return Slug{s: s}, nil
}

// wellFormedSlug matches ^[a-z0-9]+(-[a-z0-9]+)*$ with 3 to 40 characters,
// the pattern of the contract and of 0004_tenancy.
func wellFormedSlug(s string) bool {
	if len(s) < minSlugLength || len(s) > maxSlugLength || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-' && s[i-1] != '-':
		default:
			return false
		}
	}
	return true
}
