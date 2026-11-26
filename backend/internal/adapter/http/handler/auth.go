package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// AccessVerifier checks an access token (adapter/auth.AccessTokens). Logout
// uses it to stop the access token sent along, when there is a valid one.
type AccessVerifier interface {
	Verify(ctx context.Context, raw string, aud shared.Audience) (appidentity.AccessClaims, error)
}

// AuthDeps is what the /auth handlers need.
type AuthDeps struct {
	UseCases   appidentity.UseCases
	Verifier   AccessVerifier
	Limiter    ratelimit.Limiter
	Pseudonyms ratelimit.Pseudonyms
	Cookie     RefreshCookie
	Log        *slog.Logger
}

// Auth serves /auth/*: staff sign-up, login, sessions, password reset and
// invitations (ADRs 0010, 0011 and 0013). The router puts the cross-origin
// check, the deadline and the per-IP limits in front; the per-email limits
// run here, after decoding (ADR 0021).
type Auth struct {
	uc         appidentity.UseCases
	verifier   AccessVerifier
	limiter    ratelimit.Limiter
	pseudonyms ratelimit.Pseudonyms
	cookie     RefreshCookie
	log        *slog.Logger
}

// NewAuth returns the /auth handlers. It returns an error when a dependency
// is missing: a zero Pseudonyms would key emails with an empty HMAC key, a
// zero cookie would break every web session, and a nil verifier would fail
// every logout.
func NewAuth(d AuthDeps) (*Auth, error) {
	if d.UseCases == nil || d.Verifier == nil || d.Limiter == nil || d.Log == nil || d.Pseudonyms.IsZero() || d.Cookie.IsZero() {
		return nil, errors.New("handler: the /auth handlers need use cases, a verifier, a limiter, pseudonyms, the cookie and a logger")
	}
	return &Auth{
		uc: d.UseCases, verifier: d.Verifier, limiter: d.Limiter,
		pseudonyms: d.Pseudonyms, cookie: d.Cookie, log: d.Log,
	}, nil
}

// SignUp is POST /auth/signup: 201 with a session without a store.
func (a *Auth) SignUp(w http.ResponseWriter, r *http.Request) {
	var in dto.SignUpRequest
	via, ok := a.decodeStarting(w, r, &in, &in.RefreshTransport)
	if !ok || !a.allowEmail(w, r, ratelimit.SignUpByEmail, in.Email) {
		return
	}
	res, err := a.uc.SignUp(r.Context(), appidentity.SignUpInput{Name: in.Name, Email: in.Email, Password: in.Password})
	a.session(w, r, http.StatusCreated, res, via, err)
}

// LogIn is POST /auth/login. A wrong password and an unknown email give the
// same 401 invalid_credentials.
func (a *Auth) LogIn(w http.ResponseWriter, r *http.Request) {
	var in dto.LoginRequest
	via, ok := a.decodeStarting(w, r, &in, &in.RefreshTransport)
	if !ok || !a.allowEmail(w, r, ratelimit.LogInByEmail, in.Email) {
		return
	}
	tenant, err := optionalTenant(in.TenantID)
	if err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	res, err := a.uc.Login(r.Context(), appidentity.LoginInput{Email: in.Email, Password: in.Password, TenantID: tenant})
	a.session(w, r, http.StatusOK, res, via, err)
}

