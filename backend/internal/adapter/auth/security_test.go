package auth_test

import (
	"context"
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The auth security tests of docs/architecture.md section 9 for access
// tokens: expired, wrong aud, alg none, unknown kid, and every other way to
// forge or stretch a token. Each must be a 401 (mustRefuse).

// body returns c as the claims the issuer writes, for a forger to change.
func body(c appidentity.AccessClaims) jwt.MapClaims {
	return jwt.MapClaims{
		"iss": issuer, "aud": string(c.Principal.Audience()), "sub": c.Principal.UserID().String(),
		"tid": c.Principal.TenantID().String(), "role": string(c.Principal.Role()), "jti": c.JTI.String(),
		"iat": c.IssuedAt.Unix(), "exp": c.ExpiresAt.Unix(),
	}
}

// sign signs claims with key, as someone who holds the key could: for
// tokens the issuer would never write. The header is typ at+jwt with
// header's entries on top; a nil entry removes one.
func sign(t *testing.T, method jwt.SigningMethod, key any, header map[string]any, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	token.Header["typ"] = "at+jwt"
	for name, value := range header {
		if value == nil {
			delete(token.Header, name)
			continue
		}
		token.Header[name] = value
	}
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// edit changes the claims of a signed token and keeps its signature.
func edit(t *testing.T, raw string, change func(map[string]any)) string {
	t.Helper()
	parts := strings.Split(raw, ".")
	claims := decodeSegment(t, parts[1])
	change(claims)
	return parts[0] + "." + encodeSegment(t, claims) + "." + parts[2]
}

// with returns c's claims, signed with the fixture's own key and kid, after
// change: tokens that carry a valid signature but break a rule.
func (f *fixture) with(t *testing.T, c appidentity.AccessClaims, change func(jwt.MapClaims)) string {
	t.Helper()
	claims := body(c)
	change(claims)
	return sign(t, jwt.SigningMethodEdDSA, f.key, kid(f.keys.KID()), claims)
}

// kid is the header of a token named id.
func kid(id any) map[string]any { return map[string]any{"kid": id} }

func TestAccessTokens_RefuseForgedTokens(t *testing.T) {
	t.Parallel()
	backoffice := shared.AudienceBackoffice

	for name, forge := range map[string]func(t *testing.T, f *fixture, c appidentity.AccessClaims) string{
		"alg none": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			header := encodeSegment(t, map[string]any{"alg": "none", "typ": "at+jwt", "kid": f.keys.KID()})
			return header + "." + strings.Split(f.issue(t, c), ".")[1] + "."
		},
		"HS256 keyed with the public key": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			public, ok := f.key.Public().(ed25519.PublicKey)
			if !ok {
				t.Fatal("no Ed25519 public key")
			}
			return sign(t, jwt.SigningMethodHS256, []byte(public), kid(f.keys.KID()), body(c))
		},
		"unknown kid": func(t *testing.T, _ *fixture, c appidentity.AccessClaims) string {
			other, raw := newKey(t)
			return sign(t, jwt.SigningMethodEdDSA, raw, kid(other.KID()), body(c))
		},
		"known kid, another key's signature": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			_, raw := newKey(t)
			return sign(t, jwt.SigningMethodEdDSA, raw, kid(f.keys.KID()), body(c))
		},
		"no kid": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			return sign(t, jwt.SigningMethodEdDSA, f.key, nil, body(c))
		},
		"kid not a string": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			return sign(t, jwt.SigningMethodEdDSA, f.key, kid(7), body(c))
		},
		"typ JWT, as a token of another kind": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			return sign(t, jwt.SigningMethodEdDSA, f.key, map[string]any{"kid": f.keys.KID(), "typ": "JWT"}, body(c))
		},
		"no typ": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			return sign(t, jwt.SigningMethodEdDSA, f.key, map[string]any{"kid": f.keys.KID(), "typ": nil}, body(c))
		},
		"a crit header": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			return sign(t, jwt.SigningMethodEdDSA, f.key, map[string]any{"kid": f.keys.KID(), "crit": []string{"exp"}}, body(c))
		},
		"role raised to owner": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			return edit(t, f.issue(t, c), func(m map[string]any) { m["role"] = "owner" })
		},
		"moved to another store": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			return edit(t, f.issue(t, c), func(m map[string]any) { m["tid"] = f.ids.NewID().String() })
		},
		"expiry pushed back": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			return edit(t, f.issue(t, c), func(m map[string]any) { m["exp"] = c.ExpiresAt.Add(24 * time.Hour).Unix() })
		},
		"signature stripped": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			parts := strings.Split(f.issue(t, c), ".")
			return parts[0] + "." + parts[1] + "."
		},
		"another token's signature": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			parts := strings.Split(f.issue(t, c), ".")
			return parts[0] + "." + parts[1] + "." + strings.Split(f.issue(t, f.staff(t)), ".")[2]
		},
		"another issuer": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			raw, err := accessTokens(t, f.keys, "https://outra.example", f.clock).Issue(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			return raw
		},
		"a line break in the signature": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			raw := f.issue(t, c)
			return raw[:len(raw)-10] + "\n" + raw[len(raw)-10:]
		},
		"whitespace around it": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			return " " + f.issue(t, c) + " "
		},
		"padded base64": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			parts := strings.Split(f.issue(t, c), ".")
			return parts[0] + "=." + parts[1] + "." + parts[2]
		},
		"longer than any real token": func(t *testing.T, f *fixture, c appidentity.AccessClaims) string {
			return f.with(t, c, func(m jwt.MapClaims) { m["pad"] = strings.Repeat("a", 5000) })
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			mustRefuse(t, f.tokens, forge(t, f, f.staff(t)), backoffice)
		})
	}
}

