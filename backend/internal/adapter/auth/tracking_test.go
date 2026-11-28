package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestNewTrackingURLs_RefusesBadTemplates(t *testing.T) {
	t.Parallel()
	for _, tpl := range []string{
		"", "https://encomendo.com.br", "https://{slug}.{slug}.com.br", "ftp://{slug}.encomendo.com.br",
		"https://encomendo.com.br/{slug}", "https://{slug}.encomendo.com.br/lojas", "https://{slug}.encomendo.com.br?x=1",
		"https://user@{slug}.encomendo.com.br", "{slug}",
	} {
		if _, err := auth.NewTrackingURLs(tpl); !errors.Is(err, auth.ErrInvalidTrackingTemplate) {
			t.Errorf("%q: err = %v, want ErrInvalidTrackingTemplate", tpl, err)
		}
	}
}

func TestTrackingURLs_URLCarriesTheTokenOnTheStoresPage(t *testing.T) {
	t.Parallel()
	for tpl, want := range map[string]string{
		"https://{slug}.encomendo.com.br":  "https://altarpm.encomendo.com.br/p/",
		"https://{slug}.encomendo.com.br/": "https://altarpm.encomendo.com.br/p/",
		"http://{slug}.localhost:8080":     "http://altarpm.localhost:8080/p/",
	} {
		urls, err := auth.NewTrackingURLs(tpl)
		if err != nil {
			t.Fatalf("%q: %v", tpl, err)
		}
		token := shared.Secret{1, 2, 3}
		got, err := urls.URL("altarpm", token)
		if err != nil {
			t.Fatal(err)
		}
		if got != want+token.Reveal() {
			t.Errorf("%q: url = %q", tpl, got)
		}
		parsed, err := shared.ParseSecret(got[strings.LastIndex(got, "/")+1:])
		if err != nil || parsed != token {
			t.Errorf("the token does not round-trip from the URL: %v", err)
		}
	}
}

func TestTrackingURLs_RefusesBadSlugsAndTheZeroToken(t *testing.T) {
	t.Parallel()
	urls, err := auth.NewTrackingURLs("https://{slug}.encomendo.com.br")
	if err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"", "ab", "Alta", "alta.rpm", "-alta", "alta-", "al--ta", "alta/../x", strings.Repeat("a", 41)} {
		if _, err := urls.URL(slug, shared.Secret{1}); !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("slug %q: err = %v", slug, err)
		}
	}
	if _, err := urls.URL("altarpm", shared.Secret{}); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("zero token: err = %v", err)
	}
}

// The token in a URL is never what is stored: the order keeps its SHA-256,
// and two tokens from the secret generator never collide.
func TestTrackingTokens_RandomAndStoredOnlyAsHashes(t *testing.T) {
	t.Parallel()
	gen := auth.NewSecrets()
	seen := map[shared.SecretHash]bool{}
	for range 1000 {
		s, err := gen.NewSecret(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		h := s.Hash()
		if seen[h] || shared.SecretHash(s) == h {
			t.Fatal("a token repeated, or its hash is the token")
		}
		seen[h] = true
	}
}
