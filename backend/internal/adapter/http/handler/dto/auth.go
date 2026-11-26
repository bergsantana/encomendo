// Package dto holds the JSON shapes of docs/api/openapi.yaml and the
// mapping from the use cases' views to them. Requests are decoded with
// unknown fields refused (respond.Decode); responses carry only what the
// contract lists.
package dto

import (
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
)

// SignUpRequest is POST /auth/signup.
type SignUpRequest struct {
	Name             string `json:"name"`
	Email            string `json:"email"`
	Password         string `json:"password"`
	RefreshTransport string `json:"refreshTransport"`
}

// LoginRequest is POST /auth/login.
type LoginRequest struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
	TenantID         string `json:"tenantId"`
	RefreshTransport string `json:"refreshTransport"`
}

// RefreshRequest is POST /auth/refresh. Android sends the token here; the
// web app sends {} and the cookie.
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// SwitchTenantRequest is POST /auth/switch-tenant.
type SwitchTenantRequest struct {
	TenantID     string `json:"tenantId"`
	RefreshToken string `json:"refreshToken"`
}

// LogoutRequest is POST /auth/logout.
type LogoutRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// PasswordResetRequest is POST /auth/password-reset.
type PasswordResetRequest struct {
	Email string `json:"email"`
}

// PasswordResetConfirmRequest is POST /auth/password-reset/confirm.
type PasswordResetConfirmRequest struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

// InvitationTokenRequest is POST /auth/invitations/preview.
type InvitationTokenRequest struct {
	Token string `json:"token"`
}

// AcceptInvitationRequest is POST /auth/invitations/accept.
type AcceptInvitationRequest struct {
	Token            string `json:"token"`
	Name             string `json:"name"`
	Password         string `json:"password"`
	RefreshTransport string `json:"refreshTransport"`
}

// User is the User schema.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// TenantSummary is the TenantSummary schema.
type TenantSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Membership is the Membership schema.
type Membership struct {
	Tenant TenantSummary `json:"tenant"`
	Role   string        `json:"role"`
}

// Session is the Session schema: tenant and role are absent when no store
// is selected; permissions and memberships are never null.
type Session struct {
	User        User           `json:"user"`
	Tenant      *TenantSummary `json:"tenant,omitempty"`
	Role        string         `json:"role,omitempty"`
	Permissions []string       `json:"permissions"`
	Memberships []Membership   `json:"memberships"`
}

// AuthSession is the AuthSession schema. RefreshToken is set only with the
// body transport; with the cookie transport the token never reaches the
// body, so a script on the page cannot read it (ADR 0010).
type AuthSession struct {
	Session
	AccessToken  string `json:"accessToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int    `json:"expiresIn"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

// InvitationPreview is the InvitationPreview schema.
type InvitationPreview struct {
	Tenant        TenantSummary `json:"tenant"`
	Email         string        `json:"email"`
	Role          string        `json:"role"`
	AccountExists bool          `json:"accountExists"`
	ExpiresAt     time.Time     `json:"expiresAt"`
}

// NewAuthSession maps a new session. The refresh token is left out: the
// handler adds it for the body transport only.
func NewAuthSession(r appidentity.AuthResult) AuthSession {
	return AuthSession{
		Session:     NewSession(r.Session),
		AccessToken: r.AccessToken,
		TokenType:   "Bearer",
		// The token was issued just now, so its lifetime is what is left.
		ExpiresIn: int(identity.AccessTokenLifetime / time.Second),
	}
}

// NewSession maps a session view.
func NewSession(s appidentity.SessionView) Session {
	out := Session{
		User:        User{ID: s.User.ID.String(), Name: s.User.Name, Email: s.User.Email},
		Permissions: make([]string, 0, len(s.Permissions)),
		Memberships: make([]Membership, 0, len(s.Memberships)),
	}
	if s.HasTenant() {
		t := newTenant(s.Tenant)
		out.Tenant, out.Role = &t, string(s.Role)
	}
	for _, p := range s.Permissions {
		out.Permissions = append(out.Permissions, string(p))
	}
	for _, m := range s.Memberships {
		out.Memberships = append(out.Memberships, Membership{Tenant: newTenant(m.Tenant), Role: string(m.Role)})
	}
	return out
}

// NewInvitationPreview maps what an invitation link offers.
func NewInvitationPreview(p appidentity.InvitationPreview) InvitationPreview {
	return InvitationPreview{
		Tenant: newTenant(p.Tenant), Email: p.Email, Role: string(p.Role),
		AccountExists: p.AccountExists, ExpiresAt: p.ExpiresAt.UTC(),
	}
}

func newTenant(t appidentity.TenantSummary) TenantSummary {
	return TenantSummary{ID: t.ID.String(), Name: t.Name, Slug: t.Slug}
}