// TestAccessTokens_RefuseSignedTokensThatBreakARule covers tokens with a
// valid signature that the issuer would never write. Only a leaked key or a
// bug makes one, and neither may grant access.
func TestAccessTokens_RefuseSignedTokensThatBreakARule(t *testing.T) {
	t.Parallel()

	for name, change := range map[string]func(f *fixture, m jwt.MapClaims){
		"no expiry":                    func(_ *fixture, m jwt.MapClaims) { delete(m, "exp") },
		"no issue time":                func(_ *fixture, m jwt.MapClaims) { delete(m, "iat") },
		"no jti":                       func(_ *fixture, m jwt.MapClaims) { delete(m, "jti") },
		"jti not an id":                func(_ *fixture, m jwt.MapClaims) { m["jti"] = "42" },
		"no subject":                   func(_ *fixture, m jwt.MapClaims) { delete(m, "sub") },
		"subject not an id":            func(_ *fixture, m jwt.MapClaims) { m["sub"] = "ana@ex.com" },
		"store not an id":              func(_ *fixture, m jwt.MapClaims) { m["tid"] = "loja-da-ana" },
		"store without a role":         func(_ *fixture, m jwt.MapClaims) { delete(m, "role") },
		"role without a store":         func(_ *fixture, m jwt.MapClaims) { delete(m, "tid") },
		"a role that does not exist":   func(_ *fixture, m jwt.MapClaims) { m["role"] = "root" },
		"both audiences":               func(_ *fixture, m jwt.MapClaims) { m["aud"] = []string{"backoffice", "client"} },
		"audience as a one-item array": func(_ *fixture, m jwt.MapClaims) { m["aud"] = []string{"backoffice"} },
		"no audience":                  func(_ *fixture, m jwt.MapClaims) { delete(m, "aud") },
		"no issuer":                    func(_ *fixture, m jwt.MapClaims) { delete(m, "iss") },
		"not valid yet": func(f *fixture, m jwt.MapClaims) {
			m["nbf"] = f.clock.Now().Add(identity.AccessTokenLeeway + time.Second).Unix()
		},
		"issued in the future": func(f *fixture, m jwt.MapClaims) {
			m["iat"] = f.clock.Now().Add(identity.AccessTokenLeeway + time.Second).Unix()
		},
		"lives a second longer than an access token": func(f *fixture, m jwt.MapClaims) {
			m["exp"] = f.clock.Now().Add(identity.AccessTokenLifetime + time.Second).Unix()
		},
		"expires before it is issued, both within the leeway": func(f *fixture, m jwt.MapClaims) {
			m["iat"] = f.clock.Now().Add(20 * time.Second).Unix()
			m["exp"] = f.clock.Now().Add(10 * time.Second).Unix()
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			mustRefuse(t, f.tokens, f.with(t, f.staff(t), func(m jwt.MapClaims) { change(f, m) }), shared.AudienceBackoffice)
		})
	}
}

func TestAccessTokens_RefuseAClientTokenThatNamesAStore(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	client, err := shared.NewClientPrincipal(shared.ClientAccountID(f.ids.NewID()))
	if err != nil {
		t.Fatal(err)
	}
	c := f.claims(client)

	raw := f.with(t, c, func(m jwt.MapClaims) {
		m["sub"] = client.ClientAccountID().String()
		m["tid"], m["role"] = f.ids.NewID().String(), "owner"
	})
	mustRefuse(t, f.tokens, raw, shared.AudienceClient)
}

