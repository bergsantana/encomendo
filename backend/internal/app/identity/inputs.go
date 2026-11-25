package identity

import (
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// SignUpInput creates a staff account (POST /auth/signup).
type SignUpInput struct {
	Name     string
	Email    string
	Password string
}

// LoginInput logs a staff user in (POST /auth/login).
type LoginInput struct {
	Email    string
	Password string
	// TenantID is the store to select, such as the last one used on the
	// device; zero for none.
	TenantID shared.TenantID
}

// RefreshInput presents a refresh token (POST /auth/refresh).
type RefreshInput struct {
	RefreshToken string
}

// SwitchTenantInput presents a refresh token for a session in another store
// (POST /auth/switch-tenant).
type SwitchTenantInput struct {
	RefreshToken string
	TenantID     shared.TenantID
}

// LogoutInput ends a login (POST /auth/logout). Both parts are optional.
type LogoutInput struct {
	RefreshToken string
	// AccessJTI is the jti of the verified access token sent with the
	// call, if any, so it stops at once instead of at its expiry.
	AccessJTI shared.ID
}

// PasswordResetInput asks for a reset code (POST /auth/password-reset).
type PasswordResetInput struct {
	Email string
}

// ConfirmPasswordResetInput sets a new password with the emailed code (POST
// /auth/password-reset/confirm).
type ConfirmPasswordResetInput struct {
	Email    string
	Code     string
	Password string
}

// AcceptInvitationInput joins a store through an invitation link (POST
// /auth/invitations/accept). Name is used only when the invited email has no
// account yet; Password is the new password then, the account's otherwise.
type AcceptInvitationInput struct {
	Token    string
	Name     string
	Password string
}

// UserView is a staff user as sessions and other contexts show them.
type UserView struct {
	ID    shared.UserID
	Name  string
	Email string
}

// SessionView is who is logged in and where (the Session schema).
type SessionView struct {
	User UserView
	// Tenant is the selected store; its ID is zero when none is selected.
	Tenant TenantSummary
	// Role is the role in Tenant; empty when none is selected.
	Role shared.Role
	// Permissions is what Role allows, sorted; empty, never nil, when none
	// is selected. The apps only hide what it lacks; every call is checked
	// again.
	Permissions []identity.Permission
	// Memberships is every store the user belongs to, for the store
	// picker; empty, never nil, for none.
	Memberships []Membership
}

// HasTenant reports whether the session selects a store.
func (s SessionView) HasTenant() bool { return !s.Tenant.ID.IsZero() }

// AuthResult is a new session: the view, the access token and the refresh
// token. The handler sends the refresh token as a cookie or in the body
// (ADR 0010); it is never logged.
type AuthResult struct {
	Session          SessionView
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// String hides both tokens, so a log line of the result never carries a
// working session (backend/CLAUDE.md, Logging).
func (r AuthResult) String() string {
	return fmt.Sprintf("AuthResult{User:%v Tenant:%v Role:%q AccessToken:%s RefreshToken:%s}",
		r.Session.User.ID, r.Session.Tenant.ID, r.Session.Role, shared.Redacted, shared.Redacted)
}

// GoString hides the tokens from %#v too.
func (r AuthResult) GoString() string { return r.String() }

// InvitationPreview is what an invitation link offers (POST
// /auth/invitations/preview).
type InvitationPreview struct {
	Tenant    TenantSummary
	Email     string
	Role      shared.Role
	ExpiresAt time.Time
	// AccountExists tells the app to ask for the account's password instead
	// of a name and a new password.
	AccountExists bool
}
