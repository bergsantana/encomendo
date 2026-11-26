package middleware

import (
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// errCrossOrigin is a browser call from another origin.
var errCrossOrigin = fmt.Errorf("cross-origin call to /auth: %w", shared.ErrForbidden)

// CrossOrigin returns the middleware that guards every /auth/* call against
// cross-site requests (ADR 0010). The API sends no CORS headers, so the
// browser itself only stops other origins from reading answers; this stops
// them from sending requests, which covers the refresh cookie and login
// CSRF:
//   - an Origin header other than adminOrigin is 403 forbidden, and so is
//     more than one;
//   - a Sec-Fetch-Site header other than same-origin or none is 403
//     forbidden: same-site matters most, because store pages share the
//     registrable domain and SameSite cookies alone would not stop them;
//   - a request with neither header passes, because native clients send
//     neither (ADR 0013);
//   - then the one Content-Type must be application/json, parsed as a media
//     type, so parameters such as charset=utf-8 pass. Anything else is 400
//     invalid, naming the header.
//
// It returns an error when adminOrigin is not an origin as browsers write
// it: http or https, a lower-case host, an optional port, and nothing else.
func CrossOrigin(adminOrigin string, log *slog.Logger) (Middleware, error) {
	if err := checkOrigin(adminOrigin); err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if reason := crossOriginReason(r.Header, adminOrigin); reason != "" {
				log.DebugContext(r.Context(), "cross-origin call refused", "reason", reason)
				respond.Error(w, r, log, errCrossOrigin)
				return
			}
			if !jsonBody(r.Header) {
				respond.Error(w, r, log, shared.NewValidationError(shared.FieldError{
					Field: "Content-Type", Message: "Envie o corpo em JSON (application/json).",
				}))
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

// crossOriginReason returns why a request looks like a browser call from
// elsewhere, or "" when it does not.
func crossOriginReason(h http.Header, adminOrigin string) string {
	origins := h.Values("Origin")
	switch {
	case len(origins) > 1:
		return "more than one Origin header"
	case len(origins) == 1 && origins[0] != adminOrigin:
		return "another origin"
	}
	sites := h.Values("Sec-Fetch-Site")
	switch {
	case len(sites) > 1:
		return "more than one Sec-Fetch-Site header"
	case len(sites) == 1 && (sites[0] == "same-site" || sites[0] == "cross-site"):
		return "Sec-Fetch-Site " + sites[0]
	case len(sites) == 1 && sites[0] != "same-origin" && sites[0] != "none":
		// Refused too, to fail closed. The value is a stranger's text, so it
		// stays out of the log.
		return "an unknown Sec-Fetch-Site value"
	}
	return ""
}

// jsonBody reports whether h has exactly one Content-Type, and it is
// application/json.
func jsonBody(h http.Header) bool {
	types := h.Values("Content-Type")
	if len(types) != 1 {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(types[0])
	return err == nil && mediaType == "application/json"
}

// checkOrigin returns an error unless origin is serialized the way a
// browser sends it in the Origin header, so an exact comparison works.
func checkOrigin(origin string) error {
	if !browserOrigin(origin) {
		return fmt.Errorf("middleware: admin origin %q is not a browser origin such as https://admin.example.com", origin)
	}
	return nil
}

// browserOrigin reports whether origin is scheme://host[:port] as browsers
// write it (RFC 6454): http or https, lower case, no path, query or user,
// and no port when it is the scheme's default.
func browserOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	if strings.HasSuffix(u.Host, ":") || (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		return false
	}
	// Rebuilding it catches a path, query, fragment, user or upper case.
	return u.Scheme+"://"+strings.ToLower(u.Host) == origin
}
