package identityports

import (
	"context"
	"errors"
	"testing"
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// IssuerHarness is an AccessTokenIssuer with what its contract needs.
type IssuerHarness struct {
	Issuer appidentity.AccessTokenIssuer
	NewID  func() shared.ID
	// Now is a whole second: JWT writes times to the second.
	Now time.Time
}

// RunAccessTokenIssuerContract checks that an issuer signs valid claims for
// both audiences, one token per jti, and refuses with
// ErrIncompleteAccessClaims the claims CheckAccessClaims refuses, so a use
// case tested on the fake meets the same refusals as on adapter/auth.
func RunAccessTokenIssuerContract(t *testing.T, newHarness func(t *testing.T) IssuerHarness) {
	t.Run("signs staff and client claims, one token per jti", func(t *testing.T) {
		h := newHarness(t)
		seen := map[string]bool{}
		for _, c := range []appidentity.AccessClaims{staffClaims(t, h), staffClaims(t, h), clientClaims(t, h)} {
			raw, err := h.Issuer.Issue(context.Background(), c)
			if err != nil || raw == "" {
				t.Fatalf("Issue(%+v) = %q, %v; want a token", c, raw, err)
			}
			if seen[raw] {
				t.Errorf("two jtis got the same token %q", raw)
			}
			seen[raw] = true
		}
	})
	t.Run("signs the longest lifetime there is", func(t *testing.T) {
		h := newHarness(t)
		c := staffClaims(t, h)
		c.ExpiresAt = c.IssuedAt.Add(identity.AccessTokenLifetime)
		if _, err := h.Issuer.Issue(context.Background(), c); err != nil {
			t.Errorf("Issue with a lifetime of identity.AccessTokenLifetime: %v", err)
		}
	})
	for name, edit := range map[string]func(*appidentity.AccessClaims){
		"no principal":          func(c *appidentity.AccessClaims) { c.Principal = shared.Principal{} },
		"no jti":                func(c *appidentity.AccessClaims) { c.JTI = shared.ID{} },
		"expires when issued":   func(c *appidentity.AccessClaims) { c.ExpiresAt = c.IssuedAt },
		"expires before issued": func(c *appidentity.AccessClaims) { c.ExpiresAt = c.IssuedAt.Add(-time.Minute) },
		"lives a second too long": func(c *appidentity.AccessClaims) {
			c.ExpiresAt = c.IssuedAt.Add(identity.AccessTokenLifetime + time.Second)
		},
	} {
		t.Run("refuses claims with "+name, func(t *testing.T) {
			h := newHarness(t)
			c := staffClaims(t, h)
			edit(&c)
			if raw, err := h.Issuer.Issue(context.Background(), c); !errors.Is(err, appidentity.ErrIncompleteAccessClaims) || raw != "" {
				t.Errorf("Issue = %q, %v; want ErrIncompleteAccessClaims", raw, err)
			}
		})
	}
}

func staffClaims(t *testing.T, h IssuerHarness) appidentity.AccessClaims {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(shared.UserID(h.NewID()), shared.TenantID(h.NewID()), shared.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	return accessClaims(h, p)
}

func clientClaims(t *testing.T, h IssuerHarness) appidentity.AccessClaims {
	t.Helper()
	p, err := shared.NewClientPrincipal(shared.ClientAccountID(h.NewID()))
	if err != nil {
		t.Fatal(err)
	}
	return accessClaims(h, p)
}

func accessClaims(h IssuerHarness, p shared.Principal) appidentity.AccessClaims {
	return appidentity.AccessClaims{
		Principal: p, JTI: h.NewID(), IssuedAt: h.Now, ExpiresAt: h.Now.Add(identity.AccessTokenLifetime),
	}
}
