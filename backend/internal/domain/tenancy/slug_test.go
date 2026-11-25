package tenancy

import (
	"errors"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestSlugPolicy_Parse_Accepts(t *testing.T) {
	t.Parallel()

	p := NewSlugPolicy()
	for _, ok := range []string{"altarpm", "kit-scrambler", "abc", "loja2", "a1-b2-c3", strings.Repeat("a", 40)} {
		s, err := p.Parse(ok)
		if err != nil {
			t.Errorf("Parse(%q): %v", ok, err)
			continue
		}
		if s.String() != ok {
			t.Errorf("Parse(%q) = %q", ok, s)
		}
	}
}

func TestSlugPolicy_Parse_RefusesWithAFieldError(t *testing.T) {
	t.Parallel()

	p := NewSlugPolicy("painel", "API-encomendo")
	for name, bad := range map[string]string{
		"empty":                    "",
		"too short":                "ab",
		"too long":                 strings.Repeat("a", 41),
		"upper case":               "AltaRPM",
		"leading hyphen":           "-altarpm",
		"trailing hyphen":          "altarpm-",
		"double hyphen":            "alta--rpm",
		"underscore":               "alta_rpm",
		"dot":                      "alta.rpm",
		"space":                    "alta rpm",
		"accent":                   "joão",
		"reserved www":             "www",
		"reserved conta":           "conta",
		"reserved static":          "static",
		"admin host label":         "painel",
		"api host label, any case": "api-encomendo",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := p.Parse(bad)
			var ve *shared.ValidationError
			if !errors.As(err, &ve) || len(ve.Fields) != 1 || ve.Fields[0].Field != "slug" || ve.Fields[0].Message == "" {
				t.Errorf("Parse(%q) = %v, want a ValidationError on slug", bad, err)
			}
		})
	}
}

func TestSlugPolicy_IgnoresBlankHostLabels(t *testing.T) {
	t.Parallel()

	p := NewSlugPolicy("", "  ")
	if _, err := p.Parse("altarpm"); err != nil {
		t.Errorf("blank host labels broke the policy: %v", err)
	}
}

func TestRestoreSlug_ChecksOnlyTheForm(t *testing.T) {
	t.Parallel()

	// A stored slug stays valid when the configured hosts change later;
	// only its form is checked.
	if s, err := RestoreSlug("painel"); err != nil || s.String() != "painel" {
		t.Errorf("RestoreSlug(painel) = %q, %v", s, err)
	}
	if _, err := RestoreSlug("Not A Slug"); !errors.Is(err, ErrInvalidTenant) {
		t.Errorf("RestoreSlug of a malformed slug: %v, want ErrInvalidTenant", err)
	}
}

// TestReservedSlugs_MatchTheStorePage keeps the fixed list in step with the
// store app's, which turns the same labels away before asking the API (ADR
// 0011, "Store addresses").
func TestReservedSlugs_MatchTheStorePage(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("../../../../web/apps/store/src/site.ts")
	if err != nil {
		t.Fatalf("read the store app's list: %v", err)
	}
	m := regexp.MustCompile(`const reserved = new Set\(\[([^\]]*)\]\)`).FindSubmatch(src)
	if m == nil {
		t.Fatal("site.ts no longer declares `const reserved = new Set([...])`; update this test with it")
	}
	var web []string
	for _, q := range regexp.MustCompile(`'([^']*)'`).FindAllSubmatch(m[1], -1) {
		web = append(web, string(q[1]))
	}
	backend := ReservedSlugs()
	slices.Sort(web)
	slices.Sort(backend)
	if !slices.Equal(web, backend) {
		t.Errorf("reserved slugs differ: Go %v, store app %v", backend, web)
	}
}

func TestReservedSlugs_ReturnsACopy(t *testing.T) {
	t.Parallel()

	r := ReservedSlugs()
	r[0] = "altarpm"
	if _, err := NewSlugPolicy().Parse("altarpm"); err != nil {
		t.Error("changing the returned list changed the policy")
	}
}

func FuzzSlugPolicy_Parse(f *testing.F) {
	pattern := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	p := NewSlugPolicy("painel")
	for _, seed := range []string{"altarpm", "a-b", "--", "www", "Á"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		s, err := p.Parse(in)
		if err != nil {
			return
		}
		// Whatever passes fits the database CHECK of 0004_tenancy.
		if n := len(s.String()); n < 3 || n > 40 || !pattern.MatchString(s.String()) {
			t.Errorf("accepted %q", s)
		}
		if slices.Contains(ReservedSlugs(), s.String()) || s.String() == "painel" {
			t.Errorf("accepted the reserved %q", s)
		}
		if back, err := RestoreSlug(s.String()); err != nil || back != s {
			t.Errorf("RestoreSlug refused %q: %v", s, err)
		}
	})
}
