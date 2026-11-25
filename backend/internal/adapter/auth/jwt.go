package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrInvalidToken means an access token was refused: malformed, signed by
// no key of the set or with another algorithm, of another type, for another
// audience or issuer, expired or too long-lived, or with claims that make
// no principal. It matches shared.ErrUnauthenticated, a 401, and nothing
// else, so no part of a bad token can turn into a 400.
var ErrInvalidToken = fmt.Errorf("invalid access token: %w", shared.ErrUnauthenticated)

// MaxTokenLength is the longest access token Verify reads. Real tokens are
// under 500 bytes; the cap keeps a forged one from costing a large decode.
const MaxTokenLength = 2048

// tokenType is the typ header of an access token (RFC 9068). This key set
// signs nothing else, and checking it keeps any later token type from
// passing as an access token.
const tokenType = "at+jwt"

// maxReason caps the reason a refusal carries: it may quote claim text a
// forger chose, and it goes to the log.
const maxReason = 200

// AccessTokens issues and verifies access tokens: JWTs signed with Ed25519
// (EdDSA) and named by kid (docs/architecture.md 5.1, ADRs 0003 and 0019).
// It implements appidentity.AccessTokenIssuer, and the authn middleware
// calls Verify. It is safe for concurrent use, and prints its kids only.
type AccessTokens struct {
	keys    KeySet
	issuer  string
	parsers map[shared.Audience]*jwt.Parser
}

// NewAccessTokens returns the issuer and verifier for keys, writing and
// requiring iss = issuer, and taking the time from clock. It returns an
// error for the zero KeySet, a blank issuer or a nil clock.
func NewAccessTokens(keys KeySet, issuer string, clock appshared.Clock) (*AccessTokens, error) {
	if keys.signing.IsZero() || strings.TrimSpace(issuer) == "" || clock == nil {
		return nil, errors.New("auth: access tokens need keys, an issuer and a clock")
	}
	a := &AccessTokens{keys: keys, issuer: issuer, parsers: map[shared.Audience]*jwt.Parser{}}
	for _, aud := range []shared.Audience{shared.AudienceBackoffice, shared.AudienceClient} {
		// One parser per audience: the audience is a parser option.
		a.parsers[aud] = jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
			jwt.WithIssuer(issuer),
			jwt.WithAudience(string(aud)),
			jwt.WithExpirationRequired(),
			jwt.WithIssuedAt(),
			jwt.WithLeeway(identity.AccessTokenLeeway),
			jwt.WithTimeFunc(clock.Now),
			jwt.WithStrictDecoding(),
		)
	}
	return a, nil
}

// String shows the issuer and the kids only.
func (a *AccessTokens) String() string {
	return "auth.AccessTokens{issuer: " + a.issuer + ", keys: " + a.keys.String() + "}"
}

// GoString shows the issuer and the kids only, for %#v.
func (a *AccessTokens) GoString() string { return a.String() }

// LogValue shows the issuer and the kids only, for slog.
func (a *AccessTokens) LogValue() slog.Value { return slog.StringValue(a.String()) }

