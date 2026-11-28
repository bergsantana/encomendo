package auth

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrInvalidTrackingTemplate means the store origin template is not a URL
// with exactly one {slug}.
var ErrInvalidTrackingTemplate = errors.New("auth: invalid store origin template")

// TrackingURLs builds the tracking page URL of an order on its store's page:
// the store origin template (STORE_ORIGIN_TEMPLATE, such as
// https://{slug}.encomendo.com.br) with the store's slug, then /p/ and the
// token in base64url (ADR 0030). The token itself is 32 random bytes from
// the SecretGenerator, and the server keeps only its SHA-256 (the order's
// tracking hash); this URL is the only place the plain token appears, in
// the one answer that made it. The lookup by token is milestone 7's, with
// the store route.
type TrackingURLs struct{ template string }

// NewTrackingURLs returns the builder over template, which must hold
// "{slug}" exactly once, in the host, and parse as an http or https URL with
// no path, query or fragment. Whether https is required outside
// development is platform/config's check.
func NewTrackingURLs(template string) (*TrackingURLs, error) {
	if strings.Count(template, "{slug}") != 1 {
		return nil, fmt.Errorf("%w: %q needs one {slug}", ErrInvalidTrackingTemplate, template)
	}
	u, err := url.Parse(strings.Replace(template, "{slug}", "loja", 1))
	switch {
	case err != nil:
		return nil, fmt.Errorf("%w: %q: %w", ErrInvalidTrackingTemplate, template, err)
	case u.Scheme != "http" && u.Scheme != "https":
		return nil, fmt.Errorf("%w: %q is not http or https", ErrInvalidTrackingTemplate, template)
	case !strings.Contains(u.Host, "loja") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil:
		return nil, fmt.Errorf("%w: %q must be an origin with {slug} in the host", ErrInvalidTrackingTemplate, template)
	}
	return &TrackingURLs{template: strings.TrimSuffix(template, "/")}, nil
}

// URL returns the tracking page of token on the store slug. It returns
// shared.ErrInvalid for a slug that is not one (the store's slug rules) or
// a zero token, which would make a guessable link.
func (t *TrackingURLs) URL(slug string, token shared.Secret) (string, error) {
	if !validSlug(slug) {
		return "", fmt.Errorf("tracking url: slug %q: %w", slug, shared.ErrInvalid)
	}
	if token == (shared.Secret{}) {
		return "", fmt.Errorf("tracking url: zero token: %w", shared.ErrInvalid)
	}
	return strings.Replace(t.template, "{slug}", slug, 1) + "/p/" + token.Reveal(), nil
}

// validSlug is the store slug's shape (tenancy's rule: lower-case letters,
// digits and single hyphens, 3 to 40), checked again here because the slug
// becomes part of a host name.
func validSlug(s string) bool {
	if len(s) < 3 || len(s) > 40 || s[0] == '-' || s[len(s)-1] == '-' || strings.Contains(s, "--") {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
