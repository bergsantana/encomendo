package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// decodeStarting decodes the body of a call that starts a session and reads
// its refreshTransport. It writes the error and returns false on bad input.
func (a *Auth) decodeStarting(w http.ResponseWriter, r *http.Request, dst any, transportField *string) (transport, bool) {
	if err := respond.Decode(w, r, dst, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, a.log, err)
		return viaCookie, false
	}
	via, err := parseTransport(*transportField)
	if err != nil {
		respond.Error(w, r, a.log, err)
		return viaCookie, false
	}
	return via, true
}

// allowEmail counts the call under rule for email and answers 429 when the
// rule is used up (ADR 0021). An email that does not parse is not counted:
// the use case refuses it with a 400 before any password work.
func (a *Auth) allowEmail(w http.ResponseWriter, r *http.Request, rule ratelimit.Rule, raw string) bool {
	email, err := identity.ParseEmail(raw)
	if err != nil {
		return true
	}
	ok, wait := a.limiter.Allow(r.Context(), rule.Key(a.pseudonyms.Email(email)), rule.Limit, rule.Window)
	if !ok {
		respond.RateLimited(w, r, wait)
	}
	return ok
}

// session answers a call that starts or renews a session: the refresh token
// goes back the way via says, as the cookie or in the body, never both.
func (a *Auth) session(w http.ResponseWriter, r *http.Request, status int, res appidentity.AuthResult, via transport, err error) {
	if err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	out := dto.NewAuthSession(res)
	if via == viaBody {
		out.RefreshToken = res.RefreshToken
	} else {
		a.cookie.set(w, res.RefreshToken, res.RefreshExpiresAt)
	}
	// A session answer must never be stored by a cache along the way.
	w.Header().Set("Cache-Control", "no-store")
	respond.JSON(w, r, a.log, status, out)
}

// noContent answers status without a body, or the error.
func (a *Auth) noContent(w http.ResponseWriter, r *http.Request, status int, err error) {
	if err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	w.WriteHeader(status)
}

// accessJTI returns the jti of a valid backoffice access token sent along
// with logout, or the zero id. A missing or bad token is no reason to fail
// a logout, so any refusal is ignored.
func (a *Auth) accessJTI(r *http.Request) shared.ID {
	scheme, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	if !strings.EqualFold(scheme, "Bearer") || token == "" {
		return shared.ID{}
	}
	claims, err := a.verifier.Verify(r.Context(), token, shared.AudienceBackoffice)
	if errors.Is(err, shared.ErrUnauthenticated) {
		return shared.ID{}
	}
	if err != nil {
		// Not a refused token but a fault, such as a wiring bug: still no
		// reason to fail a logout, but an operator must see it.
		a.log.WarnContext(r.Context(), "logout could not check its access token", "err", err)
		return shared.ID{}
	}
	return claims.JTI
}

// optionalTenant parses a store id field: zero when empty, a ValidationError
// on "tenantId" when it is not an id.
func optionalTenant(s string) (shared.TenantID, error) {
	if s == "" {
		return shared.TenantID{}, nil
	}
	id, err := shared.ParseID(s)
	if err != nil {
		return shared.TenantID{}, shared.NewValidationError(shared.FieldError{Field: "tenantId", Message: "Loja inválida."})
	}
	return shared.TenantID(id), nil
}
