package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The two names the refresh cookie may have (ADR 0010).
const (
	// HostRefreshCookie is the production name. Its __Host- prefix makes
	// browsers refuse a cookie of that name set by a store subdomain, which
	// blocks cookie tossing.
	HostRefreshCookie = "__Host-encomendo_refresh"
	// PlainRefreshCookie is for a local stack on plain HTTP, where browsers
	// refuse __Host- cookies. The configuration refuses it outside
	// development (step 2.6).
	PlainRefreshCookie = "encomendo_refresh"
)

// transport is how a refresh token travels (the RefreshTransport schema).
type transport int

const (
	viaCookie transport = iota
	viaBody
)

// parseTransport reads a refreshTransport field: "cookie" by default.
func parseTransport(s string) (transport, error) {
	switch s {
	case "", "cookie":
		return viaCookie, nil
	case "body":
		return viaBody, nil
	default:
		return viaCookie, shared.NewValidationError(shared.FieldError{
			Field: "refreshTransport", Message: `Use "cookie" ou "body".`,
		})
	}
}

// errSeveralCookies is a refresh or switch with more than one refresh
// cookie, which a planted cookie would cause: refused rather than guessed
// (ADR 0010). Logout instead revokes every token it is given, so it always
// works (ADR 0010, amended in step 2.3).
var errSeveralCookies = fmt.Errorf("more than one refresh cookie: %w", shared.ErrUnauthenticated)

// RefreshCookie writes and reads the refresh token's cookie under one
// configured name. Only that name is ever read or counted, so a planted
// cookie of the other name is ignored rather than turned into a forced
// logout. The name, never the request's scheme, decides Secure: TLS ends
// at Caddy, so the API always sees plain HTTP.
type RefreshCookie struct {
	name string
}

// NewRefreshCookie returns the cookie named name, which must be
// HostRefreshCookie or PlainRefreshCookie.
func NewRefreshCookie(name string) (RefreshCookie, error) {
	if name != HostRefreshCookie && name != PlainRefreshCookie {
		return RefreshCookie{}, fmt.Errorf("handler: refresh cookie name %q is neither %s nor %s",
			name, HostRefreshCookie, PlainRefreshCookie)
	}
	return RefreshCookie{name: name}, nil
}

// IsZero reports whether c is the zero RefreshCookie, which names no cookie.
func (c RefreshCookie) IsZero() bool { return c.name == "" }

// values returns the token of every cookie of the configured name, in the
// order the request sent them.
func (c RefreshCookie) values(r *http.Request) []string {
	var out []string
	for _, k := range r.Cookies() {
		if k.Name == c.name {
			out = append(out, k.Value)
		}
	}
	return out
}

// set writes the cookie with token until expires: HttpOnly, SameSite=Strict,
// Path=/, no Domain, and Secure except on the plain name.
func (c RefreshCookie) set(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, c.cookie(token, expires.UTC(), 0))
}

// clear tells the browser to drop the cookie.
func (c RefreshCookie) clear(w http.ResponseWriter) {
	http.SetCookie(w, c.cookie("", time.Unix(0, 0).UTC(), -1))
}

func (c RefreshCookie) cookie(value string, expires time.Time, maxAge int) *http.Cookie {
	// Secure is off only on the plain name, which ADR 0010 keeps for a local
	// stack on plain HTTP and the configuration refuses outside development.
	return &http.Cookie{ //nolint:gosec // G124: Secure is deliberate per name, see above
		Name: c.name, Value: value, Path: "/", Expires: expires, MaxAge: maxAge,
		HttpOnly: true, Secure: c.name == HostRefreshCookie, SameSite: http.SameSiteStrictMode,
	}
}

// presented returns the refresh token a call sent and how it came: the body
// token, or the cookie. It is shared.ErrUnauthenticated for more than one
// cookie, and a ValidationError when both a cookie and a body token came
// (ADR 0010). No token at all is ("", viaCookie): the use case refuses it.
func (c RefreshCookie) presented(r *http.Request, body string) (string, transport, error) {
	cookies := c.values(r)
	count := len(cookies)
	cookie := ""
	if count == 1 {
		cookie = cookies[0]
	}
	switch {
	case count > 1:
		return "", viaCookie, errSeveralCookies
	case count == 1 && body != "":
		return "", viaCookie, shared.NewValidationError(shared.FieldError{
			Field: "refreshToken", Message: "Envie o token pelo cookie ou pelo corpo, não pelos dois.",
		})
	case body != "":
		return body, viaBody, nil
	default:
		return cookie, viaCookie, nil
	}
}