// Refresh is POST /auth/refresh. It answers the way the token came, and a
// failure behind it is a 5xx, never a 401 (ADR 0013).
func (a *Auth) Refresh(w http.ResponseWriter, r *http.Request) {
	var in dto.RefreshRequest
	if err := respond.Decode(w, r, &in, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	token, via, err := a.cookie.presented(r, in.RefreshToken)
	if err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	res, err := a.uc.Refresh(r.Context(), appidentity.RefreshInput{RefreshToken: token})
	a.session(w, r, http.StatusOK, res, via, err)
}

// SwitchTenant is POST /auth/switch-tenant. The use case checks the body
// and the membership before it rotates anything, so its 400 and 403 leave
// the session as it was (ADR 0013).
func (a *Auth) SwitchTenant(w http.ResponseWriter, r *http.Request) {
	var in dto.SwitchTenantRequest
	if err := respond.Decode(w, r, &in, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	tenant, err := optionalTenant(in.TenantID)
	if err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	token, via, err := a.cookie.presented(r, in.RefreshToken)
	if err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	res, err := a.uc.SwitchTenant(r.Context(), appidentity.SwitchTenantInput{RefreshToken: token, TenantID: tenant})
	a.session(w, r, http.StatusOK, res, via, err)
}

// LogOut is POST /auth/logout. It revokes the family of every refresh token
// the call presents, each cookie of the configured name and the body token
// alike, because logout must always work and a planted token is only the
// planter's own session (ADRs 0010 and 0016). Revoking works from any token
// of a family, even one a refresh just rotated. It stops the access token
// sent along when that is valid, and answers 204 for any token or none.
// The cookie is cleared only once every revoke has succeeded: after a store
// failure the client still holds its token and can retry.
func (a *Auth) LogOut(w http.ResponseWriter, r *http.Request) {
	var in dto.LogoutRequest
	if err := respond.Decode(w, r, &in, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	tokens := logoutTokens(append(a.cookie.values(r), in.RefreshToken))
	jti := a.accessJTI(r)
	if len(tokens) == 0 && !jti.IsZero() {
		tokens = []string{""} // only the access token to stop
	}
	for _, token := range tokens {
		if err := r.Context().Err(); err != nil {
			respond.Error(w, r, a.log, err)
			return
		}
		if err := a.uc.Logout(r.Context(), appidentity.LogoutInput{RefreshToken: token, AccessJTI: jti}); err != nil {
			respond.Error(w, r, a.log, err)
			return
		}
		jti = shared.ID{} // stopped once is enough
	}
	a.cookie.clear(w)
	w.WriteHeader(http.StatusNoContent)
}

// maxLogoutTokens bounds the refresh tokens one logout revokes. A browser
// holds one __Host- cookie at Path=/, so a real call sends one or two
// (a cookie and a body token); a call with more is hostile, and each token
// costs a transaction that outlives the request.
const maxLogoutTokens = 4

// logoutTokens keeps the well-formed, distinct tokens of presented, in
// order, at most maxLogoutTokens of them. A malformed token belongs to no
// family, so it would revoke nothing.
func logoutTokens(presented []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range presented {
		if _, err := shared.ParseSecret(t); err != nil || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) == maxLogoutTokens {
			break
		}
	}
	return out
}

// RequestPasswordReset is POST /auth/password-reset: 202 whether or not the
// account exists.
func (a *Auth) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var in dto.PasswordResetRequest
	if err := respond.Decode(w, r, &in, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	if !a.allowEmail(w, r, ratelimit.PasswordResetByEmail, in.Email) {
		return
	}
	a.noContent(w, r, http.StatusAccepted, a.uc.RequestPasswordReset(r.Context(), appidentity.PasswordResetInput{Email: in.Email}))
}

// ConfirmPasswordReset is POST /auth/password-reset/confirm: 204, and every
// device of the account must log in again.
func (a *Auth) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var in dto.PasswordResetConfirmRequest
	if err := respond.Decode(w, r, &in, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	if !a.allowEmail(w, r, ratelimit.PasswordResetConfirmByEmail, in.Email) {
		return
	}
	err := a.uc.ConfirmPasswordReset(r.Context(), appidentity.ConfirmPasswordResetInput{
		Email: in.Email, Code: in.Code, Password: in.Password,
	})
	a.noContent(w, r, http.StatusNoContent, err)
}

// PreviewInvitation is POST /auth/invitations/preview. The token comes in
// the body, never the path, so it never reaches a log.
func (a *Auth) PreviewInvitation(w http.ResponseWriter, r *http.Request) {
	var in dto.InvitationTokenRequest
	if err := respond.Decode(w, r, &in, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	p, err := a.uc.PreviewInvitation(r.Context(), in.Token)
	if err != nil {
		respond.Error(w, r, a.log, err)
		return
	}
	// It names the invitee and the store: never kept by a cache on the way.
	w.Header().Set("Cache-Control", "no-store")
	respond.JSON(w, r, a.log, http.StatusOK, dto.NewInvitationPreview(p))
}

// AcceptInvitation is POST /auth/invitations/accept: a session in the
// store the invitation is for.
func (a *Auth) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var in dto.AcceptInvitationRequest
	via, ok := a.decodeStarting(w, r, &in, &in.RefreshTransport)
	if !ok {
		return
	}
	res, err := a.uc.AcceptInvitation(r.Context(), appidentity.AcceptInvitationInput{
		Token: in.Token, Name: in.Name, Password: in.Password,
	})
	a.session(w, r, http.StatusOK, res, via, err)
}
