package shared

import (
	"context"
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var tenantA = shared.TenantID(shared.NewV7(1, [10]byte{0xa}))

func TestTenantKey_StartsWithTheTenant(t *testing.T) {
	t.Parallel()

	k, err := TenantKey(tenantA, "catalog", "item", "0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e")
	if err != nil {
		t.Fatal(err)
	}
	want := "t:" + tenantA.String() + ":catalog:item:0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e"
	if k.String() != want || k.IsZero() {
		t.Errorf("key = %q, want %q", k.String(), want)
	}
}

func TestGlobalKey(t *testing.T) {
	t.Parallel()

	k, err := GlobalKey("workflow", "presets")
	if err != nil || k.String() != "g:workflow:presets" {
		t.Errorf("GlobalKey = %q, %v", k.String(), err)
	}
}

func TestKeys_RejectPartsThatCouldEscapeTheirSpace(t *testing.T) {
	t.Parallel()

	for _, part := range []string{"", "a:b", "*", "item?", "[x]", `a\b`, "two words", "ação"} {
		t.Run(part, func(t *testing.T) {
			t.Parallel()

			if _, err := TenantKey(tenantA, "catalog", part); !errors.Is(err, ErrInvalidCacheKey) {
				t.Errorf("TenantKey part %q error = %v, want ErrInvalidCacheKey", part, err)
			}
			if _, err := GlobalKey(part); !errors.Is(err, ErrInvalidCacheKey) {
				t.Errorf("GlobalKey part %q error = %v, want ErrInvalidCacheKey", part, err)
			}
		})
	}
}

func TestKeys_RejectMissingOwnerOrParts(t *testing.T) {
	t.Parallel()

	if _, err := TenantKey(shared.TenantID{}, "catalog"); !errors.Is(err, ErrInvalidCacheKey) {
		t.Errorf("zero tenant error = %v", err)
	}
	if _, err := TenantKey(tenantA); !errors.Is(err, ErrInvalidCacheKey) {
		t.Errorf("no parts error = %v", err)
	}
	if _, err := GlobalKey(); !errors.Is(err, ErrInvalidCacheKey) {
		t.Errorf("global without parts error = %v", err)
	}
	if !(Key{}).IsZero() {
		t.Error("Key{}.IsZero() = false")
	}
}

func TestPortMisuseErrors_AreNotBadInput(t *testing.T) {
	t.Parallel()

	// These mean the calling code is wrong, not the user's input: they must
	// reach the 500 path, which logs, instead of a silent 400 (ADR 0009).
	for _, err := range []error{
		ErrNoTransaction, ErrInvalidScope, ErrInvalidCacheKey, ErrInvalidTTL,
		ErrInvalidCacheValue, ErrInvalidEvent, ErrEventWithoutTenant,
	} {
		for _, sentinel := range []error{shared.ErrInvalid, shared.ErrNotFound, shared.ErrConflict, shared.ErrForbidden, shared.ErrUnauthenticated} {
			if errors.Is(err, sentinel) {
				t.Errorf("%v matches %v", err, sentinel)
			}
		}
	}
}

func TestScope_Validate(t *testing.T) {
	t.Parallel()

	account := shared.ClientAccountID(shared.NewV7(3, [10]byte{3}))
	user := shared.UserID(shared.NewV7(2, [10]byte{2}))
	valid := []Scope{
		{},
		{TenantID: tenantA},
		{TenantID: tenantA, UserID: user},
		{UserID: user},
		{ClientAccountID: account},
	}
	for _, s := range valid {
		if err := s.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v, want nil", s, err)
		}
	}
	// Postgres ORs permissive policies: a tenant and a client account
	// together would read the whole store plus the client's orders
	// everywhere (docs/architecture.md section 5.5, item 7).
	both := Scope{TenantID: tenantA, ClientAccountID: account}
	if err := both.Validate(); !errors.Is(err, ErrInvalidScope) {
		t.Errorf("Validate(tenant + client) = %v, want ErrInvalidScope", err)
	}
	clientAndUser := Scope{UserID: user, ClientAccountID: account}
	if err := clientAndUser.Validate(); !errors.Is(err, ErrInvalidScope) {
		t.Errorf("Validate(user + client) = %v, want ErrInvalidScope", err)
	}
}

func TestScope_RoundTripsThroughContext(t *testing.T) {
	t.Parallel()

	if got := ScopeFrom(context.Background()); got != (Scope{}) {
		t.Errorf("ScopeFrom(empty ctx) = %+v, want the zero scope", got)
	}
	s := Scope{TenantID: tenantA, UserID: shared.UserID(shared.NewV7(2, [10]byte{}))}
	ctx := WithScope(context.Background(), s)
	if got := ScopeFrom(ctx); got != s {
		t.Errorf("ScopeFrom = %+v, want %+v", got, s)
	}
	replaced := WithScope(ctx, Scope{})
	if got := ScopeFrom(replaced); got != (Scope{}) {
		t.Errorf("WithScope did not replace the scope: %+v", got)
	}
}