func TestAccessTokens_RefuseMalformedText(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	for _, raw := range []string{"", ".", "..", "a.b", "a.b.c", "a.b.c.d", "Bearer x.y.z", strings.Repeat("a", 5000)} {
		mustRefuse(t, f.tokens, raw, shared.AudienceBackoffice)
	}
}

// TestAccessTokens_KeepStaffAndClientsApart: a client token is refused on
// staff routes and a staff token on the client area (docs/architecture.md
// 5.1). A caller naming neither audience is a wiring bug, refused as a 500:
// a 401 would send every client into a refresh loop.
func TestAccessTokens_KeepStaffAndClientsApart(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	client, err := shared.NewClientPrincipal(shared.ClientAccountID(f.ids.NewID()))
	if err != nil {
		t.Fatal(err)
	}
	staff, clientToken := f.issue(t, f.staff(t)), f.issue(t, f.claims(client))

	mustRefuse(t, f.tokens, staff, shared.AudienceClient)
	mustRefuse(t, f.tokens, clientToken, shared.AudienceBackoffice)
	for _, aud := range []shared.Audience{"", "admin"} {
		got, err := f.tokens.Verify(context.Background(), staff, aud)
		if err == nil || errors.Is(err, shared.ErrUnauthenticated) || got != (appidentity.AccessClaims{}) {
			t.Errorf("Verify for audience %q = %+v, %v; want an error that is not a 401", aud, got, err)
		}
	}
}

// TestAccessTokens_ExpireAfterFifteenMinutesAndThirtySecondsOfSkew pins the
// lifetime and the leeway of docs/architecture.md 5.1 at both ends.
func TestAccessTokens_ExpireAfterFifteenMinutesAndThirtySecondsOfSkew(t *testing.T) {
	t.Parallel()
	edge := identity.AccessTokenLifetime + identity.AccessTokenLeeway

	for name, tc := range map[string]struct {
		at   time.Duration
		want bool
	}{
		"a second before issue time minus the leeway": {-identity.AccessTokenLeeway - time.Second, false},
		"the leeway before issue time":                {-identity.AccessTokenLeeway, true},
		"at issue time":                               {0, true},
		"a second before expiry plus the leeway":      {edge - time.Second, true},
		"at expiry plus the leeway":                   {edge, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			raw := f.issue(t, f.staff(t))
			f.clock.Set(start.Add(tc.at))

			if !tc.want {
				mustRefuse(t, f.tokens, raw, shared.AudienceBackoffice)
				return
			}
			if _, err := f.tokens.Verify(context.Background(), raw, shared.AudienceBackoffice); err != nil {
				t.Errorf("Verify at %v: %v; want the token accepted", tc.at, err)
			}
		})
	}
}

// TestAccessTokens_RotateKeysByKid: a rotation first adds the next key as
// verify-only on every instance, then signs with it and keeps the old one
// verify-only until the old tokens expire (ADR 0019).
func TestAccessTokens_RotateKeysByKid(t *testing.T) {
	t.Parallel()
	old := newFixture(t)
	oldToken := old.issue(t, old.staff(t))
	next, _ := newKey(t)

	prepared := accessTokens(t, keySet(t, old.signing, next), issuer, old.clock)
	rotated := accessTokens(t, keySet(t, next, old.signing), issuer, old.clock)
	retired := accessTokens(t, keySet(t, next), issuer, old.clock)
	newToken, err := rotated.Issue(context.Background(), old.staff(t))
	if err != nil {
		t.Fatal(err)
	}

	if header, _ := decodeParts(t, newToken); header["kid"] != next.KID() {
		t.Errorf("kid after the rotation %v; want the next key's", header["kid"])
	}
	for name, tc := range map[string]struct {
		tokens *auth.AccessTokens
		raw    string
	}{
		"prepared, old token": {prepared, oldToken}, "prepared, new token": {prepared, newToken},
		"rotated, old token": {rotated, oldToken}, "rotated, new token": {rotated, newToken},
		"retired, new token": {retired, newToken},
	} {
		if _, err := tc.tokens.Verify(context.Background(), tc.raw, shared.AudienceBackoffice); err != nil {
			t.Errorf("%s: %v; want it accepted", name, err)
		}
	}
	mustRefuse(t, retired, oldToken, shared.AudienceBackoffice)
	mustRefuse(t, old.tokens, newToken, shared.AudienceBackoffice)
}
