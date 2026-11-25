package tenancy

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	t0       = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	tenantID = shared.TenantID(shared.NewV7(t0.UnixMilli(), [10]byte{1}))
	ownerID  = shared.UserID(shared.NewV7(t0.UnixMilli(), [10]byte{2}))
	staffID  = shared.UserID(shared.NewV7(t0.UnixMilli(), [10]byte{3}))
)

func mustSlug(t *testing.T, s string) Slug {
	t.Helper()
	slug, err := NewSlugPolicy().Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return slug
}

func mustStoreName(t *testing.T, s string) StoreName {
	t.Helper()
	n, err := ParseStoreName(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestParseStoreName(t *testing.T) {
	t.Parallel()

	if n, err := ParseStoreName("  Alta RPM "); err != nil || n.String() != "Alta RPM" {
		t.Errorf("ParseStoreName = %q, %v", n, err)
	}
	if _, err := ParseStoreName(strings.Repeat("é", 80)); err != nil {
		t.Errorf("80 characters refused: %v", err)
	}
	for _, bad := range []string{"", "  ", strings.Repeat("a", 81), "Loja\tNova"} {
		_, err := ParseStoreName(bad)
		var ve *shared.ValidationError
		if !errors.As(err, &ve) || ve.Fields[0].Field != "name" {
			t.Errorf("ParseStoreName(%q) = %v, want a ValidationError on name", bad, err)
		}
	}
}

func TestNewTenant(t *testing.T) {
	t.Parallel()

	tn, err := NewTenant(tenantID, mustStoreName(t, "Alta RPM"), mustSlug(t, "altarpm"), t0)
	if err != nil {
		t.Fatal(err)
	}
	if tn.ID() != tenantID || tn.Name().String() != "Alta RPM" || tn.Slug().String() != "altarpm" {
		t.Errorf("tenant = %v %q %q", tn.ID(), tn.Name(), tn.Slug())
	}
	if tn.Status() != StatusActive || tn.Version() != 1 || !tn.CreatedAt().Equal(t0) {
		t.Errorf("new tenant: status %q, version %d, created %v", tn.Status(), tn.Version(), tn.CreatedAt())
	}
	for name, build := range map[string]func() (*Tenant, error){
		"zero id": func() (*Tenant, error) {
			return NewTenant(shared.TenantID{}, mustStoreName(t, "A"), mustSlug(t, "abc"), t0)
		},
		"zero name": func() (*Tenant, error) { return NewTenant(tenantID, StoreName{}, mustSlug(t, "abc"), t0) },
		"zero slug": func() (*Tenant, error) { return NewTenant(tenantID, mustStoreName(t, "A"), Slug{}, t0) },
	} {
		if _, err := build(); !errors.Is(err, ErrInvalidTenant) {
			t.Errorf("%s: err = %v, want ErrInvalidTenant", name, err)
		}
	}
}

func TestRestoreTenant(t *testing.T) {
	t.Parallel()

	tn, err := RestoreTenant(TenantRecord{ID: tenantID, Slug: "altarpm", Name: "Alta RPM", Status: "suspended", Version: 3, CreatedAt: t0})
	if err != nil {
		t.Fatal(err)
	}
	if tn.Status() != StatusSuspended || tn.Version() != 3 {
		t.Errorf("restored status %q, version %d", tn.Status(), tn.Version())
	}
	for name, rec := range map[string]TenantRecord{
		"bad slug":     {ID: tenantID, Slug: "A B", Name: "x", Status: "active", Version: 1},
		"bad name":     {ID: tenantID, Slug: "abc", Name: "", Status: "active", Version: 1},
		"bad status":   {ID: tenantID, Slug: "abc", Name: "x", Status: "closed", Version: 1},
		"zero version": {ID: tenantID, Slug: "abc", Name: "x", Status: "active"},
		"zero id":      {Slug: "abc", Name: "x", Status: "active", Version: 1},
	} {
		if _, err := RestoreTenant(rec); !errors.Is(err, ErrInvalidTenant) {
			t.Errorf("%s: err = %v, want ErrInvalidTenant", name, err)
		}
	}
}