// Issue signs c with the signing key, under its kid. It follows the
// appidentity.AccessTokenIssuer contract: an error for claims without a
// principal or a jti, or whose lifetime is not more than zero and at most
// identity.AccessTokenLifetime. Times are kept to the second, as JWT writes
// them.
func (a *AccessTokens) Issue(_ context.Context, c appidentity.AccessClaims) (string, error) {
	if err := appidentity.CheckAccessClaims(c); err != nil {
		return "", err
	}
	p := c.Principal
	body := &claims{
		Issuer: a.issuer, Audience: string(p.Audience()), ID: c.JTI.String(),
		IssuedAt: jwt.NewNumericDate(c.IssuedAt), ExpiresAt: jwt.NewNumericDate(c.ExpiresAt),
	}
	if p.Audience() == shared.AudienceClient {
		body.Subject = p.ClientAccountID().String()
	} else {
		body.Subject = p.UserID().String()
	}
	if p.HasTenant() {
		body.TenantID, body.Role = p.TenantID().String(), string(p.Role())
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, body)
	token.Header["typ"] = tokenType
	token.Header["kid"] = a.keys.KID()
	signed, err := token.SignedString(a.keys.signing.m)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

// Verify checks raw as an access token for aud and returns its claims,
// times in UTC. Every refusal is an ErrInvalidToken, whose text names the
// reason but never the token: a byte outside base64url and '.', more than
// MaxTokenLength bytes, alg other than EdDSA (none, HS*), typ other than
// at+jwt, a crit header, a kid missing or not in the set, a bad signature,
// iss other than the issuer, aud other than aud (or not one string), exp
// missing or passed, iat missing or in the future, nbf in the future (each
// with identity.AccessTokenLeeway), a lifetime over
// identity.AccessTokenLifetime, and claims that make no principal, such as
// a client token naming a store. An aud other than the two audiences is a
// wiring bug: its error is not ErrInvalidToken, so it is a 500, not a 401
// that would send every client into a refresh loop. Verify does not check
// the denylist; the authn middleware does.
func (a *AccessTokens) Verify(_ context.Context, raw string, aud shared.Audience) (appidentity.AccessClaims, error) {
	parser, ok := a.parsers[aud]
	if !ok {
		return appidentity.AccessClaims{}, fmt.Errorf("auth: verify for unknown audience %q", aud)
	}
	if len(raw) > MaxTokenLength {
		return appidentity.AccessClaims{}, fmt.Errorf("%w: %d bytes", ErrInvalidToken, len(raw))
	}
	if !tokenText(raw) {
		return appidentity.AccessClaims{}, fmt.Errorf("%w: a byte outside base64url and '.'", ErrInvalidToken)
	}
	var body claims
	if _, err := parser.ParseWithClaims(raw, &body, a.key); err != nil {
		return appidentity.AccessClaims{}, refusal(err)
	}
	out, err := body.accessClaims()
	if err != nil {
		return appidentity.AccessClaims{}, refusal(err)
	}
	return out, nil
}

// refusal is ErrInvalidToken with err's text, capped. The reason goes in as
// text, not wrapped: it is a detail for the log, and a refusal must match
// ErrInvalidToken alone (shared.ErrInvalidID, say, would make it a 400).
func refusal(err error) error {
	reason := err.Error()
	if len(reason) > maxReason {
		reason = strings.ToValidUTF8(reason[:maxReason], "") + "…"
	}
	return fmt.Errorf("%w: %s", ErrInvalidToken, reason)
}

// tokenText reports whether raw holds only the bytes a JWT is written in.
// The base64 decoder skips line breaks even in strict mode, so without this
// check a break inside the signature would be a second spelling of a valid
// token.
func tokenText(raw string) bool {
	for i := range len(raw) {
		switch c := raw[i]; {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

// key finds the public key that verifies t, by its kid, after checking the
// header. The parser has already refused every algorithm but EdDSA, before
// it calls key.
func (a *AccessTokens) key(t *jwt.Token) (any, error) {
	if typ, _ := t.Header["typ"].(string); typ != tokenType {
		return nil, errors.New("not an access token (typ)")
	}
	// RFC 7515 4.1.11: a crit extension this verifier does not know must be
	// refused, and it knows none.
	if _, ok := t.Header["crit"]; ok {
		return nil, errors.New("a crit header")
	}
	kid, ok := t.Header["kid"].(string)
	if !ok {
		return nil, errors.New("no kid")
	}
	key, ok := a.keys.verifier(kid)
	if !ok {
		return nil, errors.New("unknown kid")
	}
	return key, nil
}

// claims is the JWT body (docs/architecture.md 5.1). aud is one string,
// never an array: one token valid for both audiences would pass both route
// checks. tid and role are set only for a staff member in a store.
type claims struct {
	Issuer    string           `json:"iss"`
	Subject   string           `json:"sub"`
	Audience  string           `json:"aud"`
	TenantID  string           `json:"tid,omitempty"`
	Role      string           `json:"role,omitempty"`
	ID        string           `json:"jti"`
	IssuedAt  *jwt.NumericDate `json:"iat"`
	ExpiresAt *jwt.NumericDate `json:"exp"`
	NotBefore *jwt.NumericDate `json:"nbf,omitempty"`
}

// GetExpirationTime implements jwt.Claims.
func (c *claims) GetExpirationTime() (*jwt.NumericDate, error) { return c.ExpiresAt, nil }

// GetIssuedAt implements jwt.Claims.
func (c *claims) GetIssuedAt() (*jwt.NumericDate, error) { return c.IssuedAt, nil }

// GetNotBefore implements jwt.Claims.
func (c *claims) GetNotBefore() (*jwt.NumericDate, error) { return c.NotBefore, nil }

// GetIssuer implements jwt.Claims.
func (c *claims) GetIssuer() (string, error) { return c.Issuer, nil }

// GetSubject implements jwt.Claims.
func (c *claims) GetSubject() (string, error) { return c.Subject, nil }

// GetAudience implements jwt.Claims.
func (c *claims) GetAudience() (jwt.ClaimStrings, error) { return jwt.ClaimStrings{c.Audience}, nil }

// accessClaims reads the verified body. The parser has checked iss, aud,
// exp and the times; this checks what makes a principal, and the lifetime,
// as appidentity.CheckAccessClaims does on Issue: identity.DenylistWindow
// holds only if no accepted token lives longer than
// identity.AccessTokenLifetime.
func (c *claims) accessClaims() (appidentity.AccessClaims, error) {
	jti, err := shared.ParseID(c.ID)
	if err != nil || c.IssuedAt == nil || c.ExpiresAt == nil {
		return appidentity.AccessClaims{}, errors.New("no jti or no issue time")
	}
	if lifetime := c.ExpiresAt.Sub(c.IssuedAt.Time); lifetime <= 0 || lifetime > identity.AccessTokenLifetime {
		return appidentity.AccessClaims{}, errors.New("a lifetime no access token has")
	}
	p, err := c.principal()
	if err != nil {
		return appidentity.AccessClaims{}, err
	}
	return appidentity.AccessClaims{
		Principal: p, JTI: jti, IssuedAt: c.IssuedAt.UTC(), ExpiresAt: c.ExpiresAt.UTC(),
	}, nil
}

func (c *claims) principal() (shared.Principal, error) {
	sub, err := shared.ParseID(c.Subject)
	if err != nil {
		return shared.Principal{}, errors.New("subject is not an id")
	}
	if shared.Audience(c.Audience) == shared.AudienceClient {
		if c.TenantID != "" || c.Role != "" {
			return shared.Principal{}, errors.New("a client token names a store")
		}
		return shared.NewClientPrincipal(shared.ClientAccountID(sub))
	}
	var tenant shared.TenantID
	if c.TenantID != "" {
		if tenant, err = shared.ParseTenantID(c.TenantID); err != nil {
			return shared.Principal{}, errors.New("store is not an id")
		}
	}
	return shared.NewBackofficePrincipal(shared.UserID(sub), tenant, shared.Role(c.Role))
}
