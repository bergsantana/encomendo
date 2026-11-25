package auth_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/contract/identityports"
)

const issuer = "https://api.encomendo.com.br"

var start = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

// fixture is an AccessTokens on a fixed clock, with its signing key, raw
// too for forging tokens.
type fixture struct {
	clock   *sharedfake.Clock
	ids     *sharedfake.IDs
	signing auth.Key
	key     ed25519.PrivateKey
	keys    auth.KeySet
	tokens  *auth.AccessTokens
}

func newFixture(t testing.TB) *fixture {
	t.Helper()
	clock := sharedfake.NewClock(start)
	f := &fixture{clock: clock, ids: sharedfake.NewIDs(clock)}
	f.signing, f.key = newKey(t)
	f.keys = keySet(t, f.signing)
	f.tokens = accessTokens(t, f.keys, issuer, clock)
	return f
}

func keySet(t testing.TB, signing auth.Key, verifyOnly ...auth.Key) auth.KeySet {
	t.Helper()
	keys, err := auth.NewKeySet(signing, verifyOnly...)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func accessTokens(t testing.TB, keys auth.KeySet, iss string, clock *sharedfake.Clock) *auth.AccessTokens {
	t.Helper()
	tokens, err := auth.NewAccessTokens(keys, iss, clock)
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

// staff returns the claims of a staff member's token in a store, issued now.
func (f *fixture) staff(t testing.TB) appidentity.AccessClaims {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(shared.UserID(f.ids.NewID()), shared.TenantID(f.ids.NewID()), shared.RoleStaff)
	if err != nil {
		t.Fatal(err)
	}
	return f.claims(p)
}

func (f *fixture) claims(p shared.Principal) appidentity.AccessClaims {
	now := f.clock.Now()
	return appidentity.AccessClaims{
		Principal: p, JTI: f.ids.NewID(), IssuedAt: now, ExpiresAt: now.Add(identity.AccessTokenLifetime),
	}
}

func (f *fixture) issue(t testing.TB, c appidentity.AccessClaims) string {
	t.Helper()
	raw, err := f.tokens.Issue(context.Background(), c)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return raw
}

func TestAccessTokens_VerifyReturnsWhatIssueSigned(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	noStore, err := shared.NewBackofficePrincipal(shared.UserID(f.ids.NewID()), shared.TenantID{}, "")
	if err != nil {
		t.Fatal(err)
	}
	client, err := shared.NewClientPrincipal(shared.ClientAccountID(f.ids.NewID()))
	if err != nil {
		t.Fatal(err)
	}

	for name, c := range map[string]appidentity.AccessClaims{
		"staff in a store":      f.staff(t),
		"staff without a store": f.claims(noStore),
		"client of every store": f.claims(client),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := f.tokens.Verify(context.Background(), f.issue(t, c), c.Principal.Audience())
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if got != c {
				t.Errorf("Verify = %+v; want %+v", got, c)
			}
		})
	}
}

// TestAccessTokens_WriteTheClaimsOfTheArchitecture pins the token's shape
// (docs/architecture.md 5.1, ADR 0019): EdDSA, typ at+jwt and the signing
// key's kid, aud as one string, and tid and role only for a staff member in
// a store.
func TestAccessTokens_WriteTheClaimsOfTheArchitecture(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	c := f.staff(t)
	header, body := decodeParts(t, f.issue(t, c))

	if header["alg"] != "EdDSA" || header["typ"] != "at+jwt" || header["kid"] != f.keys.KID() || len(header) != 3 {
		t.Errorf("header %v; want alg EdDSA, typ at+jwt and kid %s", header, f.keys.KID())
	}
	want := []string{"aud", "exp", "iat", "iss", "jti", "role", "sub", "tid"}
	if got := slices.Sorted(maps.Keys(body)); !slices.Equal(got, want) {
		t.Errorf("claims %v; want %v", got, want)
	}
	for claim, value := range map[string]any{
		"iss": issuer, "aud": "backoffice", "sub": c.Principal.UserID().String(),
		"tid": c.Principal.TenantID().String(), "role": "staff", "jti": c.JTI.String(),
		"iat": float64(start.Unix()), "exp": float64(start.Add(identity.AccessTokenLifetime).Unix()),
	} {
		if body[claim] != value {
			t.Errorf("%s = %v (%T); want %v", claim, body[claim], body[claim], value)
		}
	}
}

// TestAccessTokens_PassTheAccessTokenIssuerContract: the refusals of
// appidentity.CheckAccessClaims, including a lifetime over
// identity.AccessTokenLifetime, which would outlive the denylist's window.
func TestAccessTokens_PassTheAccessTokenIssuerContract(t *testing.T) {
	t.Parallel()
	identityports.RunAccessTokenIssuerContract(t, func(t *testing.T) identityports.IssuerHarness {
		f := newFixture(t)
		return identityports.IssuerHarness{Issuer: f.tokens, NewID: f.ids.NewID, Now: start}
	})
}

func TestNewAccessTokens_RefusesAMissingPart(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	for name, build := range map[string]func() (*auth.AccessTokens, error){
		"no keys":   func() (*auth.AccessTokens, error) { return auth.NewAccessTokens(auth.KeySet{}, issuer, f.clock) },
		"no issuer": func() (*auth.AccessTokens, error) { return auth.NewAccessTokens(f.keys, " ", f.clock) },
		"no clock":  func() (*auth.AccessTokens, error) { return auth.NewAccessTokens(f.keys, issuer, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if tokens, err := build(); err == nil || tokens != nil {
				t.Errorf("NewAccessTokens = %v, %v; want an error", tokens, err)
			}
		})
	}
}

// decodeParts returns a token's header and claims as JSON objects.
func decodeParts(t *testing.T, raw string) (header, body map[string]any) {
	t.Helper()
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("%d parts; want 3", len(parts))
	}
	return decodeSegment(t, parts[0]), decodeSegment(t, parts[1])
}

func decodeSegment(t *testing.T, segment string) map[string]any {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// encodeSegment is decodeSegment's inverse, for forging tokens.
func encodeSegment(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// mustRefuse checks that Verify refuses raw for aud as a 401, and never as
// a 400: no part of a bad token may match shared.ErrInvalid.
func mustRefuse(t *testing.T, tokens *auth.AccessTokens, raw string, aud shared.Audience) {
	t.Helper()
	got, err := tokens.Verify(context.Background(), raw, aud)
	if err == nil {
		t.Fatalf("Verify accepted the token: %+v", got)
	}
	if !errors.Is(err, shared.ErrUnauthenticated) || !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("Verify: %v; want ErrInvalidToken, a 401", err)
	}
	for _, other := range []error{shared.ErrInvalid, shared.ErrNotFound, shared.ErrForbidden, shared.ErrConflict} {
		if errors.Is(err, other) {
			t.Errorf("Verify: %v also matches %v", err, other)
		}
	}
	if len(raw) > 20 && strings.Contains(err.Error(), raw) {
		t.Errorf("the error carries the token: %v", err)
	}
	if got != (appidentity.AccessClaims{}) {
		t.Errorf("Verify returned claims with its error: %+v", got)
	}
}
